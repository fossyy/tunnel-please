package stream

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"tunnel_pls/internal/http/header"

	"github.com/stretchr/testify/assert"
)

type mockAddr struct {
	addr string
}

func (m *mockAddr) String() string  { return m.addr }
func (m *mockAddr) Network() string { return "tcp" }

type mockRequestMiddleware struct {
	err error
}

func (m *mockRequestMiddleware) HandleRequest(h header.RequestHeader) error {
	if m.err == nil {
		h.Set("X-Middleware", "true")
	}
	return m.err
}

type mockResponseMiddleware struct {
	err error
}

func (m *mockResponseMiddleware) HandleResponse(h header.ResponseHeader, body []byte) error {
	if m.err == nil {
		h.Set("X-Resp-Middleware", "true")
	}
	return m.err
}

type mockReadWriter struct {
	bytes.Buffer
	closed      bool
	writeClosed bool
}

func (m *mockReadWriter) Close() error {
	m.closed = true
	return nil
}

func (m *mockReadWriter) CloseWrite() error {
	m.writeClosed = true
	return nil
}

func TestHTTPMethods(t *testing.T) {
	addr := &mockAddr{addr: "1.2.3.4:1234"}
	rw := &mockReadWriter{}
	hs := New(rw, rw, addr)

	assert.Equal(t, addr, hs.RemoteAddr())

	reqMW := &mockRequestMiddleware{}
	hs.UseRequestMiddleware(reqMW)
	assert.Equal(t, 1, len(hs.RequestMiddlewares()))
	assert.Equal(t, reqMW, hs.RequestMiddlewares()[0])

	respMW := &mockResponseMiddleware{}
	hs.UseResponseMiddleware(respMW)
	assert.Equal(t, 1, len(hs.ResponseMiddlewares()))
	assert.Equal(t, respMW, hs.ResponseMiddlewares()[0])

	reqH, _ := header.NewRequest([]byte("GET / HTTP/1.1\r\n\r\n"))
	hs.SetRequestHeader(reqH)
}

func TestApplyMiddlewares(t *testing.T) {
	addr := &mockAddr{addr: "1.2.3.4:1234"}

	tests := []struct {
		name      string
		setup     func(HTTP)
		apply     func(HTTP, header.RequestHeader, header.ResponseHeader) error
		verify    func(*testing.T, header.RequestHeader, header.ResponseHeader)
		expectErr bool
	}{
		{
			name: "apply request middleware success",
			setup: func(hs HTTP) {
				hs.UseRequestMiddleware(&mockRequestMiddleware{})
			},
			apply: func(hs HTTP, reqH header.RequestHeader, respH header.ResponseHeader) error {
				return hs.ApplyRequestMiddlewares(reqH)
			},
			verify: func(t *testing.T, reqH header.RequestHeader, respH header.ResponseHeader) {
				assert.Equal(t, "true", reqH.Value("X-Middleware"))
			},
		},
		{
			name: "apply response middleware success",
			setup: func(hs HTTP) {
				hs.UseResponseMiddleware(&mockResponseMiddleware{})
			},
			apply: func(hs HTTP, reqH header.RequestHeader, respH header.ResponseHeader) error {
				return hs.ApplyResponseMiddlewares(respH, []byte("body"))
			},
			verify: func(t *testing.T, reqH header.RequestHeader, respH header.ResponseHeader) {
				assert.Equal(t, "true", respH.Value("X-Resp-Middleware"))
			},
		},
		{
			name: "apply request middleware error",
			setup: func(hs HTTP) {
				hs.UseRequestMiddleware(&mockRequestMiddleware{err: assert.AnError})
			},
			apply: func(hs HTTP, reqH header.RequestHeader, respH header.ResponseHeader) error {
				return hs.ApplyRequestMiddlewares(reqH)
			},
			expectErr: true,
		},
		{
			name: "apply response middleware error",
			setup: func(hs HTTP) {
				hs.UseResponseMiddleware(&mockResponseMiddleware{err: assert.AnError})
			},
			apply: func(hs HTTP, reqH header.RequestHeader, respH header.ResponseHeader) error {
				return hs.ApplyResponseMiddlewares(respH, []byte("body"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqH, _ := header.NewRequest([]byte("GET / HTTP/1.1\r\n\r\n"))
			respH, _ := header.NewResponse([]byte("HTTP/1.1 200 OK\r\n\r\n"))
			rw := &mockReadWriter{}
			hs := New(rw, rw, addr)
			tt.setup(hs)
			err := tt.apply(hs, reqH, respH)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.verify != nil {
					tt.verify(t, reqH, respH)
				}
			}
		})
	}
}

type mockWriterOnly struct {
	bytes.Buffer
}

func TestCloseMethods(t *testing.T) {
	addr := &mockAddr{addr: "1.2.3.4:1234"}

	tests := []struct {
		name   string
		writer any
		op     func(HTTP) error
		verify func(*testing.T, any)
	}{
		{
			name:   "Close success",
			writer: &mockReadWriter{},
			op:     func(hs HTTP) error { return hs.Close() },
			verify: func(t *testing.T, w any) {
				assert.True(t, w.(*mockReadWriter).closed)
			},
		},
		{
			name:   "CloseWrite with CloseWrite implementation",
			writer: &mockReadWriter{},
			op:     func(hs HTTP) error { return hs.CloseWrite() },
			verify: func(t *testing.T, w any) {
				assert.True(t, w.(*mockReadWriter).writeClosed)
			},
		},
		{
			name:   "CloseWrite fallback to Close",
			writer: &mockReadWriterOnlyCloser{},
			op:     func(hs HTTP) error { return hs.CloseWrite() },
			verify: func(t *testing.T, w any) {
				assert.True(t, w.(*mockReadWriterOnlyCloser).closed)
			},
		},
		{
			name:   "Close with No Closer",
			writer: &mockWriterOnly{},
			op:     func(hs HTTP) error { return hs.Close() },
		},
		{
			name:   "CloseWrite with No CloseWrite and No Closer",
			writer: &mockWriterOnly{},
			op:     func(hs HTTP) error { return hs.CloseWrite() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := New(tt.writer.(io.Writer), tt.writer.(io.Reader), addr)
			assert.NotPanics(t, func() {
				err := tt.op(hs)
				assert.NoError(t, err)
			})
			if tt.verify != nil {
				tt.verify(t, tt.writer)
			}
		})
	}
}

type mockReadWriterOnlyCloser struct {
	bytes.Buffer
	closed bool
}

func (m *mockReadWriterOnlyCloser) Close() error {
	m.closed = true
	return nil
}

func TestSplitHeaderAndBody(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		delimiterIdx int
		expectHeader []byte
		expectBody   []byte
	}{
		{
			name:         "standard",
			data:         []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\nBodyContent"),
			delimiterIdx: 31,
			expectHeader: []byte("GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"),
			expectBody:   []byte("BodyContent"),
		},
		{
			name:         "empty body",
			data:         []byte("HTTP/1.1 200 OK\r\n\r\n"),
			delimiterIdx: 15,
			expectHeader: []byte("HTTP/1.1 200 OK\r\n\r\n"),
			expectBody:   []byte(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, b := splitHeaderAndBody(tt.data, tt.delimiterIdx)
			assert.Equal(t, tt.expectHeader, h)
			assert.Equal(t, tt.expectBody, b)
		})
	}
}

func TestIsHTTPHeader(t *testing.T) {
	tests := []struct {
		name   string
		buf    []byte
		expect bool
	}{
		{
			name:   "valid request",
			buf:    []byte("GET /path HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			expect: true,
		},
		{
			name:   "valid response",
			buf:    []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n"),
			expect: true,
		},
		{
			name:   "invalid start line",
			buf:    []byte("NOT_HTTP /path\r\nHost: example.com\r\n\r\n"),
			expect: false,
		},
		{
			name:   "invalid header line (no colon)",
			buf:    []byte("GET / HTTP/1.1\r\nInvalidHeaderLine\r\n\r\n"),
			expect: false,
		},
		{
			name:   "invalid header line (colon at 0)",
			buf:    []byte("GET / HTTP/1.1\r\n: value\r\n\r\n"),
			expect: false,
		},
		{
			name:   "empty header section",
			buf:    []byte("GET / HTTP/1.1\r\n\r\n"),
			expect: true,
		},
		{
			name:   "multiple headers",
			buf:    []byte("GET / HTTP/1.1\r\nH1: V1\r\nH2: V2\r\n\r\n"),
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHTTPHeader(tt.buf)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestRead(t *testing.T) {
	tests := []struct {
		name          string
		input         []byte
		readLen       int
		expectContent string
		expectRead    int
		expectErr     bool
		middlewareErr error
	}{
		{
			name:          "valid http request",
			input:         []byte("GET / HTTP/1.1\r\nHost: test\r\n\r\nBody"),
			readLen:       100,
			expectContent: "Body",
			expectRead:    54,
		},
		{
			name:          "non-http data",
			input:         []byte("Some random data\r\n\r\nMore data"),
			readLen:       100,
			expectContent: "Some random data\r\n\r\nMore data",
			expectRead:    29,
		},
		{
			name:          "no delimiter",
			input:         []byte("Partial data without delimiter"),
			readLen:       100,
			expectContent: "Partial data without delimiter",
			expectRead:    30,
		},
		{
			name:          "middleware error",
			input:         []byte("GET / HTTP/1.1\r\nHost: test\r\n\r\n"),
			readLen:       100,
			middlewareErr: assert.AnError,
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := &mockReadWriter{}
			rw.Write(tt.input)
			hs := New(rw, rw, &mockAddr{})
			if tt.middlewareErr != nil {
				hs.UseRequestMiddleware(&mockRequestMiddleware{err: tt.middlewareErr})
			} else {
				hs.UseRequestMiddleware(&mockRequestMiddleware{})
			}

			p := make([]byte, tt.readLen)
			n, err := hs.Read(p)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectRead, n)
				if tt.name == "valid http request" {
					content := string(p[:n])
					assert.Contains(t, content, "GET / HTTP/1.1\r\n")
					assert.Contains(t, content, "Host: test\r\n")
					assert.Contains(t, content, "X-Middleware: true\r\n")
					assert.True(t, bytes.HasSuffix(p[:n], []byte("\r\n\r\nBody")))
				} else {
					assert.Equal(t, tt.expectContent, string(p[:n]))
				}
			}
		})
	}
}

func TestWrite(t *testing.T) {
	tests := []struct {
		name          string
		writes        [][]byte
		expectWritten string
		expectErr     bool
		middlewareErr error
	}{
		{
			name: "valid http response in one write",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nBody"),
			},
			expectWritten: "HTTP/1.1 200 OK\r\nContent-Length: 4\r\nX-Resp-Middleware: true\r\n\r\nBody",
		},
		{
			name: "valid http response in multiple writes",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\n"),
				[]byte("Content-Length: 4\r\n\r\n"),
				[]byte("Body"),
			},
			expectWritten: "HTTP/1.1 200 OK\r\nContent-Length: 4\r\nX-Resp-Middleware: true\r\n\r\nBody",
		},
		{
			name: "non-http data",
			writes: [][]byte{
				[]byte("Random data with delimiter\r\n\r\nFlush"),
			},
			expectWritten: "Random data with delimiter\r\n\r\nFlush",
		},
		{
			name: "bypass buffering",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\n\r\n"),
				[]byte("HTTP/1.1 200 OK\r\n\r\n"),
			},
			expectWritten: "HTTP/1.1 200 OK\r\nX-Resp-Middleware: true\r\n\r\n" +
				"HTTP/1.1 200 OK\r\nX-Resp-Middleware: true\r\n\r\n",
		},
		{
			name: "middleware error",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\n\r\n"),
			},
			middlewareErr: assert.AnError,
			expectErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := &mockReadWriter{}
			hs := New(rw, rw, &mockAddr{})
			if tt.middlewareErr != nil {
				hs.UseResponseMiddleware(&mockResponseMiddleware{err: tt.middlewareErr})
			} else {
				hs.UseResponseMiddleware(&mockResponseMiddleware{})
			}

			var totalN int
			var err error
			for _, w := range tt.writes {
				var n int
				n, err = hs.Write(w)
				if err != nil {
					break
				}
				totalN += n
			}

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if strings.HasPrefix(tt.expectWritten, "HTTP/") {
					written := rw.String()
					assert.Contains(t, written, "HTTP/1.1 200 OK\r\n")
					assert.Contains(t, written, "X-Resp-Middleware: true\r\n")
					if strings.Contains(tt.expectWritten, "Content-Length: 4") {
						assert.Contains(t, written, "Content-Length: 4\r\n")
					}
					assert.True(t, strings.HasSuffix(written, "\r\n\r\nBody") || strings.HasSuffix(written, "\r\n\r\n"))
				} else {
					assert.Equal(t, tt.expectWritten, rw.String())
				}
			}
		})
	}
}

func TestWriteErrors(t *testing.T) {
	addr := &mockAddr{addr: "1.2.3.4:1234"}

	tests := []struct {
		name   string
		writer any
		data   []byte
	}{
		{
			name:   "write error in writeHeaderAndBody",
			writer: &mockErrorWriteCloser{},
			data:   []byte("HTTP/1.1 200 OK\r\n\r\n"),
		},
		{
			name:   "write error in writeHeaderAndBody second write",
			writer: &mockFailSecondWriteCloser{},
			data:   []byte("HTTP/1.1 200 OK\r\n\r\nBody"),
		},
		{
			name:   "write error in writeRawBuffer",
			writer: &mockErrorWriteCloser{},
			data:   []byte("Not HTTP\r\n\r\nFlush"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := New(tt.writer.(io.Writer), tt.writer.(io.Reader), addr)
			_, err := hs.Write(tt.data)
			assert.Error(t, err)
		})
	}
}

func TestReadEOF(t *testing.T) {
	tests := []struct {
		name          string
		reader        io.Reader
		expectN       int
		expectErr     error
		expectContent string
	}{
		{
			name:          "read eof",
			reader:        &mockEOFReader{},
			expectN:       4,
			expectErr:     io.EOF,
			expectContent: "data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs := New(nil, tt.reader, &mockAddr{})
			p := make([]byte, 100)
			n, err := hs.Read(p)
			assert.Equal(t, tt.expectN, n)
			assert.Equal(t, tt.expectErr, err)
			assert.Equal(t, tt.expectContent, string(p[:n]))
		})
	}
}

type mockEOFReader struct {
	mockReadWriter
}

func (m *mockEOFReader) Read(p []byte) (int, error) {
	copy(p, "data")
	return 4, io.EOF
}

type mockFailSecondWriteCloser struct {
	count int
}

func (m *mockFailSecondWriteCloser) Write(p []byte) (int, error) {
	m.count++
	if m.count == 2 {
		return 0, assert.AnError
	}
	return len(p), nil
}

func (m *mockFailSecondWriteCloser) Close() error               { return nil }
func (m *mockFailSecondWriteCloser) Read(p []byte) (int, error) { return 0, nil }

type mockErrorWriteCloser struct {
	closed bool
}

func (m *mockErrorWriteCloser) Write(p []byte) (int, error) {
	return 0, assert.AnError
}

func (m *mockErrorWriteCloser) Close() error {
	m.closed = true
	return nil
}

func (m *mockErrorWriteCloser) Read(p []byte) (int, error) {
	return 0, nil
}
