package transport

import (
	"bufio"
	"context"
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

func (hh *httpHandler) badRequest(conn net.Conn) error {
	if _, err := conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n")); err != nil {
		return err
	}
	return nil
}

func (hh *httpHandler) Handler(conn net.Conn, isTLS bool) {
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
		_ = hh.redirect(conn, http.StatusMovedPermanently, fmt.Sprintf("https://%s.%s/\r\n", slug, hh.domain))
		return
	}

	if hh.handlePingRequest(slug, conn) {
		return
	}

	sshSession, err := hh.sessionRegistry.Get(types.SessionKey{
		Id:   slug,
		Type: types.TunnelTypeHTTP,
	})
	if err != nil {
		_ = hh.redirect(conn, http.StatusMovedPermanently, fmt.Sprintf("https://tunnl.live/tunnel-not-found?slug=%s\r\n", slug))
		return
	}

	hw := stream.New(conn, dstReader, conn.RemoteAddr())
	defer func(hw stream.HTTP) {
		err = hw.Close()
		if err != nil {
			log.Printf("Error closing HTTP stream: %v", err)
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
	if len(host) <= 1 {
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
		return true
	}
	return true
}

func (hh *httpHandler) forwardRequest(hw stream.HTTP, initialRequest header.RequestHeader, sshSession registry.Session) {
	payload := sshSession.Forwarder().CreateForwardedTCPIPPayload(hw.RemoteAddr())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	channel, reqs, err := sshSession.Forwarder().OpenForwardedChannel(ctx, payload)
	if err != nil {
		log.Printf("Failed to open forwarded-tcpip channel: %v", err)
		return
	}

	go ssh.DiscardRequests(reqs)

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
