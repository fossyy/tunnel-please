package middleware

import (
	"net"
	"testing"
)

type mockRequestHeader struct {
	headers map[string]string
}

func (m *mockRequestHeader) Value(key string) string {
	return m.headers[key]
}

func (m *mockRequestHeader) Set(key string, value string) {
	m.headers[key] = value
}

func (m *mockRequestHeader) Remove(key string) {
	delete(m.headers, key)
}

func (m *mockRequestHeader) Finalize() []byte {
	return []byte{}
}

func (m *mockRequestHeader) Method() string {
	return ""
}

func (m *mockRequestHeader) Path() string {
	return ""
}

func (m *mockRequestHeader) Version() string {
	return ""
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
			reqHeader := &mockRequestHeader{headers: make(map[string]string)}

			err := ff.HandleRequest(reqHeader)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				host := reqHeader.Value("X-Forwarded-For")
				if host != tc.expectedHost {
					t.Errorf("expected X-Forwarded-For header to be '%s', got '%s'", tc.expectedHost, host)
				}
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

			if ff.addr.String() != tc.expectAddr.String() {
				t.Errorf("expected addr to be '%v', got '%v'", tc.expectAddr, ff.addr)
			}
		})
	}
}
