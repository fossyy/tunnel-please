package registry

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

type mockSession struct {
	mock.Mock
}

func (m *mockSession) Lifecycle() lifecycle.Lifecycle {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(lifecycle.Lifecycle)
}
func (m *mockSession) Interaction() interaction.Interaction {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(interaction.Interaction)
}
func (m *mockSession) Forwarder() forwarder.Forwarder {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(forwarder.Forwarder)
}
func (m *mockSession) Slug() slug.Slug {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(slug.Slug)
}
func (m *mockSession) Detail() *types.Detail {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*types.Detail)
}

type mockLifecycle struct {
	mock.Mock
}

func (ml *mockLifecycle) Channel() ssh.Channel {
	args := ml.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(ssh.Channel)
}

func (ml *mockLifecycle) Connection() ssh.Conn {
	args := ml.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(ssh.Conn)
}

func (ml *mockLifecycle) PortRegistry() port.Port {
	args := ml.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(port.Port)
}

func (ml *mockLifecycle) SetChannel(channel ssh.Channel)       { ml.Called(channel) }
func (ml *mockLifecycle) SetStatus(status types.SessionStatus) { ml.Called(status) }
func (ml *mockLifecycle) IsActive() bool                       { return ml.Called().Bool(0) }
func (ml *mockLifecycle) StartedAt() time.Time                 { return ml.Called().Get(0).(time.Time) }
func (ml *mockLifecycle) Close() error                         { return ml.Called().Error(0) }
func (ml *mockLifecycle) User() string                         { return ml.Called().String(0) }

type mockSlug struct {
	mock.Mock
}

func (ms *mockSlug) Set(slug string) { ms.Called(slug) }
func (ms *mockSlug) String() string  { return ms.Called().String(0) }

func createMockSession(user ...string) *mockSession {
	u := "user1"
	if len(user) > 0 {
		u = user[0]
	}
	m := new(mockSession)
	ml := new(mockLifecycle)
	ml.On("User").Return(u).Maybe()
	m.On("Lifecycle").Return(ml).Maybe()
	ms := new(mockSlug)
	ms.On("Set", mock.Anything).Maybe()
	m.On("Slug").Return(ms).Maybe()
	m.On("Interaction").Return(nil).Maybe()
	m.On("Forwarder").Return(nil).Maybe()
	m.On("Detail").Return(nil).Maybe()
	return m
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	require.NotNil(t, r)
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
				session := createMockSession(user)

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

			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantResult, session != nil)
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
				session := createMockSession()

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

			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantResult, session != nil)
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
				session := createMockSession("user1")

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
				session := createMockSession()

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
				session := createMockSession()

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
				session := createMockSession()

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
				session := createMockSession()

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
				session := createMockSession()

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
				session := createMockSession()

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
				session := createMockSession()

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
			assert.ErrorIs(t, err, tt.wantErr)

			if err == nil {
				r.mu.RLock()
				defer r.mu.RUnlock()
				_, ok := r.byUser[tt.user][newKey]
				assert.True(t, ok, "newKey not found in registry")
				_, ok = r.byUser[tt.user][oldKey]
				assert.False(t, ok, "oldKey still exists in registry")
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
				session := createMockSession()

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
				session := createMockSession()
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
			session := createMockSession()

			ok := r.Register(key, session)
			assert.Equal(t, tt.wantOK, ok)

			if ok {
				r.mu.RLock()
				defer r.mu.RUnlock()
				assert.Equal(t, session, r.byUser[tt.user][key], "session not stored in byUser")
				assert.Equal(t, tt.user, r.slugIndex[key], "slugIndex not updated")
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
					key1: createMockSession(),
					key2: createMockSession(),
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
			assert.Len(t, sessions, tt.expectN)
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
				session := createMockSession()
				r.mu.Lock()
				r.byUser[user] = map[Key]Session{key: session}
				r.slugIndex[key] = user
				r.mu.Unlock()
				return user, key
			},
			verify: func(t *testing.T, r *registry, user string, key types.SessionKey) {
				_, ok := r.byUser[user][key]
				assert.False(t, ok, "expected key to be removed from byUser")
				_, ok = r.slugIndex[key]
				assert.False(t, ok, "expected key to be removed from slugIndex")
				_, ok = r.byUser[user]
				assert.False(t, ok, "expected user to be removed from byUser map")
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
