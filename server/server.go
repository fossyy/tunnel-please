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
	"tunnel_pls/internal/port"
	"tunnel_pls/internal/random"
	"tunnel_pls/internal/registry"
	"tunnel_pls/session"

	"golang.org/x/crypto/ssh"
)

type Server interface {
	Start()
	Close() error
}
type server struct {
	randomizer      random.Random
	config          config.Config
	sshPort         string
	sshListener     net.Listener
	sshConfig       *ssh.ServerConfig
	grpcClient      client.Client
	sessionRegistry registry.Registry
	portRegistry    port.Port
}

func New(randomizer random.Random, config config.Config, sshConfig *ssh.ServerConfig, sessionRegistry registry.Registry, grpcClient client.Client, portRegistry port.Port, sshPort string) (Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", sshPort))
	if err != nil {
		return nil, err
	}

	return &server{
		randomizer:      randomizer,
		config:          config,
		sshPort:         sshPort,
		sshListener:     listener,
		sshConfig:       sshConfig,
		grpcClient:      grpcClient,
		sessionRegistry: sessionRegistry,
		portRegistry:    portRegistry,
	}, nil
}

func (s *server) Start() {
	log.Printf("SSH server is starting on port %s", s.sshPort)
	for {
		conn, err := s.sshListener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("listener closed, stopping server")
				return
			}
			log.Printf("failed to accept connection: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *server) Close() error {
	return s.sshListener.Close()
}

func (s *server) handleConnection(conn net.Conn) {
	sshConn, chans, forwardingReqs, err := ssh.NewServerConn(conn, s.sshConfig)
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
	sshSession := session.New(s.randomizer, s.config, sshConn, forwardingReqs, chans, s.sessionRegistry, s.portRegistry, user)
	err = sshSession.Start()
	if err != nil {
		log.Printf("SSH session ended with error: %v", err)
		return
	}
	return
}
