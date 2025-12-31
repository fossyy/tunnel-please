package port

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"tunnel_pls/internal/config"
)

type Manager interface {
	AddPortRange(startPort, endPort uint16) error
	GetUnassignedPort() (uint16, bool)
	SetPortStatus(port uint16, assigned bool) error
	GetPortStatus(port uint16) (bool, bool)
}

type manager struct {
	mu          sync.RWMutex
	ports       map[uint16]bool
	sortedPorts []uint16
}

var Default Manager = &manager{
	ports:       make(map[uint16]bool),
	sortedPorts: []uint16{},
}

func init() {
	rawRange := config.Getenv("ALLOWED_PORTS", "")
	if rawRange == "" {
		return
	}

	splitRange := strings.Split(rawRange, "-")
	if len(splitRange) != 2 {
		return
	}

	start, err := strconv.ParseUint(splitRange[0], 10, 16)
	if err != nil {
		return
	}
	end, err := strconv.ParseUint(splitRange[1], 10, 16)
	if err != nil {
		return
	}
	_ = Default.AddPortRange(uint16(start), uint16(end))
}

func (pm *manager) AddPortRange(startPort, endPort uint16) error {
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

func (pm *manager) GetUnassignedPort() (uint16, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, port := range pm.sortedPorts {
		if !pm.ports[port] {
			pm.ports[port] = true
			return port, true
		}
	}
	return 0, false
}

func (pm *manager) SetPortStatus(port uint16, assigned bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.ports[port] = assigned
	return nil
}

func (pm *manager) GetPortStatus(port uint16) (bool, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	status, exists := pm.ports[port]
	return status, exists
}
