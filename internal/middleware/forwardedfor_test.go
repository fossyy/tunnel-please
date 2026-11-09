package middleware

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"tunnel_pls/internal/http/header"
)

type mockRequestHeader struct {
	mock.Mock
}

func (m *mockRequestHeader) Count(key string) int {
	return m.Called(key).Int(0)
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
				// A client-sent value must not survive next to ours.
				reqHeader.On("Remove", "X-Forwarded-For").Return()
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

// A client-sent X-Forwarded-For must not survive next to the real address:
// the parser keeps repeated fields, so Set alone would leave the second one.
func TestForwardedForReplacesRepeatedHeader(t *testing.T) {
	raw := []byte("GET / HTTP/1.1\r\nHost: a.example.com\r\n" +
		"X-Forwarded-For: 1.1.1.1\r\nX-Forwarded-For: 6.6.6.6\r\n\r\n")
	h, err := header.NewRequest(raw)
	require.NoError(t, err)

	ff := NewForwardedFor(&net.TCPAddr{IP: net.ParseIP("203.0.113.9"), Port: 1234})
	require.NoError(t, ff.HandleRequest(h))

	assert.Equal(t, 1, h.Count("X-Forwarded-For"))
	assert.Equal(t, "203.0.113.9", h.Value("X-Forwarded-For"))
}
