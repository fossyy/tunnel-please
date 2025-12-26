package session

import (
	"bytes"
	"fmt"
	"log"
	"sync"
	"time"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/utils"

	"golang.org/x/crypto/ssh"
)

var (
	clientsMutex sync.RWMutex
	Clients      = make(map[string]*SSHSession)
)

type Session interface {
	HandleGlobalRequest(ch <-chan *ssh.Request)
	HandleTCPIPForward(req *ssh.Request)
	HandleHTTPForward(req *ssh.Request, port uint16)
	HandleTCPForward(req *ssh.Request, addr string, port uint16)
}

type SSHSession struct {
	Lifecycle   lifecycle.SessionLifecycle
	Interaction interaction.Controller
	Forwarder   forwarder.ForwardingController
	SlugManager slug.Manager
}

func New(conn *ssh.ServerConn, forwardingReq <-chan *ssh.Request, sshChan <-chan ssh.NewChannel) {
	slugManager := slug.NewManager()
	forwarderManager := &forwarder.Forwarder{
		Listener:      nil,
		TunnelType:    "",
		ForwardedPort: 0,
		SlugManager:   slugManager,
	}
	interactionManager := &interaction.Interaction{
		CommandBuffer:   bytes.NewBuffer(make([]byte, 0, 20)),
		InteractiveMode: false,
		EditSlug:        "",
		SlugManager:     slugManager,
		Forwarder:       forwarderManager,
		Lifecycle:       nil,
	}
	lifecycleManager := &lifecycle.Lifecycle{
		Status:      "",
		Conn:        conn,
		Channel:     nil,
		Interaction: interactionManager,
		Forwarder:   forwarderManager,
		SlugManager: slugManager,
	}

	interactionManager.SetLifecycle(lifecycleManager)
	interactionManager.SetSlugModificator(updateClientSlug)
	forwarderManager.SetLifecycle(lifecycleManager)
	lifecycleManager.SetUnregisterClient(unregisterClient)

	session := &SSHSession{
		Lifecycle:   lifecycleManager,
		Interaction: interactionManager,
		Forwarder:   forwarderManager,
		SlugManager: slugManager,
	}

	var once sync.Once
	for channel := range sshChan {
		ch, reqs, err := channel.Accept()
		if err != nil {
			log.Printf("failed to accept channel: %v", err)
			continue
		}
		once.Do(func() {
			session.Lifecycle.SetChannel(ch)
			session.Interaction.SetChannel(ch)

			tcpipReq := session.waitForTCPIPForward(forwardingReq)
			if tcpipReq == nil {
				session.Interaction.SendMessage(fmt.Sprintf("Port forwarding request not received.\r\nEnsure you ran the correct command with -R flag.\r\nExample: ssh %s -p %s -R 80:localhost:3000\r\nFor more details, visit https://tunnl.live.\r\n\r\n", utils.Getenv("DOMAIN", "localhost"), utils.Getenv("PORT", "2200")))
				if err := session.Lifecycle.Close(); err != nil {
					log.Printf("failed to close session: %v", err)
				}
				return
			}
			session.HandleTCPIPForward(tcpipReq)
		})
		go session.HandleGlobalRequest(reqs)
	}
	if err := session.Lifecycle.Close(); err != nil {
		log.Printf("failed to close session: %v", err)
	}
}

func (s *SSHSession) waitForTCPIPForward(forwardingReq <-chan *ssh.Request) *ssh.Request {
	select {
	case req, ok := <-forwardingReq:
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

func updateClientSlug(oldSlug, newSlug string) bool {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	if _, exists := Clients[newSlug]; exists && newSlug != oldSlug {
		return false
	}

	client, ok := Clients[oldSlug]
	if !ok {
		return false
	}

	delete(Clients, oldSlug)
	client.SlugManager.Set(newSlug)
	Clients[newSlug] = client
	return true
}

func registerClient(slug string, session *SSHSession) bool {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	if _, exists := Clients[slug]; exists {
		return false
	}

	Clients[slug] = session
	return true
}

func unregisterClient(slug string) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	delete(Clients, slug)
}
