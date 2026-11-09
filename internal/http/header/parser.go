package header

import (
	"bytes"
	"fmt"
	"sync"
)

var ErrMalformedField = fmt.Errorf("malformed header field")

type field struct {
	name  string
	value string
}

// fields keeps header lines in wire order. A map would drop repeated
// Set-Cookie/Cookie lines and reorder the rest, which changes what the
// backend sees (RFC 7230 3.2.2).
type fields struct {
	mu    sync.RWMutex
	list  []field
	start []byte
}

func (f *fields) Set(key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i := range f.list {
		if equalFold(f.list[i].name, key) {
			f.list[i].name = key
			f.list[i].value = value
			return
		}
	}
	f.list = append(f.list, field{name: key, value: value})
}

func (f *fields) Value(key string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, h := range f.list {
		if equalFold(h.name, key) {
			return h.value
		}
	}
	return ""
}

// Count returns how many lines carry this field name, any case.
func (f *fields) Count(key string) int {
	f.mu.RLock()
	defer f.mu.RUnlock()

	n := 0
	for _, h := range f.list {
		if equalFold(h.name, key) {
			n++
		}
	}
	return n
}

func (f *fields) Remove(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	kept := f.list[:0]
	for _, h := range f.list {
		if !equalFold(h.name, key) {
			kept = append(kept, h)
		}
	}
	f.list = kept
}

func (f *fields) Finalize() []byte {
	f.mu.RLock()
	defer f.mu.RUnlock()

	size := len(f.start) + 4
	for _, h := range f.list {
		size += len(h.name) + len(h.value) + 4
	}

	buf := make([]byte, 0, size)
	buf = append(buf, f.start...)
	buf = append(buf, '\r', '\n')
	for _, h := range f.list {
		buf = append(buf, h.name...)
		buf = append(buf, ':', ' ')
		buf = append(buf, h.value...)
		buf = append(buf, '\r', '\n')
	}
	return append(buf, '\r', '\n')
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// isToken reports whether name is an RFC 7230 token, i.e. a valid field-name.
func isToken(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isTokenChar(name[i]) {
			return false
		}
	}
	return true
}

func isTokenChar(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

// isFieldValue reports whether value holds only field-vchar, SP and HTAB
// (RFC 7230 3.2), so a CR, LF or NUL cannot be smuggled through it.
func isFieldValue(value []byte) bool {
	for _, c := range value {
		switch {
		case c == '\t' || c == ' ':
		case 0x21 <= c && c <= 0x7e:
		case c >= 0x80: // obs-text
		default:
			return false
		}
	}
	return true
}

// parseLines reads CRLF header lines into dst. It is the only place header
// lines enter the package, so validation here covers every caller. The number
// of fields is bounded by the caller's header-size limit: lines without a
// colon are refused, so every accepted field costs at least two bytes.
func parseLines(dst []field, remaining []byte) ([]field, error) {
	for len(remaining) > 0 {
		lineEnd := bytes.Index(remaining, []byte("\r\n"))
		if lineEnd == -1 {
			lineEnd = len(remaining)
		}

		line := remaining[:lineEnd]
		if len(line) == 0 {
			return dst, nil
		}

		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx <= 0 {
			return nil, ErrMalformedField
		}
		if line[0] == ' ' || line[0] == '\t' || line[colonIdx-1] == ' ' {
			return nil, ErrMalformedField
		}

		name := string(line[:colonIdx])
		if !isToken(name) {
			return nil, ErrMalformedField
		}

		value := bytes.Trim(line[colonIdx+1:], " \t")
		if !isFieldValue(value) {
			return nil, ErrMalformedField
		}

		dst = append(dst, field{name: name, value: string(value)})

		if lineEnd == len(remaining) {
			return dst, nil
		}
		remaining = remaining[lineEnd+2:]
	}

	return dst, nil
}

// requestHeader and responseHeader embed the shared field list, so Value/Set/
// Remove/Finalize are the same code for both directions.
type requestHeader struct {
	method    string
	path      string
	version   string
	startLine []byte
	fields
}

type responseHeader struct {
	fields
}
