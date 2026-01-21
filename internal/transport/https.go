package transport

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"tunnel_pls/internal/registry"
)

type https struct {
	httpHandler *httpHandler
	domain      string
	port        string
}

func NewHTTPSServer(domain, port string, sessionRegistry registry.Registry, redirectTLS bool) Transport {
	return &https{
		httpHandler: newHTTPHandler(sessionRegistry, redirectTLS),
		domain:      domain,
		port:        port,
	}
}

func (ht *https) Listen() (net.Listener, error) {
	tlsConfig, err := NewTLSConfig(ht.domain)
	if err != nil {
		return nil, err
	}

	return tls.Listen("tcp", ":"+ht.port, tlsConfig)

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

		go ht.httpHandler.handler(conn, true)
	}
}
