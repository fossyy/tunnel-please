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
	"testing"
	"time"
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
func (m *MockConfig) PprofEnabled() bool        { return m.Called().Bool(0) }
func (m *MockConfig) PprofPort() string         { return m.Called().String(0) }
func (m *MockConfig) Mode() types.ServerMode    { return m.Called().Get(0).(types.ServerMode) }
func (m *MockConfig) GRPCAddress() string       { return m.Called().String(0) }
func (m *MockConfig) GRPCPort() string          { return m.Called().String(0) }
func (m *MockConfig) NodeToken() string         { return m.Called().String(0) }

func TestValidateCertDomains_NotFound(t *testing.T) {
	result := ValidateCertDomains("nonexistent.pem", "example.com")
	assert.False(t, result)
}

func TestValidateCertDomains_InvalidPEM(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "invalid*.pem")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, _ = tmpFile.WriteString("not a pem")
	tmpFile.Close()

	result := ValidateCertDomains(tmpFile.Name(), "example.com")
	assert.False(t, result)
}

func TestTLSManager_getTLSConfig(t *testing.T) {
	tm := &tlsManager{
		useCertMagic: false,
	}
	cfg := tm.getTLSConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
}

func TestTLSManager_getCertificate_Magic(t *testing.T) {
	tm := &tlsManager{
		useCertMagic: true,
	}
	hello := &tls.ClientHelloInfo{}
	assert.Panics(t, func() {
		_, _ = tm.getCertificate(hello)
	})
}

func TestTLSManager_userCertsExistAndValid(t *testing.T) {
	tm := &tlsManager{
		certPath: "nonexistent.pem",
		keyPath:  "nonexistent.key",
	}
	assert.False(t, tm.userCertsExistAndValid())

	keyFile, _ := os.CreateTemp("", "key*.pem")
	defer os.Remove(keyFile.Name())
	tm.keyPath = keyFile.Name()
	assert.False(t, tm.userCertsExistAndValid())
}

func createTestCert(t *testing.T, domain string, wildcard bool, expired bool, soon bool) (string, string) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)

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

	certOut, _ := os.CreateTemp("", "cert*.pem")
	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	certOut.Close()

	keyOut, _ := os.CreateTemp("", "key*.pem")
	_ = pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	keyOut.Close()

	return certOut.Name(), keyOut.Name()
}

func TestValidateCertDomains_Success(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)

	result := ValidateCertDomains(certPath, "example.com")
	assert.True(t, result)
}

func TestValidateCertDomains_Expired(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, true, false)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)

	result := ValidateCertDomains(certPath, "example.com")
	assert.False(t, result)
}

func TestValidateCertDomains_ExpiringSoon(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, true)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)

	result := ValidateCertDomains(certPath, "example.com")
	assert.False(t, result)
}

func TestValidateCertDomains_MissingWildcard(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", false, false, false)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)

	result := ValidateCertDomains(certPath, "example.com")
	assert.False(t, result)
}

func TestTLSManager_loadUserCerts_Success(t *testing.T) {
	certPath, keyPath := createTestCert(t, "example.com", true, false, false)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)

	tm := &tlsManager{
		certPath: certPath,
		keyPath:  keyPath,
	}
	err := tm.loadUserCerts()
	assert.NoError(t, err)
	assert.NotNil(t, tm.userCert)
}
