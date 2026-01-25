package header

import (
	"bytes"
	"fmt"
)

func setRemainingHeaders(remaining []byte, header interface {
	Set(key string, value string)
}) {
	for len(remaining) > 0 {
		lineEnd := bytes.Index(remaining, []byte("\r\n"))
		if lineEnd == -1 {
			lineEnd = len(remaining)
		}

		line := remaining[:lineEnd]

		if len(line) == 0 {
			break
		}

		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx != -1 {
			key := bytes.TrimSpace(line[:colonIdx])
			value := bytes.TrimSpace(line[colonIdx+1:])
			header.Set(string(key), string(value))
		}

		if lineEnd == len(remaining) {
			break
		}

		remaining = remaining[lineEnd+2:]
	}
}

func parseStartLine(startLine []byte) (method, path, version string, err error) {
	firstSpace := bytes.IndexByte(startLine, ' ')
	if firstSpace == -1 {
		return "", "", "", fmt.Errorf("invalid start line: missing method")
	}

	secondSpace := bytes.IndexByte(startLine[firstSpace+1:], ' ')
	if secondSpace == -1 {
		return "", "", "", fmt.Errorf("invalid start line: missing version")
	}
	secondSpace += firstSpace + 1

	method = string(startLine[:firstSpace])
	path = string(startLine[firstSpace+1 : secondSpace])
	version = string(startLine[secondSpace+1:])

	return method, path, version, nil
}

func finalize(startLine []byte, headers map[string]string) []byte {
	size := len(startLine) + 2
	for key, val := range headers {
		size += len(key) + 2 + len(val) + 2
	}
	size += 2

	buf := make([]byte, 0, size)
	buf = append(buf, startLine...)
	buf = append(buf, '\r', '\n')

	for key, val := range headers {
		buf = append(buf, key...)
		buf = append(buf, ':', ' ')
		buf = append(buf, val...)
		buf = append(buf, '\r', '\n')
	}

	buf = append(buf, '\r', '\n')
	return buf
}
