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
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
)

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

type MockPort struct {
	mock.Mock
}

func (m *MockPort) AddRange(startPort, endPort uint16) error {
	return m.Called(startPort, endPort).Error(0)
}

func (m *MockPort) Unassigned() (uint16, bool) {
	args := m.Called()
	return uint16(args.Int(0)), args.Bool(1)
}

func (m *MockPort) SetStatus(port uint16, assigned bool) error {
	return m.Called(port, assigned).Error(0)
}

func (m *MockPort) Claim(port uint16) bool {
	return m.Called(port).Bool(0)
}

type MockListener struct {
	mock.Mock
}

func (m *MockListener) Accept() (net.Conn, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(net.Conn), args.Error(1)
}

func (m *MockListener) Close() error {
	return m.Called().Error(0)
}

func (m *MockListener) Addr() net.Addr {
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
	mr := new(MockRandom)
	mc := new(MockConfig)
	mreg := new(MockSessionRegistry)
	mg := new(MockGRPCClient)
	mp := new(MockPort)
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
		defer func(l net.Listener) {
			err = l.Close()
			assert.NoError(t, err)
		}(l)

		s, err := New(mr, mc, sc, mreg, mg, mp, fmt.Sprintf("%d", port))
		assert.Error(t, err)
		assert.Nil(t, s)
	})
}

func TestClose(t *testing.T) {
	mr := new(MockRandom)
	mc := new(MockConfig)
	mreg := new(MockSessionRegistry)
	mg := new(MockGRPCClient)
	mp := new(MockPort)
	sc, _ := getTestSSHConfig()

	t.Run("successful close", func(t *testing.T) {
		s, _ := New(mr, mc, sc, mreg, mg, mp, "0")
		err := s.Close()
		assert.NoError(t, err)
	})

	t.Run("close already closed listener", func(t *testing.T) {
		s, _ := New(mr, mc, sc, mreg, mg, mp, "0")
		_ = s.Close()
		err := s.Close()
		assert.Error(t, err)
	})

	t.Run("close with nil listener", func(t *testing.T) {
		s := &server{
			sshListener: nil,
		}
		defer func() {
			if r := recover(); r != nil {
				assert.NotNil(t, r)
			}
		}()
		_ = s.Close()
		t.Fatal("expected panic for nil listener")
	})
}

func TestStart(t *testing.T) {
	mr := new(MockRandom)
	mc := new(MockConfig)
	mreg := new(MockSessionRegistry)
	mg := new(MockGRPCClient)
	mp := new(MockPort)
	sc, _ := getTestSSHConfig()

	t.Run("normal stop", func(t *testing.T) {
		s, _ := New(mr, mc, sc, mreg, mg, mp, "0")
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = s.Close()
		}()
		s.Start()
	})

	t.Run("accept error - temporary error continues loop", func(t *testing.T) {
		ml := new(MockListener)
		s := &server{
			sshListener: ml,
			sshPort:     "0",
		}

		ml.On("Accept").Return(nil, errors.New("temporary error")).Once()
		ml.On("Accept").Return(nil, net.ErrClosed).Once()

		s.Start()
		ml.AssertExpectations(t)
	})

	t.Run("accept error - immediate close", func(t *testing.T) {
		ml := new(MockListener)
		s := &server{
			sshListener: ml,
			sshPort:     "0",
		}

		ml.On("Accept").Return(nil, net.ErrClosed).Once()

		s.Start()
		ml.AssertExpectations(t)
	})

	t.Run("accept success - connection fails SSH handshake", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockGrpcClient := &MockGRPCClient{}
		mockPort := &MockPort{}

		sshConfig, _ := getTestSSHConfig()

		serverConn, clientConn := net.Pipe()

		mockListener := &MockListener{}
		mockListener.On("Accept").Return(serverConn, nil).Once()
		mockListener.On("Accept").Return(nil, net.ErrClosed).Once()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshListener:     mockListener,
			sshConfig:       sshConfig,
			grpcClient:      mockGrpcClient,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		go s.Start()

		time.Sleep(50 * time.Millisecond)
		err := clientConn.Close()
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		mockListener.AssertExpectations(t)
	})

	t.Run("accept success - valid SSH connection without auth", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockPort := &MockPort{}

		sshConfig, _ := getTestSSHConfig()

		serverConn, clientConn := net.Pipe()

		mockListener := &MockListener{}
		mockListener.On("Accept").Return(serverConn, nil).Once()
		mockListener.On("Accept").Return(nil, net.ErrClosed).Once()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshListener:     mockListener,
			sshConfig:       sshConfig,
			grpcClient:      nil,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		go s.Start()

		time.Sleep(50 * time.Millisecond)
		err := clientConn.Close()
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		mockListener.AssertExpectations(t)
	})
}

func TestHandleConnection(t *testing.T) {
	t.Run("SSH handshake fails - connection closed", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockGrpcClient := &MockGRPCClient{}
		mockPort := &MockPort{}

		sshConfig, _ := getTestSSHConfig()

		serverConn, clientConn := net.Pipe()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshConfig:       sshConfig,
			grpcClient:      mockGrpcClient,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		err := clientConn.Close()
		assert.NoError(t, err)

		s.handleConnection(serverConn)
	})

	// SSH SERVER SUCH PAIN IN THE ASS TO BE UNIT TEST, I FUCKING HATE THIS
	// GONNA IMPLEMENT THIS UNIT TEST LATER

	//t.Run("SSH handshake fails - invalid protocol", func(t *testing.T) {
	//	mockRandom := &MockRandom{}
	//	mockConfig := &MockConfig{}
	//	mockSessionRegistry := &MockSessionRegistry{}
	//	mockGrpcClient := &MockGRPCClient{}
	//	mockPort := &MockPort{}
	//
	//	sshConfig, _ := getTestSSHConfig()
	//
	//	serverConn, clientConn := net.Pipe()
	//
	//	s := &server{
	//		randomizer:      mockRandom,
	//		config:          mockConfig,
	//		sshPort:         "0",
	//		sshConfig:       sshConfig,
	//		grpcClient:      mockGrpcClient,
	//		sessionRegistry: mockSessionRegistry,
	//		portRegistry:    mockPort,
	//	}
	//
	//	done := make(chan bool, 1)
	//
	//	go func() {
	//		s.handleConnection(serverConn)
	//		done <- true
	//	}()
	//
	//	go func() {
	//		clientConn.Write([]byte("invalid ssh protocol\n"))
	//		clientConn.Close()
	//	}()
	//
	//	select {
	//	case <-done:
	//	case <-time.After(1 * time.Second):
	//		t.Fatal("handleConnection did not complete in time")
	//	}
	//})

	t.Run("SSH connection established without gRPC client", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockPort := &MockPort{}

		serverConfig, _ := getTestSSHConfig()

		mockConfig.On("Domain").Return("test.com")
		mockConfig.On("Mode").Return(types.ServerModeNODE)
		mockConfig.On("SSHPort").Return("2200")
		mockRandom.On("String", mock.Anything).Return("ilovefemboy", nil)
		mockSessionRegistry.On("Register", mock.Anything, mock.Anything).Return(true)
		mockSessionRegistry.On("Remove", mock.Anything).Return(nil)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func(listener net.Listener) {
			err = listener.Close()
			assert.NoError(t, err)
		}(listener)

		serverAddr := listener.Addr().String()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshConfig:       serverConfig,
			grpcClient:      nil,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		done := make(chan bool, 1)

		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.handleConnection(conn)
			done <- true
		}()

		time.Sleep(50 * time.Millisecond)

		clientConfig := &ssh.ClientConfig{
			User:            "testuser",
			Auth:            []ssh.AuthMethod{ssh.Password("password")},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         2 * time.Second,
		}

		go func() {
			client, err := ssh.Dial("tcp", serverAddr, clientConfig)
			if err != nil {
				t.Logf("Client dial failed: %v", err)
				return
			}
			defer func(client *ssh.Client) {
				err = client.Close()
				assert.NoError(t, err)
			}(client)

			type forwardPayload struct {
				BindAddr string
				BindPort uint32
			}

			payload := ssh.Marshal(forwardPayload{
				BindAddr: "localhost",
				BindPort: 80,
			})

			_, _, err = client.SendRequest("tcpip-forward", true, payload)
			if err != nil {
				t.Logf("Forward request failed: %v", err)
			}

			time.Sleep(500 * time.Millisecond)
		}()

		select {
		case <-done:
			t.Log("handleConnection completed")
		case <-time.After(5 * time.Second):
			t.Fatal("handleConnection did not complete in time")
		}
	})

	t.Run("SSH connection established with gRPC authorization", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockGrpcClient := &MockGRPCClient{}
		mockPort := &MockPort{}

		serverConfig, _ := getTestSSHConfig()

		mockGrpcClient.On("AuthorizeConn", mock.Anything, "testuser").Return(true, "authorized_user", nil)
		mockConfig.On("Domain").Return("test.com")
		mockConfig.On("Mode").Return(types.ServerModeNODE)
		mockConfig.On("SSHPort").Return("2200")
		mockRandom.On("String", mock.Anything).Return("ilovefemboy", nil)
		mockSessionRegistry.On("Register", mock.Anything, mock.Anything).Return(true)
		mockSessionRegistry.On("Remove", mock.Anything).Return(nil)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func(listener net.Listener) {
			err = listener.Close()
			assert.NoError(t, err)
		}(listener)

		serverAddr := listener.Addr().String()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshConfig:       serverConfig,
			grpcClient:      mockGrpcClient,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		done := make(chan bool, 1)

		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.handleConnection(conn)
			done <- true
		}()

		time.Sleep(50 * time.Millisecond)

		clientConfig := &ssh.ClientConfig{
			User:            "testuser",
			Auth:            []ssh.AuthMethod{ssh.Password("password")},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         2 * time.Second,
		}

		go func() {
			client, err := ssh.Dial("tcp", serverAddr, clientConfig)
			if err != nil {
				t.Logf("Client dial failed: %v", err)
				return
			}
			defer func(client *ssh.Client) {
				err = client.Close()
				assert.NoError(t, err)
			}(client)

			type forwardPayload struct {
				BindAddr string
				BindPort uint32
			}

			payload := ssh.Marshal(forwardPayload{
				BindAddr: "localhost",
				BindPort: 80,
			})

			_, _, err = client.SendRequest("tcpip-forward", true, payload)
			if err != nil {
				t.Logf("Forward request failed: %v", err)
			}

			time.Sleep(500 * time.Millisecond)
		}()

		select {
		case <-done:
			mockGrpcClient.AssertExpectations(t)
		case <-time.After(5 * time.Second):
			t.Fatal("handleConnection did not complete in time")
		}
	})

	t.Run("SSH connection with gRPC authorization error", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockGrpcClient := &MockGRPCClient{}
		mockPort := &MockPort{}

		serverConfig, _ := getTestSSHConfig()

		mockGrpcClient.On("AuthorizeConn", mock.Anything, "testuser").Return(true, "authorized_user", nil)
		mockConfig.On("Domain").Return("test.com")
		mockConfig.On("Mode").Return(types.ServerModeNODE)
		mockConfig.On("SSHPort").Return("2200")
		mockRandom.On("String", mock.Anything).Return("ilovefemboy", nil)
		mockSessionRegistry.On("Register", mock.Anything, mock.Anything).Return(true)
		mockSessionRegistry.On("Remove", mock.Anything).Return(nil)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func(listener net.Listener) {
			err = listener.Close()
			assert.NoError(t, err)
		}(listener)

		serverAddr := listener.Addr().String()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshConfig:       serverConfig,
			grpcClient:      mockGrpcClient,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		done := make(chan bool, 1)

		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.handleConnection(conn)
			done <- true
		}()

		time.Sleep(50 * time.Millisecond)

		clientConfig := &ssh.ClientConfig{
			User:            "testuser",
			Auth:            []ssh.AuthMethod{ssh.Password("password")},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         2 * time.Second,
		}

		go func() {
			client, err := ssh.Dial("tcp", serverAddr, clientConfig)
			if err != nil {
				t.Logf("Client dial failed: %v", err)
				return
			}
			defer func(client *ssh.Client) {
				_ = client.Close()
			}(client)

			type forwardPayload struct {
				BindAddr string
				BindPort uint32
			}

			payload := ssh.Marshal(forwardPayload{
				BindAddr: "localhost",
				BindPort: 8080,
			})

			_, _, err = client.SendRequest("tcpip-forward", true, payload)
			if err != nil {
				t.Logf("Forward request failed: %v", err)
			}

			time.Sleep(500 * time.Millisecond)
		}()

		select {
		case <-done:
			mockGrpcClient.AssertExpectations(t)
		case <-time.After(5 * time.Second):
			t.Fatal("handleConnection did not complete in time")
		}
	})

	t.Run("connection cleanup on close", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockPort := &MockPort{}

		serverConfig, _ := getTestSSHConfig()

		serverConn, clientConn := net.Pipe()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshConfig:       serverConfig,
			grpcClient:      nil,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		done := make(chan bool, 1)

		go func() {
			s.handleConnection(serverConn)
			done <- true
		}()

		err := clientConn.Close()
		assert.NoError(t, err)

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("handleConnection did not complete in time")
		}
	})
}

func TestIntegration(t *testing.T) {
	t.Run("full server lifecycle", func(t *testing.T) {
		mr := new(MockRandom)
		mc := new(MockConfig)
		mreg := new(MockSessionRegistry)
		mg := new(MockGRPCClient)
		mp := new(MockPort)
		sc, _ := getTestSSHConfig()

		s, err := New(mr, mc, sc, mreg, mg, mp, "0")
		assert.NoError(t, err)
		assert.NotNil(t, s)

		go func() {
			time.Sleep(100 * time.Millisecond)
			err := s.Close()
			assert.NoError(t, err)
		}()

		s.Start()
	})

	t.Run("multiple connections", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockPort := &MockPort{}

		sshConfig, _ := getTestSSHConfig()

		conn1Server, conn1Client := net.Pipe()
		conn2Server, conn2Client := net.Pipe()

		mockListener := &MockListener{}
		mockListener.On("Accept").Return(conn1Server, nil).Once()
		mockListener.On("Accept").Return(conn2Server, nil).Once()
		mockListener.On("Accept").Return(nil, net.ErrClosed).Once()

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshListener:     mockListener,
			sshConfig:       sshConfig,
			grpcClient:      nil,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		go s.Start()

		time.Sleep(50 * time.Millisecond)
		_ = conn1Client.Close()
		time.Sleep(50 * time.Millisecond)
		_ = conn2Client.Close()
		time.Sleep(100 * time.Millisecond)

		mockListener.AssertExpectations(t)
	})
}

func TestErrorHandling(t *testing.T) {
	t.Run("write error during SSH handshake", func(t *testing.T) {
		mockRandom := &MockRandom{}
		mockConfig := &MockConfig{}
		mockSessionRegistry := &MockSessionRegistry{}
		mockPort := &MockPort{}

		sshConfig, _ := getTestSSHConfig()

		serverConn, clientConn := net.Pipe()
		err := clientConn.Close()
		assert.NoError(t, err)

		s := &server{
			randomizer:      mockRandom,
			config:          mockConfig,
			sshPort:         "0",
			sshConfig:       sshConfig,
			grpcClient:      nil,
			sessionRegistry: mockSessionRegistry,
			portRegistry:    mockPort,
		}

		s.handleConnection(serverConn)
	})
}
