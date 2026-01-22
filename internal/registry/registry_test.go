package registry

import (
	"errors"
	"sync"
	"testing"
	"time"
	"tunnel_pls/internal/port"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type mockSession struct{ user string }

func (m *mockSession) Lifecycle() lifecycle.Lifecycle { return &mockLifecycle{user: m.user} }
func (m *mockSession) Interaction() interaction.Interaction {
	return nil
}
func (m *mockSession) Forwarder() forwarder.Forwarder {
	return nil
}
func (m *mockSession) Slug() slug.Slug {
	return &mockSlug{}
}
func (m *mockSession) Detail() *types.Detail {
	return nil
}

type mockLifecycle struct{ user string }

func (ml *mockLifecycle) Connection() ssh.Conn                 { return nil }
func (ml *mockLifecycle) PortRegistry() port.Port              { return nil }
func (ml *mockLifecycle) SetChannel(channel ssh.Channel)       { _ = channel }
func (ml *mockLifecycle) SetStatus(status types.SessionStatus) { _ = status }
func (ml *mockLifecycle) IsActive() bool                       { return false }
func (ml *mockLifecycle) StartedAt() time.Time                 { return time.Time{} }
func (ml *mockLifecycle) Close() error                         { return nil }
func (ml *mockLifecycle) User() string                         { return ml.user }

type mockSlug struct{}

func (ms *mockSlug) Set(slug string) { _ = slug }
func (ms *mockSlug) String() string  { return "" }

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegistry_Get(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func(r *registry)
		key        types.SessionKey
		wantErr    error
		wantResult bool
	}{
		{
			name: "session found",
			setupFunc: func(r *registry) {
				user := "user1"
				key := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: user}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser[user] = map[types.SessionKey]Session{
					key: session,
				}
				r.slugIndex[key] = user
			},
			key:        types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP},
			wantErr:    nil,
			wantResult: true,
		},
		{
			name:      "session not found in slugIndex",
			setupFunc: func(r *registry) {},
			key:       types.SessionKey{Id: "test2", Type: types.TunnelTypeHTTP},
			wantErr:   ErrSessionNotFound,
		},
		{
			name: "session not found in byUser",
			setupFunc: func(r *registry) {
				r.mu.Lock()
				defer r.mu.Unlock()
				r.slugIndex[types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}] = "invalid_user"
			},
			key:     types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP},
			wantErr: ErrSessionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &registry{
				byUser:    make(map[string]map[types.SessionKey]Session),
				slugIndex: make(map[types.SessionKey]string),
				mu:        sync.RWMutex{},
			}
			tt.setupFunc(r)

			session, err := r.Get(tt.key)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if (session != nil) != tt.wantResult {
				t.Fatalf("expected session existence to be %v, got %v", tt.wantResult, session != nil)
			}
		})
	}
}

func TestRegistry_GetWithUser(t *testing.T) {
	tests := []struct {
		name       string
		setupFunc  func(r *registry)
		user       string
		key        types.SessionKey
		wantErr    error
		wantResult bool
	}{
		{
			name: "session found",
			setupFunc: func(r *registry) {
				user := "user1"
				key := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: user}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser[user] = map[types.SessionKey]Session{
					key: session,
				}
				r.slugIndex[key] = user
			},
			user:       "user1",
			key:        types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP},
			wantErr:    nil,
			wantResult: true,
		},
		{
			name:      "session not found in slugIndex",
			setupFunc: func(r *registry) {},
			user:      "user1",
			key:       types.SessionKey{Id: "test2", Type: types.TunnelTypeHTTP},
			wantErr:   ErrSessionNotFound,
		},
		{
			name: "session not found in byUser",
			setupFunc: func(r *registry) {
				r.mu.Lock()
				defer r.mu.Unlock()
				r.slugIndex[types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}] = "invalid_user"
			},
			user:    "user1",
			key:     types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP},
			wantErr: ErrSessionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &registry{
				byUser:    make(map[string]map[types.SessionKey]Session),
				slugIndex: make(map[types.SessionKey]string),
				mu:        sync.RWMutex{},
			}
			tt.setupFunc(r)

			session, err := r.GetWithUser(tt.user, tt.key)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if (session != nil) != tt.wantResult {
				t.Fatalf("expected session existence to be %v, got %v", tt.wantResult, session != nil)
			}
		})
	}
}

func TestRegistry_Update(t *testing.T) {
	tests := []struct {
		name      string
		user      string
		setupFunc func(r *registry) (oldKey, newKey types.SessionKey)
		wantErr   error
	}{
		{
			name: "change slug success",
			user: "user1",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				newKey := types.SessionKey{Id: "test2", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					oldKey: session,
				}
				r.slugIndex[oldKey] = "user1"

				return oldKey, newKey
			},
			wantErr: nil,
		},
		{
			name: "change slug to already used slug",
			user: "user1",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				newKey := types.SessionKey{Id: "test2", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					oldKey: session,
					newKey: session,
				}
				r.slugIndex[oldKey] = "user1"
				r.slugIndex[newKey] = "user1"

				return oldKey, newKey
			},
			wantErr: ErrSlugInUse,
		},
		{
			name: "change slug to forbidden slug",
			user: "user1",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				newKey := types.SessionKey{Id: "ping", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					oldKey: session,
				}
				r.slugIndex[oldKey] = "user1"

				return oldKey, newKey
			},
			wantErr: ErrForbiddenSlug,
		},
		{
			name: "change slug to invalid slug",
			user: "user1",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				newKey := types.SessionKey{Id: "test2-", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					oldKey: session,
				}
				r.slugIndex[oldKey] = "user1"

				return oldKey, newKey
			},
			wantErr: ErrInvalidSlug,
		},
		{
			name: "change slug but session not found",
			user: "user2",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test2", Type: types.TunnelTypeHTTP}
				newKey := types.SessionKey{Id: "test4", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}: session,
				}
				r.slugIndex[types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}] = "user1"

				return oldKey, newKey
			},
			wantErr: ErrSessionNotFound,
		},
		{
			name: "change slug but session is not in the map",
			user: "user2",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test2", Type: types.TunnelTypeHTTP}
				newKey := types.SessionKey{Id: "test4", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}: session,
				}
				r.slugIndex[types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}] = "user1"

				return oldKey, newKey
			},
			wantErr: ErrSessionNotFound,
		},
		{
			name: "change slug with same slug",
			user: "user1",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				newKey := types.SessionKey{Id: "test2", Type: types.TunnelTypeTCP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					oldKey: session,
				}
				r.slugIndex[oldKey] = "user1"

				return oldKey, newKey
			},
			wantErr: ErrSlugUnchanged,
		},
		{
			name: "tcp tunnel cannot change slug",
			user: "user1",
			setupFunc: func(r *registry) (types.SessionKey, types.SessionKey) {
				oldKey := types.SessionKey{Id: "test2", Type: types.TunnelTypeTCP}
				newKey := oldKey
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				defer r.mu.Unlock()
				r.byUser["user1"] = map[types.SessionKey]Session{
					oldKey: session,
				}
				r.slugIndex[oldKey] = "user1"

				return oldKey, newKey
			},
			wantErr: ErrSlugChangeNotAllowed,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &registry{
				byUser:    make(map[string]map[types.SessionKey]Session),
				slugIndex: make(map[types.SessionKey]string),
				mu:        sync.RWMutex{},
			}

			oldKey, newKey := tt.setupFunc(r)

			err := r.Update(tt.user, oldKey, newKey)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if err == nil {
				r.mu.RLock()
				defer r.mu.RUnlock()
				if _, ok := r.byUser[tt.user][newKey]; !ok {
					t.Errorf("newKey not found in registry")
				}
				if _, ok := r.byUser[tt.user][oldKey]; ok {
					t.Errorf("oldKey still exists in registry")
				}
			}
		})
	}
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name      string
		user      string
		setupFunc func(r *registry) Key
		wantOK    bool
	}{
		{
			name: "register new key successfully",
			user: "user1",
			setupFunc: func(r *registry) Key {
				key := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				return key
			},
			wantOK: true,
		},
		{
			name: "register already existing key fails",
			user: "user1",
			setupFunc: func(r *registry) Key {
				key := types.SessionKey{Id: "test1", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}

				r.mu.Lock()
				r.byUser["user1"] = map[Key]Session{key: session}
				r.slugIndex[key] = "user1"
				r.mu.Unlock()

				return key
			},
			wantOK: false,
		},
		{
			name: "register multiple keys for same user",
			user: "user1",
			setupFunc: func(r *registry) Key {
				firstKey := types.SessionKey{Id: "first", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: "user1"}
				r.mu.Lock()
				r.byUser["user1"] = map[Key]Session{firstKey: session}
				r.slugIndex[firstKey] = "user1"
				r.mu.Unlock()

				return types.SessionKey{Id: "second", Type: types.TunnelTypeHTTP}
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &registry{
				byUser:    make(map[string]map[Key]Session),
				slugIndex: make(map[Key]string),
				mu:        sync.RWMutex{},
			}

			key := tt.setupFunc(r)
			session := &mockSession{user: tt.user}

			ok := r.Register(key, session)
			if ok != tt.wantOK {
				t.Fatalf("expected success %v, got %v", tt.wantOK, ok)
			}

			if ok {
				r.mu.RLock()
				defer r.mu.RUnlock()
				if r.byUser[tt.user][key] != session {
					t.Errorf("session not stored in byUser")
				}
				if r.slugIndex[key] != tt.user {
					t.Errorf("slugIndex not updated")
				}
			}
		})
	}
}

func TestRegistry_GetAllSessionFromUser(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(r *registry) string
		expectN   int
	}{
		{
			name: "user has no sessions",
			setupFunc: func(r *registry) string {
				return "user1"
			},
			expectN: 0,
		},
		{
			name: "user has multiple sessions",
			setupFunc: func(r *registry) string {
				user := "user1"
				key1 := types.SessionKey{Id: "a", Type: types.TunnelTypeHTTP}
				key2 := types.SessionKey{Id: "b", Type: types.TunnelTypeTCP}
				r.mu.Lock()
				r.byUser[user] = map[Key]Session{
					key1: &mockSession{user: user},
					key2: &mockSession{user: user},
				}
				r.mu.Unlock()
				return user
			},
			expectN: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &registry{
				byUser:    make(map[string]map[Key]Session),
				slugIndex: make(map[Key]string),
				mu:        sync.RWMutex{},
			}
			user := tt.setupFunc(r)
			sessions := r.GetAllSessionFromUser(user)
			if len(sessions) != tt.expectN {
				t.Errorf("expected %d sessions, got %d", tt.expectN, len(sessions))
			}
		})
	}
}

func TestRegistry_Remove(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(r *registry) (string, types.SessionKey)
		key       types.SessionKey
		verify    func(*testing.T, *registry, string, types.SessionKey)
	}{
		{
			name: "remove existing key",
			setupFunc: func(r *registry) (string, types.SessionKey) {
				user := "user1"
				key := types.SessionKey{Id: "a", Type: types.TunnelTypeHTTP}
				session := &mockSession{user: user}
				r.mu.Lock()
				r.byUser[user] = map[Key]Session{key: session}
				r.slugIndex[key] = user
				r.mu.Unlock()
				return user, key
			},
			verify: func(t *testing.T, r *registry, user string, key types.SessionKey) {
				if _, ok := r.byUser[user][key]; ok {
					t.Errorf("expected key to be removed from byUser")
				}
				if _, ok := r.slugIndex[key]; ok {
					t.Errorf("expected key to be removed from slugIndex")
				}
				if _, ok := r.byUser[user]; ok {
					t.Errorf("expected user to be removed from byUser map")
				}
			},
		},
		{
			name: "remove non-existing key",
			setupFunc: func(r *registry) (string, types.SessionKey) {
				return "", types.SessionKey{Id: "nonexist", Type: types.TunnelTypeHTTP}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &registry{
				byUser:    make(map[string]map[Key]Session),
				slugIndex: make(map[Key]string),
				mu:        sync.RWMutex{},
			}
			user, key := tt.setupFunc(r)
			if user == "" {
				key = tt.key
			}
			r.Remove(key)
			if tt.verify != nil {
				tt.verify(t, r, user, key)
			}
		})
	}
}

func TestIsValidSlug(t *testing.T) {
	tests := []struct {
		slug string
		want bool
	}{
		{"abc", true},
		{"abc-123", true},
		{"a", false},
		{"verybigdihsixsevenlabubu", false},
		{"-iamsigma", false},
		{"ligma-", false},
		{"invalid$", false},
		{"valid-slug1", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.slug, func(t *testing.T) {
			got := isValidSlug(tt.slug)
			if got != tt.want {
				t.Errorf("isValidSlug(%q) = %v; want %v", tt.slug, got, tt.want)
			}
		})
	}
}

func TestIsValidSlugChar(t *testing.T) {
	tests := []struct {
		char byte
		want bool
	}{
		{'a', true},
		{'z', true},
		{'0', true},
		{'9', true},
		{'-', true},
		{'A', false},
		{'$', false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(string(tt.char), func(t *testing.T) {
			got := isValidSlugChar(tt.char)
			if got != tt.want {
				t.Errorf("isValidSlugChar(%q) = %v; want %v", tt.char, got, tt.want)
			}
		})
	}
}

func TestIsForbiddenSlug(t *testing.T) {
	forbiddenSlugs = map[string]struct{}{
		"admin": {},
		"root":  {},
	}

	tests := []struct {
		slug string
		want bool
	}{
		{"admin", true},
		{"root", true},
		{"user", false},
		{"guest", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.slug, func(t *testing.T) {
			got := isForbiddenSlug(tt.slug)
			if got != tt.want {
				t.Errorf("isForbiddenSlug(%q) = %v; want %v", tt.slug, got, tt.want)
			}
		})
	}
}
