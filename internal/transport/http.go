package transport

import (
	"errors"
	"log"
	"net"
	"tunnel_pls/internal/registry"
)

type httpServer struct {
	handler *httpHandler
	port    string
}

func NewHTTPServer(port string, sessionRegistry registry.Registry, redirectTLS bool) Transport {
	return &httpServer{
		handler: newHTTPHandler(sessionRegistry, redirectTLS),
		port:    port,
	}
}

func (ht *httpServer) Listen() (net.Listener, error) {
	return net.Listen("tcp", ":"+ht.port)
}

func (ht *httpServer) Serve(listener net.Listener) error {
	log.Printf("HTTP server is starting on port %s", ht.port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		go ht.handler.handler(conn, false)
	}
}
