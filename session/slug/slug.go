package slug

import "sync"

type Manager interface {
	Get() string
	Set(slug string)
}

type manager struct {
	slug   string
	slugMu sync.RWMutex
}

func NewManager() Manager {
	return &manager{
		slug:   "",
		slugMu: sync.RWMutex{},
	}
}

func (s *manager) Get() string {
	s.slugMu.RLock()
	defer s.slugMu.RUnlock()
	return s.slug
}

func (s *manager) Set(slug string) {
	s.slugMu.Lock()
	s.slug = slug
	s.slugMu.Unlock()
}
