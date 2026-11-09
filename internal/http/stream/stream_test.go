package stream

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"

	"tunnel_pls/internal/http/header"
	"tunnel_pls/internal/middleware"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockAddr struct {
	mock.Mock
}

func (m *MockAddr) String() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockAddr) Network() string {
	args := m.Called()
	return args.String(0)
}

type MockRequestMiddleware struct {
	mock.Mock
}

func (m *MockRequestMiddleware) HandleRequest(h header.RequestHeader) error {
	args := m.Called(h)
	return args.Error(0)
}

type MockResponseMiddleware struct {
	mock.Mock
}

func (m *MockResponseMiddleware) HandleResponse(h header.ResponseHeader, body []byte) error {
	args := m.Called(h, body)
	return args.Error(0)
}

type MockReadWriter struct {
	mock.Mock
	bytes.Buffer
}

func (m *MockReadWriter) Read(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockReadWriter) Write(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockReadWriter) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockReadWriter) CloseWrite() error {
	args := m.Called()
	return args.Error(0)
}

type MockReadWriterOnlyCloser struct {
	mock.Mock
	bytes.Buffer
}

func (m *MockReadWriterOnlyCloser) Read(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockReadWriterOnlyCloser) Write(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockReadWriterOnlyCloser) Close() error {
	args := m.Called()
	return args.Error(0)
}

type MockWriterOnly struct {
	mock.Mock
}

func (m *MockWriterOnly) Write(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockWriterOnly) Read(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

type MockReader struct {
	mock.Mock
}

func (m *MockReader) Read(p []byte) (int, error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

type MockWriter struct {
	mock.Mock
}

func (m *MockWriter) Write(p []byte) (int, error) {
	ret := m.Called(p)

	var n int
	var err error

	switch v := ret.Get(0).(type) {
	case func([]byte) int:
		n = v(p)
	case int:
		n = v
	default:
		n = len(p)
	}

	switch v := ret.Get(1).(type) {
	case func([]byte) error:
		err = v(p)
	case error:
		err = v
	default:
		err = nil
	}

	return n, err
}

func (m *MockWriter) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestHTTPMethods(t *testing.T) {
	addr := new(MockAddr)
	addr.On("String").Return("1.2.3.4:1234")

	rw := new(MockReadWriter)
	hs := New(rw, rw, addr, 4096)

	assert.Equal(t, addr, hs.RemoteAddr())

	reqMW := new(MockRequestMiddleware)
	hs.UseRequestMiddleware(reqMW)
	assert.Equal(t, 1, len(hs.RequestMiddlewares()))
	assert.Equal(t, reqMW, hs.RequestMiddlewares()[0])

	respMW := new(MockResponseMiddleware)
	hs.UseResponseMiddleware(respMW)
	assert.Equal(t, 1, len(hs.ResponseMiddlewares()))
	assert.Equal(t, respMW, hs.ResponseMiddlewares()[0])

	reqH, _ := header.NewRequest([]byte("GET / HTTP/1.1\r\n\r\n"))
	hs.SetRequestHeader(reqH)
}

func TestApplyMiddlewares(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(HTTP, *MockRequestMiddleware, *MockResponseMiddleware)
		apply     func(HTTP, header.RequestHeader, header.ResponseHeader) error
		verify    func(*testing.T, header.RequestHeader, header.ResponseHeader)
		expectErr bool
	}{
		{
			name: "apply request middleware success",
			setup: func(hs HTTP, reqMW *MockRequestMiddleware, respMW *MockResponseMiddleware) {
				reqMW.On("HandleRequest", mock.Anything).Run(func(args mock.Arguments) {
					h := args.Get(0).(header.RequestHeader)
					h.Set("X-Middleware", "true")
				}).Return(nil)
				hs.UseRequestMiddleware(reqMW)
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
			setup: func(hs HTTP, reqMW *MockRequestMiddleware, respMW *MockResponseMiddleware) {
				respMW.On("HandleResponse", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					h := args.Get(0).(header.ResponseHeader)
					h.Set("X-Resp-Middleware", "true")
				}).Return(nil)
				hs.UseResponseMiddleware(respMW)
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
			setup: func(hs HTTP, reqMW *MockRequestMiddleware, respMW *MockResponseMiddleware) {
				reqMW.On("HandleRequest", mock.Anything).Return(assert.AnError)
				hs.UseRequestMiddleware(reqMW)
			},
			apply: func(hs HTTP, reqH header.RequestHeader, respH header.ResponseHeader) error {
				return hs.ApplyRequestMiddlewares(reqH)
			},
			expectErr: true,
		},
		{
			name: "apply response middleware error",
			setup: func(hs HTTP, reqMW *MockRequestMiddleware, respMW *MockResponseMiddleware) {
				respMW.On("HandleResponse", mock.Anything, mock.Anything).Return(assert.AnError)
				hs.UseResponseMiddleware(respMW)
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

			addr := new(MockAddr)
			addr.On("String").Return("1.2.3.4:1234")

			rw := new(MockReadWriter)
			hs := New(rw, rw, addr, 4096)

			reqMW := new(MockRequestMiddleware)
			respMW := new(MockResponseMiddleware)
			tt.setup(hs, reqMW, respMW)

			err := tt.apply(hs, reqH, respH)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.verify != nil {
					tt.verify(t, reqH, respH)
				}
			}

			reqMW.AssertExpectations(t)
			respMW.AssertExpectations(t)
		})
	}
}

func TestCloseMethods(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (io.Writer, io.Reader)
		op     func(HTTP) error
		verify func(*testing.T, io.Writer)
	}{
		{
			name: "Close success",
			setup: func() (io.Writer, io.Reader) {
				rw := new(MockReadWriter)
				rw.On("Close").Return(nil)
				return rw, rw
			},
			op: func(hs HTTP) error { return hs.Close() },
			verify: func(t *testing.T, w io.Writer) {
				w.(*MockReadWriter).AssertCalled(t, "Close")
			},
		},
		{
			name: "CloseWrite with CloseWrite implementation",
			setup: func() (io.Writer, io.Reader) {
				rw := new(MockReadWriter)
				rw.On("CloseWrite").Return(nil)
				return rw, rw
			},
			op: func(hs HTTP) error { return hs.CloseWrite() },
			verify: func(t *testing.T, w io.Writer) {
				w.(*MockReadWriter).AssertCalled(t, "CloseWrite")
			},
		},
		{
			name: "CloseWrite fallback to Close",
			setup: func() (io.Writer, io.Reader) {
				rw := new(MockReadWriterOnlyCloser)
				rw.On("Close").Return(nil)
				return rw, rw
			},
			op: func(hs HTTP) error { return hs.CloseWrite() },
			verify: func(t *testing.T, w io.Writer) {
				w.(*MockReadWriterOnlyCloser).AssertCalled(t, "Close")
			},
		},
		{
			name: "Close with No Closer",
			setup: func() (io.Writer, io.Reader) {
				w := new(MockWriterOnly)
				r := new(MockReader)
				return w, r
			},
			op: func(hs HTTP) error { return hs.Close() },
		},
		{
			name: "CloseWrite with No CloseWrite and No Closer",
			setup: func() (io.Writer, io.Reader) {
				w := new(MockWriterOnly)
				r := new(MockReader)
				return w, r
			},
			op: func(hs HTTP) error { return hs.CloseWrite() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := new(MockAddr)
			addr.On("String").Return("1.2.3.4:1234")

			w, r := tt.setup()
			hs := New(w, r, addr, 4096)

			assert.NotPanics(t, func() {
				err := tt.op(hs)
				assert.NoError(t, err)
			})

			if tt.verify != nil {
				tt.verify(t, w)
			}
		})
	}
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
			name:   "obs-fold line",
			buf:    []byte("GET / HTTP/1.1\r\nX-A: one\r\n two\r\n\r\n"),
			expect: false,
		},
		{
			name:   "space before colon",
			buf:    []byte("GET / HTTP/1.1\r\nX-A : one\r\n\r\n"),
			expect: false,
		},
		{
			name:   "bare CR in value",
			buf:    []byte("GET / HTTP/1.1\r\nX-A: v\rl\r\n\r\n"),
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

// requestCase is one input for TestStreamRead: what the client sends, and what
// Read must hand out in the end.
type requestCase struct {
	name          string
	input         []byte
	want          string
	middleware    bool
	middlewareErr error
}

var readCases = []requestCase{
	{
		name:       "valid http request",
		input:      []byte("GET / HTTP/1.1\r\nHost: test\r\n\r\nBody"),
		want:       "GET / HTTP/1.1\r\nHost: test\r\nX-Middleware: true\r\n\r\nBody",
		middleware: true,
	},
	{
		// The rewrite adds bytes, so a full caller buffer cannot take it all:
		// the rest has to come out on the next Read, never more than len(p).
		name:       "rewritten request larger than a full buffer",
		input:      []byte("GET / HTTP/1.1\r\nHost: a.example.com\r\n\r\n" + strings.Repeat("x", 63000)),
		want:       "GET / HTTP/1.1\r\nHost: a.example.com\r\nX-Middleware: true\r\n\r\n" + strings.Repeat("x", 63000),
		middleware: true,
	},
	{
		// A header that arrives in pieces still gets the header added. Fewer
		// than 64 fields: the parser caps those.
		name:       "header split across reads",
		input:      []byte("GET / HTTP/1.1\r\n" + strings.Repeat("X-Pad: "+strings.Repeat("a", 200)+"\r\n", 10) + "\r\nBody"),
		want:       "GET / HTTP/1.1\r\n" + strings.Repeat("X-Pad: "+strings.Repeat("a", 200)+"\r\n", 10) + "X-Middleware: true\r\n\r\nBody",
		middleware: true,
	},
	{
		name:  "non-http data",
		input: []byte("Some random data\r\n\r\nMore data"),
		want:  "Some random data\r\n\r\nMore data",
	},
	{
		name:  "no delimiter",
		input: []byte("Partial data without delimiter"),
		want:  "Partial data without delimiter",
	},
	{
		name:  "malformed block passes through untouched",
		input: []byte("GET / HTTP/1.1\r\nHost: test\r\nNoColon\r\n\r\n"),
		want:  "GET / HTTP/1.1\r\nHost: test\r\nNoColon\r\n\r\n",
	},
	{
		name:  "plain body passes through",
		input: []byte("GET / HTTP/1.1\r\nno colon here\r\n\r\n"),
		want:  "GET / HTTP/1.1\r\nno colon here\r\n\r\n",
	},
	{
		name:          "middleware error",
		input:         []byte("GET / HTTP/1.1\r\nHost: test\r\n\r\n"),
		middlewareErr: assert.AnError,
	},
}

// TestStreamRead reads each case with several buffer sizes. The bytes handed
// out must always be the same, and never more than the buffer.
func TestStreamRead(t *testing.T) {
	for _, tt := range readCases {
		t.Run(tt.name, func(t *testing.T) {
			for _, size := range []int{1, 7, 64, 4096, 32768} {
				addr := new(MockAddr)
				addr.On("String").Return("1.2.3.4:1234")
				hs := New(io.Discard, bytes.NewReader(tt.input), addr, 4096)

				var reqMW *MockRequestMiddleware
				if tt.middleware || tt.middlewareErr != nil {
					reqMW = new(MockRequestMiddleware)
					if tt.middlewareErr != nil {
						reqMW.On("HandleRequest", mock.Anything).Return(tt.middlewareErr)
					} else {
						reqMW.On("HandleRequest", mock.Anything).Run(func(args mock.Arguments) {
							h := args.Get(0).(header.RequestHeader)
							h.Set("X-Middleware", "true")
						}).Return(nil)
					}
					hs.UseRequestMiddleware(reqMW)
				}

				var got []byte
				var readErr error
				p := make([]byte, size)
				for i := 0; i < 100000; i++ {
					n, err := hs.Read(p)
					// Never more than the buffer: io.Copy panics on that.
					assert.LessOrEqual(t, n, size)
					got = append(got, p[:n]...)
					if err != nil {
						readErr = err
						break
					}
				}

				if tt.middlewareErr != nil {
					assert.Error(t, readErr)
				} else {
					assert.Equal(t, tt.want, string(got), "buffer size %d", size)
					assert.ErrorIs(t, readErr, io.EOF)
				}
				if reqMW != nil {
					reqMW.AssertExpectations(t)
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
		isHTTP        bool
	}{
		{
			name: "valid http response in one write",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\nBody"),
			},
			expectWritten: "HTTP/1.1 200 OK\r\nContent-Length: 4\r\nX-Resp-Middleware: true\r\n\r\nBody",
			isHTTP:        true,
		},
		{
			name: "valid http response in multiple writes",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\n"),
				[]byte("Content-Length: 4\r\n\r\n"),
				[]byte("Body"),
			},
			expectWritten: "HTTP/1.1 200 OK\r\nContent-Length: 4\r\nX-Resp-Middleware: true\r\n\r\nBody",
			isHTTP:        true,
		},
		{
			name: "non-http data",
			writes: [][]byte{
				[]byte("Random data with delimiter\r\n\r\nFlush"),
			},
			expectWritten: "Random data with delimiter\r\n\r\nFlush",
			isHTTP:        false,
		},
		{
			name: "bypass buffering",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\n\r\n"),
				[]byte("HTTP/1.1 200 OK\r\n\r\n"),
			},
			expectWritten: "HTTP/1.1 200 OK\r\nX-Resp-Middleware: true\r\n\r\n" +
				"HTTP/1.1 200 OK\r\nX-Resp-Middleware: true\r\n\r\n",
			isHTTP: true,
		},
		{
			name: "middleware error",
			writes: [][]byte{
				[]byte("HTTP/1.1 200 OK\r\n\r\n"),
			},
			middlewareErr: assert.AnError,
			expectErr:     true,
			isHTTP:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := new(MockAddr)
			addr.On("String").Return("1.2.3.4:1234")

			var writtenData bytes.Buffer
			writer := new(MockWriter)

			writer.On("Write", mock.Anything).Run(func(args mock.Arguments) {
				p := args.Get(0).([]byte)
				writtenData.Write(p)
			}).Return(func(p []byte) int {
				return len(p)
			}, nil)

			reader := new(MockReader)
			hs := New(writer, reader, addr, 4096)

			respMW := new(MockResponseMiddleware)
			if tt.isHTTP {
				if tt.middlewareErr != nil {
					respMW.On("HandleResponse", mock.Anything, mock.Anything).Return(tt.middlewareErr)
				} else {
					respMW.On("HandleResponse", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
						h := args.Get(0).(header.ResponseHeader)
						h.Set("X-Resp-Middleware", "true")
					}).Return(nil)
				}
			}
			hs.UseResponseMiddleware(respMW)

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
				written := writtenData.String()
				if strings.HasPrefix(tt.expectWritten, "HTTP/") {
					assert.Contains(t, written, "HTTP/1.1 200 OK\r\n")
					assert.Contains(t, written, "X-Resp-Middleware: true\r\n")
					if strings.Contains(tt.expectWritten, "Content-Length: 4") {
						assert.Contains(t, written, "Content-Length: 4\r\n")
					}
					assert.True(t, strings.HasSuffix(written, "\r\n\r\nBody") || strings.HasSuffix(written, "\r\n\r\n"))
				} else {
					assert.Equal(t, tt.expectWritten, written)
				}
			}

			if tt.isHTTP {
				respMW.AssertExpectations(t)
			}
			if tt.middlewareErr == nil {
				writer.AssertExpectations(t)
			}
		})
	}
}

func TestWriteErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func() (io.Writer, io.Reader)
		data  []byte
	}{
		{
			name: "write error in writeHeaderAndBody",
			setup: func() (io.Writer, io.Reader) {
				writer := new(MockWriter)
				writer.On("Write", mock.Anything).Return(0, assert.AnError)
				reader := new(MockReader)
				return writer, reader
			},
			data: []byte("HTTP/1.1 200 OK\r\n\r\n"),
		},
		{
			name: "write error in writeHeaderAndBody second write",
			setup: func() (io.Writer, io.Reader) {
				writer := new(MockWriter)
				writer.On("Write", mock.Anything).Return(len([]byte("HTTP/1.1 200 OK\r\n\r\n")), nil).Once()
				writer.On("Write", mock.Anything).Return(0, assert.AnError).Once()
				reader := new(MockReader)
				return writer, reader
			},
			data: []byte("HTTP/1.1 200 OK\r\n\r\nBody"),
		},
		{
			name: "write error in writeRawBuffer",
			setup: func() (io.Writer, io.Reader) {
				writer := new(MockWriter)
				writer.On("Write", mock.Anything).Return(0, assert.AnError)
				reader := new(MockReader)
				return writer, reader
			},
			data: []byte("Not HTTP\r\n\r\nFlush"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := new(MockAddr)
			addr.On("String").Return("1.2.3.4:1234")

			w, r := tt.setup()
			hs := New(w, r, addr, 4096)

			_, err := hs.Write(tt.data)
			assert.Error(t, err)

			w.(*MockWriter).AssertExpectations(t)
		})
	}
}

func TestReadEOF(t *testing.T) {
	tests := []struct {
		name          string
		setup         func() io.Reader
		expectN       int
		expectErr     error
		expectContent string
	}{
		{
			// Read hands out what it took before the error, and the error on
			// the next call -- the io.Reader contract.
			name: "read eof",
			setup: func() io.Reader {
				reader := new(MockReader)
				reader.On("Read", mock.Anything).Run(func(args mock.Arguments) {
					p := args.Get(0).([]byte)
					copy(p, "data")
				}).Return(4, io.EOF).Once()
				return reader
			},
			expectN:       4,
			expectErr:     io.EOF,
			expectContent: "data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := new(MockAddr)
			addr.On("String").Return("1.2.3.4:1234")

			reader := tt.setup()
			hs := New(nil, reader, addr, 4096)

			p := make([]byte, 100)
			n, err := hs.Read(p)

			assert.Equal(t, tt.expectN, n)
			assert.Equal(t, tt.expectErr, err)
			assert.Equal(t, tt.expectContent, string(p[:n]))

			reader.(*MockReader).AssertExpectations(t)
		})
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (discardWriter) Close() error                { return nil }

type emptyReader struct{}

func (emptyReader) Read(p []byte) (int, error) { return 0, io.EOF }

// A response shape the tunnel actually forwards: a few headers, short body.
func benchResponse() []byte {
	return []byte("HTTP/1.1 200 OK\r\n" +
		"Content-Type: application/json\r\n" +
		"Content-Length: 3\r\n" +
		"Connection: keep-alive\r\n" +
		"Date: Tue, 10 Nov 2026 00:00:00 GMT\r\n" +
		"Server: upstream\r\n" +
		"\r\nabc")
}

func benchAddr() net.Addr {
	a, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:1234")
	return a
}

// One response header parsed and rewritten: the cost per tunneled response.
func BenchmarkNewResponse(b *testing.B) {
	in := benchResponse()
	frame := in[:bytes.Index(in, DELIMITER)+len(DELIMITER)]

	b.ReportAllocs()
	b.SetBytes(int64(len(frame)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := header.NewResponse(frame); err != nil {
			b.Fatal(err)
		}
	}
}

// End-to-end: stream.Write parses, runs middleware wiped and emits one response.
func BenchmarkStreamWriteOneResponse(b *testing.B) {
	in := benchResponse()

	b.ReportAllocs()
	b.SetBytes(int64(len(in)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hs := New(discardWriter{}, emptyReader{}, benchAddr(), 4096)
		hs.UseResponseMiddleware(middleware.NewTunnelFingerprint())
		if _, err := hs.Write(in); err != nil {
			b.Fatal(err)
		}
	}
}
