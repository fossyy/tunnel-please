package session

import (
	"log"
	"sync"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type Session interface {
	lifecycle.Lifecycle
	interaction.InteractionController
	forwarder.ForwardingController

	HandleGlobalRequest(ch <-chan *ssh.Request)
	HandleTCPIPForward(req *ssh.Request)
	HandleHTTPForward(req *ssh.Request, port uint16)
	HandleTCPForward(req *ssh.Request, addr string, port uint16)
}

type SSHSession struct {
	Lifecycle   lifecycle.SessionLifecycle
	Interaction interaction.InteractionController
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
		CommandBuffer: nil,
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
	session := &SSHSession{
		Lifecycle:   lifecycleManager,
		Interaction: interactionManager,
		Forwarder:   forwarderManager,
		SlugManager: slugManager,
	}
	interactionManager.SetLifecycle(lifecycleManager)

	go func() {
		go session.Lifecycle.WaitForRunningStatus()

		for channel := range sshChan {
			ch, reqs, _ := channel.Accept()
			if session.Lifecycle.GetChannel() == nil {
				session.Lifecycle.SetChannel(ch)
				session.Interaction.SetChannel(ch)
				//session.Interaction.channel = ch
				session.Lifecycle.SetStatus(types.SETUP)
				go session.HandleGlobalRequest(forwardingReq)
			}
			go session.HandleGlobalRequest(reqs)
		}
		err := session.Lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return
	}()
}

var (
	clientsMutex sync.RWMutex
	Clients      = make(map[string]*SSHSession)
)

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
