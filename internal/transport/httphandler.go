package transport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"tunnel_pls/internal/http/header"
	"tunnel_pls/internal/http/stream"
	"tunnel_pls/internal/middleware"
	"tunnel_pls/internal/registry"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type httpHandler struct {
	domain          string
	sessionRegistry registry.Registry
	redirectTLS     bool
}

func newHTTPHandler(domain string, sessionRegistry registry.Registry, redirectTLS bool) *httpHandler {
	return &httpHandler{
		domain:          domain,
		sessionRegistry: sessionRegistry,
		redirectTLS:     redirectTLS,
	}
}

func (hh *httpHandler) redirect(conn net.Conn, status int, location string) error {
	_, err := conn.Write([]byte(fmt.Sprintf("TunnelTypeHTTP/1.1 %d Moved Permanently\r\n", status) +
		fmt.Sprintf("Location: %s", location) +
		"Content-Length: 0\r\n" +
		"Connection: close\r\n" +
		"\r\n"))
	if err != nil {
		return err
	}
	return nil
}

func (hh *httpHandler) badRequest(conn net.Conn) error {
	if _, err := conn.Write([]byte("TunnelTypeHTTP/1.1 400 Bad Request\r\n\r\n")); err != nil {
		return err
	}
	return nil
}

func (hh *httpHandler) handler(conn net.Conn, isTLS bool) {
	defer hh.closeConnection(conn)

	dstReader := bufio.NewReader(conn)
	reqhf, err := header.NewRequest(dstReader)
	if err != nil {
		log.Printf("Error creating request header: %v", err)
		return
	}

	slug, err := hh.extractSlug(reqhf)
	if err != nil {
		_ = hh.badRequest(conn)
		return
	}

	if hh.shouldRedirectToTLS(isTLS) {
		_ = hh.redirect(conn, http.StatusMovedPermanently, fmt.Sprintf("Location: https://%s.%s/\r\n", slug, hh.domain))
		return
	}

	if hh.handlePingRequest(slug, conn) {
		return
	}

	sshSession, err := hh.getSession(slug)
	if err != nil {
		_ = hh.redirect(conn, http.StatusMovedPermanently, fmt.Sprintf("https://tunnl.live/tunnel-not-found?slug=%s\r\n", slug))
		return
	}

	hw := stream.New(conn, dstReader, conn.RemoteAddr())
	defer func(hw stream.HTTP) {
		err = hw.Close()
		if err != nil {
			log.Printf("Error closing TunnelTypeHTTP stream: %v", err)
		}
	}(hw)
	hh.forwardRequest(hw, reqhf, sshSession)
}

func (hh *httpHandler) closeConnection(conn net.Conn) {
	err := conn.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		log.Printf("Error closing connection: %v", err)
	}
}

func (hh *httpHandler) extractSlug(reqhf header.RequestHeader) (string, error) {
	host := strings.Split(reqhf.Value("Host"), ".")
	if len(host) < 1 {
		return "", errors.New("invalid host")
	}
	return host[0], nil
}

func (hh *httpHandler) shouldRedirectToTLS(isTLS bool) bool {
	return !isTLS && hh.redirectTLS
}

func (hh *httpHandler) handlePingRequest(slug string, conn net.Conn) bool {
	if slug != "ping" {
		return false
	}

	_, err := conn.Write([]byte(
		"TunnelTypeHTTP/1.1 200 OK\r\n" +
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

func (hh *httpHandler) getSession(slug string) (registry.Session, error) {
	sshSession, err := hh.sessionRegistry.Get(types.SessionKey{
		Id:   slug,
		Type: types.TunnelTypeHTTP,
	})
	if err != nil {
		return nil, err
	}
	return sshSession, nil
}

func (hh *httpHandler) forwardRequest(hw stream.HTTP, initialRequest header.RequestHeader, sshSession registry.Session) {
	channel, err := hh.openForwardedChannel(hw, sshSession)
	if err != nil {
		log.Printf("Failed to establish channel: %v", err)
		sshSession.Forwarder().WriteBadGatewayResponse(hw)
		return
	}

	defer func() {
		err = channel.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			log.Printf("Error closing forwarded channel: %v", err)
		}
	}()

	hh.setupMiddlewares(hw)

	if err = hh.sendInitialRequest(hw, initialRequest, channel); err != nil {
		log.Printf("Failed to forward initial request: %v", err)
		return
	}
	sshSession.Forwarder().HandleConnection(hw, channel)
}

func (hh *httpHandler) openForwardedChannel(hw stream.HTTP, sshSession registry.Session) (ssh.Channel, error) {
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
			hh.cleanupUnusedChannel(channel, reqs)
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

func (hh *httpHandler) cleanupUnusedChannel(channel ssh.Channel, reqs <-chan *ssh.Request) {
	if channel != nil {
		if err := channel.Close(); err != nil {
			log.Printf("Failed to close unused channel: %v", err)
		}
		go ssh.DiscardRequests(reqs)
	}
}

func (hh *httpHandler) setupMiddlewares(hw stream.HTTP) {
	fingerprintMiddleware := middleware.NewTunnelFingerprint()
	forwardedForMiddleware := middleware.NewForwardedFor(hw.RemoteAddr())

	hw.UseResponseMiddleware(fingerprintMiddleware)
	hw.UseRequestMiddleware(forwardedForMiddleware)
}

func (hh *httpHandler) sendInitialRequest(hw stream.HTTP, initialRequest header.RequestHeader, channel ssh.Channel) error {
	hw.SetRequestHeader(initialRequest)

	if err := hw.ApplyRequestMiddlewares(initialRequest); err != nil {
		return fmt.Errorf("error applying request middlewares: %w", err)
	}

	if _, err := channel.Write(initialRequest.Finalize()); err != nil {
		return fmt.Errorf("error writing to channel: %w", err)
	}

	return nil
}
