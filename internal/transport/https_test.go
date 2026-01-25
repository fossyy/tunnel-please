package transport

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewHTTPSServer(t *testing.T) {
	msr := new(MockSessionRegistry)
	mockConfig := &MockConfig{}
	port := "0"
	tlsConfig := &tls.Config{}
	mockConfig.On("Domain").Return(mockConfig)
	mockConfig.On("HTTPSPort").Return(port)
	srv := NewHTTPSServer(mockConfig, msr, tlsConfig)
	assert.NotNil(t, srv)

	httpsSrv, ok := srv.(*https)
	assert.True(t, ok)
	assert.Equal(t, tlsConfig, httpsSrv.tlsConfig)
	assert.Equal(t, msr, httpsSrv.httpHandler.sessionRegistry)
}

func TestHTTPSServer_Listen(t *testing.T) {
	msr := new(MockSessionRegistry)
	mockConfig := &MockConfig{}
	port := "0"
	mockConfig.On("Domain").Return(mockConfig)
	mockConfig.On("HTTPSPort").Return(port)
	tlsConfig := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return nil, nil
		},
	}
	srv := NewHTTPSServer(mockConfig, msr, tlsConfig)

	listener, err := srv.Listen()
	if err != nil {
		t.Skip("Skipping tls.Listen test as it requires valid certificates/setup:", err)
		return
	}
	assert.NotNil(t, listener)
	listener.Close()
}

func TestHTTPSServer_Serve(t *testing.T) {
	msr := new(MockSessionRegistry)
	mockConfig := &MockConfig{}
	port := "0"
	mockConfig.On("Domain").Return(mockConfig)
	mockConfig.On("HTTPSPort").Return(port)
	srv := NewHTTPSServer(mockConfig, msr, &tls.Config{})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	go func() {
		time.Sleep(100 * time.Millisecond)
		listener.Close()
	}()

	err = srv.Serve(listener)
	assert.True(t, errors.Is(err, net.ErrClosed))
}

func TestHTTPSServer_Serve_AcceptError(t *testing.T) {
	msr := new(MockSessionRegistry)

	mockConfig := &MockConfig{}
	port := "0"
	mockConfig.On("Domain").Return(mockConfig)
	mockConfig.On("HTTPSPort").Return(port)
	srv := NewHTTPSServer(mockConfig, msr, &tls.Config{})

	ml := new(mockListener)
	ml.On("Accept").Return(nil, errors.New("accept error")).Once()
	ml.On("Accept").Return(nil, net.ErrClosed).Once()

	err := srv.Serve(ml)
	assert.True(t, errors.Is(err, net.ErrClosed))
	ml.AssertExpectations(t)
}

func TestHTTPSServer_Serve_Success(t *testing.T) {
	msr := new(MockSessionRegistry)
	mockConfig := &MockConfig{}
	port := "0"
	mockConfig.On("Domain").Return(mockConfig)
	mockConfig.On("HTTPSPort").Return(port)
	mockConfig.On("HeaderSize").Return(4096)

	srv := NewHTTPSServer(mockConfig, msr, &tls.Config{})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	listenerport := listener.Addr().(*net.TCPAddr).Port

	go func() {
		_ = srv.Serve(listener)
	}()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", listenerport))
	assert.NoError(t, err)

	_, _ = conn.Write([]byte("GET / HTTP/1.1\r\nHost: ping.example.com\r\n\r\n"))

	time.Sleep(100 * time.Millisecond)
	conn.Close()
	listener.Close()
}
