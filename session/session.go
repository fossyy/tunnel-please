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
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type Detail struct {
	ForwardingType string    `json:"forwarding_type,omitempty"`
	Slug           string    `json:"slug,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
	Active         bool      `json:"active,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
}

type Session interface {
	HandleGlobalRequest(ch <-chan *ssh.Request)
	HandleTCPIPForward(req *ssh.Request)
	HandleHTTPForward(req *ssh.Request, port uint16)
	HandleTCPForward(req *ssh.Request, addr string, port uint16)
	Lifecycle() lifecycle.Lifecycle
	Interaction() interaction.Interaction
	Forwarder() forwarder.Forwarder
	Slug() slug.Slug
	Detail() *Detail
	Start() error
}

type session struct {
	initialReq  <-chan *ssh.Request
	sshChan     <-chan ssh.NewChannel
	lifecycle   lifecycle.Lifecycle
	interaction interaction.Interaction
	forwarder   forwarder.Forwarder
	slug        slug.Slug
	registry    Registry
}

func New(conn *ssh.ServerConn, initialReq <-chan *ssh.Request, sshChan <-chan ssh.NewChannel, sessionRegistry Registry, user string) Session {
	slugManager := slug.New()
	forwarderManager := forwarder.New(slugManager)
	interactionManager := interaction.New(slugManager, forwarderManager)
	lifecycleManager := lifecycle.New(conn, forwarderManager, slugManager, user)

	interactionManager.SetLifecycle(lifecycleManager)
	forwarderManager.SetLifecycle(lifecycleManager)
	interactionManager.SetSessionRegistry(sessionRegistry)
	lifecycleManager.SetSessionRegistry(sessionRegistry)

	return &session{
		initialReq:  initialReq,
		sshChan:     sshChan,
		lifecycle:   lifecycleManager,
		interaction: interactionManager,
		forwarder:   forwarderManager,
		slug:        slugManager,
		registry:    sessionRegistry,
	}
}

func (s *session) Lifecycle() lifecycle.Lifecycle {
	return s.lifecycle
}

func (s *session) Interaction() interaction.Interaction {
	return s.interaction
}

func (s *session) Forwarder() forwarder.Forwarder {
	return s.forwarder
}

func (s *session) Slug() slug.Slug {
	return s.slug
}

func (s *session) Detail() *Detail {
	var tunnelType string
	if s.forwarder.TunnelType() == types.HTTP {
		tunnelType = "HTTP"
	} else if s.forwarder.TunnelType() == types.TCP {
		tunnelType = "TCP"
	} else {
		tunnelType = "UNKNOWN"
	}
	return &Detail{
		ForwardingType: tunnelType,
		Slug:           s.slug.String(),
		UserID:         s.lifecycle.User(),
		Active:         s.lifecycle.IsActive(),
		StartedAt:      s.lifecycle.StartedAt(),
	}
}

func (s *session) Start() error {
	var channel ssh.NewChannel
	var ok bool
	select {
	case channel, ok = <-s.sshChan:
		if !ok {
			log.Println("Forwarding request channel closed")
			return nil
		}
		ch, reqs, err := channel.Accept()
		if err != nil {
			log.Printf("failed to accept channel: %v", err)
			return err
		}
		go s.HandleGlobalRequest(reqs)

		s.lifecycle.SetChannel(ch)
		s.interaction.SetChannel(ch)
		s.interaction.SetMode(types.INTERACTIVE)
	case <-time.After(500 * time.Millisecond):
		s.interaction.SetMode(types.HEADLESS)
	}

	tcpipReq := s.waitForTCPIPForward()
	if tcpipReq == nil {
		err := s.interaction.Send(fmt.Sprintf("Port forwarding request not received. Ensure you ran the correct command with -R flag. Example: ssh %s -p %s -R 80:localhost:3000", config.Getenv("DOMAIN", "localhost"), config.Getenv("PORT", "2200")))
		if err != nil {
			return err
		}
		if err = s.lifecycle.Close(); err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return fmt.Errorf("no forwarding Request")
	}

	if (s.interaction.Mode() == types.HEADLESS && config.Getenv("MODE", "standalone") == "standalone") && s.lifecycle.User() == "UNAUTHORIZED" {
		if err := tcpipReq.Reply(false, nil); err != nil {
			log.Printf("cannot reply to tcpip req: %s\n", err)
			return err
		}
		if err := s.lifecycle.Close(); err != nil {
			log.Printf("failed to close session: %v", err)
			return err
		}
		return nil
	}

	s.HandleTCPIPForward(tcpipReq)
	s.interaction.Start()

	s.lifecycle.Connection().Wait()
	if err := s.lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
		return err
	}
	return nil
}

func (s *session) waitForTCPIPForward() *ssh.Request {
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
