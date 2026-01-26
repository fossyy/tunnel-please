package port

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddRange(t *testing.T) {
	tests := []struct {
		name      string
		startPort uint16
		endPort   uint16
		wantErr   bool
	}{
		{"normal range", 1000, 1002, false},
		{"invalid range", 2000, 1999, true},
		{"single port range", 3000, 3000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := New()
			err := pm.AddRange(tt.startPort, tt.endPort)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUnassigned(t *testing.T) {
	pm := New()
	_ = pm.AddRange(1000, 1002)

	tests := []struct {
		name   string
		status map[uint16]bool
		want   uint16
		wantOk bool
	}{
		{"all unassigned", map[uint16]bool{1000: false, 1001: false, 1002: false}, 1000, true},
		{"some assigned", map[uint16]bool{1000: true, 1001: false, 1002: true}, 1001, true},
		{"all assigned", map[uint16]bool{1000: true, 1001: true, 1002: true}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.status {
				_ = pm.SetStatus(k, v)
			}
			got, gotOk := pm.Unassigned()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, gotOk)
		})
	}
}

func TestSetStatus(t *testing.T) {
	pm := New()
	_ = pm.AddRange(1000, 1002)

	tests := []struct {
		name     string
		port     uint16
		assigned bool
	}{
		{"assign port 1000", 1000, true},
		{"unassign port 1001", 1001, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pm.SetStatus(tt.port, tt.assigned)
			assert.NoError(t, err)

			status, ok := pm.(*port).ports[tt.port]
			assert.True(t, ok)
			assert.Equal(t, tt.assigned, status)
		})
	}
}

func TestClaim(t *testing.T) {
	pm := New()
	_ = pm.AddRange(1000, 1002)

	tests := []struct {
		name   string
		port   uint16
		status bool
		want   bool
	}{
		{"claim unassigned port", 1000, false, true},
		{"claim already assigned port", 1001, true, false},
		{"claim non-existent port", 5000, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, exists := pm.(*port).ports[tt.port]; exists {
				_ = pm.SetStatus(tt.port, tt.status)
			}

			got := pm.Claim(tt.port)
			assert.Equal(t, tt.want, got)

			finalState := pm.(*port).ports[tt.port]
			assert.True(t, finalState)
		})
	}
}
