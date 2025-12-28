package lifecycle

import (
	"errors"
	"io"
	"net"
	portUtil "tunnel_pls/internal/port"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type Interaction interface {
	SendMessage(string)
}

type Forwarder interface {
	Close() error
	GetTunnelType() types.TunnelType
	GetForwardedPort() uint16
}

type Lifecycle struct {
	Status  types.Status
	Conn    ssh.Conn
	Channel ssh.Channel

	Interaction      Interaction
	Forwarder        Forwarder
	SlugManager      slug.Manager
	unregisterClient func(slug string)
}

func (l *Lifecycle) SetUnregisterClient(unregisterClient func(slug string)) {
	l.unregisterClient = unregisterClient
}

type SessionLifecycle interface {
	Close() error
	SetStatus(status types.Status)
	GetConnection() ssh.Conn
	GetChannel() ssh.Channel
	SetChannel(channel ssh.Channel)
	SetUnregisterClient(unregisterClient func(slug string))
}

func (l *Lifecycle) GetChannel() ssh.Channel {
	return l.Channel
}

func (l *Lifecycle) SetChannel(channel ssh.Channel) {
	l.Channel = channel
}
func (l *Lifecycle) GetConnection() ssh.Conn {
	return l.Conn
}
func (l *Lifecycle) SetStatus(status types.Status) {
	l.Status = status
}

func (l *Lifecycle) Close() error {
	err := l.Forwarder.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	if l.Channel != nil {
		err := l.Channel.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}

	if l.Conn != nil {
		err := l.Conn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
	}

	clientSlug := l.SlugManager.Get()
	if clientSlug != "" {
		l.unregisterClient(clientSlug)
	}

	if l.Forwarder.GetTunnelType() == types.TCP {
		err := portUtil.Manager.SetPortStatus(l.Forwarder.GetForwardedPort(), false)
		if err != nil {
			return err
		}
	}

	return nil
}
