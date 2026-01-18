package port

import (
	"fmt"
	"sort"
	"sync"
)

type Registry interface {
	AddPortRange(startPort, endPort uint16) error
	GetUnassignedPort() (uint16, bool)
	SetPortStatus(port uint16, assigned bool) error
	ClaimPort(port uint16) (claimed bool)
}

type registry struct {
	mu          sync.RWMutex
	ports       map[uint16]bool
	sortedPorts []uint16
}

func New() Registry {
	return &registry{
		ports:       make(map[uint16]bool),
		sortedPorts: []uint16{},
	}
}

func (pm *registry) AddPortRange(startPort, endPort uint16) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if startPort > endPort {
		return fmt.Errorf("start port cannot be greater than end port")
	}
	for port := startPort; port <= endPort; port++ {
		if _, exists := pm.ports[port]; !exists {
			pm.ports[port] = false
			pm.sortedPorts = append(pm.sortedPorts, port)
		}
	}
	sort.Slice(pm.sortedPorts, func(i, j int) bool {
		return pm.sortedPorts[i] < pm.sortedPorts[j]
	})
	return nil
}

func (pm *registry) GetUnassignedPort() (uint16, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, port := range pm.sortedPorts {
		if !pm.ports[port] {
			return port, true
		}
	}
	return 0, false
}

func (pm *registry) SetPortStatus(port uint16, assigned bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.ports[port] = assigned
	return nil
}

func (pm *registry) ClaimPort(port uint16) (claimed bool) {
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
