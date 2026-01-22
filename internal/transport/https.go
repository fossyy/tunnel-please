package transport

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"tunnel_pls/internal/registry"
)

type https struct {
	tlsConfig   *tls.Config
	httpHandler *httpHandler
	domain      string
	port        string
}

func NewHTTPSServer(domain, port string, sessionRegistry registry.Registry, redirectTLS bool, tlsConfig *tls.Config) Transport {
	return &https{
		tlsConfig:   tlsConfig,
		httpHandler: newHTTPHandler(domain, sessionRegistry, redirectTLS),
		domain:      domain,
		port:        port,
	}
}

func (ht *https) Listen() (net.Listener, error) {
	return tls.Listen("tcp", ":"+ht.port, ht.tlsConfig)
}

func (ht *https) Serve(listener net.Listener) error {
	log.Printf("HTTPS server is starting on port %s", ht.port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go ht.httpHandler.Handler(conn, true)
	}
}
