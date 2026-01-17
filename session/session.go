package session

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
	"tunnel_pls/internal/config"
	portUtil "tunnel_pls/internal/port"
	"tunnel_pls/internal/random"
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

var blockedReservedPorts = []uint16{1080, 1433, 1521, 1900, 2049, 3306, 3389, 5432, 5900, 6379, 8080, 8443, 9000, 9200, 27017}

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

	if err := s.lifecycle.Connection().Wait(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		log.Printf("ssh connection closed with error: %v", err)
	}

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

func (s *session) HandleGlobalRequest(GlobalRequest <-chan *ssh.Request) {
	for req := range GlobalRequest {
		switch req.Type {
		case "shell", "pty-req":
			err := req.Reply(true, nil)
			if err != nil {
				log.Println("Failed to reply to request:", err)
				return
			}
		case "window-change":
			p := req.Payload
			if len(p) < 16 {
				log.Println("invalid window-change payload")
				err := req.Reply(false, nil)
				if err != nil {
					log.Println("Failed to reply to request:", err)
					return
				}
				return
			}
			cols := binary.BigEndian.Uint32(p[0:4])
			rows := binary.BigEndian.Uint32(p[4:8])

			s.interaction.SetWH(int(cols), int(rows))

			err := req.Reply(true, nil)
			if err != nil {
				log.Println("Failed to reply to request:", err)
				return
			}
		default:
			log.Println("Unknown request type:", req.Type)
			err := req.Reply(false, nil)
			if err != nil {
				log.Println("Failed to reply to request:", err)
				return
			}
		}
	}
}

func (s *session) HandleTCPIPForward(req *ssh.Request) {
	log.Println("Port forwarding request detected")

	fail := func(msg string) {
		log.Println(msg)
		if err := req.Reply(false, nil); err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		if err := s.lifecycle.Close(); err != nil {
			log.Printf("failed to close session: %v", err)
		}
	}

	reader := bytes.NewReader(req.Payload)

	addr, err := readSSHString(reader)
	if err != nil {
		fail(fmt.Sprintf("Failed to read address from payload: %v", err))
		return
	}

	var rawPortToBind uint32
	if err = binary.Read(reader, binary.BigEndian, &rawPortToBind); err != nil {
		fail(fmt.Sprintf("Failed to read port from payload: %v", err))
		return
	}

	if rawPortToBind > 65535 {
		fail(fmt.Sprintf("Port %d is larger than allowed port of 65535", rawPortToBind))
		return
	}

	portToBind := uint16(rawPortToBind)
	if isBlockedPort(portToBind) {
		fail(fmt.Sprintf("Port %d is blocked or restricted", portToBind))
		return
	}

	switch portToBind {
	case 80, 443:
		s.HandleHTTPForward(req, portToBind)
	default:
		s.HandleTCPForward(req, addr, portToBind)
	}
}

func (s *session) HandleHTTPForward(req *ssh.Request, portToBind uint16) {
	fail := func(msg string, key *types.SessionKey) {
		log.Println(msg)
		if key != nil {
			s.registry.Remove(*key)
		}
		if err := req.Reply(false, nil); err != nil {
			log.Println("Failed to reply to request:", err)
		}
	}

	randomString, err := random.GenerateRandomString(20)
	if err != nil {
		fail(fmt.Sprintf("Failed to create slug: %s", err), nil)
		return
	}
	key := types.SessionKey{Id: randomString, Type: types.HTTP}
	if !s.registry.Register(key, s) {
		fail(fmt.Sprintf("Failed to register client with slug: %s", randomString), nil)
		return
	}

	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, uint32(portToBind))
	if err != nil {
		fail(fmt.Sprintf("Failed to write port to buffer: %v", err), &key)
		return
	}
	log.Printf("HTTP forwarding approved on port: %d", portToBind)

	err = req.Reply(true, buf.Bytes())
	if err != nil {
		fail(fmt.Sprintf("Failed to reply to request: %v", err), &key)
		return
	}

	s.forwarder.SetType(types.HTTP)
	s.forwarder.SetForwardedPort(portToBind)
	s.slug.Set(randomString)
	s.lifecycle.SetStatus(types.RUNNING)
}

func (s *session) HandleTCPForward(req *ssh.Request, addr string, portToBind uint16) {
	fail := func(msg string) {
		log.Println(msg)
		if err := req.Reply(false, nil); err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		if err := s.lifecycle.Close(); err != nil {
			log.Printf("failed to close session: %v", err)
		}
	}

	cleanup := func(msg string, port uint16, listener net.Listener, key *types.SessionKey) {
		log.Println(msg)
		if key != nil {
			s.registry.Remove(*key)
		}
		if port != 0 {
			if setErr := portUtil.Default.SetPortStatus(port, false); setErr != nil {
				log.Printf("Failed to reset port status: %v", setErr)
			}
		}
		if listener != nil {
			if closeErr := listener.Close(); closeErr != nil {
				log.Printf("Failed to close listener: %v", closeErr)
			}
		}
		if err := req.Reply(false, nil); err != nil {
			log.Println("Failed to reply to request:", err)
		}
		_ = s.lifecycle.Close()
	}

	if portToBind == 0 {
		unassigned, ok := portUtil.Default.GetUnassignedPort()
		if !ok {
			fail("No available port")
			return
		}
		portToBind = unassigned
	}

	if claimed := portUtil.Default.ClaimPort(portToBind); !claimed {
		fail(fmt.Sprintf("Port %d is already in use or restricted", portToBind))
		return
	}

	log.Printf("Requested forwarding on %s:%d", addr, portToBind)
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", portToBind))
	if err != nil {
		cleanup(fmt.Sprintf("Port %d is already in use or restricted", portToBind), portToBind, nil, nil)
		return
	}

	key := types.SessionKey{Id: fmt.Sprintf("%d", portToBind), Type: types.TCP}

	if !s.registry.Register(key, s) {
		cleanup(fmt.Sprintf("Failed to register TCP client with id: %s", key.Id), portToBind, listener, nil)
		return
	}

	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, uint32(portToBind))
	if err != nil {
		cleanup(fmt.Sprintf("Failed to write port to buffer: %v", err), portToBind, listener, &key)
		return
	}

	log.Printf("TCP forwarding approved on port: %d", portToBind)
	err = req.Reply(true, buf.Bytes())
	if err != nil {
		cleanup(fmt.Sprintf("Failed to reply to request: %v", err), portToBind, listener, &key)
		return
	}

	s.forwarder.SetType(types.TCP)
	s.forwarder.SetListener(listener)
	s.forwarder.SetForwardedPort(portToBind)
	s.slug.Set(key.Id)
	s.lifecycle.SetStatus(types.RUNNING)
	go s.forwarder.AcceptTCPConnections()
}

func readSSHString(reader *bytes.Reader) (string, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return "", err
	}
	strBytes := make([]byte, length)
	if _, err := reader.Read(strBytes); err != nil {
		return "", err
	}
	return string(strBytes), nil
}

func isBlockedPort(port uint16) bool {
	if port == 80 || port == 443 {
		return false
	}
	if port < 1024 && port != 0 {
		return true
	}
	for _, p := range blockedReservedPorts {
		if p == port {
			return true
		}
	}
	return false
}
