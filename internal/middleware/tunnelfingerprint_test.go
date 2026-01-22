package middleware

import (
	"errors"
	"testing"
)

type mockResponseHeader struct {
	headers map[string]string
}

func (m *mockResponseHeader) Value(key string) string {
	return m.headers[key]
}

func (m *mockResponseHeader) Set(key string, value string) {
	m.headers[key] = value
}

func (m *mockResponseHeader) Remove(key string) {
	delete(m.headers, key)
}

func (m *mockResponseHeader) Finalize() []byte {
	return nil
}

func TestTunnelFingerprintHandleResponse(t *testing.T) {
	tests := []struct {
		name         string
		initialState map[string]string
		expected     map[string]string
		body         []byte
		wantErr      error
	}{
		{
			name:         "Sets Server Header",
			initialState: map[string]string{},
			expected:     map[string]string{"Server": "Tunnel Please"},
			body:         []byte("Sample body"),
			wantErr:      nil,
		},
		{
			name:         "Overwrites Server Header",
			initialState: map[string]string{"Server": "Old Value"},
			expected:     map[string]string{"Server": "Tunnel Please"},
			body:         nil,
			wantErr:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHeader := &mockResponseHeader{headers: tt.initialState}
			tunnelFingerprint := NewTunnelFingerprint()

			err := tunnelFingerprint.HandleResponse(mockHeader, tt.body)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("unexpected error, got: %v, want: %v", err, tt.wantErr)
			}

			for key, expectedValue := range tt.expected {
				if val := mockHeader.Value(key); val != expectedValue {
					t.Errorf("header[%q] = %q; want %q", key, val, expectedValue)
				}
			}
		})
	}
}

func TestNewTunnelFingerprint(t *testing.T) {
	instance := NewTunnelFingerprint()
	if instance == nil {
		t.Errorf("NewTunnelFingerprint() = nil; want non-nil instance")
	}
}
