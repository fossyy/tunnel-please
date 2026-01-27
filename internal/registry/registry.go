package registry

import (
	"fmt"
	"sync"
	"tunnel_pls/session/forwarder"
	"tunnel_pls/session/interaction"
	"tunnel_pls/session/lifecycle"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"
)

type Key = types.SessionKey

type Session interface {
	Lifecycle() lifecycle.Lifecycle
	Interaction() interaction.Interaction
	Forwarder() forwarder.Forwarder
	Slug() slug.Slug
	Detail() *types.Detail
}

type Registry interface {
	Get(key Key) (session Session, err error)
	GetWithUser(user string, key Key) (session Session, err error)
	Update(user string, oldKey, newKey Key) error
	Register(key Key, session Session) (success bool)
	Remove(key Key)
	GetAllSessionFromUser(user string) []Session
}
type registry struct {
	mu        sync.RWMutex
	byUser    map[string]map[Key]Session
	slugIndex map[Key]string
}

var (
	ErrSessionNotFound      = fmt.Errorf("session not found")
	ErrSlugInUse            = fmt.Errorf("slug already in use")
	ErrInvalidSlug          = fmt.Errorf("invalid slug")
	ErrForbiddenSlug        = fmt.Errorf("forbidden slug")
	ErrSlugChangeNotAllowed = fmt.Errorf("slug change not allowed for this tunnel type")
	ErrSlugUnchanged        = fmt.Errorf("slug is unchanged")
)

func NewRegistry() Registry {
	return &registry{
		byUser:    make(map[string]map[Key]Session),
		slugIndex: make(map[Key]string),
	}
}

func (r *registry) Get(key Key) (session Session, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userID, ok := r.slugIndex[key]
	if !ok {
		return nil, ErrSessionNotFound
	}

	client, ok := r.byUser[userID][key]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return client, nil
}

func (r *registry) GetWithUser(user string, key Key) (session Session, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.byUser[user][key]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return client, nil
}

func (r *registry) Update(user string, oldKey, newKey Key) error {
	if oldKey.Type != newKey.Type {
		return ErrSlugUnchanged
	}

	if newKey.Type != types.TunnelTypeHTTP {
		return ErrSlugChangeNotAllowed
	}

	if isForbiddenSlug(newKey.Id) {
		return ErrForbiddenSlug
	}

	if !isValidSlug(newKey.Id) {
		return ErrInvalidSlug
	}

	if _, exists := r.slugIndex[newKey]; exists && newKey != oldKey {
		return ErrSlugInUse
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	client, ok := r.byUser[user][oldKey]
	if !ok {
		return ErrSessionNotFound
	}

	delete(r.byUser[user], oldKey)
	delete(r.slugIndex, oldKey)

	client.Slug().Set(newKey.Id)
	r.slugIndex[newKey] = user

	r.byUser[user][newKey] = client
	return nil
}

func (r *registry) Register(key Key, userSession Session) (success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.slugIndex[key]; exists {
		return false
	}

	userID := userSession.Lifecycle().User()
	if r.byUser[userID] == nil {
		r.byUser[userID] = make(map[Key]Session)
	}

	r.byUser[userID][key] = userSession
	r.slugIndex[key] = userID
	return true
}

func (r *registry) GetAllSessionFromUser(user string) []Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m := r.byUser[user]
	if len(m) == 0 {
		return []Session{}
	}

	sessions := make([]Session, 0, len(m))
	for _, s := range m {
		sessions = append(sessions, s)
	}
	return sessions
}

func (r *registry) Remove(key Key) {
	r.mu.Lock()
	defer r.mu.Unlock()

	userID, ok := r.slugIndex[key]
	if !ok {
		return
	}

	delete(r.byUser[userID], key)
	if len(r.byUser[userID]) == 0 {
		delete(r.byUser, userID)
	}
	delete(r.slugIndex, key)
}

func isValidSlug(slug string) bool {
	if len(slug) < minSlugLength || len(slug) > maxSlugLength {
		return false
	}

	if slug[0] == '-' || slug[len(slug)-1] == '-' {
		return false
	}

	for _, c := range slug {
		if !isValidSlugChar(byte(c)) {
			return false
		}
	}

	return true
}

func isValidSlugChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
}

func isForbiddenSlug(slug string) bool {
	_, ok := forbiddenSlugs[slug]
	return ok
}

var forbiddenSlugs = map[string]struct{}{
	"ping":          {},
	"staging":       {},
	"admin":         {},
	"root":          {},
	"api":           {},
	"www":           {},
	"support":       {},
	"help":          {},
	"status":        {},
	"health":        {},
	"login":         {},
	"logout":        {},
	"signup":        {},
	"register":      {},
	"settings":      {},
	"config":        {},
	"null":          {},
	"undefined":     {},
	"example":       {},
	"test":          {},
	"dev":           {},
	"system":        {},
	"administrator": {},
	"dashboard":     {},
	"account":       {},
	"profile":       {},
	"user":          {},
	"users":         {},
	"auth":          {},
	"oauth":         {},
	"callback":      {},
	"webhook":       {},
	"webhooks":      {},
	"static":        {},
	"assets":        {},
	"cdn":           {},
	"mail":          {},
	"email":         {},
	"ftp":           {},
	"ssh":           {},
	"git":           {},
	"svn":           {},
	"blog":          {},
	"news":          {},
	"about":         {},
	"contact":       {},
	"terms":         {},
	"privacy":       {},
	"legal":         {},
	"billing":       {},
	"payment":       {},
	"checkout":      {},
	"cart":          {},
	"shop":          {},
	"store":         {},
	"download":      {},
	"uploads":       {},
	"images":        {},
	"img":           {},
	"css":           {},
	"js":            {},
	"fonts":         {},
	"public":        {},
	"private":       {},
	"internal":      {},
	"external":      {},
	"proxy":         {},
	"cache":         {},
	"debug":         {},
	"metrics":       {},
	"monitoring":    {},
	"graphql":       {},
	"rest":          {},
	"rpc":           {},
	"socket":        {},
	"ws":            {},
	"wss":           {},
	"app":           {},
	"apps":          {},
	"mobile":        {},
	"desktop":       {},
	"embed":         {},
	"widget":        {},
	"docs":          {},
	"documentation": {},
	"wiki":          {},
	"forum":         {},
	"community":     {},
	"feedback":      {},
	"report":        {},
	"abuse":         {},
	"spam":          {},
	"security":      {},
	"verify":        {},
	"confirm":       {},
	"reset":         {},
	"password":      {},
	"recovery":      {},
	"unsubscribe":   {},
	"subscribe":     {},
	"notifications": {},
	"alerts":        {},
	"messages":      {},
	"inbox":         {},
	"outbox":        {},
	"sent":          {},
	"draft":         {},
	"trash":         {},
	"archive":       {},
	"search":        {},
	"explore":       {},
	"discover":      {},
	"trending":      {},
	"popular":       {},
	"featured":      {},
	"new":           {},
	"latest":        {},
	"top":           {},
	"best":          {},
	"hot":           {},
	"random":        {},
	"all":           {},
	"any":           {},
	"none":          {},
	"true":          {},
	"false":         {},
}

var (
	minSlugLength = 3
	maxSlugLength = 20
)
