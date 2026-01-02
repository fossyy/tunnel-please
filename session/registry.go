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
}
type registry struct {
	mu      sync.RWMutex
	clients map[string]*SSHSession
}

func NewRegistry() Registry {
	return &registry{
		clients: make(map[string]*SSHSession),
	}
}

func (r *registry) Get(slug string) (session *SSHSession, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.clients[slug]
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

	if _, exists := r.clients[newSlug]; exists && newSlug != oldSlug {
		return fmt.Errorf("someone already uses this subdomain")
	}

	client, ok := r.clients[oldSlug]
	if !ok {
		return fmt.Errorf("session not found")
	}

	delete(r.clients, oldSlug)
	client.slugManager.Set(newSlug)
	r.clients[newSlug] = client
	return nil
}

func (r *registry) Register(slug string, session *SSHSession) (success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[slug]; exists {
		return false
	}

	r.clients[slug] = session
	return true
}

func (r *registry) Remove(slug string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, slug)
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
