package slug

import "sync"

type Slug interface {
	String() string
	Set(slug string)
}

type slug struct {
	mu   sync.RWMutex
	slug string
}

func New() Slug {
	return &slug{
		slug: "",
	}
}

func (s *slug) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.slug
}

func (s *slug) Set(slug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.slug = slug
}
