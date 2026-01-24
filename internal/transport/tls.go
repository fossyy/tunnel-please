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

type TLSManager interface {
	userCertsExistAndValid() bool
	loadUserCerts() error
	startCertWatcher()
	initCertMagic() error
	getTLSConfig() *tls.Config
	getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
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

var globalTLSManager TLSManager
var tlsManagerOnce sync.Once

func NewTLSConfig(config config.Config) (*tls.Config, error) {
	var initErr error

	tlsManagerOnce.Do(func() {
		storagePath := config.TLSStoragePath()
		cleanBase := filepath.Clean(storagePath)

		certPath := filepath.Join(cleanBase, "cert.pem")
		keyPath := filepath.Join(cleanBase, "privkey.pem")
		storagePathCertMagic := filepath.Join(cleanBase, "certmagic")

		tm := &tlsManager{
			config:      config,
			certPath:    certPath,
			keyPath:     keyPath,
			storagePath: storagePathCertMagic,
		}

		if tm.userCertsExistAndValid() {
			log.Printf("Using user-provided certificates from %s and %s", certPath, keyPath)
			if err := tm.loadUserCerts(); err != nil {
				initErr = fmt.Errorf("failed to load user certificates: %w", err)
				return
			}
			tm.useCertMagic = false
			tm.startCertWatcher()
		} else {
			log.Printf("User certificates missing or don't cover %s and *.%s, using CertMagic", config.Domain(), config.Domain())
			if err := tm.initCertMagic(); err != nil {
				initErr = fmt.Errorf("failed to initialize CertMagic: %w", err)
				return
			}
			tm.useCertMagic = true
		}

		globalTLSManager = tm
	})

	if initErr != nil {
		return nil, initErr
	}

	return globalTLSManager.getTLSConfig(), nil
}

func (tm *tlsManager) userCertsExistAndValid() bool {
	if _, err := os.Stat(tm.certPath); os.IsNotExist(err) {
		log.Printf("Certificate file not found: %s", tm.certPath)
		return false
	}
	if _, err := os.Stat(tm.keyPath); os.IsNotExist(err) {
		log.Printf("Key file not found: %s", tm.keyPath)
		return false
	}

	return ValidateCertDomains(tm.certPath, tm.config.Domain())
}

func ValidateCertDomains(certPath, domain string) bool {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		log.Printf("Failed to read certificate: %v", err)
		return false
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		log.Printf("Failed to decode PEM block from certificate")
		return false
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		log.Printf("Failed to parse certificate: %v", err)
		return false
	}

	if time.Now().After(cert.NotAfter) {
		log.Printf("Certificate has expired (NotAfter: %v)", cert.NotAfter)
		return false
	}

	if time.Now().Add(30 * 24 * time.Hour).After(cert.NotAfter) {
		log.Printf("Certificate expiring soon (NotAfter: %v), will use CertMagic for renewal", cert.NotAfter)
		return false
	}

	var certDomains []string
	if cert.Subject.CommonName != "" {
		certDomains = append(certDomains, cert.Subject.CommonName)
	}
	certDomains = append(certDomains, cert.DNSNames...)

	hasBase := false
	hasWildcard := false
	wildcardDomain := "*." + domain

	for _, d := range certDomains {
		if d == domain {
			hasBase = true
		}
		if d == wildcardDomain {
			hasWildcard = true
		}
	}

	if !hasBase {
		log.Printf("Certificate does not cover base domain: %s", domain)
	}
	if !hasWildcard {
		log.Printf("Certificate does not cover wildcard domain: %s", wildcardDomain)
	}

	return hasBase && hasWildcard
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
		var lastCertMod, lastKeyMod time.Time

		if info, err := os.Stat(tm.certPath); err == nil {
			lastCertMod = info.ModTime()
		}
		if info, err := os.Stat(tm.keyPath); err == nil {
			lastKeyMod = info.ModTime()
		}

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			certInfo, certErr := os.Stat(tm.certPath)
			keyInfo, keyErr := os.Stat(tm.keyPath)

			if certErr != nil || keyErr != nil {
				continue
			}

			if certInfo.ModTime().After(lastCertMod) || keyInfo.ModTime().After(lastKeyMod) {
				log.Printf("Certificate files changed, reloading...")

				if !ValidateCertDomains(tm.certPath, tm.config.Domain()) {
					log.Printf("New certificates don't cover required domains")

					if err := tm.initCertMagic(); err != nil {
						log.Printf("Failed to initialize CertMagic: %v", err)
						continue
					}
					tm.useCertMagic = true
					return
				}

				if err := tm.loadUserCerts(); err != nil {
					log.Printf("Failed to reload certificates: %v", err)
					continue
				}

				lastCertMod = certInfo.ModTime()
				lastKeyMod = keyInfo.ModTime()
				log.Printf("Certificates reloaded successfully")
			}
		}
	}()
}

func (tm *tlsManager) initCertMagic() error {
	if err := os.MkdirAll(tm.storagePath, 0700); err != nil {
		return fmt.Errorf("failed to create cert storage directory: %w", err)
	}

	if tm.config.CFAPIToken() == "" {
		return fmt.Errorf("CF_API_TOKEN environment variable is required for automatic certificate generation")
	}

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

	magic.Issuers = []certmagic.Issuer{acmeIssuer}
	tm.magic = magic

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
