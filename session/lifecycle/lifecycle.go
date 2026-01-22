package lifecycle

import (
	"errors"
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
	status          types.SessionStatus
	conn            ssh.Conn
	channel         ssh.Channel
	forwarder       Forwarder
	slug            slug.Slug
	startedAt       time.Time
	sessionRegistry SessionRegistry
	portRegistry    portUtil.Port
	user            string
}

func New(conn ssh.Conn, forwarder Forwarder, slugManager slug.Slug, port portUtil.Port, sessionRegistry SessionRegistry, user string) Lifecycle {
	return &lifecycle{
		status:          types.SessionStatusINITIALIZING,
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
	PortRegistry() portUtil.Port
	User() string
	SetChannel(channel ssh.Channel)
	SetStatus(status types.SessionStatus)
	IsActive() bool
	StartedAt() time.Time
	Close() error
}

func (l *lifecycle) PortRegistry() portUtil.Port {
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
func (l *lifecycle) SetStatus(status types.SessionStatus) {
	l.status = status
	if status == types.SessionStatusRUNNING && l.startedAt.IsZero() {
		l.startedAt = time.Now()
	}
}

func closeIfNotNil(c interface{ Close() error }) error {
	if c != nil {
		return c.Close()
	}
	return nil
}

func (l *lifecycle) Close() error {
	var errs []error
	tunnelType := l.forwarder.TunnelType()

	if err := closeIfNotNil(l.channel); err != nil {
		errs = append(errs, err)
	}

	if err := closeIfNotNil(l.conn); err != nil {
		errs = append(errs, err)
	}

	clientSlug := l.slug.String()
	key := types.SessionKey{
		Id:   clientSlug,
		Type: tunnelType,
	}
	l.sessionRegistry.Remove(key)

	if tunnelType == types.TunnelTypeTCP {
		if err := l.PortRegistry().SetStatus(l.forwarder.ForwardedPort(), false); err != nil {
			errs = append(errs, err)
		}
		if err := l.forwarder.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (l *lifecycle) IsActive() bool {
	return l.status == types.SessionStatusRUNNING
}

func (l *lifecycle) StartedAt() time.Time {
	return l.startedAt
}
