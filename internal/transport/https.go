package transport

import (
	"crypto/tls"
	"errors"
	"log"
	"net"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/registry"
)

type https struct {
	config      config.Config
	tlsConfig   *tls.Config
	httpHandler *httpHandler
}

func NewHTTPSServer(config config.Config, sessionRegistry registry.Registry, tlsConfig *tls.Config) Transport {
	return &https{
		config:      config,
		tlsConfig:   tlsConfig,
		httpHandler: newHTTPHandler(config, sessionRegistry),
	}
}

func (ht *https) Listen() (net.Listener, error) {
	return tls.Listen("tcp", ":"+ht.config.HTTPSPort(), ht.tlsConfig)
}

func (ht *https) Serve(listener net.Listener) error {
	log.Printf("HTTPS server is starting on port %s", ht.config.HTTPSPort())
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
