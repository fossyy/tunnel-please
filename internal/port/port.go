package port

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"tunnel_pls/utils"
)

type PortManager struct {
	ports       map[uint16]bool
	sortedPorts []uint16
}

var Manager = PortManager{
	ports:       make(map[uint16]bool),
	sortedPorts: []uint16{},
}

func init() {
	rawRange := utils.Getenv("ALLOWED_PORTS")
	splitRange := strings.Split(rawRange, "-")
	if len(splitRange) != 2 {
		Manager.AddPortRange(30000, 31000)
	} else {
		start, err := strconv.ParseUint(splitRange[0], 10, 16)
		if err != nil {
			start = 30000
		}
		end, err := strconv.ParseUint(splitRange[1], 10, 16)
		if err != nil {
			end = 31000
		}
		Manager.AddPortRange(uint16(start), uint16(end))
	}
}

func (pm *PortManager) AddPortRange(startPort, endPort uint16) error {
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

func (pm *PortManager) GetUnassignedPort() (uint16, bool) {
	for _, port := range pm.sortedPorts {
		if !pm.ports[port] {
			return port, true
		}
	}
	return 0, false
}

func (pm *PortManager) SetPortStatus(port uint16, assigned bool) error {
	if _, exists := pm.ports[port]; !exists {
		return fmt.Errorf("port %d is not in the allowed range", port)
	}
	pm.ports[port] = assigned
	return nil
}

func (pm *PortManager) GetPortStatus(port uint16) (bool, bool) {
	status, exists := pm.ports[port]
	return status, exists
}
