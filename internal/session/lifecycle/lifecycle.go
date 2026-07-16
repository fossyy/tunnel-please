package lifecycle

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"tunnel_pls/internal/session/slug"
	"tunnel_pls/internal/types"

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

type PortRegistry interface {
	Unassigned() (uint16, bool)
	Claim(port uint16) bool
	SetStatus(port uint16, assigned bool) error
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
	portRegistry    PortRegistry
	user            string
}

func New(conn ssh.Conn, forwarder Forwarder, slugManager slug.Slug, port PortRegistry, sessionRegistry SessionRegistry, user string) Lifecycle {
	return &lifecycle{
		status:          types.SessionStatusINITIALIZING,
		conn:            conn,
		channel:         nil,
		forwarder:       forwarder,
		slug:            slugManager,
		startedAt:       time.Time{},
		sessionRegistry: sessionRegistry,
		portRegistry:    port,
		user:            user,
	}
}

type Lifecycle interface {
	Connection() ssh.Conn
	Channel() ssh.Channel
	PortRegistry() PortRegistry
	User() string
	SetChannel(channel ssh.Channel) error
	SetStatus(status types.SessionStatus)
	IsActive() bool
	StartedAt() time.Time
	Close() error
}

func (l *lifecycle) PortRegistry() PortRegistry {
	return l.portRegistry
}

func (l *lifecycle) User() string {
	return l.user
}

func (l *lifecycle) SetChannel(channel ssh.Channel) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.status == types.SessionStatusCLOSED {
		return fmt.Errorf("lifecycle is closed")
	}
	if channel == nil {
		return fmt.Errorf("channel cannot be nil")
	}
	if l.channel != nil {
		return fmt.Errorf("channel already set")
	}
	l.channel = channel
	return nil
}

func (l *lifecycle) Channel() ssh.Channel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.channel
}

func (l *lifecycle) Connection() ssh.Conn {
	return l.conn
}

func (l *lifecycle) SetStatus(status types.SessionStatus) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.status == types.SessionStatusCLOSED {
		return
	}
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
	if l.status == types.SessionStatusCLOSED {
		closeErr := l.closeErr
		l.mu.Unlock()
		return closeErr
	}
	l.status = types.SessionStatusCLOSED

	channel := l.channel
	conn := l.conn
	l.mu.Unlock()

	var errs []error
	if channel != nil {
		if err := channel.Close(); err != nil && !isClosedError(err) {
			errs = append(errs, err)
		}
	}
	if conn != nil {
		if err := conn.Close(); err != nil && !isClosedError(err) {
			errs = append(errs, err)
		}
	}

	l.cleanupRegistry()
	if err := l.cleanupForwarder(); err != nil {
		errs = append(errs, err)
	}

	closeErr := errors.Join(errs...)

	l.mu.Lock()
	l.closeErr = closeErr
	l.mu.Unlock()

	return closeErr
}

func (l *lifecycle) cleanupRegistry() {
	slugStr := l.slug.String()
	if slugStr == "" {
		return
	}
	key := types.SessionKey{
		Id:   slugStr,
		Type: l.forwarder.TunnelType(),
	}
	l.sessionRegistry.Remove(key)
}

func (l *lifecycle) cleanupForwarder() error {
	if l.forwarder.TunnelType() != types.TunnelTypeTCP {
		return nil
	}
	var errs []error
	errs = append(errs, l.portRegistry.SetStatus(l.forwarder.ForwardedPort(), false))
	errs = append(errs, l.forwarder.Close())
	return errors.Join(errs...)
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed)
}

func (l *lifecycle) StartedAt() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.startedAt
}
