package session

import (
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
	lifecycle   lifecycle.SessionLifecycle
	interaction interaction.Controller
	forwarder   forwarder.ForwardingController
	slugManager slug.Manager
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

func New(conn *ssh.ServerConn, forwardingReq <-chan *ssh.Request, sshChan <-chan ssh.NewChannel) {
	slugManager := slug.NewManager()
	forwarderManager := forwarder.NewForwarder(slugManager)
	interactionManager := interaction.NewInteraction(slugManager, forwarderManager)
	lifecycleManager := lifecycle.NewLifecycle(conn, forwarderManager, slugManager)

	interactionManager.SetLifecycle(lifecycleManager)
	interactionManager.SetSlugModificator(updateClientSlug)
	forwarderManager.SetLifecycle(lifecycleManager)
	lifecycleManager.SetUnregisterClient(unregisterClient)

	session := &SSHSession{
		lifecycle:   lifecycleManager,
		interaction: interactionManager,
		forwarder:   forwarderManager,
		slugManager: slugManager,
	}

	var once sync.Once
	for channel := range sshChan {
		ch, reqs, err := channel.Accept()
		if err != nil {
			log.Printf("failed to accept channel: %v", err)
			continue
		}
		once.Do(func() {
			session.lifecycle.SetChannel(ch)
			session.interaction.SetChannel(ch)

			tcpipReq := session.waitForTCPIPForward(forwardingReq)
			if tcpipReq == nil {
				log.Printf("Port forwarding request not received. Ensure you ran the correct command with -R flag. Example: ssh %s -p %s -R 80:localhost:3000", utils.Getenv("DOMAIN", "localhost"), utils.Getenv("PORT", "2200"))
				if err := session.lifecycle.Close(); err != nil {
					log.Printf("failed to close session: %v", err)
				}
				return
			}
			go session.HandleTCPIPForward(tcpipReq)
		})
		session.HandleGlobalRequest(reqs)
	}
	if err := session.lifecycle.Close(); err != nil {
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
	client.slugManager.Set(newSlug)
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
