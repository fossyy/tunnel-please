package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"tunnel_pls/internal/config"

	"golang.org/x/crypto/ssh"
)

type Server struct {
	conn       *net.Listener
	config     *ssh.ServerConfig
	httpServer *http.Server
}

func (s *Server) GetConn() *net.Listener {
	return s.conn
}

func (s *Server) GetConfig() *ssh.ServerConfig {
	return s.config
}

func (s *Server) GetHttpServer() *http.Server {
	return s.httpServer
}

func NewServer(sshConfig *ssh.ServerConfig) *Server {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", config.Getenv("PORT", "2200")))
	if err != nil {
		log.Fatalf("failed to listen on port 2200: %v", err)
		return nil
	}
	if config.Getenv("TLS_ENABLED", "false") == "true" {
		err = NewHTTPSServer()
		if err != nil {
			log.Fatalf("failed to start https server: %v", err)
		}
	}
	err = NewHTTPServer()
	if err != nil {
		log.Fatalf("failed to start http server: %v", err)
	}
	return &Server{
		conn:   &listener,
		config: sshConfig,
	}
}

func (s *Server) Start() {
	log.Println("SSH server is starting on port 2200...")
	for {
		conn, err := (*s.conn).Accept()
		if err != nil {
			log.Printf("failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}
