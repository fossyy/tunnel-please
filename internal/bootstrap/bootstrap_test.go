package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/port"
	"tunnel_pls/internal/registry"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

type MockSessionRegistry struct {
	mock.Mock
}

func (m *MockSessionRegistry) Get(key registry.Key) (registry.Session, error) {
	args := m.Called(key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(registry.Session), args.Error(1)
}

func (m *MockSessionRegistry) GetWithUser(user string, key registry.Key) (registry.Session, error) {
	args := m.Called(user, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(registry.Session), args.Error(1)
}

func (m *MockSessionRegistry) Update(user string, oldKey, newKey registry.Key) error {
	args := m.Called(user, oldKey, newKey)
	return args.Error(0)
}

func (m *MockSessionRegistry) Register(key registry.Key, session registry.Session) bool {
	args := m.Called(key, session)
	return args.Bool(0)
}

func (m *MockSessionRegistry) Remove(key registry.Key) {
	m.Called(key)
}

func (m *MockSessionRegistry) GetAllSessionFromUser(user string) []registry.Session {
	args := m.Called(user)
	return args.Get(0).([]registry.Session)
}

func (m *MockSessionRegistry) Slug() slug.Slug {
	args := m.Called()
	return args.Get(0).(slug.Slug)
}

type MockRandom struct {
	mock.Mock
}

func (m *MockRandom) String(length int) (string, error) {
	args := m.Called(length)
	return args.String(0), args.Error(1)
}

type MockConfig struct {
	mock.Mock
}

func (m *MockConfig) Domain() string            { return m.Called().String(0) }
func (m *MockConfig) SSHPort() string           { return m.Called().String(0) }
func (m *MockConfig) HTTPPort() string          { return m.Called().String(0) }
func (m *MockConfig) HTTPSPort() string         { return m.Called().String(0) }
func (m *MockConfig) TLSEnabled() bool          { return m.Called().Bool(0) }
func (m *MockConfig) TLSRedirect() bool         { return m.Called().Bool(0) }
func (m *MockConfig) TLSStoragePath() string    { return m.Called().String(0) }
func (m *MockConfig) ACMEEmail() string         { return m.Called().String(0) }
func (m *MockConfig) CFAPIToken() string        { return m.Called().String(0) }
func (m *MockConfig) ACMEStaging() bool         { return m.Called().Bool(0) }
func (m *MockConfig) AllowedPortsStart() uint16 { return uint16(m.Called().Int(0)) }
func (m *MockConfig) AllowedPortsEnd() uint16   { return uint16(m.Called().Int(0)) }
func (m *MockConfig) BufferSize() int           { return m.Called().Int(0) }
func (m *MockConfig) HeaderSize() int           { return m.Called().Int(0) }
func (m *MockConfig) PprofEnabled() bool        { return m.Called().Bool(0) }
func (m *MockConfig) PprofPort() string         { return m.Called().String(0) }
func (m *MockConfig) Mode() types.ServerMode {
	args := m.Called()
	if args.Get(0) == nil {
		return 0
	}
	switch v := args.Get(0).(type) {
	case types.ServerMode:
		return v
	case int:
		return types.ServerMode(v)
	default:
		return types.ServerMode(args.Int(0))
	}
}
func (m *MockConfig) GRPCAddress() string { return m.Called().String(0) }
func (m *MockConfig) GRPCPort() string    { return m.Called().String(0) }
func (m *MockConfig) NodeToken() string   { return m.Called().String(0) }
func (m *MockConfig) KeyLoc() string      { return m.Called().String(0) }

type MockPort struct {
	mock.Mock
}

func (m *MockPort) AddRange(startPort, endPort uint16) error {
	return m.Called(startPort, endPort).Error(0)
}
func (m *MockPort) Unassigned() (uint16, bool) {
	args := m.Called()
	var mPort uint16
	if args.Get(0) != nil {
		switch v := args.Get(0).(type) {
		case int:
			mPort = uint16(v)
		case uint16:
			mPort = v
		case uint32:
			mPort = uint16(v)
		case int32:
			mPort = uint16(v)
		case float64:
			mPort = uint16(v)
		default:
			mPort = uint16(args.Int(0))
		}
	}
	return mPort, args.Bool(1)
}
func (m *MockPort) SetStatus(port uint16, assigned bool) error {
	return m.Called(port, assigned).Error(0)
}
func (m *MockPort) Claim(port uint16) bool {
	return m.Called(port).Bool(0)
}

type MockGRPCClient struct {
	mock.Mock
}

func (m *MockGRPCClient) ClientConn() *grpc.ClientConn {
	args := m.Called()
	return args.Get(0).(*grpc.ClientConn)
}

func (m *MockGRPCClient) AuthorizeConn(ctx context.Context, token string) (authorized bool, user string, err error) {
	args := m.Called(ctx, token)
	return args.Bool(0), args.String(1), args.Error(2)
}

func (m *MockGRPCClient) CheckServerHealth(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockGRPCClient) SubscribeEvents(ctx context.Context, domain, token string) error {
	args := m.Called(ctx, domain, token)
	return args.Error(0)
}

func (m *MockGRPCClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() config.Config
		setupPort   func() port.Port
		wantErr     bool
		errContains string
	}{
		{
			name:    "Success New with default value",
			wantErr: false,
		},
		{
			name: "Error when AddRange fails",
			setupPort: func() port.Port {
				mockPort := &MockPort{}
				mockPort.On("AddRange", mock.Anything, mock.Anything).Return(fmt.Errorf("invalid port range"))
				return mockPort
			},
			wantErr:     true,
			errContains: "invalid port range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockPort port.Port
			if tt.setupPort != nil {
				mockPort = tt.setupPort()
			} else {
				mockPort = port.New()
			}

			var mockConfig config.Config
			if tt.setupConfig != nil {
				mockConfig = tt.setupConfig()
			} else {
				var err error
				mockConfig, err = config.MustLoad()
				assert.NoError(t, err)
			}

			bootstrap, err := New(mockConfig, mockPort)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, bootstrap)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, bootstrap)
				assert.NotNil(t, bootstrap.Randomizer)
				assert.NotNil(t, bootstrap.SessionRegistry)
				assert.NotNil(t, bootstrap.Config)
				assert.NotNil(t, bootstrap.Port)
				assert.NotNil(t, bootstrap.ErrChan)
				assert.NotNil(t, bootstrap.SignalChan)
			}
		})
	}
}

func randomAvailablePort() (string, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", err
	}
	defer func(listener net.Listener) {
		_ = listener.Close()
	}(listener)

	mPort := listener.Addr().(*net.TCPAddr).Port
	return strconv.Itoa(mPort), nil
}

func TestRun(t *testing.T) {
	mockRandom := &MockRandom{}
	mockErrChan := make(chan error, 1)
	mockSignalChan := make(chan os.Signal, 1)
	mockSessionRegistry := &MockSessionRegistry{}
	mockPort := &MockPort{}

	tmpDir := t.TempDir()
	keyLoc := filepath.Join(tmpDir, "key.key")

	tests := []struct {
		name            string
		setupConfig     func() *MockConfig
		setupGrpcClient func() *MockGRPCClient
		needCerts       bool
		expectError     bool
	}{
		{
			name: "successful run and termination",
			setupConfig: func() *MockConfig {
				mockConfig := &MockConfig{}
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeSTANDALONE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("0")
				mockConfig.On("HTTPPort").Return("0")
				mockConfig.On("HTTPSPort").Return("0")
				mockConfig.On("TLSEnabled").Return(false)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(false)
				mockConfig.On("PprofPort").Return("0")
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("0")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			expectError: false,
		},
		{
			name: "error from SSH server invalid port",
			setupConfig: func() *MockConfig {
				mockConfig := &MockConfig{}
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeSTANDALONE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("invalid")
				mockConfig.On("HTTPPort").Return("0")
				mockConfig.On("HTTPSPort").Return("0")
				mockConfig.On("TLSEnabled").Return(false)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(false)
				mockConfig.On("PprofPort").Return("0")
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("0")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			expectError: true,
		},
		{
			name: "error from HTTP server invalid port",
			setupConfig: func() *MockConfig {
				mockConfig := &MockConfig{}
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeSTANDALONE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("0")
				mockConfig.On("HTTPPort").Return("invalid")
				mockConfig.On("HTTPSPort").Return("0")
				mockConfig.On("TLSEnabled").Return(false)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(false)
				mockConfig.On("PprofPort").Return("0")
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("0")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			expectError: true,
		},
		{
			name: "error from HTTPS server invalid port",
			setupConfig: func() *MockConfig {
				tempDir := os.TempDir()
				mockConfig := &MockConfig{}
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeSTANDALONE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("0")
				mockConfig.On("HTTPPort").Return("0")
				mockConfig.On("HTTPSPort").Return("invalid")
				mockConfig.On("TLSEnabled").Return(true)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("TLSStoragePath").Return(tempDir)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(false)
				mockConfig.On("PprofPort").Return("0")
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("0")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			expectError: true,
		},
		{
			name: "grpc health check failed",
			setupConfig: func() *MockConfig {
				mockConfig := &MockConfig{}
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeNODE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("0")
				mockConfig.On("HTTPPort").Return("0")
				mockConfig.On("HTTPSPort").Return("0")
				mockConfig.On("TLSEnabled").Return(false)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(false)
				mockConfig.On("PprofPort").Return("0")
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("invalid")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			setupGrpcClient: func() *MockGRPCClient {
				mockGRPCClient := &MockGRPCClient{}
				mockGRPCClient.On("CheckServerHealth", mock.Anything).Return(fmt.Errorf("health check failed"))
				return mockGRPCClient
			},
			expectError: true,
		},
		{
			name: "successful run with pprof enabled",
			setupConfig: func() *MockConfig {
				mockConfig := &MockConfig{}
				pprofPort, _ := randomAvailablePort()
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeSTANDALONE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("0")
				mockConfig.On("HTTPPort").Return("0")
				mockConfig.On("HTTPSPort").Return("0")
				mockConfig.On("TLSEnabled").Return(false)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(true)
				mockConfig.On("PprofPort").Return(pprofPort)
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("0")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			expectError: false,
		}, {
			name: "successful run in NODE mode with signal",
			setupConfig: func() *MockConfig {
				mockConfig := &MockConfig{}
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeNODE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("0")
				mockConfig.On("HTTPPort").Return("0")
				mockConfig.On("HTTPSPort").Return("0")
				mockConfig.On("TLSEnabled").Return(false)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(false)
				mockConfig.On("PprofPort").Return("0")
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("0")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			setupGrpcClient: func() *MockGRPCClient {
				mockGRPCClient := &MockGRPCClient{}
				mockGRPCClient.On("CheckServerHealth", mock.Anything).Return(nil)
				mockGRPCClient.On("SubscribeEvents", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockGRPCClient.On("Close").Return(nil)
				return mockGRPCClient
			},
			expectError: false,
		}, {
			name: "successful run in NODE mode with signal buf error when closing",
			setupConfig: func() *MockConfig {
				mockConfig := &MockConfig{}
				mockConfig.On("KeyLoc").Return(keyLoc)
				mockConfig.On("Mode").Return(types.ServerModeNODE)
				mockConfig.On("Domain").Return("example.com")
				mockConfig.On("SSHPort").Return("0")
				mockConfig.On("HTTPPort").Return("0")
				mockConfig.On("HTTPSPort").Return("0")
				mockConfig.On("TLSEnabled").Return(false)
				mockConfig.On("TLSRedirect").Return(false)
				mockConfig.On("ACMEEmail").Return("test@example.com")
				mockConfig.On("CFAPIToken").Return("fake-token")
				mockConfig.On("ACMEStaging").Return(true)
				mockConfig.On("AllowedPortsStart").Return(uint16(1024))
				mockConfig.On("AllowedPortsEnd").Return(uint16(65535))
				mockConfig.On("BufferSize").Return(4096)
				mockConfig.On("PprofEnabled").Return(false)
				mockConfig.On("PprofPort").Return("0")
				mockConfig.On("GRPCAddress").Return("localhost")
				mockConfig.On("GRPCPort").Return("0")
				mockConfig.On("NodeToken").Return("fake-node-token")
				return mockConfig
			},
			setupGrpcClient: func() *MockGRPCClient {
				mockGRPCClient := &MockGRPCClient{}
				mockGRPCClient.On("CheckServerHealth", mock.Anything).Return(nil)
				mockGRPCClient.On("SubscribeEvents", mock.Anything, mock.Anything, mock.Anything).Return(nil)
				mockGRPCClient.On("Close").Return(fmt.Errorf("you fucked up, buddy"))
				return mockGRPCClient
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := tt.setupConfig()
			mockGRPCClient := &MockGRPCClient{}
			bootstrap := &Bootstrap{
				Randomizer:      mockRandom,
				Config:          mockConfig,
				SessionRegistry: mockSessionRegistry,
				Port:            mockPort,
				ErrChan:         mockErrChan,
				SignalChan:      mockSignalChan,
				GrpcClient:      mockGRPCClient,
			}

			if tt.setupGrpcClient != nil {
				bootstrap.GrpcClient = tt.setupGrpcClient()
			}

			done := make(chan error, 1)
			go func() {
				done <- bootstrap.Run()
			}()

			if tt.expectError {
				err := <-done
				assert.Error(t, err)
			} else if tt.name == "successful run with pprof enabled" {
				time.Sleep(200 * time.Millisecond)
				fmt.Println(mockConfig.PprofPort())
				resp, err := http.Get(fmt.Sprintf("http://localhost:%s/debug/pprof/", mockConfig.PprofPort()))
				assert.NoError(t, err)
				assert.Equal(t, 200, resp.StatusCode)
				err = resp.Body.Close()
				assert.NoError(t, err)
				mockSignalChan <- os.Interrupt
				err = <-done
				assert.NoError(t, err)
			} else {
				time.Sleep(time.Second)
				mockSignalChan <- os.Interrupt
				err := <-done
				assert.NoError(t, err)
			}
		})
	}
}
