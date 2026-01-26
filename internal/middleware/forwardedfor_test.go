package middleware

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net"
	"testing"
)

type mockRequestHeader struct {
	mock.Mock
}

func (m *mockRequestHeader) Value(key string) string {
	return m.Called(key).String(0)
}

func (m *mockRequestHeader) Set(key string, value string) {
	m.Called(key, value)
}

func (m *mockRequestHeader) Remove(key string) {
	m.Called(key)
}

func (m *mockRequestHeader) Finalize() []byte {
	return m.Called().Get(0).([]byte)
}

func (m *mockRequestHeader) Method() string {
	return m.Called().String(0)
}

func (m *mockRequestHeader) Path() string {
	return m.Called().String(0)
}

func (m *mockRequestHeader) Version() string {
	return m.Called().String(0)
}

func TestForwardedFor_HandleRequest(t *testing.T) {
	tests := []struct {
		name         string
		addr         net.Addr
		expectedHost string
		expectError  bool
	}{
		{
			name:         "valid IPv4 address",
			addr:         &net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 8080},
			expectedHost: "192.168.1.100",
			expectError:  false,
		},
		{
			name:         "valid IPv6 address",
			addr:         &net.TCPAddr{IP: net.ParseIP("2001:db8::ff00:42:8329"), Port: 8080},
			expectedHost: "2001:db8::ff00:42:8329",
			expectError:  false,
		},
		{
			name:         "invalid address format",
			addr:         &net.UnixAddr{Name: "/tmp/socket", Net: "unix"},
			expectedHost: "",
			expectError:  true,
		},
		{
			name:         "valid IPv4 address with port",
			addr:         &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234},
			expectedHost: "127.0.0.1",
			expectError:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ff := NewForwardedFor(tc.addr)
			reqHeader := new(mockRequestHeader)

			if !tc.expectError {
				reqHeader.On("Set", "X-Forwarded-For", tc.expectedHost).Return()
			}

			err := ff.HandleRequest(reqHeader)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				reqHeader.AssertExpectations(t)
			}
		})
	}
}

func TestNewForwardedFor(t *testing.T) {
	tests := []struct {
		name       string
		addr       net.Addr
		expectAddr net.Addr
	}{
		{
			name:       "IPv4 address",
			addr:       &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
			expectAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080},
		},
		{
			name:       "IPv6 address",
			addr:       &net.TCPAddr{IP: net.ParseIP("2001:db8::ff00:42:8329"), Port: 0},
			expectAddr: &net.TCPAddr{IP: net.ParseIP("2001:db8::ff00:42:8329"), Port: 0},
		},
		{
			name:       "Unix address",
			addr:       &net.UnixAddr{Name: "/tmp/socket", Net: "unix"},
			expectAddr: &net.UnixAddr{Name: "/tmp/socket", Net: "unix"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ff := NewForwardedFor(tc.addr)
			assert.Equal(t, tc.expectAddr.String(), ff.addr.String())
		})
	}
}
