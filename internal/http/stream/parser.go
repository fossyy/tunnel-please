package stream

import "bytes"

func splitHeaderAndBody(data []byte, delimiterIdx int) ([]byte, []byte) {
	headerByte := data[:delimiterIdx+len(DELIMITER)]
	body := data[delimiterIdx+len(DELIMITER):]
	return headerByte, body
}

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
	}
	return true
}
