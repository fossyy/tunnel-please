package header

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRequest(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		expectErr     bool
		errContains   string
		expectMethod  string
		expectPath    string
		expectVersion string
		expectHeaders map[string]string
	}{
		{
			name:          "success",
			data:          []byte("GET /path HTTP/1.1\r\nHost: example.com\r\nX-Custom: value\r\n\r\n"),
			expectErr:     false,
			expectMethod:  "GET",
			expectPath:    "/path",
			expectVersion: "HTTP/1.1",
			expectHeaders: map[string]string{
				"Host":     "example.com",
				"X-Custom": "value",
			},
		},
		{
			name:        "no CRLF in start line",
			data:        []byte("GET /path HTTP/1.1"),
			expectErr:   true,
			errContains: "no CRLF found in start line",
		},
		{
			name:        "invalid start line - missing method",
			data:        []byte("INVALID\r\n\r\n"),
			expectErr:   true,
			errContains: "invalid start line: missing method",
		},
		{
			name:        "invalid start line - missing version",
			data:        []byte("GET /path\r\n\r\n"),
			expectErr:   true,
			errContains: "invalid start line: missing version",
		},
		{
			name:          "invalid start line - multiple spaces",
			data:          []byte("GET  /path  HTTP/1.1\r\n\r\n"),
			expectErr:     false,
			expectMethod:  "GET",
			expectPath:    "",
			expectVersion: "/path  HTTP/1.1",
			expectHeaders: map[string]string{},
		},
		{
			name:          "start line with trailing space",
			data:          []byte("GET / HTTP/1.1 \r\n\r\n"),
			expectErr:     false,
			expectMethod:  "GET",
			expectPath:    "/",
			expectVersion: "HTTP/1.1 ",
			expectHeaders: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewRequest(tt.data)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, req)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, req)
				assert.Equal(t, tt.expectMethod, req.Method())
				assert.Equal(t, tt.expectPath, req.Path())
				assert.Equal(t, tt.expectVersion, req.Version())
				for k, v := range tt.expectHeaders {
					assert.Equal(t, v, req.Value(k))
				}
			}
		})
	}
}

func TestRequestHeaderMethods(t *testing.T) {
	data := []byte("GET / HTTP/1.1\r\nHost: original\r\n\r\n")
	req, _ := NewRequest(data)

	req.Set("Host", "updated")
	req.Set("X-New", "new-value")
	assert.Equal(t, "updated", req.Value("Host"))
	assert.Equal(t, "new-value", req.Value("X-New"))

	assert.Equal(t, "", req.Value("Non-Existent"))

	req.Remove("X-New")
	assert.Equal(t, "", req.Value("X-New"))

	final := req.Finalize()
	assert.Contains(t, string(final), "GET / HTTP/1.1\r\n")
	assert.Contains(t, string(final), "Host: updated\r\n")
	assert.True(t, bytes.HasSuffix(final, []byte("\r\n\r\n")))
}

func TestNewResponse(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		expectErr     bool
		errContains   string
		expectHeaders map[string]string
	}{
		{
			name:      "success",
			data:      []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"),
			expectErr: false,
			expectHeaders: map[string]string{
				"Content-Length": "0",
			},
		},
		{
			name:        "invalid response - no CRLF",
			data:        []byte("HTTP/1.1 200 OK"),
			expectErr:   true,
			errContains: "no CRLF found in start line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := NewResponse(tt.data)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
				for k, v := range tt.expectHeaders {
					assert.Equal(t, v, resp.Value(k))
				}
			}
		})
	}
}

// 70 fields in both directions. A fixed 64-field cap used to refuse these
// before any of the real rules were applied.
func TestManyFieldsRoundTrip(t *testing.T) {
	req, err := NewRequest(append([]byte("GET / HTTP/1.1\r\n"), manyHeaders(70)...))
	if !assert.NoError(t, err) {
		return
	}
	reqOut := string(req.Finalize())
	for i := 0; i < 70; i++ {
		assert.Equal(t, "v", req.Value(fmt.Sprintf("X-%02d", i)))
		assert.Contains(t, reqOut, fmt.Sprintf("X-%02d: v\r\n", i))
	}

	resp, err := NewResponse(append([]byte("HTTP/1.1 200 OK\r\n"), manyHeaders(70)...))
	if !assert.NoError(t, err) {
		return
	}
	respOut := string(resp.Finalize())
	for i := 0; i < 70; i++ {
		assert.Contains(t, respOut, fmt.Sprintf("X-%02d: v\r\n", i))
	}
}

func TestResponseHeaderMethods(t *testing.T) {
	data := []byte("HTTP/1.1 200 OK\r\nServer: old\r\n\r\n")
	resp, _ := NewResponse(data)

	resp.Set("Server", "new")
	resp.Set("X-Res", "val")
	assert.Equal(t, "new", resp.Value("Server"))
	assert.Equal(t, "val", resp.Value("X-Res"))

	resp.Remove("X-Res")
	assert.Equal(t, "", resp.Value("X-Res"))

	final := resp.Finalize()
	assert.Contains(t, string(final), "HTTP/1.1 200 OK\r\n")
	assert.Contains(t, string(final), "Server: new\r\n")
	assert.True(t, bytes.HasSuffix(final, []byte("\r\n\r\n")))
}

func TestSetRemainingHeaders(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		wantErr    bool
		expectKept []field
	}{
		{
			name: "various header formats",
			data: []byte("K1: V1\r\nK2:V2\r\nK3 : V3 \r\n\r\n"),
			// "Name space: value" is refused thanks to K3, RFC 7230 3.2.4.
			wantErr: true,
		},
		{
			name: "OWS around value trimmed",
			data: []byte("K1:  V1 \t\r\n\r\n"),
			expectKept: []field{
				{"K1", "V1"},
			},
		},
		{
			name:    "no colon line refused",
			data:    []byte("NoColon\r\n\r\n"),
			wantErr: true,
		},
		{
			name: "no trailing CRLF",
			data: []byte("K1: V1"),
			expectKept: []field{
				{"K1", "V1"},
			},
		},
		{
			name:       "empty lines",
			data:       []byte("\r\nK1: V1"),
			expectKept: nil,
		},
		{
			name:    "empty name refused",
			data:    []byte(": value\r\n\r\n"),
			wantErr: true,
		},
		{
			name: "empty value allowed",
			data: []byte("key:\r\n\r\n"),
			expectKept: []field{
				{"key", ""},
			},
		},
		{
			name:    "space before colon refused",
			data:    []byte("Host : a\r\n\r\n"),
			wantErr: true,
		},
		{
			name:    "tab before colon refused",
			data:    []byte("Host\t: a\r\n\r\n"),
			wantErr: true,
		},
		{
			name:    "obs-fold refused",
			data:    []byte("A: one\r\n two\r\n\r\n"),
			wantErr: true,
		},
		{
			name:    "bare CR in value refused",
			data:    []byte("A: v\rl\r\n\r\n"),
			wantErr: true,
		},
		{
			name:    "bare LF in value refused",
			data:    []byte("A: v\nl\r\n\r\n"),
			wantErr: true,
		},
		{
			name:    "NUL in value refused",
			data:    []byte("A: v\x00l\r\n\r\n"),
			wantErr: true,
		},
		{
			name:    "non-token name refused",
			data:    []byte("A@B: v\r\n\r\n"),
			wantErr: true,
		},
		{
			// No fixed field limit: 70 Set-Cookie lines are a real response,
			// not an attack. The byte limit is the bound.
			name:       "many fields kept",
			data:       manyHeaders(70),
			expectKept: fieldsFromHeaders(70),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := parseLines(nil, tt.data)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrMalformedField)
				return
			}

			assert.NoError(t, err)

			if tt.expectKept != nil {
				assert.Equal(t, tt.expectKept, list)
				return
			}

			assert.Nil(t, list)
		})
	}
}

// manyHeaders builds n "X-NN: v" lines then the blank line, with text/template
// out of the loop: a plain fmt format keeps the generator obvious.
func manyHeaders(n int) []byte {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "X-%02d: v\r\n", i)
	}
	b.WriteString("\r\n")
	return b.Bytes()
}

func fieldsFromHeaders(n int) []field {
	list := make([]field, 0, n)
	for i := 0; i < n; i++ {
		list = append(list, field{name: fmt.Sprintf("X-%02d", i), value: "v"})
	}
	return list
}
