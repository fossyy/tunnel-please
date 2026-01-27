package transport

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockConfig struct {
	mock.Mock
}

func (m *MockConfig) Domain() string            { return m.Called().String(0) }
func (m *MockConfig) SSHPort() string           { return m.Called().String(0) }
func (m *MockConfig) HTTPPort() string          { return m.Called().String(0) }
func (m *MockConfig) HTTPSPort() string         { return m.Called().String(0) }
func (m *MockConfig) TLSEnabled() bool          { return m.Called().Bool(0) }
func (m *MockConfig) TLSRedirect() bool         { return m.Called().Bool(0) }
func (m *MockConfig) ACMEEmail() string         { return m.Called().String(0) }
func (m *MockConfig) CFAPIToken() string        { return m.Called().String(0) }
func (m *MockConfig) ACMEStaging() bool         { return m.Called().Bool(0) }
func (m *MockConfig) AllowedPortsStart() uint16 { return uint16(m.Called().Int(0)) }
func (m *MockConfig) AllowedPortsEnd() uint16   { return uint16(m.Called().Int(0)) }
func (m *MockConfig) BufferSize() int           { return m.Called().Int(0) }
func (m *MockConfig) HeaderSize() int           { return m.Called().Int(0) }
func (m *MockConfig) PprofEnabled() bool        { return m.Called().Bool(0) }
func (m *MockConfig) PprofPort() string         { return m.Called().String(0) }
func (m *MockConfig) Mode() types.ServerMode    { return m.Called().Get(0).(types.ServerMode) }
func (m *MockConfig) GRPCAddress() string       { return m.Called().String(0) }
func (m *MockConfig) GRPCPort() string          { return m.Called().String(0) }
func (m *MockConfig) NodeToken() string         { return m.Called().String(0) }
func (m *MockConfig) TLSStoragePath() string    { return m.Called().String(0) }
func (m *MockConfig) KeyLoc() string            { return m.Called().String(0) }

func createTestCert(t *testing.T, domain string, wildcard bool, expired bool, soon bool) (string, string) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	notAfter := time.Now().Add(365 * 24 * time.Hour)
	if expired {
		notAfter = time.Now().Add(-24 * time.Hour)
	} else if soon {
		notAfter = time.Now().Add(15 * 24 * time.Hour)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  notAfter,
		DNSNames:  []string{domain},
	}

	if wildcard {
		template.DNSNames = append(template.DNSNames, "*."+domain)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	assert.NoError(t, err)

	certOut, err := os.CreateTemp("", "cert*.pem")
	assert.NoError(t, err)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	assert.NoError(t, err)
	err = certOut.Close()
	assert.NoError(t, err)

	keyOut, err := os.CreateTemp("", "key*.pem")
	assert.NoError(t, err)
	err = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	assert.NoError(t, err)
	err = keyOut.Close()
	assert.NoError(t, err)

	return certOut.Name(), keyOut.Name()
}

func setupTestDir(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "tls-test-*")
	assert.NoError(t, err)

	t.Cleanup(func() {
		err = os.RemoveAll(tmpDir)
		assert.NoError(t, err)
	})

	return tmpDir
}

func TestValidateCertDomains(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) (certPath string, cleanup func())
		domain   string
		expected bool
	}{
		{
			name: "file not found",
			setup: func(t *testing.T) (string, func()) {
				return "nonexistent.pem", func() {}
			},
			domain:   "example.com",
			expected: false,
		},
		{
			name: "invalid PEM",
			setup: func(t *testing.T) (string, func()) {
				tmpFile, err := os.CreateTemp("", "invalid*.pem")
				assert.NoError(t, err)
				_, err = tmpFile.WriteString("not a pem")
				assert.NoError(t, err)
				err = tmpFile.Close()
				assert.NoError(t, err)
				return tmpFile.Name(), func() {
					_ = os.Remove(tmpFile.Name())
				}
			},
			domain:   "example.com",
			expected: false,
		},
		{
			name: "valid cert with wildcard",
			setup: func(t *testing.T) (string, func()) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				return certPath, func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				}
			},
			domain:   "example.com",
			expected: true,
		},
		{
			name: "expired cert",
			setup: func(t *testing.T) (string, func()) {
				certPath, keyPath := createTestCert(t, "example.com", true, true, false)
				return certPath, func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				}
			},
			domain:   "example.com",
			expected: false,
		},
		{
			name: "cert expiring soon",
			setup: func(t *testing.T) (string, func()) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, true)
				return certPath, func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				}
			},
			domain:   "example.com",
			expected: false,
		},
		{
			name: "missing wildcard",
			setup: func(t *testing.T) (string, func()) {
				certPath, keyPath := createTestCert(t, "example.com", false, false, false)
				return certPath, func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				}
			},
			domain:   "example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPath, cleanup := tt.setup(t)
			defer cleanup()

			result := validateCertDomains(certPath, tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadAndParseCertificate(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (certPath string, cleanup func())
		wantError bool
		validate  func(t *testing.T, cert *x509.Certificate)
	}{
		{
			name: "success",
			setup: func(t *testing.T) (string, func()) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				return certPath, func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				}
			},
			wantError: false,
			validate: func(t *testing.T, cert *x509.Certificate) {
				assert.Equal(t, "example.com", cert.Subject.CommonName)
			},
		},
		{
			name: "file not found",
			setup: func(t *testing.T) (string, func()) {
				return "nonexistent.pem", func() {}
			},
			wantError: true,
			validate:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPath, cleanup := tt.setup(t)
			defer cleanup()

			cert, err := loadAndParseCertificate(certPath)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, cert)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cert)
				if tt.validate != nil {
					tt.validate(t, cert)
				}
			}
		})
	}
}

func TestIsCertificateValid(t *testing.T) {
	tests := []struct {
		name     string
		expired  bool
		soon     bool
		expected bool
	}{
		{
			name:     "valid certificate",
			expired:  false,
			soon:     false,
			expected: true,
		},
		{
			name:     "expired certificate",
			expired:  true,
			soon:     false,
			expected: false,
		},
		{
			name:     "expiring soon",
			expired:  false,
			soon:     true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certPath, keyPath := createTestCert(t, "example.com", true, tt.expired, tt.soon)
			defer func(name string) {
				err := os.Remove(name)
				assert.NoError(t, err)
			}(certPath)
			defer func(name string) {
				err := os.Remove(name)
				assert.NoError(t, err)
			}(keyPath)

			cert, err := loadAndParseCertificate(certPath)
			assert.NoError(t, err)

			result := isCertificateValid(cert)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCertDomains(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(certPath)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(keyPath)

	cert, err := loadAndParseCertificate(certPath)
	assert.NoError(t, err)

	domains := extractCertDomains(cert)
	assert.Contains(t, domains, "example.com")
	assert.Contains(t, domains, "*.example.com")
}

func TestCheckDomainCoverage(t *testing.T) {
	tests := []struct {
		name         string
		certDomains  []string
		domain       string
		wantBase     bool
		wantWildcard bool
	}{
		{
			name:         "both covered",
			certDomains:  []string{"example.com", "*.example.com"},
			domain:       "example.com",
			wantBase:     true,
			wantWildcard: true,
		},
		{
			name:         "only base",
			certDomains:  []string{"example.com"},
			domain:       "example.com",
			wantBase:     true,
			wantWildcard: false,
		},
		{
			name:         "only wildcard",
			certDomains:  []string{"*.example.com"},
			domain:       "example.com",
			wantBase:     false,
			wantWildcard: true,
		},
		{
			name:         "neither",
			certDomains:  []string{"other.com"},
			domain:       "example.com",
			wantBase:     false,
			wantWildcard: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasBase, hasWildcard := checkDomainCoverage(tt.certDomains, tt.domain)
			assert.Equal(t, tt.wantBase, hasBase)
			assert.Equal(t, tt.wantWildcard, hasWildcard)
		})
	}
}

func TestTLSManager_getTLSConfig(t *testing.T) {
	tm := &tlsManager{
		useCertMagic: false,
	}
	cfg := tm.getTLSConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MaxVersion)
	assert.NotNil(t, cfg.GetCertificate)
}

func TestTLSManager_getCertificate(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T) *tlsManager
		wantError     bool
		errorContains string
	}{
		{
			name: "no certificate available",
			setup: func(t *testing.T) *tlsManager {
				return &tlsManager{
					useCertMagic: false,
					userCert:     nil,
				}
			},
			wantError:     true,
			errorContains: "no certificate available",
		},
		{
			name: "with user certificate",
			setup: func(t *testing.T) *tlsManager {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				cert, err := tls.LoadX509KeyPair(certPath, keyPath)
				assert.NoError(t, err)

				return &tlsManager{
					useCertMagic: false,
					userCert:     &cert,
				}
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := tt.setup(t)
			hello := &tls.ClientHelloInfo{
				ServerName: "example.com",
			}

			cert, err := tm.getCertificate(hello)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, cert)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cert)
			}
		})
	}
}

func TestTLSManager_userCertsExistAndValid(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) *tlsManager
		expected bool
	}{
		{
			name: "no files",
			setup: func(t *testing.T) *tlsManager {
				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")

				return &tlsManager{
					config:   mockCfg,
					certPath: "nonexistent.pem",
					keyPath:  "nonexistent.key",
				}
			},
			expected: false,
		},
		{
			name: "missing key file",
			setup: func(t *testing.T) *tlsManager {
				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")

				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() { _ = os.Remove(certPath) })
				err := os.Remove(keyPath)
				assert.NoError(t, err)

				return &tlsManager{
					config:   mockCfg,
					certPath: certPath,
					keyPath:  keyPath,
				}
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := tt.setup(t)
			result := tm.userCertsExistAndValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTLSManager_certFilesExist(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(certPath)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(keyPath)

	tm := &tlsManager{
		certPath: certPath,
		keyPath:  keyPath,
	}

	result := tm.certFilesExist()
	assert.True(t, result)
}

func TestTLSManager_loadUserCerts(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *tlsManager
		wantError bool
	}{
		{
			name: "success",
			setup: func(t *testing.T) *tlsManager {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				return &tlsManager{
					certPath: certPath,
					keyPath:  keyPath,
				}
			},
			wantError: false,
		},
		{
			name: "invalid path",
			setup: func(t *testing.T) *tlsManager {
				return &tlsManager{
					certPath: "nonexistent.pem",
					keyPath:  "nonexistent.key",
				}
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := tt.setup(t)
			err := tm.loadUserCerts()

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, tm.userCert)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tm.userCert)
			}
		})
	}
}

func TestCreateTLSManager(t *testing.T) {
	tmpDir := setupTestDir(t)

	mockCfg := &MockConfig{}
	mockCfg.On("TLSStoragePath").Return(tmpDir)

	tm := createTLSManager(mockCfg)

	assert.NotNil(t, tm)
	assert.Equal(t, mockCfg, tm.config)
	assert.Equal(t, filepath.Join(tmpDir, "cert.pem"), tm.certPath)
	assert.Equal(t, filepath.Join(tmpDir, "privkey.pem"), tm.keyPath)
	assert.Equal(t, filepath.Join(tmpDir, "certmagic"), tm.storagePath)
}

func TestNewCertWatcher(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(certPath)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(keyPath)

	mockCfg := &MockConfig{}

	tm := &tlsManager{
		config:   mockCfg,
		certPath: certPath,
		keyPath:  keyPath,
	}

	watcher := newCertWatcher(tm)

	assert.NotNil(t, watcher)
	assert.Equal(t, tm, watcher.tm)
	assert.False(t, watcher.lastCertMod.IsZero())
	assert.False(t, watcher.lastKeyMod.IsZero())
}

func TestCertWatcher_filesModified(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(certPath)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(keyPath)

	mockCfg := &MockConfig{}

	tm := &tlsManager{
		config:   mockCfg,
		certPath: certPath,
		keyPath:  keyPath,
	}

	watcher := newCertWatcher(tm)

	certInfo, err := os.Stat(certPath)
	assert.NoError(t, err)
	keyInfo, err := os.Stat(keyPath)
	assert.NoError(t, err)

	result := watcher.filesModified(certInfo, keyInfo)
	assert.False(t, result)

	watcher.lastCertMod = time.Now().Add(-1 * time.Hour)

	result = watcher.filesModified(certInfo, keyInfo)
	assert.True(t, result)
}

func TestCertWatcher_updateModTimes(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(certPath)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(keyPath)

	mockCfg := &MockConfig{}

	tm := &tlsManager{
		config:   mockCfg,
		certPath: certPath,
		keyPath:  keyPath,
	}

	watcher := newCertWatcher(tm)

	certInfo, err := os.Stat(certPath)
	assert.NoError(t, err)
	keyInfo, err := os.Stat(keyPath)
	assert.NoError(t, err)

	time.Sleep(10 * time.Millisecond)
	watcher.updateModTimes(certInfo, keyInfo)

	assert.Equal(t, certInfo.ModTime(), watcher.lastCertMod)
	assert.Equal(t, keyInfo.ModTime(), watcher.lastKeyMod)
}

func TestCertWatcher_getFileInfo(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *tlsManager
		wantError bool
		validate  func(t *testing.T, certInfo, keyInfo os.FileInfo)
	}{
		{
			name: "success",
			setup: func(t *testing.T) *tlsManager {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				return &tlsManager{
					config:   &MockConfig{},
					certPath: certPath,
					keyPath:  keyPath,
				}
			},
			wantError: false,
			validate: func(t *testing.T, certInfo, keyInfo os.FileInfo) {
				assert.NotNil(t, certInfo)
				assert.NotNil(t, keyInfo)
			},
		},
		{
			name: "missing cert file",
			setup: func(t *testing.T) *tlsManager {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				err := os.Remove(certPath)
				assert.NoError(t, err)
				t.Cleanup(func() { _ = os.Remove(keyPath) })

				return &tlsManager{
					config:   &MockConfig{},
					certPath: certPath,
					keyPath:  keyPath,
				}
			},
			wantError: true,
		},
		{
			name: "missing key file",
			setup: func(t *testing.T) *tlsManager {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				err := os.Remove(keyPath)
				assert.NoError(t, err)
				t.Cleanup(func() { _ = os.Remove(certPath) })

				return &tlsManager{
					config:   &MockConfig{},
					certPath: certPath,
					keyPath:  keyPath,
				}
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := tt.setup(t)
			watcher := newCertWatcher(tm)

			certInfo, keyInfo, err := watcher.getFileInfo()

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, certInfo)
				assert.Nil(t, keyInfo)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, certInfo, keyInfo)
				}
			}
		})
	}
}

func TestCertWatcher_checkAndReloadCerts(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) (*tlsManager, *certWatcher)
		expected bool
	}{
		{
			name: "file error",
			setup: func(t *testing.T) (*tlsManager, *certWatcher) {
				tm := &tlsManager{
					config:   &MockConfig{},
					certPath: "nonexistent.pem",
					keyPath:  "nonexistent.key",
				}
				return tm, newCertWatcher(tm)
			},
			expected: false,
		},
		{
			name: "no modification",
			setup: func(t *testing.T) (*tlsManager, *certWatcher) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				tm := &tlsManager{
					config:   &MockConfig{},
					certPath: certPath,
					keyPath:  keyPath,
				}
				return tm, newCertWatcher(tm)
			},
			expected: false,
		},
		{
			name: "with modification",
			setup: func(t *testing.T) (*tlsManager, *certWatcher) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")

				tm := &tlsManager{
					config:   mockCfg,
					certPath: certPath,
					keyPath:  keyPath,
				}

				err := tm.loadUserCerts()
				assert.NoError(t, err)

				watcher := newCertWatcher(tm)
				watcher.lastCertMod = time.Now().Add(-1 * time.Hour)

				return tm, watcher
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, watcher := tt.setup(t)
			result := watcher.checkAndReloadCerts()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCertWatcher_handleCertificateChange(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) (*tlsManager, *certWatcher, os.FileInfo, os.FileInfo)
		expected bool
	}{
		{
			name: "successful reload",
			setup: func(t *testing.T) (*tlsManager, *certWatcher, os.FileInfo, os.FileInfo) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")

				tm := &tlsManager{
					config:   mockCfg,
					certPath: certPath,
					keyPath:  keyPath,
				}

				watcher := newCertWatcher(tm)

				certInfo, _ := os.Stat(certPath)
				keyInfo, _ := os.Stat(keyPath)

				return tm, watcher, certInfo, keyInfo
			},
			expected: false,
		},
		{
			name: "invalid cert triggers certmagic",
			setup: func(t *testing.T) (*tlsManager, *certWatcher, os.FileInfo, os.FileInfo) {
				certPath, keyPath := createTestCert(t, "example.com", false, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				tmpDir := setupTestDir(t)

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")
				mockCfg.On("CFAPIToken").Return("")

				tm := &tlsManager{
					config:      mockCfg,
					certPath:    certPath,
					keyPath:     keyPath,
					storagePath: tmpDir,
				}

				watcher := newCertWatcher(tm)

				certInfo, _ := os.Stat(certPath)
				keyInfo, _ := os.Stat(keyPath)

				return tm, watcher, certInfo, keyInfo
			},
			expected: false,
		},
		{
			name: "load error",
			setup: func(t *testing.T) (*tlsManager, *certWatcher, os.FileInfo, os.FileInfo) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")

				tm := &tlsManager{
					config:   mockCfg,
					certPath: certPath,
					keyPath:  "nonexistent.key",
				}

				watcher := newCertWatcher(tm)

				certInfo, _ := os.Stat(certPath)
				keyInfo, _ := os.Stat(keyPath)

				return tm, watcher, certInfo, keyInfo
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, watcher, certInfo, keyInfo := tt.setup(t)
			result := watcher.handleCertificateChange(certInfo, keyInfo)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCertWatcher_switchToCertMagic(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) *tlsManager
		expected bool
	}{
		{
			name: "with staging token",
			setup: func(t *testing.T) *tlsManager {
				tmpDir := setupTestDir(t)

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")
				mockCfg.On("CFAPIToken").Return("test-token")
				mockCfg.On("ACMEEmail").Return("test@example.com")
				mockCfg.On("ACMEStaging").Return(true)

				return &tlsManager{
					config:      mockCfg,
					storagePath: tmpDir,
				}
			},
			expected: false,
		},
		{
			name: "missing token",
			setup: func(t *testing.T) *tlsManager {
				tmpDir := setupTestDir(t)

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")
				mockCfg.On("CFAPIToken").Return("")

				return &tlsManager{
					config:      mockCfg,
					storagePath: tmpDir,
				}
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := tt.setup(t)
			watcher := newCertWatcher(tm)
			result := watcher.switchToCertMagic()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCertWatcher_watch(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) (*tlsManager, *certWatcher)
		expected bool
	}{
		{
			name: "exits on certmagic switch attempt",
			setup: func(t *testing.T) (*tlsManager, *certWatcher) {
				certPath, keyPath := createTestCert(t, "example.com", false, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				tmpDir := setupTestDir(t)

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")
				mockCfg.On("CFAPIToken").Return("")

				tm := &tlsManager{
					config:      mockCfg,
					certPath:    certPath,
					keyPath:     keyPath,
					storagePath: tmpDir,
				}

				watcher := newCertWatcher(tm)
				watcher.lastCertMod = time.Now().Add(-1 * time.Hour)
				watcher.lastKeyMod = time.Now().Add(-1 * time.Hour)

				return tm, watcher
			},
			expected: false,
		},
		{
			name: "continues on no modification",
			setup: func(t *testing.T) (*tlsManager, *certWatcher) {
				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				mockCfg := &MockConfig{}
				mockCfg.On("Domain").Return("example.com")

				tm := &tlsManager{
					config:   mockCfg,
					certPath: certPath,
					keyPath:  keyPath,
				}

				return tm, newCertWatcher(tm)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, watcher := tt.setup(t)
			result := watcher.checkAndReloadCerts()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCertWatcher_watch_Integration(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(certPath)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(keyPath)

	mockCfg := &MockConfig{}
	mockCfg.On("Domain").Return("example.com")

	tm := &tlsManager{
		config:   mockCfg,
		certPath: certPath,
		keyPath:  keyPath,
	}

	err := tm.loadUserCerts()
	assert.NoError(t, err)
	initialCert := tm.userCert

	watcher := newCertWatcher(tm)

	go watcher.watch()

	time.Sleep(50 * time.Millisecond)

	newCertPath, newKeyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err = os.Remove(name)
		assert.NoError(t, err)
	}(newCertPath)
	defer func(name string) {
		err = os.Remove(name)
		assert.NoError(t, err)
	}(newKeyPath)

	newCertData, err := os.ReadFile(newCertPath)
	assert.NoError(t, err)
	newKeyData, err := os.ReadFile(newKeyPath)
	assert.NoError(t, err)

	err = os.WriteFile(certPath, newCertData, 0644)
	assert.NoError(t, err)
	err = os.WriteFile(keyPath, newKeyData, 0644)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	assert.NotNil(t, tm.userCert)
	assert.Equal(t, initialCert, tm.userCert)
}

func TestNewTLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) config.Config
		wantError bool
		errorMsg  string
		validate  func(t *testing.T, cfg *tls.Config)
	}{
		{
			name: "with valid user certs",
			setup: func(t *testing.T) config.Config {
				globalTLSManager = nil
				tlsManagerOnce = sync.Once{}

				tmpDir := setupTestDir(t)

				certPath, keyPath := createTestCert(t, "example.com", true, false, false)
				t.Cleanup(func() {
					_ = os.Remove(certPath)
					_ = os.Remove(keyPath)
				})

				certData, err := os.ReadFile(certPath)
				assert.NoError(t, err)
				keyData, err := os.ReadFile(keyPath)
				assert.NoError(t, err)

				err = os.WriteFile(filepath.Join(tmpDir, "cert.pem"), certData, 0644)
				assert.NoError(t, err)
				err = os.WriteFile(filepath.Join(tmpDir, "privkey.pem"), keyData, 0644)
				assert.NoError(t, err)

				mockCfg := &MockConfig{}
				mockCfg.On("TLSStoragePath").Return(tmpDir)
				mockCfg.On("Domain").Return("example.com")

				return mockCfg
			},
			wantError: false,
			validate: func(t *testing.T, cfg *tls.Config) {
				assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
				assert.NotNil(t, cfg.GetCertificate)
			},
		},
		{
			name: "missing certs requires certmagic",
			setup: func(t *testing.T) config.Config {
				globalTLSManager = nil
				tlsManagerOnce = sync.Once{}

				tmpDir := setupTestDir(t)

				mockCfg := &MockConfig{}
				mockCfg.On("TLSStoragePath").Return(tmpDir)
				mockCfg.On("Domain").Return("example.com")
				mockCfg.On("CFAPIToken").Return("")

				return mockCfg
			},
			wantError: true,
			errorMsg:  "CF_API_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setup(t)
			tlsConfig, err := NewTLSConfig(cfg)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tlsConfig)
				if tt.validate != nil {
					tt.validate(t, tlsConfig)
				}
			}
		})
	}
}

func TestNewTLSConfig_Singleton(t *testing.T) {
	globalTLSManager = nil
	tlsManagerOnce = sync.Once{}

	tmpDir := setupTestDir(t)

	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(certPath)
	defer func(name string) {
		err := os.Remove(name)
		assert.NoError(t, err)
	}(keyPath)

	certData, err := os.ReadFile(certPath)
	assert.NoError(t, err)
	keyData, err := os.ReadFile(keyPath)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "cert.pem"), certData, 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "privkey.pem"), keyData, 0644)
	assert.NoError(t, err)

	mockCfg := &MockConfig{}
	mockCfg.On("TLSStoragePath").Return(tmpDir)
	mockCfg.On("Domain").Return("example.com")

	tlsConfig1, err1 := NewTLSConfig(mockCfg)
	tlsConfig2, err2 := NewTLSConfig(mockCfg)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotNil(t, tlsConfig1)
	assert.NotNil(t, tlsConfig2)

	assert.Equal(t, tlsConfig1.MinVersion, tlsConfig2.MinVersion)
	assert.Equal(t, tlsConfig1.MaxVersion, tlsConfig2.MaxVersion)
	assert.Equal(t, tlsConfig1.SessionTicketsDisabled, tlsConfig2.SessionTicketsDisabled)
	assert.Equal(t, tlsConfig1.ClientAuth, tlsConfig2.ClientAuth)

	hello := &tls.ClientHelloInfo{ServerName: "example.com"}
	cert1, err1 := tlsConfig1.GetCertificate(hello)
	cert2, err2 := tlsConfig2.GetCertificate(hello)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotNil(t, cert1)
	assert.NotNil(t, cert2)

	assert.Equal(t, cert1, cert2)
}
