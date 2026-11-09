package stream

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"tunnel_pls/internal/http/header"
)

// reqState is what Read has taken from the source but not handed out yet.
type reqState struct {
	// in holds bytes read from the source and not processed yet.
	in []byte
	// out holds bytes that did not fit the caller's buffer.
	out []byte
	// err is the source error, reported after in and out are drained.
	err error
}

// Read adds request headers (X-Forwarded-For) to header blocks it can see whole.
//
// Read never returns more than len(p): io.Copy panics on that, and a full
// caller buffer is exactly the case where the added header overflows it. What
// does not fit waits in hs.req and goes out on the next call.
func (hs *http) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for {
		if len(hs.req.out) > 0 {
			n := copy(p, hs.req.out)
			hs.req.out = hs.req.out[n:]
			return n, nil
		}

		if len(hs.req.in) > 0 {
			n := hs.emit(p)
			if len(hs.req.out) == 0 && hs.req.err != nil {
				err := hs.req.err
				hs.req.err = nil
				return n, err
			}
			return n, nil
		}

		if hs.req.err != nil {
			err := hs.req.err
			hs.req.err = nil
			return 0, err
		}

		if err := hs.fill(p); err != nil {
			return 0, err
		}
	}
}

// fill reads once. If that leaves a request header still arriving, it keeps
// reading until the blank line arrives, the header limit is reached, or the
// bytes stop looking like a request. Bounded, so a body that happens to start
// with "GET / HTTP/1.1" cannot hold the connection open.
func (hs *http) fill(p []byte) error {
	for {
		start := len(hs.req.in)
		hs.req.in = slices.Grow(hs.req.in, len(p))
		hs.req.in = hs.req.in[:start+len(p)]
		n, err := hs.reader.Read(hs.req.in[start:])
		hs.req.in = hs.req.in[:start+n]

		if err != nil {
			hs.req.err = err
			return nil
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		if bytes.Index(hs.req.in, DELIMITER) != -1 {
			return nil
		}
		if len(hs.req.in) >= hs.maxHeader || !couldStartRequest(hs.req.in) {
			return nil
		}
	}
}

var requestMethods = []string{
	"GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "TRACE", "CONNECT",
}

// couldStartRequest says whether in is consistent with the beginning of a
// request line, so the bytes so far are a header still arriving rather than a
// body. It accepts a partly read start line, which TCP may split anywhere.
func couldStartRequest(in []byte) bool {
	if end := bytes.IndexByte(in, '\n'); end >= 0 {
		// The first line is complete: it must be a valid request line, or these
		// bytes are a body (or a malformed request) and must not be held.
		return requestLine.Match(bytes.TrimSuffix(in[:end], []byte("\r")))
	}

	// A trailing CR is the half of the CRLF that has not arrived yet.
	line := bytes.TrimSuffix(in, []byte("\r"))
	sp := bytes.IndexByte(line, ' ')
	if sp < 0 {
		for _, m := range requestMethods {
			if strings.HasPrefix(m, string(line)) {
				return true
			}
		}
		return false
	}
	if !slices.Contains(requestMethods, string(line[:sp])) {
		return false
	}

	rest := line[sp+1:]
	if len(rest) == 0 {
		// The path has not started yet.
		return true
	}
	sp = bytes.LastIndexByte(rest, ' ')
	if sp < 0 {
		// Still the path.
		return !bytes.ContainsAny(rest, "\r\n\x00")
	}
	if len(rest[:sp]) == 0 || bytes.ContainsAny(rest[:sp], "\r\n\x00") {
		return false
	}
	version := string(rest[sp+1:])
	for _, v := range []string{"HTTP/1.0", "HTTP/1.1", "HTTP/2.0"} {
		if strings.HasPrefix(v, version) {
			return true
		}
	}
	return false
}

// emit hands out what can go now and keeps the rest.
func (hs *http) emit(p []byte) int {
	in := hs.req.in
	idx := bytes.Index(in, DELIMITER)
	if idx == -1 {
		return hs.handOut(p, in)
	}

	headerByte, bodyByte := splitHeaderAndBody(in, idx)
	if !isHTTPHeader(headerByte) {
		// Not a header block (a body that merely starts with a request-looking
		// line, or a malformed one): pass it through byte for byte. The backend
		// is the HTTP parser of record here; this side only adds headers to real
		// requests, and there is no channel to answer with a 400.
		return hs.handOut(p, in)
	}

	final, err := hs.rewriteRequest(headerByte, bodyByte)
	if err != nil {
		// Hand the bytes through untouched; the error follows once they are out.
		hs.req.err = err
		return hs.handOut(p, in)
	}
	return hs.handOut(p, final)
}

// handOut copies block into p, keeps the remainder for the next Read, and
// drops what was consumed from the input.
func (hs *http) handOut(p, block []byte) int {
	n := copy(p, block)
	hs.req.in = hs.req.in[:0]
	if n < len(block) {
		hs.req.out = append(hs.req.out[:0], block[n:]...)
	}
	return n
}

// rewriteRequest returns the header block with request middleware applied,
// followed by the body. It builds a fresh buffer so a longer added header
// cannot overwrite the start of a body that follows it.
func (hs *http) rewriteRequest(headerByte, bodyByte []byte) ([]byte, error) {
	reqhf, err := header.NewRequest(headerByte)
	if err != nil {
		return nil, err
	}
	if err = hs.ApplyRequestMiddlewares(reqhf); err != nil {
		return nil, err
	}
	hs.reqHeader = reqhf

	final := reqhf.Finalize()
	out := make([]byte, 0, len(final)+len(bodyByte))
	out = append(out, final...)
	out = append(out, bodyByte...)
	return out, nil
}
