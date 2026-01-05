package lifecycle

import (
	"errors"
	"io"
	"net"
	"time"

	portUtil "tunnel_pls/internal/port"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type Forwarder interface {
	Close() error
	GetTunnelType() types.TunnelType
	GetForwardedPort() uint16
}

type SessionRegistry interface {
	Remove(key types.SessionKey)
}

type Lifecycle struct {
	status          types.Status
	conn            ssh.Conn
	channel         ssh.Channel
	forwarder       Forwarder
	sessionRegistry SessionRegistry
	slugManager     slug.Manager
	startedAt       time.Time
	user            string
}

func NewLifecycle(conn ssh.Conn, forwarder Forwarder, slugManager slug.Manager, user string) *Lifecycle {
	return &Lifecycle{
		status:          types.INITIALIZING,
		conn:            conn,
		channel:         nil,
		forwarder:       forwarder,
		slugManager:     slugManager,
		sessionRegistry: nil,
		startedAt:       time.Now(),
		user:            user,
	}
}

func (l *Lifecycle) SetSessionRegistry(registry SessionRegistry) {
	l.sessionRegistry = registry
}

type SessionLifecycle interface {
	Close() error
	SetStatus(status types.Status)
	GetConnection() ssh.Conn
	GetChannel() ssh.Channel
	GetUser() string
	SetChannel(channel ssh.Channel)
	SetSessionRegistry(registry SessionRegistry)
	IsActive() bool
	StartedAt() time.Time
}

func (l *Lifecycle) GetUser() string {
	return l.user
}

func (l *Lifecycle) GetChannel() ssh.Channel {
	return l.channel
}

func (l *Lifecycle) SetChannel(channel ssh.Channel) {
	l.channel = channel
}
func (l *Lifecycle) GetConnection() ssh.Conn {
	return l.conn
}
func (l *Lifecycle) SetStatus(status types.Status) {
	l.status = status
	if status == types.RUNNING && l.startedAt.IsZero() {
		l.startedAt = time.Now()
	}
}

func (l *Lifecycle) Close() error {
	err := l.forwarder.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	if l.channel != nil {
		err := l.channel.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}

	if l.conn != nil {
		err := l.conn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
	}

	clientSlug := l.slugManager.Get()
	if clientSlug != "" && l.sessionRegistry.Remove != nil {
		key := types.SessionKey{Id: clientSlug, Type: l.forwarder.GetTunnelType()}
		l.sessionRegistry.Remove(key)
	}

	if l.forwarder.GetTunnelType() == types.TCP {
		err = portUtil.Default.SetPortStatus(l.forwarder.GetForwardedPort(), false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *Lifecycle) IsActive() bool {
	return l.status == types.RUNNING
}

func (l *Lifecycle) StartedAt() time.Time {
	return l.startedAt
}
