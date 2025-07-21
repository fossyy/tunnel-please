package session

import (
	"golang.org/x/crypto/ssh"
	"net"
	"sync"
)

type TunnelType string

const (
	HTTP    TunnelType = "http"
	TCP     TunnelType = "tcp"
	UDP     TunnelType = "udp"
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
	ChannelChan   chan ssh.NewChannel
	Done          chan bool
	once          sync.Once
}

func New(conn *ssh.ServerConn, forwardingReq <-chan *ssh.Request) *Session {
	session := &Session{
		Status:      SETUP,
		Slug:        "",
		ConnChannel: nil,
		Connection:  conn,
		TunnelType:  UNKNOWN,
		ChannelChan: make(chan ssh.NewChannel),
		Done:        make(chan bool),
	}

	go func() {
		for channel := range session.ChannelChan {
			ch, reqs, _ := channel.Accept()
			if session.ConnChannel == nil {
				session.ConnChannel = ch
				session.Status = RUNNING
				go session.HandleGlobalRequest(forwardingReq)
			}
			go session.HandleGlobalRequest(reqs)
		}
	}()

	return session
}
