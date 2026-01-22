package port

import (
	"fmt"
	"sort"
	"sync"
)

type Port interface {
	AddRange(startPort, endPort uint16) error
	Unassigned() (uint16, bool)
	SetStatus(port uint16, assigned bool) error
	Claim(port uint16) (claimed bool)
}

type port struct {
	mu          sync.RWMutex
	ports       map[uint16]bool
	sortedPorts []uint16
}

func New() Port {
	return &port{
		ports:       make(map[uint16]bool),
		sortedPorts: []uint16{},
	}
}

func (pm *port) AddRange(startPort, endPort uint16) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if startPort > endPort {
		return fmt.Errorf("start port cannot be greater than end port")
	}
	for index := startPort; index <= endPort; index++ {
		if _, exists := pm.ports[index]; !exists {
			pm.ports[index] = false
			pm.sortedPorts = append(pm.sortedPorts, index)
		}
	}
	sort.Slice(pm.sortedPorts, func(i, j int) bool {
		return pm.sortedPorts[i] < pm.sortedPorts[j]
	})
	return nil
}

func (pm *port) Unassigned() (uint16, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, index := range pm.sortedPorts {
		if !pm.ports[index] {
			return index, true
		}
	}
	return 0, false
}

func (pm *port) SetStatus(port uint16, assigned bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.ports[port] = assigned
	return nil
}

func (pm *port) Claim(port uint16) (claimed bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	status, exists := pm.ports[port]

	if exists && status {
		return false
	}

	if !exists {
		pm.ports[port] = true
		return true
	}

	pm.ports[port] = true
	return true
}
