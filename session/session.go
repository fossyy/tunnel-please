package session

import (
	"fmt"
	"log"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"

	"golang.org/x/crypto/ssh"
)

type Session interface {
	HandleGlobalRequest(ch <-chan *ssh.Request)
	HandleTCPIPForward(req *ssh.Request)
	HandleHTTPForward(req *ssh.Request, port uint16)
	HandleTCPForward(req *ssh.Request, addr string, port uint16)
}

type SSHSession struct {
	initialReq    <-chan *ssh.Request
	sshReqChannel <-chan ssh.NewChannel
	lifecycle     lifecycle.SessionLifecycle
	interaction   interaction.Controller
	forwarder     forwarder.ForwardingController
	slugManager   slug.Manager
	registry      Registry
}

func (s *SSHSession) GetLifecycle() lifecycle.SessionLifecycle {
	return s.lifecycle
}

func (s *SSHSession) GetInteraction() interaction.Controller {
	return s.interaction
}

func (s *SSHSession) GetForwarder() forwarder.ForwardingController {
	return s.forwarder
}

func (s *SSHSession) GetSlugManager() slug.Manager {
	return s.slugManager
}

func New(conn *ssh.ServerConn, forwardingReq <-chan *ssh.Request, sshChan <-chan ssh.NewChannel, sessionRegistry Registry, user string) *SSHSession {
	slugManager := slug.NewManager()
	forwarderManager := forwarder.NewForwarder(slugManager)
	interactionManager := interaction.NewInteraction(slugManager, forwarderManager)
	lifecycleManager := lifecycle.NewLifecycle(conn, forwarderManager, slugManager, user)

	interactionManager.SetLifecycle(lifecycleManager)
	forwarderManager.SetLifecycle(lifecycleManager)
	interactionManager.SetSessionRegistry(sessionRegistry)
	lifecycleManager.SetSessionRegistry(sessionRegistry)

	return &SSHSession{
		initialReq:    forwardingReq,
		sshReqChannel: sshChan,
		lifecycle:     lifecycleManager,
		interaction:   interactionManager,
		forwarder:     forwarderManager,
		slugManager:   slugManager,
		registry:      sessionRegistry,
	}
}

type Detail struct {
	ForwardingType string    `json:"forwarding_type,omitempty"`
	Slug           string    `json:"slug,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
	Active         bool      `json:"active,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
}

func (s *SSHSession) Detail() Detail {
	return Detail{
		ForwardingType: string(s.forwarder.GetTunnelType()),
		Slug:           s.slugManager.Get(),
		UserID:         s.lifecycle.GetUser(),
		Active:         s.lifecycle.IsActive(),
		StartedAt:      s.lifecycle.StartedAt(),
	}
}

func (s *SSHSession) Start() error {
	channel := <-s.sshReqChannel
	ch, reqs, err := channel.Accept()
	if err != nil {
		log.Printf("failed to accept channel: %v", err)
		return err
	}
	go s.HandleGlobalRequest(reqs)

	tcpipReq := s.waitForTCPIPForward()
	if tcpipReq == nil {
		_, err := ch.Write([]byte(fmt.Sprintf("Port forwarding request not received. Ensure you ran the correct command with -R flag. Example: ssh %s -p %s -R 80:localhost:3000", config.Getenv("DOMAIN", "localhost"), config.Getenv("PORT", "2200"))))
		if err != nil {
			return err
		}
		if err := s.lifecycle.Close(); err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return fmt.Errorf("no forwarding Request")
	}

	s.lifecycle.SetChannel(ch)
	s.interaction.SetChannel(ch)

	s.HandleTCPIPForward(tcpipReq)

	if err := s.lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
		return err
	}
	return nil
}

func (s *SSHSession) waitForTCPIPForward() *ssh.Request {
	select {
	case req, ok := <-s.initialReq:
		if !ok {
			log.Println("Forwarding request channel closed")
			return nil
		}
		if req.Type == "tcpip-forward" {
			return req
		}
		if err := req.Reply(false, nil); err != nil {
			log.Printf("Failed to reply to request: %v", err)
		}
		log.Printf("Expected tcpip-forward request, got: %s", req.Type)
		return nil
	case <-time.After(500 * time.Millisecond):
		log.Println("No forwarding request received")
		return nil
	}
}
