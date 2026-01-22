package transport

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewHTTPServer(t *testing.T) {
	msr := new(MockSessionRegistry)
	domain := "example.com"
	port := "8080"
	redirectTLS := true

	srv := NewHTTPServer(domain, port, msr, redirectTLS)
	assert.NotNil(t, srv)

	httpSrv, ok := srv.(*httpServer)
	assert.True(t, ok)
	assert.Equal(t, port, httpSrv.port)
	assert.Equal(t, domain, httpSrv.handler.domain)
	assert.Equal(t, msr, httpSrv.handler.sessionRegistry)
	assert.Equal(t, redirectTLS, httpSrv.handler.redirectTLS)
}

func TestHTTPServer_Listen(t *testing.T) {
	msr := new(MockSessionRegistry)
	srv := NewHTTPServer("example.com", "0", msr, false)

	listener, err := srv.Listen()
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	listener.Close()
}

func TestHTTPServer_Serve(t *testing.T) {
	msr := new(MockSessionRegistry)
	srv := NewHTTPServer("example.com", "0", msr, false)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	go func() {
		time.Sleep(100 * time.Millisecond)
		listener.Close()
	}()

	err = srv.Serve(listener)
	assert.True(t, errors.Is(err, net.ErrClosed))
}

func TestHTTPServer_Serve_AcceptError(t *testing.T) {
	msr := new(MockSessionRegistry)
	srv := NewHTTPServer("example.com", "0", msr, false)

	ml := new(mockListener)
	ml.On("Accept").Return(nil, errors.New("accept error")).Once()
	ml.On("Accept").Return(nil, net.ErrClosed).Once()

	err := srv.Serve(ml)
	assert.True(t, errors.Is(err, net.ErrClosed))
	ml.AssertExpectations(t)
}

func TestHTTPServer_Serve_Success(t *testing.T) {
	msr := new(MockSessionRegistry)
	srv := NewHTTPServer("example.com", "0", msr, false)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		_ = srv.Serve(listener)
	}()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	assert.NoError(t, err)

	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: ping.example.com\r\n\r\n"))

	time.Sleep(100 * time.Millisecond)
	conn.Close()
	listener.Close()
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
	args := m.Called()
	return args.Error(0)
}

func (m *mockListener) Addr() net.Addr {
	args := m.Called()
	return args.Get(0).(net.Addr)
}
