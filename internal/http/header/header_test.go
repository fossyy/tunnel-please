package header

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewRequestFromBytes(t *testing.T) {
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

func TestNewRequestFromReader(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		expectErr     bool
		errContains   string
		expectEOF     bool
		expectMethod  string
		expectPath    string
		expectVersion string
		expectHeaders map[string]string
	}{
		{
			name:          "success",
			data:          []byte("POST /api HTTP/1.1\r\nContent-Type: application/json\r\n\r\n"),
			expectErr:     false,
			expectMethod:  "POST",
			expectPath:    "/api",
			expectVersion: "HTTP/1.1",
			expectHeaders: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name:      "read error on start line",
			data:      []byte{},
			expectErr: true,
			expectEOF: true,
		},
		{
			name:        "invalid start line",
			data:        []byte("INVALID\n\n"),
			expectErr:   true,
			errContains: "invalid start line",
		},
		{
			name:      "read error on headers",
			data:      []byte("GET / HTTP/1.1\nHost: example.com"),
			expectErr: true,
			expectEOF: true,
		},
		{
			name:          "multiple colons in header",
			data:          []byte("GET / HTTP/1.1\r\nX-Custom: value:with:colons\r\n\r\n"),
			expectErr:     false,
			expectMethod:  "GET",
			expectPath:    "/",
			expectVersion: "HTTP/1.1",
			expectHeaders: map[string]string{
				"X-Custom": "value:with:colons",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := bufio.NewReader(bytes.NewReader(tt.data))
			req, err := NewRequest(br)
			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectEOF {
					assert.Equal(t, io.EOF, err)
				}
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

func TestNewRequestUnsupportedType(t *testing.T) {
	req, err := NewRequest(123)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type: int")
	assert.Nil(t, req)
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
		name           string
		data           []byte
		initialHeaders map[string]string
		expectHeaders  map[string]string
	}{
		{
			name: "various header formats",
			data: []byte("K1: V1\r\nK2:V2\r\n K3 : V3 \r\nNoColon\r\n\r\n"),
			expectHeaders: map[string]string{
				"K1": "V1",
				"K2": "V2",
				"K3": "V3",
			},
		},
		{
			name: "no trailing CRLF",
			data: []byte("K1: V1"),
			expectHeaders: map[string]string{
				"K1": "V1",
			},
		},
		{
			name:          "empty lines",
			data:          []byte("\r\nK1: V1"),
			expectHeaders: map[string]string{},
		},
		{
			name: "headers with only colon",
			data: []byte(": value\r\nkey:\r\n"),
			expectHeaders: map[string]string{
				"":    "value",
				"key": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &requestHeader{headers: make(map[string]string)}
			if tt.initialHeaders != nil {
				req.headers = tt.initialHeaders
			}
			setRemainingHeaders(tt.data, req)
			assert.Equal(t, len(tt.expectHeaders), len(req.headers))
			for k, v := range tt.expectHeaders {
				assert.Equal(t, v, req.headers[k])
			}
		})
	}
}

func TestParseHeadersFromReaderEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		expectHeaders map[string]string
	}{
		{
			name: "malformed header line",
			data: []byte("GET / HTTP/1.1\r\nMalformedLine\r\nK1: V1\r\n\r\n"),
			expectHeaders: map[string]string{
				"K1": "V1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := bufio.NewReader(bytes.NewReader(tt.data))
			req, err := parseHeadersFromReader(br)
			assert.NoError(t, err)
			for k, v := range tt.expectHeaders {
				assert.Equal(t, v, req.Value(k))
			}
		})
	}
}
