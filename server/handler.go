package server

import (
	"log"
	"net"
	"tunnel_pls/session"

	"golang.org/x/crypto/ssh"
)

func (s *Server) handleConnection(conn net.Conn) {
	sshConn, chans, forwardingReqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		log.Printf("failed to establish SSH connection: %v", err)
		err := conn.Close()
		if err != nil {
			log.Printf("failed to close SSH connection: %v", err)
			return
		}
		return
	}

	log.Println("SSH connection established:", sshConn.User())

	session.New(sshConn, forwardingReqs, chans)

	return
}
