package session

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/registry"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
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
	config.Config
}

func (m *mockConfig) Domain() string  { return m.Called().String(0) }
func (m *mockConfig) SSHPort() string { return m.Called().String(0) }
func (m *mockConfig) Mode() types.ServerMode {
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
func (m *mockConfig) TLSEnabled() bool { return m.Called().Bool(0) }

type mockRegistry struct {
	mock.Mock
	registry.Registry
	removedKey types.SessionKey
}

func (m *mockRegistry) Register(key types.SessionKey, session registry.Session) bool {
	return m.Called(key, session).Bool(0)
}

func (m *mockRegistry) Remove(key types.SessionKey) {
	m.removedKey = key
}

type mockPort struct {
	mock.Mock
}

func (m *mockPort) AddRange(startPort, endPort uint16) error {
	return m.Called(startPort, endPort).Error(0)
}
func (m *mockPort) Unassigned() (uint16, bool) {
	args := m.Called()
	var port uint16
	if args.Get(0) != nil {
		switch v := args.Get(0).(type) {
		case int:
			port = uint16(v)
		case uint16:
			port = v
		case uint32:
			port = uint16(v)
		case int32:
			port = uint16(v)
		case float64:
			port = uint16(v)
		default:
			port = uint16(args.Int(0))
		}
	}
	return port, args.Bool(1)
}
func (m *mockPort) SetStatus(port uint16, assigned bool) error {
	return m.Called(port, assigned).Error(0)
}
func (m *mockPort) Claim(port uint16) bool {
	return m.Called(port).Bool(0)
}

type mockSSHConn struct {
	ssh.Conn
	mock.Mock
}

func (m *mockSSHConn) Wait() error {
	return m.Called().Error(0)
}

func (m *mockSSHConn) Close() error {
	return m.Called().Error(0)
}

func (m *mockSSHConn) User() string {
	return m.Called().String(0)
}

func setupSSH(t *testing.T) (sConn *ssh.ServerConn, sReqs <-chan *ssh.Request, sChans <-chan ssh.NewChannel, cConn ssh.Conn, cleanup func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privDER := x509.MarshalPKCS1PrivateKey(key)
	privBlock := pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privDER,
	}
	pk, err := ssh.ParsePrivateKey(pem.EncodeToMemory(&privBlock))
	require.NoError(t, err)

	sCfg := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	sCfg.AddHostKey(pk)

	cCfg := &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	var sConnObj *ssh.ServerConn
	var sChansChan <-chan ssh.NewChannel
	var sReqsChan <-chan *ssh.Request

	errChan := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			errChan <- err
			return
		}
		sConnObj, sChansChan, sReqsChan, err = ssh.NewServerConn(conn, sCfg)
		errChan <- err
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	require.NoError(t, err)
	cConnObj, cChans, cReqs, err := ssh.NewClientConn(conn, "pipe", cCfg)
	require.NoError(t, err)

	go ssh.DiscardRequests(cReqs)
	go func() {
		for newChan := range cChans {
			if newChan.ChannelType() == "session" {
				continue
			}
			err = newChan.Reject(ssh.Prohibited, "")
			assert.NoError(t, err)
		}
	}()

	select {
	case err := <-errChan:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("SSH handshake timed out")
	}

	return sConnObj, sReqsChan, sChansChan, cConnObj, func() {
		_ = cConnObj.Close()
		_ = sConnObj.Close()
		_ = l.Close()
	}
}

func TestNew(t *testing.T) {
	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          &mockConfig{},
		Conn:            &ssh.ServerConn{},
		InitialReq:      make(chan *ssh.Request),
		SshChan:         make(chan ssh.NewChannel),
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}

	s := New(conf)
	assert.NotNil(t, s)
	assert.NotNil(t, s.Lifecycle())
	assert.NotNil(t, s.Interaction())
	assert.NotNil(t, s.Forwarder())
	assert.NotNil(t, s.Slug())
}

func TestDetail(t *testing.T) {
	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          &mockConfig{},
		Conn:            &ssh.ServerConn{},
		InitialReq:      make(chan *ssh.Request),
		SshChan:         make(chan ssh.NewChannel),
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}

	s := New(conf).(*session)
	s.forwarder.SetType(types.TunnelTypeHTTP)
	s.slug.Set("test-slug")
	s.lifecycle.SetStatus(types.SessionStatusRUNNING)

	detail := s.Detail()
	assert.Equal(t, "HTTP", detail.ForwardingType)
	assert.Equal(t, "test-slug", detail.Slug)
	assert.Equal(t, "testuser", detail.UserID)
	assert.True(t, detail.Active)

	s.forwarder.SetType(types.TunnelTypeTCP)
	detail = s.Detail()
	assert.Equal(t, "TCP", detail.ForwardingType)

	s.forwarder.SetType(types.TunnelTypeUNKNOWN)
	detail = s.Detail()
	assert.Equal(t, "UNKNOWN", detail.ForwardingType)
}

func TestIsBlockedPort(t *testing.T) {
	tests := []struct {
		port     uint16
		expected bool
	}{
		{80, false},
		{443, false},
		{22, true},
		{1023, true},
		{1024, false},
		{1080, true},
		{3306, true},
		{8080, true},
		{0, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Port %d", tt.port), func(t *testing.T) {
			assert.Equal(t, tt.expected, isBlockedPort(tt.port))
		})
	}
}

func TestHandleGlobalRequest(t *testing.T) {
	_, sReqs, _, cConn, cleanup := setupSSH(t)
	defer cleanup()

	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          &mockConfig{},
		Conn:            &ssh.ServerConn{},
		InitialReq:      make(chan *ssh.Request),
		SshChan:         make(chan ssh.NewChannel),
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}
	s := New(conf).(*session)

	done := make(chan struct{})
	go func() {
		_ = s.HandleGlobalRequest(sReqs)
		close(done)
	}()

	tests := []struct {
		name      string
		reqType   string
		payload   []byte
		wantReply bool
		expected  bool
	}{
		{"shell", "shell", nil, true, true},
		{"pty-req", "pty-req", nil, true, true},
		{"window-change valid", "window-change", make([]byte, 16), true, true},
		{"window-change invalid", "window-change", make([]byte, 4), true, false},
		{"unknown", "unknown", nil, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _, err := cConn.SendRequest(tt.reqType, tt.wantReply, tt.payload)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, ok)
		})
	}

	err := cConn.Close()
	assert.NoError(t, err)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleGlobalRequest timed out after cConn.Close()")
	}
}

func TestHandleTCPIPForward_Table(t *testing.T) {
	setup := func(t *testing.T) (*session, *mockRegistry, *mockPort, *mockRandom, *ssh.ServerConn, <-chan *ssh.Request, ssh.Conn, func()) {
		sConn, sReqs, _, cConn, cleanup := setupSSH(t)
		mRegistry := &mockRegistry{}
		mPort := &mockPort{}
		mRandom := &mockRandom{}
		conf := &Config{
			Randomizer:      mRandom,
			Config:          &mockConfig{},
			Conn:            sConn,
			InitialReq:      make(chan *ssh.Request),
			SshChan:         make(chan ssh.NewChannel),
			SessionRegistry: mRegistry,
			PortRegistry:    mPort,
			User:            "testuser",
		}
		s := New(conf).(*session)
		return s, mRegistry, mPort, mRandom, sConn, sReqs, cConn, cleanup
	}

	t.Run("HTTP Forward Success", func(t *testing.T) {
		s, mRegistry, _, mRandom, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mRandom.On("String", 20).Return("test-slug-1234567890", nil)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(true)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 80)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		var req *ssh.Request
		select {
		case req = <-sReqs:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for tcpip-forward request")
		}

		err := s.HandleTCPIPForward(req)
		assert.NoError(t, err)
		assert.Equal(t, "test-slug-1234567890", s.slug.String())
	})

	t.Run("TCP Forward Success", func(t *testing.T) {
		s, mRegistry, mPort, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(true)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(true)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 0)

		mPort.On("Unassigned").Return(uint16(12345), true)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		var req *ssh.Request
		select {
		case req = <-sReqs:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for tcpip-forward request")
		}

		err := s.HandleTCPIPForward(req)
		assert.NoError(t, err)
		assert.Equal(t, uint16(12345), s.forwarder.ForwardedPort())
	})

	t.Run("Invalid Payload", func(t *testing.T) {
		s, _, _, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		payload := []byte{0, 0, 0}

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		var req *ssh.Request
		select {
		case req = <-sReqs:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for tcpip-forward request")
		}

		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
	})

	t.Run("Blocked Port", func(t *testing.T) {
		s, _, _, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 22)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		var req *ssh.Request
		select {
		case req = <-sReqs:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for tcpip-forward request")
		}

		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
	})
}

func TestStart_Table(t *testing.T) {
	setup := func(t *testing.T) (*session, *Config, ssh.Conn, func()) {
		sConn, sReqs, sChans, cConn, cleanup := setupSSH(t)
		mRegistry := &mockRegistry{}
		mPort := &mockPort{}
		mRandom := &mockRandom{}
		mConfig := &mockConfig{}
		mConfig.On("Mode").Return(types.ServerModeSTANDALONE)
		mConfig.On("Domain").Return("example.com")
		mConfig.On("SSHPort").Return("2222")

		conf := &Config{
			Randomizer:      mRandom,
			Config:          mConfig,
			Conn:            sConn,
			InitialReq:      sReqs,
			SshChan:         sChans,
			SessionRegistry: mRegistry,
			PortRegistry:    mPort,
			User:            "testuser",
		}
		s := New(conf).(*session)
		return s, conf, cConn, cleanup
	}

	t.Run("Full Success TCP", func(t *testing.T) {
		s, conf, cConn, cleanup := setup(t)
		defer cleanup()

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 0)

		conf.PortRegistry.(*mockPort).On("Claim", mock.Anything).Return(true)
		conf.PortRegistry.(*mockPort).On("Unassigned").Return(uint16(0), true)
		conf.PortRegistry.(*mockPort).On("SetStatus", mock.AnythingOfType("uint16"), mock.Anything).Return(nil)
		conf.SessionRegistry.(*mockRegistry).On("Register", mock.Anything, mock.Anything).Return(true)
		conf.Config.(*mockConfig).On("TLSEnabled").Return(false)
		go func() {
			time.Sleep(200 * time.Millisecond)
			ch, reqs, err := cConn.OpenChannel("session", nil)
			if err == nil {
				go ssh.DiscardRequests(reqs)
				time.Sleep(200 * time.Millisecond)
				_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
				time.Sleep(200 * time.Millisecond)
				write, err := ch.Write([]byte("q"))
				assert.NoError(t, err)
				assert.NotZero(t, write)
				time.Sleep(100 * time.Millisecond)
				_ = ch.Close()
			}
			_ = cConn.Close()
		}()

		err := s.Start()
		assert.NoError(t, err)
	})

	t.Run("Headless mode success", func(t *testing.T) {
		s, conf, cConn, cleanup := setup(t)
		defer cleanup()

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 80)

		conf.Randomizer.(*mockRandom).On("String", 20).Return("headless-slug", nil)
		conf.SessionRegistry.(*mockRegistry).On("Register", mock.Anything, mock.Anything).Return(true)

		go func() {
			time.Sleep(600 * time.Millisecond)
			_, _, err := cConn.SendRequest("tcpip-forward", true, payload)
			assert.NoError(t, err)

			time.Sleep(100 * time.Millisecond)
			err = cConn.Close()
			assert.NoError(t, err)

		}()

		err := s.Start()
		assert.NoError(t, err)
	})

	t.Run("Missing Forward Request", func(t *testing.T) {
		s, _, cConn, cleanup := setup(t)
		defer cleanup()

		go func() {
			time.Sleep(1200 * time.Millisecond)
			_ = cConn.Close()
		}()

		err := s.Start()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no forwarding Request")
	})

	t.Run("Unauthorized Headless", func(t *testing.T) {
		_, conf, cConn, cleanup := setup(t)
		defer cleanup()

		conf.User = "UNAUTHORIZED"
		s := New(conf).(*session)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 80)

		go func() {
			time.Sleep(600 * time.Millisecond)
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		err := s.Start()
		assert.Error(t, err)
	})
}

func TestForwardingFailures(t *testing.T) {
	setup := func(t *testing.T) (*session, *mockRegistry, *mockPort, *mockRandom, *ssh.ServerConn, <-chan *ssh.Request, ssh.Conn, func()) {
		sConn, sReqs, _, cConn, cleanup := setupSSH(t)
		mRegistry := &mockRegistry{}
		mPort := &mockPort{}
		mRandom := &mockRandom{}
		conf := &Config{
			Randomizer:      mRandom,
			Config:          &mockConfig{},
			Conn:            sConn,
			InitialReq:      make(chan *ssh.Request),
			SshChan:         make(chan ssh.NewChannel),
			SessionRegistry: mRegistry,
			PortRegistry:    mPort,
			User:            "testuser",
		}
		s := New(conf).(*session)
		return s, mRegistry, mPort, mRandom, sConn, sReqs, cConn, cleanup
	}

	t.Run("HTTP Registration Failed", func(t *testing.T) {
		s, mRegistry, _, mRandom, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mRandom.On("String", 20).Return("test-slug", nil)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(false)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 80)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		var req *ssh.Request
		select {
		case req = <-sReqs:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for tcpip-forward request")
		}

		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
	})

	t.Run("TCP Port Claim Failed", func(t *testing.T) {
		s, _, mPort, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(false)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 1234)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		var req *ssh.Request
		select {
		case req = <-sReqs:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for tcpip-forward request")
		}

		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
	})

	t.Run("HTTP Randomizer Error", func(t *testing.T) {
		s, _, _, mRandom, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mRandom.On("String", 20).Return("", fmt.Errorf("random error"))

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 80)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		req := <-sReqs
		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "random error")
	})

	t.Run("Port Registry No Port", func(t *testing.T) {
		s, _, mPort, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Unassigned").Return(uint16(0), false)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 0)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		req := <-sReqs
		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no available port")
	})

	t.Run("Port too large", func(t *testing.T) {
		s, _, _, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 70000)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		req := <-sReqs
		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "port is larger than allowed")
	})

	t.Run("TCP Registration Failed", func(t *testing.T) {
		s, mRegistry, mPort, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(true)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(false)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 1234)

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		req := <-sReqs
		err := s.HandleTCPIPForward(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Failed to register TunnelTypeTCP client")
	})

	t.Run("Finalize Forwarding Failure", func(t *testing.T) {
		s, mRegistry, _, mRandom, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mRandom.On("String", 20).Return("test-slug", nil)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(true)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], 80)

		go func() {
			_, _, err := cConn.SendRequest("tcpip-forward", true, payload)
			assert.Error(t, err, io.EOF)
		}()

		req := <-sReqs
		err := cConn.Close()
		assert.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		err = s.HandleTCPIPForward(req)
		assert.Error(t, err)
	})

	t.Run("TCP Listen Failure", func(t *testing.T) {
		s, mRegistry, mPort, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(true)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(true)

		l, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func(l net.Listener) {
			err = l.Close()
			assert.NoError(t, err)
		}(l)
		_, portStr, _ := net.SplitHostPort(l.Addr().String())
		port, _ := strconv.Atoi(portStr)

		payload := make([]byte, 4+9+4)
		binary.BigEndian.PutUint32(payload[0:4], 9)
		copy(payload[4:13], "localhost")
		binary.BigEndian.PutUint32(payload[13:17], uint32(port))

		go func() {
			_, _, _ = cConn.SendRequest("tcpip-forward", true, payload)
		}()

		req := <-sReqs
		err = s.HandleTCPIPForward(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is already in use or restricted")
	})
}

func TestSetupInteractiveMode_Error(t *testing.T) {
	sConn, _, sChans, _, cleanup := setupSSH(t)
	defer cleanup()

	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          &mockConfig{},
		Conn:            sConn,
		InitialReq:      make(chan *ssh.Request),
		SshChan:         sChans,
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}
	s := New(conf).(*session)

	mockChan := &mockNewChanFail{}
	err := s.setupInteractiveMode(mockChan)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

type mockNewChanFail struct {
	ssh.NewChannel
}

func (m *mockNewChanFail) Accept() (ssh.Channel, <-chan *ssh.Request, error) {
	return nil, nil, fmt.Errorf("accept failed")
}

func TestWaitForTCPIPForward_EdgeCases(t *testing.T) {
	t.Run("Wrong Request Type", func(t *testing.T) {
		_, sReqs, _, cConn, cleanup := setupSSH(t)
		defer cleanup()

		s := &session{initialReq: sReqs}

		go func() {
			_, _, _ = cConn.SendRequest("not-tcpip-forward", true, nil)
		}()

		req := s.waitForTCPIPForward()
		if req != nil {
			t.Error("expected nil request")
		}
	})

	t.Run("Channel Closed", func(t *testing.T) {
		initialReq := make(chan *ssh.Request)
		s := &session{initialReq: initialReq}
		close(initialReq)

		req := s.waitForTCPIPForward()
		if req != nil {
			t.Error("expected nil request")
		}
	})
}

func TestSetupSessionMode_ChannelClosed(t *testing.T) {
	sshChan := make(chan ssh.NewChannel)
	s := &session{sshChan: sshChan}
	close(sshChan)

	err := s.setupSessionMode()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStart_SetupSessionModeError(t *testing.T) {
	sshChan := make(chan ssh.NewChannel, 1)
	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          &mockConfig{},
		Conn:            &ssh.ServerConn{},
		InitialReq:      make(chan *ssh.Request),
		SshChan:         sshChan,
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}
	s := New(conf).(*session)

	mockChan := &mockNewChanFail{}
	sshChan <- mockChan

	err := s.Start()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestWaitForSessionEnd_Error(t *testing.T) {
	mConn := &mockSSHConn{}
	mConn.On("Wait").Return(fmt.Errorf("wait error"))
	mConn.On("Close").Return(nil)

	mForwarder := &mockLifecycleForwarder{}
	mForwarder.On("TunnelType").Return(types.TunnelTypeTCP)
	mForwarder.On("ForwardedPort").Return(uint16(80))
	mForwarder.On("Close").Return(fmt.Errorf("close error"))

	mSlug := &mockLifecycleSlug{}
	mSlug.On("String").Return("slug")

	mPort := &mockPort{}
	mPort.On("SetStatus", mock.Anything, mock.Anything).Return(nil)

	mRegistry := &mockRegistry{}
	mRegistry.On("Remove", mock.Anything).Return()

	l := lifecycle.New(mConn, mForwarder, mSlug, mPort, mRegistry, "testuser")
	s := &session{
		lifecycle: l,
	}

	err := s.waitForSessionEnd()
	assert.Error(t, err)
}

type mockLifecycleForwarder struct {
	mock.Mock
	lifecycle.Forwarder
}

func (m *mockLifecycleForwarder) TunnelType() types.TunnelType {
	return m.Called().Get(0).(types.TunnelType)
}
func (m *mockLifecycleForwarder) ForwardedPort() uint16 {
	args := m.Called()
	if args.Get(0) == nil {
		return 0
	}
	switch v := args.Get(0).(type) {
	case uint16:
		return v
	case uint32:
		return uint16(v)
	case uint64:
		return uint16(v)
	case uint8:
		return uint16(v)
	case uint:
		return uint16(v)
	case int:
		return uint16(v)
	case int8:
		return uint16(v)
	case int16:
		return uint16(v)
	case int32:
		return uint16(v)
	case int64:
		return uint16(v)
	case float32:
		return uint16(v)
	case float64:
		return uint16(v)
	default:
		return uint16(args.Int(0))
	}
}
func (m *mockLifecycleForwarder) Close() error {
	return m.Called().Error(0)
}

type mockLifecycleSlug struct {
	mock.Mock
}

func (m *mockLifecycleSlug) String() string { return m.Called().String(0) }
func (m *mockLifecycleSlug) Set(slug string) {
	m.Called(slug)
}

func TestHandleMissingForwardRequest(t *testing.T) {
	mConn := &mockSSHConn{}
	mConfig := &mockConfig{}
	mConfig.On("Domain").Return("example.com")
	mConfig.On("SSHPort").Return("2222")
	mConn.On("Close").Return(nil)

	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          mConfig,
		Conn:            &ssh.ServerConn{Conn: mConn},
		InitialReq:      make(chan *ssh.Request),
		SshChan:         make(chan ssh.NewChannel),
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}

	s := New(conf).(*session)

	err := s.handleMissingForwardRequest()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestParseForwardPayload_Errors(t *testing.T) {
	s := &session{}

	t.Run("Short Address", func(t *testing.T) {
		_, _, err := s.parseForwardPayload([]byte{0, 0, 0, 4})
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("Short Port", func(t *testing.T) {
		payload := append([]byte{0, 0, 0, 4}, []byte("addr")...)
		_, _, err := s.parseForwardPayload(payload)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("Blocked Port", func(t *testing.T) {
		payload := append([]byte{0, 0, 0, 4}, []byte("addr")...)
		portBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(portBuf, 22)
		payload = append(payload, portBuf...)
		_, _, err := s.parseForwardPayload(payload)
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "port is block") {
			t.Errorf("expected error to contain %q, got %q", "port is block", err.Error())
		}
	})
}

func TestDenyForwardingRequest_TunnelNotSetupYet(t *testing.T) {
	sConn, sReqs, _, cConn, cleanup := setupSSH(t)
	defer cleanup()

	mRegistry := &mockRegistry{}
	mPort := &mockPort{}
	mRandom := &mockRandom{}
	conf := &Config{
		Randomizer:      mRandom,
		Config:          &mockConfig{},
		Conn:            sConn,
		InitialReq:      sReqs,
		SshChan:         make(chan ssh.NewChannel),
		SessionRegistry: mRegistry,
		PortRegistry:    mPort,
		User:            "testuser",
	}
	s := New(conf).(*session)

	go func() {
		_, _, _ = cConn.SendRequest("tcpip-forward", true, nil)
	}()

	var req *ssh.Request
	select {
	case req = <-sReqs:
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	key := &types.SessionKey{Id: "", Type: types.TunnelTypeUNKNOWN}
	err := s.denyForwardingRequest(req, key, &mockCloser{}, "test error")
	if err == nil {
		t.Error("expected error, got nil")
	} else if !strings.Contains(err.Error(), "test error") {
		t.Errorf("expected error to contain %q, got %q", "test error", err.Error())
	}
	assert.Equal(t, *key, mRegistry.removedKey)
}

func TestDenyForwardingRequest_Full(t *testing.T) {
	setup := func(t *testing.T) (*session, *mockRegistry, *ssh.ServerConn, <-chan *ssh.Request, ssh.Conn, func()) {
		sConn, sReqs, _, cConn, cleanup := setupSSH(t)
		mRegistry := &mockRegistry{}
		conf := &Config{
			Randomizer:      &mockRandom{},
			Config:          &mockConfig{},
			Conn:            sConn,
			InitialReq:      sReqs,
			SshChan:         make(chan ssh.NewChannel),
			SessionRegistry: mRegistry,
			PortRegistry:    &mockPort{},
			User:            "testuser",
		}
		s := New(conf).(*session)
		return s, mRegistry, sConn, sReqs, cConn, cleanup
	}

	getReq := func(t *testing.T, client ssh.Conn, serverReqs <-chan *ssh.Request) *ssh.Request {
		go func() {
			_, _, _ = client.SendRequest("tcpip-forward", true, nil)
		}()
		select {
		case req, ok := <-serverReqs:
			if !ok {
				t.Fatal("channel closed")
			}
			return req
		case <-time.After(2 * time.Second):
			t.Fatal("timeout getting request")
			return nil
		}
	}

	t.Run("All Success", func(t *testing.T) {
		s, mRegistry, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		req := getReq(t, cConn, sReqs)
		key := &types.SessionKey{Id: "test", Type: types.TunnelTypeHTTP}

		s.slug.Set("test")
		s.forwarder.SetType(types.TunnelTypeHTTP)

		mCloser := &mockCloser{}
		err := s.denyForwardingRequest(req, key, mCloser, "error")
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "error") {
			t.Errorf("expected error to contain %q, got %q", "error", err.Error())
		}
		assert.Equal(t, *key, mRegistry.removedKey)
	})

	t.Run("Listener Close error", func(t *testing.T) {
		s, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		req := getReq(t, cConn, sReqs)
		mCloser := &mockCloser{err: fmt.Errorf("close error")}
		err := s.denyForwardingRequest(req, nil, mCloser, "error")
		assert.Error(t, err, net.ErrClosed)
	})

	t.Run("Reply error", func(t *testing.T) {
		s, _, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		req := getReq(t, cConn, sReqs)
		err := cConn.Close()
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = s.denyForwardingRequest(req, nil, nil, assert.AnError.Error())
		assert.Error(t, err, assert.AnError)
	})
}

func TestHandleTCPForward_Failures(t *testing.T) {
	setup := func(t *testing.T) (*session, *mockRegistry, *mockPort, *ssh.ServerConn, <-chan *ssh.Request, ssh.Conn, func()) {
		sConn, sReqs, _, cConn, cleanup := setupSSH(t)
		mRegistry := &mockRegistry{}
		mPort := &mockPort{}
		conf := &Config{
			Randomizer:      &mockRandom{},
			Config:          &mockConfig{},
			Conn:            sConn,
			InitialReq:      sReqs,
			SshChan:         make(chan ssh.NewChannel),
			SessionRegistry: mRegistry,
			PortRegistry:    mPort,
			User:            "testuser",
		}
		s := New(conf).(*session)
		return s, mRegistry, mPort, sConn, sReqs, cConn, cleanup
	}

	getReq := func(t *testing.T, client ssh.Conn, serverReqs <-chan *ssh.Request) *ssh.Request {
		go func() {
			_, _, _ = client.SendRequest("tcpip-forward", true, nil)
		}()
		select {
		case req, ok := <-serverReqs:
			if !ok {
				t.Fatal("channel closed")
			}
			return req
		case <-time.After(2 * time.Second):
			t.Fatal("timeout getting request")
			return nil
		}
	}

	t.Run("Port Claim fail", func(t *testing.T) {
		s, _, mPort, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(false)
		err := s.HandleTCPForward(getReq(t, cConn, sReqs), "localhost", 1234)
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "already in use") {
			t.Errorf("expected error to contain %q, got %q", "already in use", err.Error())
		}
	})

	t.Run("Listen fail", func(t *testing.T) {
		s, _, mPort, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(true)
		l, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func(l net.Listener) {
			err = l.Close()
			assert.NoError(t, err)
		}(l)
		port := uint16(l.Addr().(*net.TCPAddr).Port)

		err = s.HandleTCPForward(getReq(t, cConn, sReqs), "localhost", port)
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "already in use") {
			t.Errorf("expected error to contain %q, got %q", "already in use", err.Error())
		}
	})

	t.Run("Registry Register fail", func(t *testing.T) {
		s, mRegistry, mPort, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(true)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(false)
		err := s.HandleTCPForward(getReq(t, cConn, sReqs), "localhost", 0)
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "Failed to register") {
			t.Errorf("expected error to contain %q, got %q", "Failed to register", err.Error())
		}
	})

	t.Run("Finalize fail (Reply fail)", func(t *testing.T) {
		s, mRegistry, mPort, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mPort.On("Claim", mock.Anything).Return(true)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(true)
		req := getReq(t, cConn, sReqs)
		err := cConn.Close()
		assert.NoError(t, err)
		time.Sleep(100 * time.Millisecond)

		err = s.HandleTCPForward(req, "localhost", 0)
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "Failed to finalize forwarding") {
			t.Errorf("expected error to contain %q, got %q", "Failed to finalize forwarding", err.Error())
		}
	})
}

func TestHandleHTTPForward_Failures(t *testing.T) {
	setup := func(t *testing.T) (*session, *mockRegistry, *mockRandom, *ssh.ServerConn, <-chan *ssh.Request, ssh.Conn, func()) {
		sConn, sReqs, _, cConn, cleanup := setupSSH(t)
		mRegistry := &mockRegistry{}
		mRandom := &mockRandom{}
		s := New(&Config{
			Randomizer:      mRandom,
			Config:          &mockConfig{},
			Conn:            sConn,
			InitialReq:      sReqs,
			SshChan:         make(chan ssh.NewChannel),
			SessionRegistry: mRegistry,
			PortRegistry:    &mockPort{},
			User:            "testuser",
		}).(*session)
		return s, mRegistry, mRandom, sConn, sReqs, cConn, cleanup
	}

	getReq := func(t *testing.T, client ssh.Conn, serverReqs <-chan *ssh.Request) *ssh.Request {
		go func() { _, _, _ = client.SendRequest("tcpip-forward", true, nil) }()
		return <-serverReqs
	}

	t.Run("Random fail", func(t *testing.T) {
		s, _, mRandom, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mRandom.On("String", 20).Return("", fmt.Errorf("random error"))
		err := s.HandleHTTPForward(getReq(t, cConn, sReqs), 80)
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "Failed to create slug") {
			t.Errorf("expected error to contain %q, got %q", "Failed to create slug", err.Error())
		}
	})

	t.Run("Register fail", func(t *testing.T) {
		s, mRegistry, mRandom, _, sReqs, cConn, cleanup := setup(t)
		defer cleanup()
		mRandom.On("String", 20).Return("slug", nil)
		mRegistry.On("Register", mock.Anything, mock.Anything).Return(false)
		err := s.HandleHTTPForward(getReq(t, cConn, sReqs), 80)
		if err == nil {
			t.Error("expected error, got nil")
		} else if !strings.Contains(err.Error(), "Failed to register") {
			t.Errorf("expected error to contain %q, got %q", "Failed to register", err.Error())
		}
	})
}

func TestHandleGlobalRequest_Failures(t *testing.T) {
	_, sReqs, _, cConn, cleanup := setupSSH(t)
	defer cleanup()

	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          &mockConfig{},
		Conn:            &ssh.ServerConn{},
		InitialReq:      make(chan *ssh.Request),
		SshChan:         make(chan ssh.NewChannel),
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}
	s := New(conf).(*session)

	done := make(chan struct{})
	go func() {
		_ = s.HandleGlobalRequest(sReqs)
		close(done)
	}()

	tests := []struct {
		name      string
		reqType   string
		payload   []byte
		wantReply bool
		expected  bool
	}{
		{"shell", "shell", nil, true, true},
		{"pty-req", "pty-req", nil, true, true},
		{"window-change valid", "window-change", make([]byte, 16), true, true},
		{"window-change invalid", "window-change", make([]byte, 4), true, false},
		{"unknown", "unknown", nil, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, _, err := cConn.SendRequest(tt.reqType, tt.wantReply, tt.payload)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, ok)
		})
	}

	err := cConn.Close()
	assert.NoError(t, err)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleGlobalRequest timed out after cConn.Close()")
	}
}

func TestSetupInteractiveMode_GlobalRequestError(t *testing.T) {
	sConn, _, sChans, _, cleanup := setupSSH(t)
	defer cleanup()

	conf := &Config{
		Randomizer:      &mockRandom{},
		Config:          &mockConfig{},
		Conn:            sConn,
		InitialReq:      make(chan *ssh.Request),
		SshChan:         sChans,
		SessionRegistry: &mockRegistry{},
		PortRegistry:    &mockPort{},
		User:            "testuser",
	}
	s := New(conf).(*session)

	mockChan := &mockNewChanFail{}
	err := s.setupInteractiveMode(mockChan)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

type mockCloser struct {
	err error
}

func (m *mockCloser) Close() error { return m.err }
