package server

import (
	"context"
	"fmt"
	"log"
	"net"
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
	defer func(sshConn *ssh.ServerConn) {
		err = sshConn.Close()
		if err != nil {
			log.Printf("failed to close SSH server: %v", err)
		}
	}(sshConn)

	if err != nil {
		log.Printf("failed to establish SSH connection: %v", err)
		err := conn.Close()
		if err != nil {
			log.Printf("failed to close SSH connection: %v", err)
			return
		}
		return
	}
	ctx := context.Background()
	log.Println("SSH connection established:", sshConn.User())

	//Fallback: kalau auth gagal userID di set UNAUTHORIZED
	authorized, _ := s.grpcClient.AuthorizeConn(ctx, sshConn.User())

	var userID string
	if authorized {
		userID = sshConn.User()
	} else {
		userID = "UNAUTHORIZED"
	}

	sshSession := session.New(sshConn, forwardingReqs, chans, s.sessionRegistry, userID)
	err = sshSession.Start()
	if err != nil {
		log.Printf("SSH session ended with error: %v", err)
		return
	}
	return
}
