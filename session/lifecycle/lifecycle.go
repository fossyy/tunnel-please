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
	TunnelType() types.TunnelType
	ForwardedPort() uint16
}

type SessionRegistry interface {
	Remove(key types.SessionKey)
}

type lifecycle struct {
	status          types.Status
	conn            ssh.Conn
	channel         ssh.Channel
	forwarder       Forwarder
	slug            slug.Slug
	startedAt       time.Time
	sessionRegistry SessionRegistry
	portRegistry    portUtil.Registry
	user            string
}

func New(conn ssh.Conn, forwarder Forwarder, slugManager slug.Slug, port portUtil.Registry, sessionRegistry SessionRegistry, user string) Lifecycle {
	return &lifecycle{
		status:          types.INITIALIZING,
		conn:            conn,
		channel:         nil,
		forwarder:       forwarder,
		slug:            slugManager,
		startedAt:       time.Now(),
		sessionRegistry: sessionRegistry,
		portRegistry:    port,
		user:            user,
	}
}

type Lifecycle interface {
	Connection() ssh.Conn
	PortRegistry() portUtil.Registry
	User() string
	SetChannel(channel ssh.Channel)
	SetStatus(status types.Status)
	IsActive() bool
	StartedAt() time.Time
	Close() error
}

func (l *lifecycle) PortRegistry() portUtil.Registry {
	return l.portRegistry
}

func (l *lifecycle) User() string {
	return l.user
}

func (l *lifecycle) SetChannel(channel ssh.Channel) {
	l.channel = channel
}
func (l *lifecycle) Connection() ssh.Conn {
	return l.conn
}
func (l *lifecycle) SetStatus(status types.Status) {
	l.status = status
	if status == types.RUNNING && l.startedAt.IsZero() {
		l.startedAt = time.Now()
	}
}

func (l *lifecycle) Close() error {
	var firstErr error
	tunnelType := l.forwarder.TunnelType()

	if err := l.forwarder.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		firstErr = err
	}

	if l.channel != nil {
		if err := l.channel.Close(); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if l.conn != nil {
		if err := l.conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	clientSlug := l.slug.String()
	key := types.SessionKey{
		Id:   clientSlug,
		Type: tunnelType,
	}
	l.sessionRegistry.Remove(key)

	if tunnelType == types.TCP {
		if err := l.PortRegistry().SetPortStatus(l.forwarder.ForwardedPort(), false); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (l *lifecycle) IsActive() bool {
	return l.status == types.RUNNING
}

func (l *lifecycle) StartedAt() time.Time {
	return l.startedAt
}
