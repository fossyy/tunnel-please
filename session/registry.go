package session

import (
	"fmt"
	"sync"
)

type Registry interface {
	Get(slug string) (session *SSHSession, err error)
	Update(oldSlug, newSlug string) error
	Register(slug string, session *SSHSession) (success bool)
	Remove(slug string)
	GetAllSessionFromUser(user string) []*SSHSession
}
type registry struct {
	mu        sync.RWMutex
	byUser    map[string]map[string]*SSHSession
	slugIndex map[string]string
}

func NewRegistry() Registry {
	return &registry{
		byUser:    make(map[string]map[string]*SSHSession),
		slugIndex: make(map[string]string),
	}
}

func (r *registry) Get(slug string) (session *SSHSession, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userID, ok := r.slugIndex[slug]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	client, ok := r.byUser[userID][slug]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return client, nil
}

func (r *registry) Update(oldSlug, newSlug string) error {
	if isForbiddenSlug(newSlug) {
		return fmt.Errorf("this subdomain is reserved. Please choose a different one")
	} else if !isValidSlug(newSlug) {
		return fmt.Errorf("invalid subdomain. Follow the rules")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	userID, ok := r.slugIndex[oldSlug]
	if !ok {
		return fmt.Errorf("session not found")
	}

	if _, exists := r.slugIndex[newSlug]; exists && newSlug != oldSlug {
		return fmt.Errorf("someone already uses this subdomain")
	}

	client, ok := r.byUser[userID][oldSlug]
	if !ok {
		return fmt.Errorf("session not found")
	}

	delete(r.byUser[userID], oldSlug)
	delete(r.slugIndex, oldSlug)

	client.slugManager.Set(newSlug)
	r.slugIndex[newSlug] = userID

	if r.byUser[userID] == nil {
		r.byUser[userID] = make(map[string]*SSHSession)
	}
	r.byUser[userID][newSlug] = client
	return nil
}

func (r *registry) Register(slug string, session *SSHSession) (success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.slugIndex[slug]; exists {
		return false
	}

	userID := session.userID
	if r.byUser[userID] == nil {
		r.byUser[userID] = make(map[string]*SSHSession)
	}

	r.byUser[userID][slug] = session
	r.slugIndex[slug] = userID
	return true
}

func (r *registry) GetAllSessionFromUser(user string) []*SSHSession {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m := r.byUser[user]
	if len(m) == 0 {
		return []*SSHSession{}
	}

	sessions := make([]*SSHSession, 0, len(m))
	for _, s := range m {
		sessions = append(sessions, s)
	}
	return sessions
}

func (r *registry) Remove(slug string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	userID, ok := r.slugIndex[slug]
	if !ok {
		return
	}

	delete(r.byUser[userID], slug)
	if len(r.byUser[userID]) == 0 {
		delete(r.byUser, userID)
	}
	delete(r.slugIndex, slug)
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
