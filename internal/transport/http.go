package transport

import (
	"errors"
	"log"
	"net"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/registry"
)

type httpServer struct {
	handler *httpHandler
	config  config.Config
}

func NewHTTPServer(config config.Config, sessionRegistry registry.Registry) Transport {
	return &httpServer{
		handler: newHTTPHandler(config, sessionRegistry),
		config:  config,
	}
}

func (ht *httpServer) Listen() (net.Listener, error) {
	return net.Listen("tcp", ":"+ht.config.HTTPPort())
}

func (ht *httpServer) Serve(listener net.Listener) error {
	log.Printf("HTTP server is starting on port %s", ht.config.HTTPPort())
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go ht.handler.Handler(conn, false)
	}
}
