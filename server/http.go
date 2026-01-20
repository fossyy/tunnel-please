package server

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/httpheader"
	"tunnel_pls/session"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type HTTPServer interface {
	ListenAndServe() error
	ListenAndServeTLS() error
}
type httpServer struct {
	sessionRegistry session.Registry
	redirectTLS     bool
}

func NewHTTPServer(sessionRegistry session.Registry, redirectTLS bool) HTTPServer {
	return &httpServer{
		sessionRegistry: sessionRegistry,
		redirectTLS:     redirectTLS,
	}
}

func (hs *httpServer) ListenAndServe() error {
	httpPort := config.Getenv("HTTP_PORT", "8080")
	listener, err := net.Listen("tcp", ":"+httpPort)
	if err != nil {
		return errors.New("Error listening: " + err.Error())
	}
	go func() {
		for {
			var conn net.Conn
			conn, err = listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				log.Printf("Error accepting connection: %v", err)
				continue
			}

			go hs.handler(conn, false)
		}
	}()
	return nil
}

func (hs *httpServer) ListenAndServeTLS() error {
	domain := config.Getenv("DOMAIN", "localhost")
	httpsPort := config.Getenv("HTTPS_PORT", "8443")

	tlsConfig, err := NewTLSConfig(domain)
	if err != nil {
		return fmt.Errorf("failed to initialize TLS config: %w", err)
	}

	ln, err := tls.Listen("tcp", ":"+httpsPort, tlsConfig)
	if err != nil {
		return err
	}

	go func() {
		for {
			var conn net.Conn
			conn, err = ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Println("https server closed")
				}
				log.Printf("Error accepting connection: %v", err)
				continue
			}

			go hs.handler(conn, true)
		}
	}()
	return nil
}

func (hs *httpServer) redirect(conn net.Conn, status int, location string) error {
	_, err := conn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d Moved Permanently\r\n", status) +
		fmt.Sprintf("Location: %s", location) +
		"Content-Length: 0\r\n" +
		"Connection: close\r\n" +
		"\r\n"))
	if err != nil {
		return err
	}
	return nil
}

func (hs *httpServer) badRequest(conn net.Conn) error {
	if _, err := conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n")); err != nil {
		return err
	}
	return nil
}

func (hs *httpServer) handler(conn net.Conn, isTLS bool) {
	defer hs.closeConnection(conn)

	dstReader := bufio.NewReader(conn)
	reqhf, err := httpheader.NewRequestHeader(dstReader)
	if err != nil {
		log.Printf("Error creating request header: %v", err)
		return
	}

	slug, err := hs.extractSlug(reqhf)
	if err != nil {
		_ = hs.badRequest(conn)
		return
	}

	if hs.shouldRedirectToTLS(isTLS) {
		_ = hs.redirect(conn, http.StatusMovedPermanently, fmt.Sprintf("Location: https://%s.%s/\r\n", slug, config.Getenv("DOMAIN", "localhost")))
		return
	}

	if hs.handlePingRequest(slug, conn) {
		return
	}

	sshSession, err := hs.getSession(slug)
	if err != nil {
		_ = hs.redirect(conn, http.StatusMovedPermanently, fmt.Sprintf("https://tunnl.live/tunnel-not-found?slug=%s\r\n", slug))
		return
	}

	hw := NewHTTPWriter(conn, dstReader, conn.RemoteAddr())
	hs.forwardRequest(hw, reqhf, sshSession)
}

func (hs *httpServer) closeConnection(conn net.Conn) {
	err := conn.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		log.Printf("Error closing connection: %v", err)
	}
}

func (hs *httpServer) extractSlug(reqhf httpheader.RequestHeader) (string, error) {
	host := strings.Split(reqhf.Value("Host"), ".")
	if len(host) < 1 {
		return "", errors.New("invalid host")
	}
	return host[0], nil
}

func (hs *httpServer) shouldRedirectToTLS(isTLS bool) bool {
	return !isTLS && hs.redirectTLS
}

func (hs *httpServer) handlePingRequest(slug string, conn net.Conn) bool {
	if slug != "ping" {
		return false
	}

	_, err := conn.Write([]byte(
		"HTTP/1.1 200 OK\r\n" +
			"Content-Length: 0\r\n" +
			"Connection: close\r\n" +
			"Access-Control-Allow-Origin: *\r\n" +
			"Access-Control-Allow-Methods: GET, HEAD, OPTIONS\r\n" +
			"Access-Control-Allow-Headers: *\r\n" +
			"\r\n",
	))
	if err != nil {
		log.Println("Failed to write 200 OK:", err)
	}
	return true
}

func (hs *httpServer) getSession(slug string) (session.Session, error) {
	sshSession, err := hs.sessionRegistry.Get(types.SessionKey{
		Id:   slug,
		Type: types.HTTP,
	})
	if err != nil {
		return nil, err
	}
	return sshSession, nil
}

func (hs *httpServer) forwardRequest(hw HTTPWriter, initialRequest httpheader.RequestHeader, sshSession session.Session) {
	channel, err := hs.openForwardedChannel(hw, sshSession)
	if err != nil {
		log.Printf("Failed to establish channel: %v", err)
		sshSession.Forwarder().WriteBadGatewayResponse(hw)
		return
	}

	hs.setupMiddlewares(hw)

	if err := hs.sendInitialRequest(hw, initialRequest, channel); err != nil {
		log.Printf("Failed to forward initial request: %v", err)
		return
	}

	sshSession.Forwarder().HandleConnection(hw, channel, hw.RemoteAddr())
}

func (hs *httpServer) openForwardedChannel(hw HTTPWriter, sshSession session.Session) (ssh.Channel, error) {
	payload := sshSession.Forwarder().CreateForwardedTCPIPPayload(hw.RemoteAddr())

	type channelResult struct {
		channel ssh.Channel
		reqs    <-chan *ssh.Request
		err     error
	}

	resultChan := make(chan channelResult, 1)

	go func() {
		channel, reqs, err := sshSession.Lifecycle().Connection().OpenChannel("forwarded-tcpip", payload)
		select {
		case resultChan <- channelResult{channel, reqs, err}:
		default:
			hs.cleanupUnusedChannel(channel, reqs)
		}
	}()

	select {
	case result := <-resultChan:
		if result.err != nil {
			return nil, result.err
		}
		go ssh.DiscardRequests(result.reqs)
		return result.channel, nil
	case <-time.After(5 * time.Second):
		return nil, errors.New("timeout opening forwarded-tcpip channel")
	}
}

func (hs *httpServer) cleanupUnusedChannel(channel ssh.Channel, reqs <-chan *ssh.Request) {
	if channel != nil {
		if err := channel.Close(); err != nil {
			log.Printf("Failed to close unused channel: %v", err)
		}
		go ssh.DiscardRequests(reqs)
	}
}

func (hs *httpServer) setupMiddlewares(hw HTTPWriter) {
	fingerprintMiddleware := NewTunnelFingerprint()
	forwardedForMiddleware := NewForwardedFor(hw.RemoteAddr())

	hw.UseResponseMiddleware(fingerprintMiddleware)
	hw.UseRequestMiddleware(forwardedForMiddleware)
}

func (hs *httpServer) sendInitialRequest(hw HTTPWriter, initialRequest httpheader.RequestHeader, channel ssh.Channel) error {
	hw.SetRequestHeader(initialRequest)

	if err := hw.ApplyRequestMiddlewares(initialRequest); err != nil {
		return fmt.Errorf("error applying request middlewares: %w", err)
	}

	if _, err := channel.Write(initialRequest.Finalize()); err != nil {
		return fmt.Errorf("error writing to channel: %w", err)
	}

	return nil
}
