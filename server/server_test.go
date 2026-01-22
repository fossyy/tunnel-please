package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
	"tunnel_pls/internal/registry"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
)

type mockRandom struct {
	mock.Mock
}

func (m *mockRandom) String(length int) (string, error) {
	args := m.Called(length)
	return args.String(0), args.Error(1)
}

type mockConfig struct {
	mock.Mock
}

func (m *mockConfig) Domain() string            { return m.Called().String(0) }
func (m *mockConfig) SSHPort() string           { return m.Called().String(0) }
func (m *mockConfig) HTTPPort() string          { return m.Called().String(0) }
func (m *mockConfig) HTTPSPort() string         { return m.Called().String(0) }
func (m *mockConfig) TLSEnabled() bool          { return m.Called().Bool(0) }
func (m *mockConfig) TLSRedirect() bool         { return m.Called().Bool(0) }
func (m *mockConfig) ACMEEmail() string         { return m.Called().String(0) }
func (m *mockConfig) CFAPIToken() string        { return m.Called().String(0) }
func (m *mockConfig) ACMEStaging() bool         { return m.Called().Bool(0) }
func (m *mockConfig) AllowedPortsStart() uint16 { return uint16(m.Called().Int(0)) }
func (m *mockConfig) AllowedPortsEnd() uint16   { return uint16(m.Called().Int(0)) }
func (m *mockConfig) BufferSize() int           { return m.Called().Int(0) }
func (m *mockConfig) PprofEnabled() bool        { return m.Called().Bool(0) }
func (m *mockConfig) PprofPort() string         { return m.Called().String(0) }
func (m *mockConfig) Mode() types.ServerMode    { return m.Called().Get(0).(types.ServerMode) }
func (m *mockConfig) GRPCAddress() string       { return m.Called().String(0) }
func (m *mockConfig) GRPCPort() string          { return m.Called().String(0) }
func (m *mockConfig) NodeToken() string         { return m.Called().String(0) }

type mockRegistry struct {
	mock.Mock
}

func (m *mockRegistry) Get(key types.SessionKey) (registry.Session, error) {
	args := m.Called(key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(registry.Session), args.Error(1)
}

func (m *mockRegistry) GetWithUser(user string, key types.SessionKey) (registry.Session, error) {
	args := m.Called(user, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(registry.Session), args.Error(1)
}

func (m *mockRegistry) Register(key types.SessionKey, session registry.Session) bool {
	return m.Called(key, session).Bool(0)
}

func (m *mockRegistry) Update(user string, oldKey types.SessionKey, newKey types.SessionKey) error {
	return m.Called(user, oldKey, newKey).Error(0)
}

func (m *mockRegistry) Remove(key types.SessionKey) {
	m.Called(key)
}

func (m *mockRegistry) GetAllSessionFromUser(user string) []registry.Session {
	return m.Called(user).Get(0).([]registry.Session)
}

type mockGrpcClient struct {
	mock.Mock
}

func (m *mockGrpcClient) SubscribeEvents(ctx context.Context, identity string, authToken string) error {
	return m.Called(ctx, identity, authToken).Error(0)
}

func (m *mockGrpcClient) ClientConn() *grpc.ClientConn {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*grpc.ClientConn)
}

func (m *mockGrpcClient) AuthorizeConn(ctx context.Context, token string) (bool, string, error) {
	args := m.Called(ctx, token)
	return args.Bool(0), args.String(1), args.Error(2)
}

func (m *mockGrpcClient) CheckServerHealth(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *mockGrpcClient) Close() error {
	return m.Called().Error(0)
}

type mockPort struct {
	mock.Mock
}

func (m *mockPort) AddRange(startPort, endPort uint16) error {
	return m.Called(startPort, endPort).Error(0)
}

func (m *mockPort) Unassigned() (uint16, bool) {
	args := m.Called()
	return uint16(args.Int(0)), args.Bool(1)
}

func (m *mockPort) SetStatus(port uint16, assigned bool) error {
	return m.Called(port, assigned).Error(0)
}

func (m *mockPort) Claim(port uint16) bool {
	return m.Called(port).Bool(0)
}

type mockListener struct {
	mock.Mock
}

func (m *mockListener) Accept() (net.Conn, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(net.Conn), args.Error(1)
}

func (m *mockListener) Close() error {
	return m.Called().Error(0)
}

func (m *mockListener) Addr() net.Addr {
	return m.Called().Get(0).(net.Addr)
}

func getTestSSHConfig() (*ssh.ServerConfig, ssh.Signer) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	signer, _ := ssh.NewSignerFromKey(key)
	config := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	config.AddHostKey(signer)
	return config, signer
}

func TestNew(t *testing.T) {
	mr := new(mockRandom)
	mc := new(mockConfig)
	mreg := new(mockRegistry)
	mg := new(mockGrpcClient)
	mp := new(mockPort)
	sc, _ := getTestSSHConfig()

	tests := []struct {
		name    string
		port    string
		wantErr bool
	}{
		{
			name:    "success",
			port:    "0",
			wantErr: false,
		},
		{
			name:    "invalid port",
			port:    "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := New(mr, mc, sc, mreg, mg, mp, tt.port)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, s)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, s)
				_ = s.Close()
			}
		})
	}

	t.Run("port already in use", func(t *testing.T) {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		defer l.Close()

		s, err := New(mr, mc, sc, mreg, mg, mp, fmt.Sprintf("%d", port))
		assert.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestClose(t *testing.T) {
	mr := new(mockRandom)
	mc := new(mockConfig)
	mreg := new(mockRegistry)
	mg := new(mockGrpcClient)
	mp := new(mockPort)
	sc, _ := getTestSSHConfig()

	s, _ := New(mr, mc, sc, mreg, mg, mp, "0")
	err := s.Close()
	assert.NoError(t, err)

	err = s.Close()
	assert.Error(t, err)
}

func TestStart(t *testing.T) {
	mr := new(mockRandom)
	mc := new(mockConfig)
	mreg := new(mockRegistry)
	mg := new(mockGrpcClient)
	mp := new(mockPort)
	sc, _ := getTestSSHConfig()

	t.Run("normal stop", func(t *testing.T) {
		s, _ := New(mr, mc, sc, mreg, mg, mp, "0")
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = s.Close()
		}()
		s.Start()
	})

	t.Run("accept error", func(t *testing.T) {
		ml := new(mockListener)
		s := &server{
			sshListener: ml,
			sshPort:     "0",
		}

		ml.On("Accept").Return(nil, errors.New("temporary error")).Once()
		ml.On("Accept").Return(nil, net.ErrClosed).Once()

		s.Start()
		ml.AssertExpectations(t)
	})
}
