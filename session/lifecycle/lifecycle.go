package lifecycle

import (
	"errors"
	"io"
	"net"
	"sync"
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
	mu              sync.Mutex
	status          types.SessionStatus
	closeErr        error
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
	l.mu.Lock()
	defer l.mu.Unlock()
	l.status = status
	if status == types.SessionStatusRUNNING && l.startedAt.IsZero() {
		l.startedAt = time.Now()
	}
}

func (l *lifecycle) IsActive() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status == types.SessionStatusRUNNING
}

func (l *lifecycle) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.status == types.SessionStatusCLOSED {
		return l.closeErr
	}
	l.status = types.SessionStatusCLOSED

	var errs []error
	tunnelType := l.forwarder.TunnelType()

	if l.channel != nil {
		if err := l.channel.Close(); err != nil && !isClosedError(err) {
			errs = append(errs, err)
		}
	}

	if l.conn != nil {
		if err := l.conn.Close(); err != nil && !isClosedError(err) {
			errs = append(errs, err)
		}
	}

	clientSlug := l.slug.String()
	key := types.SessionKey{
		Id:   clientSlug,
		Type: tunnelType,
	}
	l.sessionRegistry.Remove(key)

	if tunnelType == types.TunnelTypeTCP {
		errs = append(errs, l.PortRegistry().SetStatus(l.forwarder.ForwardedPort(), false))
		errs = append(errs, l.forwarder.Close())
	}

	l.closeErr = errors.Join(errs...)
	return l.closeErr
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || err.Error() == "EOF"
}

func (l *lifecycle) StartedAt() time.Time {
	return l.startedAt
}
