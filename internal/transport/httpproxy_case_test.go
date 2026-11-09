package transport

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/ssh"

	"tunnel_pls/internal/types"
)

// forwardThrough drives Handler for slug "test" with rawRequest and returns
// what was written toward the tunneled app and what the visitor received.
func forwardThrough(t *testing.T, rawRequest, backendReply string) (toApp, toVisitor []byte) {
	t.Helper()

	sessionRegistry := new(MockSessionRegistry)
	cfg := &MockConfig{}
	cfg.On("Domain").Return("example.com")
	cfg.On("FrontendURL").Return("https://example.com")
	cfg.On("TLSRedirect").Return(false)
	cfg.On("HeaderSize").Return(4096)

	hh := &httpHandler{sessionRegistry: sessionRegistry, config: cfg}
	mockSession := new(MockSession)
	mockForwarder := new(MockForwarder)
	mockSSHChannel := new(MockSSHChannel)

	sessionRegistry.On("Get", types.SessionKey{Id: "test", Type: types.TunnelTypeHTTP}).Return(mockSession, nil)
	mockSession.On("Forwarder").Return(mockForwarder)

	reqCh := make(chan *ssh.Request)
	mockForwarder.On("OpenForwardedChannel", mock.Anything, mock.Anything).Return(mockSSHChannel, (<-chan *ssh.Request)(reqCh), nil)

	var mu sync.Mutex
	mockSSHChannel.On("Write", mock.Anything).Run(func(args mock.Arguments) {
		mu.Lock()
		toApp = append(toApp, args.Get(0).([]byte)...)
		mu.Unlock()
	}).Return(0, nil)
	mockSSHChannel.On("Close").Return(nil)

	mockForwarder.On("HandleConnection", mock.Anything, mockSSHChannel).Run(func(args mock.Arguments) {
		w := args.Get(0).(io.ReadWriter)
		_, _ = w.Write([]byte(backendReply))
	})

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")

	go hh.Handler(&wrappedConn{Conn: serverConn, remoteAddr: addr}, true)
	go func() { _, _ = io.WriteString(clientConn, rawRequest) }()

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, _ := clientConn.Read(buf)

	return toApp, bytes.Clone(buf[:n])
}

// A visitor can spell a header any way it likes. RFC 7230 3.2 says field names
// are case-insensitive, so a forged lowercase one must not survive next to the
// address the tunnel sets itself.
func TestForwardsLowercaseForgedForwardedFor(t *testing.T) {
	toApp, _ := forwardThrough(t,
		"GET / HTTP/1.1\r\nHost: test.domain\r\nx-forwarded-for: 6.6.6.6\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok",
	)

	got := string(toApp)
	if strings.Contains(got, "6.6.6.6") {
		t.Fatalf("forged client address survived: %q", got)
	}
	if strings.Count(strings.ToLower(got), "x-forwarded-for: 127.0.0.1") != 1 {
		t.Fatalf("want exactly one X-Forwarded-For: 127.0.0.1, got %q", got)
	}
}

func TestForwardsLowercaseHost(t *testing.T) {
	toApp, _ := forwardThrough(t,
		"GET / HTTP/1.1\r\nhost: test.domain\r\n\r\n",
		"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok",
	)

	got := strings.ToLower(string(toApp))
	if strings.Count(got, "host: test.domain") != 1 {
		t.Fatalf("want exactly one host header, got %q", got)
	}
}

// Two cookies come back as two Set-Cookie lines; they must not be merged.
func TestForwardsRepeatedSetCookie(t *testing.T) {
	_, toVisitor := forwardThrough(t,
		"GET / HTTP/1.1\r\nHost: test.domain\r\n\r\n",
		"HTTP/1.1 200 OK\r\nSet-Cookie: a=1\r\nSet-Cookie: b=2\r\n\r\nok",
	)

	got := string(toVisitor)
	if !strings.Contains(got, "Set-Cookie: a=1\r\nSet-Cookie: b=2\r\n") {
		t.Fatalf("repeated Set-Cookie lost or reordered: %q", got)
	}
}
