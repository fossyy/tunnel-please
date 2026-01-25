package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"tunnel_pls/internal/port"
	"tunnel_pls/internal/registry"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

type MockSession struct {
	mock.Mock
}

func (m *MockSession) Lifecycle() lifecycle.Lifecycle {
	args := m.Called()
	return args.Get(0).(lifecycle.Lifecycle)
}

func (m *MockSession) Interaction() interaction.Interaction {
	args := m.Called()
	return args.Get(0).(interaction.Interaction)
}

func (m *MockSession) Forwarder() forwarder.Forwarder {
	args := m.Called()
	return args.Get(0).(forwarder.Forwarder)
}

func (m *MockSession) Slug() slug.Slug {
	args := m.Called()
	return args.Get(0).(slug.Slug)
}

func (m *MockSession) Detail() *types.Detail {
	args := m.Called()
	return args.Get(0).(*types.Detail)
}

type MockLifecycle struct {
	mock.Mock
}

func (m *MockLifecycle) Channel() ssh.Channel {
	args := m.Called()
	return args.Get(0).(ssh.Channel)
}

func (m *MockLifecycle) Connection() ssh.Conn {
	args := m.Called()
	return args.Get(0).(ssh.Conn)
}

func (m *MockLifecycle) PortRegistry() port.Port {
	args := m.Called()
	return args.Get(0).(port.Port)
}

func (m *MockLifecycle) User() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockLifecycle) SetChannel(channel ssh.Channel) {
	m.Called(channel)
}

func (m *MockLifecycle) SetStatus(status types.SessionStatus) {
	m.Called(status)
}

func (m *MockLifecycle) IsActive() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockLifecycle) StartedAt() time.Time {
	args := m.Called()
	return args.Get(0).(time.Time)
}

func (m *MockLifecycle) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockSSHConn struct {
	ssh.Conn
	mock.Mock
}

func (m *MockSSHConn) OpenChannel(name string, data []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	args := m.Called(name, data)
	if args.Get(0) == nil {
		return nil, args.Get(1).(<-chan *ssh.Request), args.Error(2)
	}
	return args.Get(0).(ssh.Channel), args.Get(1).(<-chan *ssh.Request), args.Error(2)
}

type MockSSHChannel struct {
	ssh.Channel
	mock.Mock
}

func (m *MockSSHChannel) Write(data []byte) (int, error) {
	args := m.Called(data)
	return args.Int(0), args.Error(1)
}

func (m *MockSSHChannel) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockForwarder struct {
	mock.Mock
}

func (m *MockForwarder) HandleConnection(dst io.ReadWriter, src ssh.Channel) {
	m.Called(dst, src)
}

func (m *MockForwarder) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockForwarder) TunnelType() types.TunnelType {
	args := m.Called()
	return args.Get(0).(types.TunnelType)
}

func (m *MockForwarder) ForwardedPort() uint16 {
	args := m.Called()
	return uint16(args.Int(0))
}

func (m *MockForwarder) SetType(tunnelType types.TunnelType) {
	m.Called(tunnelType)
}

func (m *MockForwarder) SetForwardedPort(port uint16) {
	m.Called(port)
}

func (m *MockForwarder) SetListener(listener net.Listener) {
	m.Called(listener)
}

func (m *MockForwarder) Listener() net.Listener {
	args := m.Called()
	return args.Get(0).(net.Listener)
}

func (m *MockForwarder) OpenForwardedChannel(ctx context.Context, origin net.Addr) (ssh.Channel, <-chan *ssh.Request, error) {
	args := m.Called(ctx, origin)
	if args.Get(0) == nil {
		return nil, args.Get(1).(<-chan *ssh.Request), args.Error(2)
	}
	return args.Get(0).(ssh.Channel), args.Get(1).(<-chan *ssh.Request), args.Error(2)
}

type MockConn struct {
	mock.Mock
	ReadBuffer *bytes.Buffer
}

func (m *MockConn) LocalAddr() net.Addr {
	args := m.Called()
	return args.Get(0).(net.Addr)
}

func (m *MockConn) SetDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockConn) SetReadDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockConn) SetWriteDeadline(t time.Time) error {
	args := m.Called(t)
	return args.Error(0)
}

func (m *MockConn) Read(b []byte) (n int, err error) {
	if m.ReadBuffer != nil {
		return m.ReadBuffer.Read(b)
	}
	args := m.Called(b)
	return args.Int(0), args.Error(1)
}

func (m *MockConn) Write(b []byte) (n int, err error) {
	args := m.Called(b)
	if args.Int(0) == -1 {
		return len(b), args.Error(1)
	}
	return args.Int(0), args.Error(1)
}

func (m *MockConn) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockConn) RemoteAddr() net.Addr {
	args := m.Called()
	return args.Get(0).(net.Addr)
}

type wrappedConn struct {
	net.Conn
	remoteAddr net.Addr
}

func (c *wrappedConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func TestNewHTTPHandler(t *testing.T) {
	msr := new(MockSessionRegistry)
	mockConfig := &MockConfig{}
	mockConfig.On("Domain").Return("domain")
	mockConfig.On("TLSRedirect").Return(false)
	hh := newHTTPHandler(mockConfig, msr)
	assert.NotNil(t, hh)
	assert.Equal(t, msr, hh.sessionRegistry)
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name        string
		isTLS       bool
		redirectTLS bool
		request     []byte
		expected    []byte
		setupMocks  func(*MockSessionRegistry)
		setupConn   func() (net.Conn, net.Conn)
		expectError bool
	}{
		{
			name:        "bad request - invalid host",
			isTLS:       false,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: invalid\r\n\r\n"),
			expected:    []byte("HTTP/1.1 400 Bad Request\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "bad request - missing host",
			isTLS:       false,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\n\r\n"),
			expected:    []byte("HTTP/1.1 400 Bad Request\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "isTLS true and redirectTLS true - no redirect",
			isTLS:       true,
			redirectTLS: true,
			request:     []byte("GET / HTTP/1.1\r\nHost: ping.domain\r\n\r\n"),
			expected:    []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\nAccess-Control-Allow-Origin: *\r\nAccess-Control-Allow-Methods: GET, HEAD, OPTIONS\r\nAccess-Control-Allow-Headers: *\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "redirect to TLS",
			isTLS:       false,
			redirectTLS: true,
			request:     []byte("GET / HTTP/1.1\r\nHost: tunnel.example.com\r\n\r\n"),
			expected:    []byte("HTTP/1.1 301 Moved Permanently\r\nLocation: https://tunnel.example.com/\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "handle ping request",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: ping.domain\r\n\r\n"),
			expected:    []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\nAccess-Control-Allow-Origin: *\r\nAccess-Control-Allow-Methods: GET, HEAD, OPTIONS\r\nAccess-Control-Allow-Headers: *\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "session not found",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte("HTTP/1.1 301 Moved Permanently\r\nLocation: https://tunnl.live/tunnel-not-found?slug=test\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
				msr.On("Get", types.SessionKey{
					Id:   "test",
					Type: types.TunnelTypeHTTP,
				}).Return((registry.Session)(nil), fmt.Errorf("session not found"))
			},
		},
		{
			name:        "bad request - invalid http",
			isTLS:       false,
			redirectTLS: false,
			request:     []byte("INVALID\r\n\r\n"),
			expected:    []byte("HTTP/1.1 400 Bad Request\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "bad request - header too large",
			isTLS:       false,
			redirectTLS: false,
			request:     []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: test.domain\r\n%s\r\n\r\n", strings.Repeat("test", 10000))),
			expected:    []byte("HTTP/1.1 400 Bad Request\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "bad request - no request",
			isTLS:       false,
			redirectTLS: false,
			request:     []byte(""),
			expected:    []byte("HTTP/1.1 400 Bad Request\r\n\r\n"),
			setupMocks: func(msr *MockSessionRegistry) {
			},
		},
		{
			name:        "forwarding - open channel fails",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte(""),
			setupMocks: func(msr *MockSessionRegistry) {
				mockSession := new(MockSession)
				mockForwarder := new(MockForwarder)

				msr.On("Get", types.SessionKey{
					Id:   "test",
					Type: types.TunnelTypeHTTP,
				}).Return(mockSession, nil)

				mockSession.On("Forwarder").Return(mockForwarder)

				mockForwarder.On("CreateForwardedTCPIPPayload", mock.Anything).Return([]byte("payload"))
				mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return((ssh.Channel)(nil), (<-chan *ssh.Request)(nil), fmt.Errorf("open channel failed"))
			},
		},
		{
			name:        "forwarding - send initial request fails",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte(""),
			setupMocks: func(msr *MockSessionRegistry) {
				mockSession := new(MockSession)
				mockForwarder := new(MockForwarder)
				mockSSHChannel := new(MockSSHChannel)

				msr.On("Get", types.SessionKey{
					Id:   "test",
					Type: types.TunnelTypeHTTP,
				}).Return(mockSession, nil)

				mockSession.On("Forwarder").Return(mockForwarder)

				mockForwarder.On("CreateForwardedTCPIPPayload", mock.Anything).Return([]byte("payload"))

				reqCh := make(chan *ssh.Request)
				mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(mockSSHChannel, (<-chan *ssh.Request)(reqCh), nil)

				mockSSHChannel.On("Write", mock.Anything).Return(0, fmt.Errorf("write error"))
				mockSSHChannel.On("Close").Return(nil)

				go func() {
					for range reqCh {
					}
				}()
			},
		},
		{
			name:        "forwarding - success",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nServer: Tunnel Please\r\n\r\nhello"),
			setupMocks: func(msr *MockSessionRegistry) {
				mockSession := new(MockSession)
				mockForwarder := new(MockForwarder)
				mockSSHChannel := new(MockSSHChannel)

				msr.On("Get", types.SessionKey{
					Id:   "test",
					Type: types.TunnelTypeHTTP,
				}).Return(mockSession, nil)

				mockSession.On("Forwarder").Return(mockForwarder)

				mockForwarder.On("CreateForwardedTCPIPPayload", mock.Anything).Return([]byte("payload"))

				reqCh := make(chan *ssh.Request)
				mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(mockSSHChannel, (<-chan *ssh.Request)(reqCh), nil)

				mockSSHChannel.On("Write", mock.Anything).Return(0, nil)
				mockSSHChannel.On("Close").Return(nil)

				mockForwarder.On("HandleConnection", mock.Anything, mockSSHChannel).Run(func(args mock.Arguments) {
					w := args.Get(0).(io.ReadWriter)
					_, _ = w.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
				})

				go func() {
					for range reqCh {
					}
				}()
			},
		},
		{
			name:        "redirect - write failure",
			isTLS:       false,
			redirectTLS: true,
			request:     []byte("GET / HTTP/1.1\r\nHost: example.domain\r\n\r\n"),
			expected:    []byte(""),
			setupConn: func() (net.Conn, net.Conn) {
				mc := new(MockConn)
				mc.ReadBuffer = bytes.NewBuffer([]byte("GET / HTTP/1.1\r\nHost: example.domain\r\n\r\n"))
				mc.On("SetReadDeadline", mock.Anything).Return(nil)
				mc.On("Write", mock.Anything).Return(-1, fmt.Errorf("write error"))
				mc.On("Close").Return(nil)
				return mc, nil
			},
		},
		{
			name:        "bad request - write failure",
			isTLS:       false,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\n\r\n"),
			expected:    []byte(""),
			setupConn: func() (net.Conn, net.Conn) {
				mc := new(MockConn)
				mc.ReadBuffer = bytes.NewBuffer([]byte("GET / HTTP/1.1\r\n\r\n"))
				mc.On("SetReadDeadline", mock.Anything).Return(nil)
				mc.On("Write", mock.Anything).Return(0, fmt.Errorf("write error"))
				mc.On("Close").Return(nil)
				return mc, nil
			},
		},
		{
			name:        "read error - connection failure",
			isTLS:       false,
			redirectTLS: false,
			request:     []byte(""),
			expected:    []byte(""),
			setupConn: func() (net.Conn, net.Conn) {
				mc := new(MockConn)
				mc.On("SetReadDeadline", mock.Anything).Return(nil)
				mc.On("Write", mock.Anything).Return(0, fmt.Errorf("write error"))
				mc.On("Read", mock.Anything).Return(0, fmt.Errorf("connection reset by peer"))
				mc.On("Close").Return(nil)
				return mc, nil
			},
		},
		{
			name:        "handle ping request - write failure",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: ping.domain\r\n\r\n"),
			expected:    []byte(""),
			setupConn: func() (net.Conn, net.Conn) {
				mc := new(MockConn)
				mc.ReadBuffer = bytes.NewBuffer([]byte("GET / HTTP/1.1\r\nHost: ping.domain\r\n\r\n"))
				mc.On("SetReadDeadline", mock.Anything).Return(nil)
				mc.On("Write", mock.Anything).Return(0, fmt.Errorf("write error"))
				mc.On("Close").Return(nil)
				return mc, nil
			},
		},
		{
			name:        "close connection - error",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: ping.domain\r\n\r\n"),
			expected:    []byte(""),
			setupConn: func() (net.Conn, net.Conn) {
				mc := new(MockConn)
				mc.ReadBuffer = bytes.NewBuffer([]byte("GET / HTTP/1.1\r\nHost: ping.domain\r\n\r\n"))
				mc.On("SetReadDeadline", mock.Anything).Return(nil)
				mc.On("Write", mock.Anything).Return(182, nil)
				mc.On("Close").Return(fmt.Errorf("close error"))
				return mc, nil
			},
		},
		{
			name:        "forwarding - stream close error",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte(""),
			setupMocks: func(msr *MockSessionRegistry) {
				mockSession := new(MockSession)
				mockForwarder := new(MockForwarder)
				mockSSHChannel := new(MockSSHChannel)

				msr.On("Get", mock.Anything).Return(mockSession, nil)
				mockSession.On("Forwarder").Return(mockForwarder)

				mockForwarder.On("CreateForwardedTCPIPPayload", mock.Anything).Return([]byte("payload"))

				reqCh := make(chan *ssh.Request)
				mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(mockSSHChannel, (<-chan *ssh.Request)(reqCh), nil)

				mockSSHChannel.On("Write", mock.Anything).Return(0, nil)
				mockSSHChannel.On("Close").Return(nil)

				mockForwarder.On("HandleConnection", mock.Anything, mockSSHChannel).Return()
			},
			setupConn: func() (net.Conn, net.Conn) {
				mc := new(MockConn)
				mc.ReadBuffer = bytes.NewBuffer([]byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"))
				mc.On("SetReadDeadline", mock.Anything).Return(nil)
				mc.On("Close").Return(fmt.Errorf("stream close error")).Times(2)
				addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")
				mc.On("RemoteAddr").Return(addr)
				return mc, nil
			},
		},
		{
			name:        "forwarding - middleware failure",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte(""),
			setupMocks: func(msr *MockSessionRegistry) {
				mockSession := new(MockSession)
				mockForwarder := new(MockForwarder)
				mockSSHChannel := new(MockSSHChannel)

				msr.On("Get", mock.MatchedBy(func(k types.SessionKey) bool {
					return k.Id == "test"
				})).Return(mockSession, nil)
				mockSession.On("Forwarder").Return(mockForwarder)
				mockForwarder.On("CreateForwardedTCPIPPayload", mock.Anything).Return([]byte("payload"))

				reqCh := make(chan *ssh.Request)
				mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(mockSSHChannel, (<-chan *ssh.Request)(reqCh), nil)
				mockSSHChannel.On("Close").Return(nil)
			},
			setupConn: func() (net.Conn, net.Conn) {
				mc := new(MockConn)
				mc.ReadBuffer = bytes.NewBuffer([]byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"))
				mc.On("SetReadDeadline", mock.Anything).Return(nil)
				mc.On("Close").Return(nil).Times(2)
				mc.On("RemoteAddr").Return(&net.IPAddr{IP: net.ParseIP("127.0.0.1")})
				return mc, nil
			},
		},
		{
			name:        "forwarding - channel close error",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nServer: Tunnel Please\r\n\r\nhello"),
			setupMocks: func(msr *MockSessionRegistry) {
				mockSession := new(MockSession)
				mockForwarder := new(MockForwarder)
				mockSSHChannel := new(MockSSHChannel)

				msr.On("Get", mock.Anything).Return(mockSession, nil)
				mockSession.On("Forwarder").Return(mockForwarder)

				mockForwarder.On("CreateForwardedTCPIPPayload", mock.Anything).Return([]byte("payload"))
				reqCh := make(chan *ssh.Request)
				mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(mockSSHChannel, (<-chan *ssh.Request)(reqCh), nil)

				mockSSHChannel.On("Write", mock.Anything).Return(0, nil)
				mockSSHChannel.On("Close").Return(fmt.Errorf("close error"))

				mockForwarder.On("HandleConnection", mock.Anything, mockSSHChannel).Run(func(args mock.Arguments) {
					w := args.Get(0).(io.ReadWriter)
					_, _ = w.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
				})
			},
		},
		{
			name:        "forwarding - open channel timeout",
			isTLS:       true,
			redirectTLS: false,
			request:     []byte("GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n"),
			expected:    []byte(""),
			setupMocks: func(msr *MockSessionRegistry) {
				mockSession := new(MockSession)
				mockForwarder := new(MockForwarder)

				msr.On("Get", mock.Anything).Return(mockSession, nil)
				mockSession.On("Forwarder").Return(mockForwarder)

				mockForwarder.On("CreateForwardedTCPIPPayload", mock.Anything).Return([]byte("payload"))

				mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					ctx := args.Get(0).(context.Context)
					<-ctx.Done()
				}).Return((ssh.Channel)(nil), (<-chan *ssh.Request)(nil), context.DeadlineExceeded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSessionRegistry := new(MockSessionRegistry)
			mockConfig := &MockConfig{}
			port := "0"
			mockConfig.On("Domain").Return("example.com")
			mockConfig.On("HTTPPort").Return(port)
			mockConfig.On("HeaderSize").Return(4096)
			mockConfig.On("TLSRedirect").Return(true)
			hh := &httpHandler{
				sessionRegistry: mockSessionRegistry,
				config:          mockConfig,
			}

			if tt.setupMocks != nil {
				tt.setupMocks(mockSessionRegistry)
			}

			var serverConn, clientConn net.Conn
			if tt.setupConn != nil {
				serverConn, clientConn = tt.setupConn()
			} else {
				serverConn, clientConn = net.Pipe()
			}

			if clientConn != nil {
				defer clientConn.Close()
			}

			remoteAddr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")
			var wrappedServerConn net.Conn
			if _, ok := serverConn.(*MockConn); ok {
				wrappedServerConn = serverConn
			} else {
				wrappedServerConn = &wrappedConn{Conn: serverConn, remoteAddr: remoteAddr}
			}

			responseChan := make(chan []byte, 1)
			doneChan := make(chan struct{})

			if clientConn != nil {
				go func() {
					defer close(doneChan)
					var res []byte
					for {
						buf := make([]byte, 4096)
						n, err := clientConn.Read(buf)
						if err != nil {
							if err != io.EOF {
								t.Logf("Error reading response: %v", err)
							}
							break
						}
						res = append(res, buf[:n]...)
						if len(tt.expected) > 0 && len(res) >= len(tt.expected) {
							break
						}
					}
					responseChan <- res
				}()

				go func() {
					_, err := clientConn.Write(tt.request)
					if err != nil {
						t.Logf("Error writing request: %v", err)
					}
				}()
			} else {
				close(responseChan)
				close(doneChan)
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				hh.Handler(wrappedServerConn, tt.isTLS)
			}()

			select {
			case response := <-responseChan:
				if tt.name == "forwarding - success" || tt.name == "forwarding - channel close error" {
					resStr := string(response)
					assert.True(t, strings.HasPrefix(resStr, "HTTP/1.1 200 OK\r\n"))
					assert.Contains(t, resStr, "Content-Length: 5\r\n")
					assert.Contains(t, resStr, "Server: Tunnel Please\r\n")
					assert.True(t, strings.HasSuffix(resStr, "\r\n\r\nhello"))
				} else {
					assert.Equal(t, string(tt.expected), string(response))
				}
			case <-time.After(10 * time.Second):
				if clientConn != nil {
					t.Fatal("Test timeout - no response received")
				}
			}

			wg.Wait()
			if clientConn != nil {
				<-doneChan
			}

			mockSessionRegistry.AssertExpectations(t)
			if mc, ok := serverConn.(*MockConn); ok {
				mc.AssertExpectations(t)
			}
		})
	}
}
