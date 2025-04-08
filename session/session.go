package session

import (
	"golang.org/x/crypto/ssh"
)

type TunnelType string

const (
	HTTP    TunnelType = "http"
	TCP     TunnelType = "tcp"
	UDP     TunnelType = "udp"
	UNKNOWN TunnelType = "unknown"
)

func New(conn *ssh.ServerConn, sshChannel <-chan ssh.NewChannel, req <-chan *ssh.Request) *Session {
	session := &Session{
		Status:        SETUP,
		Slug:          "",
		ConnChannels:  []ssh.Channel{},
		Connection:    conn,
		GlobalRequest: req,
		TunnelType:    UNKNOWN,
		SlugChannel:   make(chan bool),
		Done:          make(chan bool),
	}

	go func() {
		for newChannel := range sshChannel {
			go session.HandleSessionChannel(newChannel)
		}
	}()

	return session
}
