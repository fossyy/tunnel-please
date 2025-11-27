package session

import (
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

const (
	INITIALIZING SessionStatus = "INITIALIZING"
	RUNNING      SessionStatus = "RUNNING"
	SETUP        SessionStatus = "SETUP"
)

type TunnelType string

const (
	HTTP    TunnelType = "http"
	TCP     TunnelType = "tcp"
	UNKNOWN TunnelType = "unknown"
)

type Session struct {
	Connection    *ssh.ServerConn
	ConnChannel   ssh.Channel
	Listener      net.Listener
	TunnelType    TunnelType
	ForwardedPort uint16
	Status        SessionStatus
	Slug          string
}

func New(conn *ssh.ServerConn, forwardingReq <-chan *ssh.Request, sshChan <-chan ssh.NewChannel) {
	session := &Session{
		Status:      INITIALIZING,
		Slug:        "",
		ConnChannel: nil,
		Connection:  conn,
		TunnelType:  UNKNOWN,
	}

	go func() {
		go session.waitForRunningStatus()

		for channel := range sshChan {
			ch, reqs, _ := channel.Accept()
			if session.ConnChannel == nil {
				session.ConnChannel = ch
				session.Status = SETUP
				go session.HandleGlobalRequest(forwardingReq)
			}
			go session.HandleGlobalRequest(reqs)
		}
		err := session.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
	}()
}
