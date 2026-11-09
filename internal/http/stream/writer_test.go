package stream

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"tunnel_pls/internal/http/header"

	"github.com/stretchr/testify/mock"
)

type captureWriter struct{ b bytes.Buffer }

func (w *captureWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

func newCaptureWriter() (HTTP, *captureWriter) {
	w := &captureWriter{}
	addr, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:1234")
	return New(w, bytes.NewReader(nil), addr, 4096), w
}

func writeAll(t *testing.T, writes []string) string {
	t.Helper()

	var out bytes.Buffer
	w := new(MockWriter)
	w.On("Write", mock.Anything).Run(func(a mock.Arguments) {
		out.Write(a.Get(0).([]byte))
	}).Return(func(p []byte) int { return len(p) }, nil)

	addr, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:1234")
	hs := New(w, bytes.NewReader(nil), addr, 4096)

	mw := new(MockResponseMiddleware)
	mw.On("HandleResponse", mock.Anything, mock.Anything).Run(func(a mock.Arguments) {
		a.Get(0).(header.ResponseHeader).Set("Server", "Tunnel Please")
	}).Return(nil)
	hs.UseResponseMiddleware(mw)

	for _, p := range writes {
		_, err := hs.Write([]byte(p))
		if err != nil {
			t.Fatalf("Write(%q): %v", p, err)
		}
	}
	return out.String()
}

// The body may arrive across writes, not in one. Bytes after the header block
// are body, and must be written straight through -- never buffered and looked
// at for a "\r\n\r\n" again.
func TestWrite_BodySplitAcrossWrites(t *testing.T) {
	var out bytes.Buffer

	w := new(MockWriter)
	w.On("Write", mock.Anything).Run(func(a mock.Arguments) {
		out.Write(a.Get(0).([]byte))
	}).Return(func(p []byte) int { return len(p) }, nil)

	addr, _ := net.ResolveTCPAddr("tcp", "1.2.3.4:1234")
	hs := New(w, bytes.NewReader(nil), addr, 4096)

	writes := []string{
		"HTTP/1.1 200 OK\r\nContent-Length: 9\r\n\r\nabc",
		"def",
		"ghi",
	}

	for _, s := range writes {
		if _, err := hs.Write([]byte(s)); err != nil {
			t.Fatalf("Write(%q): %v", s, err)
		}
	}

	if got, want := out.String(), "HTTP/1.1 200 OK\r\nContent-Length: 9\r\n\r\nabcdefghi"; got != want {
		t.Fatalf("body bytes lost or reordered:\n got %q\nwant %q", got, want)
	}
}

// A response whose body continues across writes, where the write that finishes
// it also carries the whole next response. bodyLeft must stop at exactly the
// declared end, and the remainder must be parsed as a new response -- not
// appended to the old body, which would corrupt or drop it. The middleware
// count is what proves the second block was parsed, since the bytes alone are
// the same either way.
func TestWrite_BodyLeftThenNextResponseSameWrite(t *testing.T) {
	hs, out := newCaptureWriter()
	hs.UseResponseMiddleware(headerMW{"Server", "Tunnel Please"})

	// Declares 5 body bytes, only 2 arrive.
	if _, err := hs.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nab")); err != nil {
		t.Fatal(err)
	}

	// Finishes the body (3 bytes) and carries the next response in one write.
	if _, err := hs.Write([]byte("cdeHTTP/1.1 201 Created\r\nContent-Length: 2\r\n\r\nzz")); err != nil {
		t.Fatal(err)
	}

	want := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\nServer: Tunnel Please\r\n\r\nabcde" +
		"HTTP/1.1 201 Created\r\nContent-Length: 2\r\nServer: Tunnel Please\r\n\r\nzz"
	if got := out.b.String(); got != want {
		t.Fatalf("body split or next response mangled:\n got %q\nwant %q", got, want)
	}
}

// An app that sends a body longer than its own Content-Length used to have the
// excess held in the buffer until a blank line arrived, so on connection close
// those bytes were never written. They are body; stream them through.
func TestWrite_BodyLongerThanContentLength(t *testing.T) {
	tests := []struct {
		name   string
		writes []string
	}{
		{
			name:   "excess in the same write",
			writes: []string{"HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nabcd"},
		},
		{
			name:   "excess arrives in a later write",
			writes: []string{"HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nabc", "d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hs, out := newCaptureWriter()

			for _, s := range tt.writes {
				if _, err := hs.Write([]byte(s)); err != nil {
					t.Fatalf("Write(%q): %v", s, err)
				}
			}

			if got, want := out.b.String(), "HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nabcd"; got != want {
				t.Fatalf("excess body bytes were dropped or re-framed:\n got %q\nwant %q", got, want)
			}
		})
	}
}

// Both responses in one Write must be parsed, not one parsed and one swallowed.
func TestWrite_TwoResponsesInOneWrite(t *testing.T) {
	got := writeAll(t, []string{
		"HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello" +
			"HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nworld",
	})

	if n := strings.Count(got, "Server: Tunnel Please\r\n"); n != 2 {
		t.Fatalf("want middleware on 2 responses, got %d in %q", n, got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("bodies mangled: %q", got)
	}
}

// A second response arriving in a later Write must be parsed too.
func TestWrite_HeaderInLaterWrite(t *testing.T) {
	got := writeAll(t, []string{
		"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok",
		"HTTP/1.1 404 Not Found\r\nContent-Length: 4\r\n\r\nnope",
	})

	if n := strings.Count(got, "Server: Tunnel Please\r\n"); n != 2 {
		t.Fatalf("want middleware on both responses, got %d in %q", n, got)
	}
	if !strings.Contains(got, "ok") || !strings.Contains(got, "nope") {
		t.Fatalf("bodies mangled: %q", got)
	}
}

// Body text that looks like a header must pass through untouched.
func TestWrite_BodyLooksLikeHeader(t *testing.T) {
	got := writeAll(t, []string{
		"HTTP/1.1 200 OK\r\nContent-Length: 30\r\n\r\nHTTP/1.1 200 OK\r\nFoo: bar..",
	})

	if !strings.HasSuffix(got, "HTTP/1.1 200 OK\r\nFoo: bar..") {
		t.Fatalf("body starting with a status line was rewritten: %q", got)
	}
}
