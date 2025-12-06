package session

import (
	"bytes"
	"log"
	"sync"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

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

	channelOnce sync.Once
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
		CommandBuffer: bytes.NewBuffer(make([]byte, 0, 20)),
		EditMode:      false,
		EditSlug:      "",
		SlugManager:   slugManager,
		Forwarder:     forwarderManager,
		Lifecycle:     nil,
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

	go func() {
		go session.Lifecycle.WaitForRunningStatus()

		for channel := range sshChan {
			ch, reqs, err := channel.Accept()
			if err != nil {
				log.Printf("failed to accept channel: %v", err)
				continue
			}
			session.channelOnce.Do(func() {
				session.Lifecycle.SetChannel(ch)
				session.Interaction.SetChannel(ch)
				session.Lifecycle.SetStatus(types.SETUP)
				go session.HandleGlobalRequest(forwardingReq)
			})

			go session.HandleGlobalRequest(reqs)
		}
		if err := session.Lifecycle.Close(); err != nil {
			log.Printf("failed to close session: %v", err)
		}
	}()
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
