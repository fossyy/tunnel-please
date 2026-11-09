package stream

import (
	"bytes"
	"strconv"
	"strings"
	"tunnel_pls/internal/http/header"
)

// Write splits the app's output on response boundaries. The app writes headers
// and bodies in whatever chunks it likes, so one Write may carry a header
// block, a body, the tail of a body, or two whole responses.
//
// A response ends where its Content-Length says. Bytes up to that end are
// parsed (header rewritten, middleware applied), and whatever follows is
// as the next response: if it opens with a status line it is one, otherwise it
// is excess body from the app and is streamed through untouched. Framing that
// cannot be measured -- chunked, or no length -- passes through as before.
func (hs *http) Write(p []byte) (int, error) {
	// Still in the body of the last response: nothing here can be a header.
	// Write it through and never search it for "\r\n\r\n".
	if hs.bodyLeft > 0 {
		n := min(hs.bodyLeft, len(p))
		if _, err := hs.writer.Write(p[:n]); err != nil {
			return 0, err
		}
		hs.bodyLeft -= n
		p = p[n:]

		if len(p) == 0 {
			return n, nil
		}
	}

	hs.buf = append(hs.buf, p...)

	written, rest, err := hs.drainBuffer()
	if err != nil {
		return 0, err
	}
	_ = written

	hs.buf = rest
	if len(rest) == 0 {
		hs.buf = nil
	}

	return len(p), nil
}

// drainBuffer handles every complete response already in hs.buf and returns
// what is left over (an incomplete header block).
func (hs *http) drainBuffer() (int, []byte, error) {
	written := 0

	for {
		// After a response has been parsed, bytes that cannot begin another
		// one are that response's excess body (the app sent more than its own
		// Content-Length said). Streaming them through is safe; holding them
		// would lose them if the connection closes before a blank line arrives.
		if hs.respHeader != nil && len(hs.buf) > 0 && !bytes.HasPrefix(hs.buf, []byte("HTTP/")) {
			if err := hs.writeRawBuffer(); err != nil {
				return written, nil, err
			}
			return written, nil, nil
		}

		headerEndIdx := bytes.Index(hs.buf, DELIMITER)
		if headerEndIdx == -1 {
			// Too big to be a header (or one that never ends): stop holding it,
			// or the app can grow this buffer until the process runs out of
			// memory.
			if len(hs.buf) > hs.maxHeader {
				if err := hs.writeRawBuffer(); err != nil {
					return written, nil, err
				}
				return written, nil, nil
			}
			return written, hs.buf, nil
		}

		headerByte, bodyByte := splitHeaderAndBody(hs.buf, headerEndIdx)
		if !isHTTPHeader(headerByte) {
			if err := hs.writeRawBuffer(); err != nil {
				return written, nil, err
			}
			return written, nil, nil
		}

		// One parse per response: the headers and the body length come from it.
		resphf, err := header.NewResponse(headerByte)
		if err != nil {
			return written, nil, err
		}

		n, known := bodyLength(resphf)
		if !known || n > len(bodyByte) {
			// The body continues past this write, or its framing is unknown:
			// write the header block and the body part in hand, and let the
			// next writes stream straight through.
			hs.bodyLeft = 0
			if known {
				hs.bodyLeft = n - len(bodyByte)
			}

			if err := hs.processHTTPResponse(resphf, bodyByte); err != nil {
				return written, nil, err
			}
			return written, nil, nil
		}

		if err := hs.processHTTPResponse(resphf, bodyByte[:n]); err != nil {
			return written, nil, err
		}
		written++

		hs.buf = bodyByte[n:]
		if len(hs.buf) == 0 {
			hs.buf = nil
			return written, nil, nil
		}
	}
}

// bodyLength is the body length the response declares. known is false when the
// framing cannot be trusted (chunked, or absent or not a number), in which case
// the rest of the stream is passed through untouched.
func bodyLength(resphf header.ResponseHeader) (n int, known bool) {
	if strings.Contains(strings.ToLower(resphf.Value("Transfer-Encoding")), "chunked") {
		return 0, false
	}

	n, err := strconv.Atoi(strings.TrimSpace(resphf.Value("Content-Length")))
	if err != nil || n < 0 {
		return 0, false
	}

	return n, true
}

// processHTTPResponse applies the middleware to an already parsed header and
// writes the rewritten header followed by bodyByte.
func (hs *http) processHTTPResponse(resphf header.ResponseHeader, bodyByte []byte) error {
	if err := hs.ApplyResponseMiddlewares(resphf, bodyByte); err != nil {
		return err
	}

	hs.respHeader = resphf
	finalHeader := resphf.Finalize()

	return hs.writeHeaderAndBody(finalHeader, bodyByte)
}

func (hs *http) writeHeaderAndBody(header, bodyByte []byte) error {
	if _, err := hs.writer.Write(header); err != nil {
		return err
	}

	if len(bodyByte) > 0 {
		if _, err := hs.writer.Write(bodyByte); err != nil {
			return err
		}
	}

	return nil
}

// writeRawBuffer passes the buffered bytes through untouched.
func (hs *http) writeRawBuffer() error {
	_, err := hs.writer.Write(hs.buf)
	hs.buf = nil
	return err
}
