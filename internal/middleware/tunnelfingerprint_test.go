package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockResponseHeader struct {
	mock.Mock
}

func (m *mockResponseHeader) Value(key string) string {
	return m.Called(key).String(0)
}

func (m *mockResponseHeader) Set(key string, value string) {
	m.Called(key, value)
}

func (m *mockResponseHeader) Remove(key string) {
	m.Called(key)
}

func (m *mockResponseHeader) Finalize() []byte {
	return m.Called().Get(0).([]byte)
}

func TestTunnelFingerprintHandleResponse(t *testing.T) {
	tests := []struct {
		name     string
		expected map[string]string
		body     []byte
		wantErr  error
	}{
		{
			name:     "Sets Server Header",
			expected: map[string]string{"Server": "Tunnel Please"},
			body:     []byte("Sample body"),
			wantErr:  nil,
		},
		{
			name:     "Overwrites Server Header",
			expected: map[string]string{"Server": "Tunnel Please"},
			body:     nil,
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHeader := new(mockResponseHeader)
			for k, v := range tt.expected {
				mockHeader.On("Set", k, v).Return()
			}

			tunnelFingerprint := NewTunnelFingerprint()

			err := tunnelFingerprint.HandleResponse(mockHeader, tt.body)
			assert.ErrorIs(t, err, tt.wantErr)
			mockHeader.AssertExpectations(t)
		})
	}
}

func TestNewTunnelFingerprint(t *testing.T) {
	instance := NewTunnelFingerprint()
	assert.NotNil(t, instance)
}
