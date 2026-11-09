package stream

import "bytes"

func splitHeaderAndBody(data []byte, delimiterIdx int) ([]byte, []byte) {
	headerByte := data[:delimiterIdx+len(DELIMITER)]
	body := data[delimiterIdx+len(DELIMITER):]
	return headerByte, body
}

// isHTTPHeader reports whether buf is shaped like an HTTP header block: a
// request or status line, then fields that the header parser would also
// accept. It is a shape test, not the parser.
func isHTTPHeader(buf []byte) bool {
	lines := bytes.Split(buf, []byte("\r\n"))

	startLine := string(lines[0])
	if !requestLine.MatchString(startLine) && !responseLine.MatchString(startLine) {
		return false
	}

	for _, line := range lines[1:] {
		if len(line) == 0 {
			break
		}
		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx <= 0 {
			return false
		}
		// The same rules header.parseLines enforces: no whitespace before the
		// colon and no bare CR/LF inside a value.
		if line[0] == ' ' || line[0] == '\t' || line[colonIdx-1] == ' ' {
			return false
		}
		if bytes.IndexAny(line[colonIdx+1:], "\r\n") != -1 {
			return false
		}
	}
	return true
}
