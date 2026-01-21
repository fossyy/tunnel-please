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
	"tunnel_pls/internal/registry"
	"tunnel_pls/internal/transport"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type Session interface {
	HandleGlobalRequest(ch <-chan *ssh.Request) error
	HandleTCPIPForward(req *ssh.Request) error
	HandleHTTPForward(req *ssh.Request, port uint16) error
	HandleTCPForward(req *ssh.Request, addr string, port uint16) error
	Lifecycle() lifecycle.Lifecycle
	Interaction() interaction.Interaction
	Forwarder() forwarder.Forwarder
	Slug() slug.Slug
	Detail() *types.Detail
	Start() error
}

type session struct {
	config      config.Config
	initialReq  <-chan *ssh.Request
	sshChan     <-chan ssh.NewChannel
	lifecycle   lifecycle.Lifecycle
	interaction interaction.Interaction
	forwarder   forwarder.Forwarder
	slug        slug.Slug
	registry    registry.Registry
}

var blockedReservedPorts = []uint16{1080, 1433, 1521, 1900, 2049, 3306, 3389, 5432, 5900, 6379, 8080, 8443, 9000, 9200, 27017}

func New(config config.Config, conn *ssh.ServerConn, initialReq <-chan *ssh.Request, sshChan <-chan ssh.NewChannel, sessionRegistry registry.Registry, portRegistry portUtil.Port, user string) Session {
	slugManager := slug.New()
	forwarderManager := forwarder.New(config, slugManager, conn)
	lifecycleManager := lifecycle.New(conn, forwarderManager, slugManager, portRegistry, sessionRegistry, user)
	interactionManager := interaction.New(config, slugManager, forwarderManager, sessionRegistry, user, lifecycleManager.Close)

	return &session{
		config:      config,
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

func (s *session) Detail() *types.Detail {
	tunnelTypeMap := map[types.TunnelType]string{
		types.TunnelTypeHTTP: "TunnelTypeHTTP",
		types.TunnelTypeTCP:  "TunnelTypeTCP",
	}
	tunnelType, ok := tunnelTypeMap[s.forwarder.TunnelType()]
	if !ok {
		tunnelType = "TunnelTypeUNKNOWN"
	}

	return &types.Detail{
		ForwardingType: tunnelType,
		Slug:           s.slug.String(),
		UserID:         s.lifecycle.User(),
		Active:         s.lifecycle.IsActive(),
		StartedAt:      s.lifecycle.StartedAt(),
	}
}

func (s *session) Start() error {
	if err := s.setupSessionMode(); err != nil {
		return err
	}

	tcpipReq := s.waitForTCPIPForward()
	if tcpipReq == nil {
		return s.handleMissingForwardRequest()
	}

	if s.shouldRejectUnauthorized() {
		return s.denyForwardingRequest(tcpipReq, nil, nil, fmt.Sprintf("headless forwarding only allowed on node mode"))
	}

	if err := s.HandleTCPIPForward(tcpipReq); err != nil {
		return err
	}
	s.interaction.Start()

	return s.waitForSessionEnd()
}

func (s *session) setupSessionMode() error {
	select {
	case channel, ok := <-s.sshChan:
		if !ok {
			log.Println("Forwarding request channel closed")
			return nil
		}
		return s.setupInteractiveMode(channel)
	case <-time.After(500 * time.Millisecond):
		s.interaction.SetMode(types.InteractiveModeHEADLESS)
		return nil
	}
}

func (s *session) setupInteractiveMode(channel ssh.NewChannel) error {
	ch, reqs, err := channel.Accept()
	if err != nil {
		log.Printf("failed to accept channel: %v", err)
		return err
	}

	go func() {
		err = s.HandleGlobalRequest(reqs)
		if err != nil {
			log.Printf("global request handler error: %v", err)
		}
	}()

	s.lifecycle.SetChannel(ch)
	s.interaction.SetChannel(ch)
	s.interaction.SetMode(types.InteractiveModeINTERACTIVE)

	return nil
}

func (s *session) handleMissingForwardRequest() error {
	err := s.interaction.Send(fmt.Sprintf("PortRegistry forwarding request not received. Ensure you ran the correct command with -R flag. Example: ssh %s -p %s -R 80:localhost:3000", s.config.Domain, s.config.SSHPort))
	if err != nil {
		return err
	}
	if err = s.lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
	}
	return fmt.Errorf("no forwarding Request")
}

func (s *session) shouldRejectUnauthorized() bool {
	return s.interaction.Mode() == types.InteractiveModeHEADLESS &&
		s.config.Mode() == types.ServerModeSTANDALONE &&
		s.lifecycle.User() == "UNAUTHORIZED"
}

func (s *session) waitForSessionEnd() error {
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

func (s *session) handleWindowChange(req *ssh.Request) error {
	p := req.Payload
	if len(p) < 16 {
		log.Println("invalid window-change payload")
		return req.Reply(false, nil)
	}

	cols := binary.BigEndian.Uint32(p[0:4])
	rows := binary.BigEndian.Uint32(p[4:8])

	s.interaction.SetWH(int(cols), int(rows))
	return req.Reply(true, nil)
}

func (s *session) HandleGlobalRequest(GlobalRequest <-chan *ssh.Request) error {
	for req := range GlobalRequest {
		switch req.Type {
		case "shell", "pty-req":
			err := req.Reply(true, nil)
			if err != nil {
				return err
			}
		case "window-change":
			if err := s.handleWindowChange(req); err != nil {
				return err
			}
		default:
			log.Println("Unknown request type:", req.Type)
			err := req.Reply(false, nil)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *session) parseForwardPayload(payloadReader io.Reader) (address string, port uint16, err error) {
	address, err = readSSHString(payloadReader)
	if err != nil {
		return "", 0, err
	}

	var rawPortToBind uint32
	if err = binary.Read(payloadReader, binary.BigEndian, &rawPortToBind); err != nil {
		return "", 0, err
	}

	if rawPortToBind > 65535 {
		return "", 0, fmt.Errorf("port is larger than allowed port of 65535")
	}

	port = uint16(rawPortToBind)
	if isBlockedPort(port) {
		return "", 0, fmt.Errorf("port is block")
	}

	if port == 0 {
		unassigned, ok := s.lifecycle.PortRegistry().Unassigned()
		if !ok {
			return "", 0, fmt.Errorf("no available port")
		}
		return address, unassigned, err
	}

	return address, port, err
}

func (s *session) denyForwardingRequest(req *ssh.Request, key *types.SessionKey, listener io.Closer, msg string) error {
	var errs []error
	if key != nil {
		s.registry.Remove(*key)
	}
	if listener != nil {
		if err := listener.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close listener: %w", err))
		}
	}
	if err := req.Reply(false, nil); err != nil {
		errs = append(errs, fmt.Errorf("reply request: %w", err))
	}
	if err := s.lifecycle.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close session: %w", err))
	}
	errs = append(errs, fmt.Errorf("deny forwarding request: %s", msg))
	return errors.Join(errs...)
}

func (s *session) approveForwardingRequest(req *ssh.Request, port uint16) (err error) {
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, uint32(port))
	if err != nil {
		return err
	}

	err = req.Reply(true, buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func (s *session) finalizeForwarding(req *ssh.Request, portToBind uint16, listener net.Listener, tunnelType types.TunnelType, slug string) error {
	err := s.approveForwardingRequest(req, portToBind)
	if err != nil {
		return err
	}

	s.forwarder.SetType(tunnelType)
	s.forwarder.SetForwardedPort(portToBind)
	s.slug.Set(slug)
	s.lifecycle.SetStatus(types.SessionStatusRUNNING)

	if listener != nil {
		s.forwarder.SetListener(listener)
	}

	return nil
}

func (s *session) HandleTCPIPForward(req *ssh.Request) error {
	reader := bytes.NewReader(req.Payload)

	address, port, err := s.parseForwardPayload(reader)
	if err != nil {
		return s.denyForwardingRequest(req, nil, nil, fmt.Sprintf("cannot parse forwarded payload: %s", err.Error()))
	}

	switch port {
	case 80, 443:
		return s.HandleHTTPForward(req, port)
	default:
		return s.HandleTCPForward(req, address, port)
	}
}

func (s *session) HandleHTTPForward(req *ssh.Request, portToBind uint16) error {
	randomString, err := random.GenerateRandomString(20)
	if err != nil {
		return s.denyForwardingRequest(req, nil, nil, fmt.Sprintf("Failed to create slug: %s", err))
	}
	key := types.SessionKey{Id: randomString, Type: types.TunnelTypeHTTP}
	if !s.registry.Register(key, s) {
		return s.denyForwardingRequest(req, nil, nil, fmt.Sprintf("Failed to register client with slug: %s", randomString))
	}

	err = s.finalizeForwarding(req, portToBind, nil, types.TunnelTypeHTTP, key.Id)
	if err != nil {
		return s.denyForwardingRequest(req, &key, nil, fmt.Sprintf("Failed to finalize forwarding: %s", err))
	}
	return nil
}

func (s *session) HandleTCPForward(req *ssh.Request, addr string, portToBind uint16) error {
	if claimed := s.lifecycle.PortRegistry().Claim(portToBind); !claimed {
		return s.denyForwardingRequest(req, nil, nil, fmt.Sprintf("PortRegistry %d is already in use or restricted", portToBind))
	}

	tcpServer := transport.NewTCPServer(portToBind, s.forwarder)
	listener, err := tcpServer.Listen()
	if err != nil {
		return s.denyForwardingRequest(req, nil, listener, fmt.Sprintf("PortRegistry %d is already in use or restricted", portToBind))
	}

	key := types.SessionKey{Id: fmt.Sprintf("%d", portToBind), Type: types.TunnelTypeTCP}
	if !s.registry.Register(key, s) {
		return s.denyForwardingRequest(req, nil, listener, fmt.Sprintf("Failed to register TunnelTypeTCP client with id: %s", key.Id))
	}

	err = s.finalizeForwarding(req, portToBind, listener, types.TunnelTypeTCP, key.Id)
	if err != nil {
		return s.denyForwardingRequest(req, &key, listener, fmt.Sprintf("Failed to finalize forwarding: %s", err))
	}

	go func() {
		err = tcpServer.Serve(listener)
		if err != nil {
			log.Printf("Failed serving tcp server: %s\n", err)
		}
	}()

	return nil
}

func readSSHString(reader io.Reader) (string, error) {
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
