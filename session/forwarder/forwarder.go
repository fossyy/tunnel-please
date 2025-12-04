package forwarder

import (
	"net"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"
)

type Forwarder struct {
	Listener      net.Listener
	TunnelType    types.TunnelType
	ForwardedPort uint16
	SlugManager   slug.Manager
}

func (f *Forwarder) AcceptTCPConnections() {
	panic("implement me")
}

func (f *Forwarder) UpdateClientSlug(oldSlug, newSlug string) bool {
	panic("implement me")
}

func (f *Forwarder) SetType(tunnelType types.TunnelType) {
	f.TunnelType = tunnelType
}

func (f *Forwarder) GetTunnelType() types.TunnelType {
	return f.TunnelType
}

func (f *Forwarder) GetForwardedPort() uint16 {
	return f.ForwardedPort
}

func (f *Forwarder) SetForwardedPort(port uint16) {
	f.ForwardedPort = port
}

func (f *Forwarder) SetListener(listener net.Listener) {
	f.Listener = listener
}

func (f *Forwarder) GetListener() net.Listener {
	return f.Listener
}

func (f *Forwarder) Close() error {
	if f.GetTunnelType() != types.HTTP {
		return f.Listener.Close()
	}
	return nil
}

type ForwardingController interface {
	AcceptTCPConnections()
	UpdateClientSlug(oldSlug, newSlug string) bool
	SetType(tunnelType types.TunnelType)
	GetTunnelType() types.TunnelType
	GetForwardedPort() uint16
	SetForwardedPort(port uint16)
	SetListener(listener net.Listener)
	GetListener() net.Listener
	Close() error
}

//func (f *Forwarder) UpdateClientSlug(oldSlug, newSlug string) bool {
//	session.clientsMutex.Lock()
//	defer session.clientsMutex.Unlock()
//
//	if _, exists := session.Clients[newSlug]; exists && newSlug != oldSlug {
//		return false
//	}
//
//	client, ok := session.Clients[oldSlug]
//	if !ok {
//		return false
//	}
//
//	delete(session.Clients, oldSlug)
//	f.SlugManager.Set(newSlug)
//	session.Clients[newSlug] = client
//	return true
//}
