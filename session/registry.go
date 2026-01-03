package session

import "sync"

type Registry interface {
	Get(slug string) (session *SSHSession, exist bool)
	Update(oldSlug, newSlug string) (success bool)
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

func (r *registry) Get(slug string) (session *SSHSession, exist bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exist = r.clients[slug]
	return
}

func (r *registry) Update(oldSlug, newSlug string) (success bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[newSlug]; exists && newSlug != oldSlug {
		return false
	}

	client, ok := r.clients[oldSlug]
	if !ok {
		return false
	}

	delete(r.clients, oldSlug)
	client.slugManager.Set(newSlug)
	r.clients[newSlug] = client
	return true
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
