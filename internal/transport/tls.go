package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
	"tunnel_pls/internal/config"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
)

func NewTLSConfig(config config.Config) (*tls.Config, error) {
	var initErr error

	tlsManagerOnce.Do(func() {
		tm := createTLSManager(config)
		initErr = tm.initialize()
		if initErr == nil {
			globalTLSManager = tm
		}
	})

	if initErr != nil {
		return nil, initErr
	}

	return globalTLSManager.getTLSConfig(), nil
}

type tlsManager struct {
	config config.Config

	certPath    string
	keyPath     string
	storagePath string

	userCert   *tls.Certificate
	userCertMu sync.RWMutex

	magic *certmagic.Config

	useCertMagic bool
}

var globalTLSManager *tlsManager
var tlsManagerOnce sync.Once

func createTLSManager(cfg config.Config) *tlsManager {
	storagePath := cfg.TLSStoragePath()
	cleanBase := filepath.Clean(storagePath)

	return &tlsManager{
		config:      cfg,
		certPath:    filepath.Join(cleanBase, "cert.pem"),
		keyPath:     filepath.Join(cleanBase, "privkey.pem"),
		storagePath: filepath.Join(cleanBase, "certmagic"),
	}
}

func (tm *tlsManager) initialize() error {
	if tm.userCertsExistAndValid() {
		return tm.initializeWithUserCerts()
	}
	return tm.initializeWithCertMagic()
}

func (tm *tlsManager) initializeWithUserCerts() error {
	log.Printf("Using user-provided certificates from %s and %s", tm.certPath, tm.keyPath)

	if err := tm.loadUserCerts(); err != nil {
		return fmt.Errorf("failed to load user certificates: %w", err)
	}

	tm.useCertMagic = false
	tm.startCertWatcher()
	return nil
}

func (tm *tlsManager) initializeWithCertMagic() error {
	log.Printf("User certificates missing or don't cover %s and *.%s, using CertMagic",
		tm.config.Domain(), tm.config.Domain())

	if err := tm.initCertMagic(); err != nil {
		return fmt.Errorf("failed to initialize CertMagic: %w", err)
	}

	tm.useCertMagic = true
	return nil
}

func (tm *tlsManager) userCertsExistAndValid() bool {
	if !tm.certFilesExist() {
		return false
	}
	return validateCertDomains(tm.certPath, tm.config.Domain())
}

func (tm *tlsManager) certFilesExist() bool {
	if _, err := os.Stat(tm.certPath); os.IsNotExist(err) {
		log.Printf("Certificate file not found: %s", tm.certPath)
		return false
	}
	if _, err := os.Stat(tm.keyPath); os.IsNotExist(err) {
		log.Printf("Key file not found: %s", tm.keyPath)
		return false
	}
	return true
}

func (tm *tlsManager) loadUserCerts() error {
	cert, err := tls.LoadX509KeyPair(tm.certPath, tm.keyPath)
	if err != nil {
		return err
	}

	tm.userCertMu.Lock()
	tm.userCert = &cert
	tm.userCertMu.Unlock()

	log.Printf("Loaded user certificates successfully")
	return nil
}

func (tm *tlsManager) startCertWatcher() {
	go func() {
		watcher := newCertWatcher(tm)
		watcher.watch()
	}()
}

func (tm *tlsManager) initCertMagic() error {
	if err := tm.createStorageDirectory(); err != nil {
		return err
	}

	if tm.config.CFAPIToken() == "" {
		return fmt.Errorf("CF_API_TOKEN environment variable is required for automatic certificate generation")
	}

	magic := tm.createCertMagicConfig()
	tm.magic = magic

	return tm.obtainCertificates(magic)
}

func (tm *tlsManager) createStorageDirectory() error {
	if err := os.MkdirAll(tm.storagePath, 0700); err != nil {
		return fmt.Errorf("failed to create cert storage directory: %w", err)
	}
	return nil
}

func (tm *tlsManager) createCertMagicConfig() *certmagic.Config {
	cfProvider := &cloudflare.Provider{
		APIToken: tm.config.CFAPIToken(),
	}

	storage := &certmagic.FileStorage{Path: tm.storagePath}

	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return tm.magic, nil
		},
	})

	magic := certmagic.New(cache, certmagic.Config{
		Storage: storage,
	})

	acmeIssuer := tm.createACMEIssuer(magic, cfProvider)
	magic.Issuers = []certmagic.Issuer{acmeIssuer}

	return magic
}

func (tm *tlsManager) createACMEIssuer(magic *certmagic.Config, cfProvider *cloudflare.Provider) *certmagic.ACMEIssuer {
	acmeIssuer := certmagic.NewACMEIssuer(magic, certmagic.ACMEIssuer{
		Email:  tm.config.ACMEEmail(),
		Agreed: true,
		DNS01Solver: &certmagic.DNS01Solver{
			DNSManager: certmagic.DNSManager{
				DNSProvider: cfProvider,
			},
		},
	})

	if tm.config.ACMEStaging() {
		acmeIssuer.CA = certmagic.LetsEncryptStagingCA
		log.Printf("Using Let's Encrypt staging server")
	} else {
		acmeIssuer.CA = certmagic.LetsEncryptProductionCA
		log.Printf("Using Let's Encrypt production server")
	}

	return acmeIssuer
}

func (tm *tlsManager) obtainCertificates(magic *certmagic.Config) error {
	domains := []string{tm.config.Domain(), "*." + tm.config.Domain()}
	log.Printf("Requesting certificates for: %v", domains)

	ctx := context.Background()
	if err := magic.ManageSync(ctx, domains); err != nil {
		return fmt.Errorf("failed to obtain certificates: %w", err)
	}

	log.Printf("Certificates obtained successfully for %v", domains)
	return nil
}

func (tm *tlsManager) getTLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: tm.getCertificate,

		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,

		CurvePreferences: []tls.CurveID{
			tls.X25519,
		},

		SessionTicketsDisabled: false,
		ClientAuth:             tls.NoClientCert,
	}
}

func (tm *tlsManager) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if tm.useCertMagic {
		return tm.magic.GetCertificate(hello)
	}

	tm.userCertMu.RLock()
	defer tm.userCertMu.RUnlock()

	if tm.userCert == nil {
		return nil, fmt.Errorf("no certificate available")
	}

	return tm.userCert, nil
}

func validateCertDomains(certPath, domain string) bool {
	cert, err := loadAndParseCertificate(certPath)
	if err != nil {
		return false
	}

	if !isCertificateValid(cert) {
		return false
	}

	return certCoversRequiredDomains(cert, domain)
}

func loadAndParseCertificate(certPath string) (*x509.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		log.Printf("Failed to read certificate: %v", err)
		return nil, err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		log.Printf("Failed to decode PEM block from certificate")
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Printf("Failed to parse certificate: %v", err)
		return nil, err
	}

	return cert, nil
}

func isCertificateValid(cert *x509.Certificate) bool {
	now := time.Now()

	if now.After(cert.NotAfter) {
		log.Printf("Certificate has expired (NotAfter: %v)", cert.NotAfter)
		return false
	}

	thirtyDaysFromNow := now.Add(30 * 24 * time.Hour)
	if thirtyDaysFromNow.After(cert.NotAfter) {
		log.Printf("Certificate expiring soon (NotAfter: %v), will use CertMagic for renewal", cert.NotAfter)
		return false
	}

	return true
}

func certCoversRequiredDomains(cert *x509.Certificate, domain string) bool {
	certDomains := extractCertDomains(cert)
	hasBase, hasWildcard := checkDomainCoverage(certDomains, domain)

	logDomainCoverage(hasBase, hasWildcard, domain)
	return hasBase && hasWildcard
}

func extractCertDomains(cert *x509.Certificate) []string {
	var domains []string
	if cert.Subject.CommonName != "" {
		domains = append(domains, cert.Subject.CommonName)
	}
	domains = append(domains, cert.DNSNames...)
	return domains
}

func checkDomainCoverage(certDomains []string, domain string) (hasBase, hasWildcard bool) {
	wildcardDomain := "*." + domain

	for _, d := range certDomains {
		if d == domain {
			hasBase = true
		}
		if d == wildcardDomain {
			hasWildcard = true
		}
	}

	return hasBase, hasWildcard
}

func logDomainCoverage(hasBase, hasWildcard bool, domain string) {
	if !hasBase {
		log.Printf("Certificate does not cover base domain: %s", domain)
	}
	if !hasWildcard {
		log.Printf("Certificate does not cover wildcard domain: *.%s", domain)
	}
}

type certWatcher struct {
	tm          *tlsManager
	lastCertMod time.Time
	lastKeyMod  time.Time
}

func newCertWatcher(tm *tlsManager) *certWatcher {
	watcher := &certWatcher{tm: tm}
	watcher.initializeModTimes()
	return watcher
}

func (cw *certWatcher) initializeModTimes() {
	if info, err := os.Stat(cw.tm.certPath); err == nil {
		cw.lastCertMod = info.ModTime()
	}
	if info, err := os.Stat(cw.tm.keyPath); err == nil {
		cw.lastKeyMod = info.ModTime()
	}
}

func (cw *certWatcher) watch() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if cw.checkAndReloadCerts() {
			return
		}
	}
}

func (cw *certWatcher) checkAndReloadCerts() bool {
	certInfo, keyInfo, err := cw.getFileInfo()
	if err != nil {
		return false
	}

	if !cw.filesModified(certInfo, keyInfo) {
		return false
	}

	return cw.handleCertificateChange(certInfo, keyInfo)
}

func (cw *certWatcher) getFileInfo() (os.FileInfo, os.FileInfo, error) {
	certInfo, certErr := os.Stat(cw.tm.certPath)
	keyInfo, keyErr := os.Stat(cw.tm.keyPath)

	if certErr != nil || keyErr != nil {
		return nil, nil, fmt.Errorf("file stat error")
	}

	return certInfo, keyInfo, nil
}

func (cw *certWatcher) filesModified(certInfo, keyInfo os.FileInfo) bool {
	return certInfo.ModTime().After(cw.lastCertMod) || keyInfo.ModTime().After(cw.lastKeyMod)
}

func (cw *certWatcher) handleCertificateChange(certInfo, keyInfo os.FileInfo) bool {
	log.Printf("Certificate files changed, reloading...")

	if !validateCertDomains(cw.tm.certPath, cw.tm.config.Domain()) {
		return cw.switchToCertMagic()
	}

	if err := cw.tm.loadUserCerts(); err != nil {
		log.Printf("Failed to reload certificates: %v", err)
		return false
	}

	cw.updateModTimes(certInfo, keyInfo)
	log.Printf("Certificates reloaded successfully")
	return false
}

func (cw *certWatcher) switchToCertMagic() bool {
	log.Printf("New certificates don't cover required domains")

	if err := cw.tm.initCertMagic(); err != nil {
		log.Printf("Failed to initialize CertMagic: %v", err)
		return false
	}

	cw.tm.useCertMagic = true
	return true
}

func (cw *certWatcher) updateModTimes(certInfo, keyInfo os.FileInfo) {
	cw.lastCertMod = certInfo.ModTime()
	cw.lastKeyMod = keyInfo.ModTime()
}
