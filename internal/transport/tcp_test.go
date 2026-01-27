package transport

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"
)

func TestNewTCPServer(t *testing.T) {
	mf := new(MockForwarder)
	port := uint16(9000)

	srv := NewTCPServer(port, mf)
	assert.NotNil(t, srv)

	tcpSrv, ok := srv.(*tcp)
	assert.True(t, ok)
	assert.Equal(t, port, tcpSrv.port)
	assert.Equal(t, mf, tcpSrv.forwarder)
}

func TestTCPServer_Listen(t *testing.T) {
	mf := new(MockForwarder)
	srv := NewTCPServer(0, mf)

	listener, err := srv.Listen()
	assert.NoError(t, err)
	assert.NotNil(t, listener)
	err = listener.Close()
	assert.NoError(t, err)
}

func TestTCPServer_Serve(t *testing.T) {
	mf := new(MockForwarder)
	srv := NewTCPServer(0, mf)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	go func() {
		time.Sleep(100 * time.Millisecond)
		err = listener.Close()
		assert.NoError(t, err)
	}()

	err = srv.Serve(listener)
	assert.Nil(t, err)
}

func TestTCPServer_Serve_AcceptError(t *testing.T) {
	mf := new(MockForwarder)
	srv := NewTCPServer(0, mf)

	ml := new(mockListener)
	ml.On("Accept").Return(nil, errors.New("accept error")).Once()
	ml.On("Accept").Return(nil, net.ErrClosed).Once()

	err := srv.Serve(ml)
	assert.Nil(t, err)
	ml.AssertExpectations(t)
}

func TestTCPServer_Serve_Success(t *testing.T) {
	mf := new(MockForwarder)
	srv := NewTCPServer(0, mf)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	reqs := make(chan *ssh.Request)
	mf.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(new(MockSSHChannel), (<-chan *ssh.Request)(reqs), nil)
	mf.On("HandleConnection", mock.Anything, mock.Anything).Return()

	go func() {
		_ = srv.Serve(listener)
	}()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	err = conn.Close()
	assert.NoError(t, err)
	err = listener.Close()
	assert.NoError(t, err)
	mf.AssertExpectations(t)
}

func TestTCPServer_handleTcp_Success(t *testing.T) {
	mf := new(MockForwarder)
	srv := NewTCPServer(0, mf).(*tcp)

	serverConn, clientConn := net.Pipe()
	defer func(clientConn net.Conn) {
		err := clientConn.Close()
		assert.NoError(t, err)
	}(clientConn)

	reqs := make(chan *ssh.Request)
	mockChannel := new(MockSSHChannel)
	mf.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(mockChannel, (<-chan *ssh.Request)(reqs), nil)

	mf.On("HandleConnection", serverConn, mockChannel).Return()

	srv.handleTcp(serverConn)

	mf.AssertExpectations(t)
}

func TestTCPServer_handleTcp_CloseError(t *testing.T) {
	mf := new(MockForwarder)
	srv := NewTCPServer(0, mf).(*tcp)

	mc := new(MockConn)
	mc.On("Close").Return(errors.New("close error"))
	mc.On("RemoteAddr").Return(&net.TCPAddr{})

	mf.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(nil, (<-chan *ssh.Request)(nil), errors.New("open error"))

	srv.handleTcp(mc)
	mc.AssertExpectations(t)
}

func TestTCPServer_handleTcp_OpenChannelError(t *testing.T) {
	mf := new(MockForwarder)
	srv := NewTCPServer(0, mf).(*tcp)

	serverConn, clientConn := net.Pipe()
	defer func(clientConn net.Conn) {
		err := clientConn.Close()
		assert.NoError(t, err)
	}(clientConn)

	mf.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(nil, (<-chan *ssh.Request)(nil), errors.New("open error"))

	srv.handleTcp(serverConn)

	mf.AssertExpectations(t)
}
