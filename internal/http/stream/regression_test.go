package stream

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tunnel_pls/internal/http/header"
)

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type headerMW struct{ name, value string }

func (m headerMW) HandleRequest(h header.RequestHeader) error {
	h.Set(m.name, m.value)
	return nil
}

func (m headerMW) HandleResponse(h header.ResponseHeader, _ []byte) error {
	h.Set(m.name, m.value)
	return nil
}

// A read that fills the copy buffer used to return more bytes than the buffer
// held, which panics io.Copy, and dropped the tail of the body. It must hand
// out exactly the rewritten request and then the body.
func TestReadFullBufferDoesNotOverrun(t *testing.T) {
	body := strings.Repeat("x", 40000)
	in := []byte("POST /upload HTTP/1.1\r\nHost: a.example.com\r\nContent-Length: 40000\r\n\r\n" + body)

	var out bytes.Buffer
	hs := New(&out, bytes.NewReader(in), testAddr("203.0.113.9:1234"), 4096)
	hs.UseRequestMiddleware(headerMW{"X-Forwarded-For", "203.0.113.9"})

	_, err := io.CopyBuffer(&out, hs, make([]byte, 32768))
	require.NoError(t, err)

	got := out.String()
	added := "X-Forwarded-For: 203.0.113.9\r\n"
	assert.True(t, strings.HasPrefix(got, "POST /upload HTTP/1.1\r\nHost: a.example.com\r\nContent-Length: 40000\r\n"+added+"\r\n"))
	assert.True(t, strings.HasSuffix(got, body), "body must arrive whole")
	assert.Equal(t, len(in)+len(added), len(got), "not one byte lost or added")
}

// A backend that never sends the blank line must not grow the response buffer
// without limit: past the header cap the bytes are passed straight through.
func TestWriteStopsBufferingPastHeaderCap(t *testing.T) {
	var out bytes.Buffer
	hs := New(&out, bytes.NewReader(nil), testAddr("203.0.113.9:1234"), 512)

	frag := []byte("HTTP/1.1 200 OK\r\nX-Big: " + strings.Repeat("a", 1000))
	n, err := hs.Write(frag)
	require.NoError(t, err)
	assert.Equal(t, len(frag), n)
	assert.Equal(t, string(frag), out.String(), "bytes must pass through once the cap is reached")
	assert.Empty(t, hs.(*http).buf, "nothing may stay buffered past the cap")

	_, err = hs.Write([]byte("\r\n\r\nbody"))
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(out.String(), "\r\n\r\nbody"))
}

// 70 Set-Cookie lines are a normal response, not an attack. Every line must
// reach the client and the response middleware must still run.
func TestWriteKeepsManySetCookieHeaders(t *testing.T) {
	var resp strings.Builder
	resp.WriteString("HTTP/1.1 200 OK\r\n")
	for i := 0; i < 70; i++ {
		fmt.Fprintf(&resp, "Set-Cookie: c%d=v\r\n", i)
	}
	resp.WriteString("\r\n")

	var out bytes.Buffer
	hs := New(&out, bytes.NewReader(nil), testAddr("203.0.113.9:1234"), 4096)
	hs.UseResponseMiddleware(headerMW{"Server", "Tunnel Please"})

	_, err := hs.Write([]byte(resp.String()))
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "Server: Tunnel Please\r\n")
	for i := 0; i < 70; i++ {
		assert.Contains(t, got, fmt.Sprintf("Set-Cookie: c%d=v\r\n", i))
	}
	assert.True(t, strings.HasSuffix(got, "\r\n\r\n"))
}

// Same on the request side: 70 headers plus the body must all reach the app,
// with the real X-Forwarded-For added.
func TestReadKeepsManyHeaders(t *testing.T) {
	var req strings.Builder
	req.WriteString("POST / HTTP/1.1\r\nHost: a.example.com\r\n")
	for i := 0; i < 70; i++ {
		fmt.Fprintf(&req, "X-Custom-%02d: v\r\n", i)
	}
	req.WriteString("Content-Length: 4\r\n\r\nbody")

	var out bytes.Buffer
	hs := New(&out, strings.NewReader(req.String()), testAddr("203.0.113.9:1234"), 4096)
	hs.UseRequestMiddleware(headerMW{"X-Forwarded-For", "203.0.113.9"})

	_, err := io.CopyBuffer(&out, hs, make([]byte, 32768))
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "X-Forwarded-For: 203.0.113.9\r\n")
	for i := 0; i < 70; i++ {
		assert.Contains(t, got, fmt.Sprintf("X-Custom-%02d: v\r\n", i))
	}
	assert.True(t, strings.HasSuffix(got, "\r\n\r\nbody"))
}

// Read and Write run in two goroutines on one connection. They must not touch
// the same state; run this with -race.
func TestConcurrentReadWrite(t *testing.T) {
	requests := bytes.Repeat([]byte("GET / HTTP/1.1\r\nHost: a.example.com\r\n\r\n"), 200)

	hs := New(io.Discard, bytes.NewReader(requests), testAddr("203.0.113.9:1234"), 4096)
	hs.UseRequestMiddleware(headerMW{"X-Forwarded-For", "203.0.113.9"})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		p := make([]byte, 4096)
		for {
			if _, err := hs.Read(p); err != nil {
				return
			}
		}
	}()

	go func() {
		defer wg.Done()
		response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		for i := 0; i < 200; i++ {
			_, _ = hs.Write(response[:20])
			_, _ = hs.Write(response[20:])
		}
	}()

	wg.Wait()
}

// The two copier goroutines used to share one buffer: a short request read
// truncated a half-received response. Keeping them apart fixes that.
func TestReadDoesNotTruncateBufferedResponse(t *testing.T) {
	var out bytes.Buffer
	hs := New(&out, bytes.NewReader([]byte("x")), testAddr("203.0.113.9:1234"), 4096)
	// No response middleware: the output is the header exactly as sent.

	_, err := hs.Write([]byte("HTTP/1.1 200 OK\r\nContent-Le"))
	require.NoError(t, err)
	_, err = hs.Read(make([]byte, 8))
	require.NoError(t, err)
	_, err = hs.Write([]byte("ngth: 5\r\n\r\nhello"))
	require.NoError(t, err)

	assert.Equal(t, "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello", out.String())
}

var _ net.Addr = testAddr("")
