package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/grpc/client"
	"tunnel_pls/session"

	"golang.org/x/crypto/ssh"
)

type Server struct {
	conn            *net.Listener
	config          *ssh.ServerConfig
	sessionRegistry session.Registry
	grpcClient      *client.Client
}

func NewServer(sshConfig *ssh.ServerConfig, sessionRegistry session.Registry, grpcClient *client.Client) (*Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", config.Getenv("PORT", "2200")))
	if err != nil {
		log.Fatalf("failed to listen on port 2200: %v", err)
		return nil, err
	}

	HttpServer := NewHTTPServer(sessionRegistry)
	err = HttpServer.ListenAndServe()
	if err != nil {
		log.Fatalf("failed to start http server: %v", err)
		return nil, err
	}

	if config.Getenv("TLS_ENABLED", "false") == "true" {
		err = HttpServer.ListenAndServeTLS()
		if err != nil {
			log.Fatalf("failed to start https server: %v", err)
			return nil, err
		}
	}

	return &Server{
		conn:            &listener,
		config:          sshConfig,
		sessionRegistry: sessionRegistry,
		grpcClient:      grpcClient,
	}, nil
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

func (s *Server) handleConnection(conn net.Conn) {
	sshConn, chans, forwardingReqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("failed to establish SSH connection: %v", err)
		err = conn.Close()
		if err != nil {
			log.Printf("failed to close SSH connection: %v", err)
			return
		}
		return
	}

	defer func(sshConn *ssh.ServerConn) {
		err = sshConn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("failed to close SSH server: %v", err)
		}
	}(sshConn)

	user := "UNAUTHORIZED"
	if s.grpcClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		_, u, _ := s.grpcClient.AuthorizeConn(ctx, sshConn.User())
		user = u
		cancel()
	}

	log.Println("SSH connection established:", sshConn.User())
	sshSession := session.New(sshConn, forwardingReqs, chans, s.sessionRegistry, user)
	err = sshSession.Start()
	if err != nil {
		log.Printf("SSH session ended with error: %v", err)
		return
	}
	return
}
