package interaction

import (
	"bytes"
	"io"
)

// crlfWriter performs the newline translation the terminal driver would have
// done (ONLCR) if the program's output were a real tty.
//
// Bubble Tea v2 writes bare "\n" line breaks and relies on the tty to turn them
// into "\r\n". Our output is an ssh.Channel, not a tty, so nothing translates
// them: the cursor advances a row but keeps its column, and every line after
// the first shifts right until it wraps. Existing "\r\n" sequences are left
// alone. v1 wrote "\r\n" itself, which is why this never surfaced before.
type crlfWriter struct {
	dst io.Writer
}

func (w crlfWriter) Write(p []byte) (int, error) {
	// bubbletea's renderer can still flush after the program is torn down and
	// the channel has been dropped; discard rather than panic in that goroutine.
	if w.dst == nil {
		return len(p), nil
	}

	// The caller's count is what must be reported, not the expanded length, or
	// the renderer/io.Copy would think the write was short and repeat it.
	out := make([]byte, 0, len(p)+bytes.Count(p, []byte("\n")))
	for i, b := range p {
		if b == '\n' && (i == 0 || p[i-1] != '\r') {
			out = append(out, '\r')
		}
		out = append(out, b)
	}

	if _, err := w.dst.Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}
