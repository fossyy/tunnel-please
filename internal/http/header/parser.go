package header

import (
	"bufio"
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

func parseHeadersFromBytes(headerData []byte) (RequestHeader, error) {
	header := &requestHeader{
		headers: make(map[string]string, 16),
	}

	lineEnd := bytes.Index(headerData, []byte("\r\n"))
	if lineEnd == -1 {
		return nil, fmt.Errorf("invalid request: no CRLF found in start line")
	}

	startLine := headerData[:lineEnd]
	header.startLine = startLine
	var err error
	header.method, header.path, header.version, err = parseStartLine(startLine)
	if err != nil {
		return nil, err
	}

	remaining := headerData[lineEnd+2:]

	setRemainingHeaders(remaining, header)

	return header, nil
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

func parseHeadersFromReader(br *bufio.Reader) (RequestHeader, error) {
	header := &requestHeader{
		headers: make(map[string]string, 16),
	}

	startLineBytes, err := br.ReadSlice('\n')
	if err != nil {
		return nil, err
	}

	startLineBytes = bytes.TrimRight(startLineBytes, "\r\n")
	header.startLine = make([]byte, len(startLineBytes))
	copy(header.startLine, startLineBytes)

	header.method, header.path, header.version, err = parseStartLine(header.startLine)
	if err != nil {
		return nil, err
	}

	for {
		lineBytes, err := br.ReadSlice('\n')
		if err != nil {
			return nil, err
		}

		lineBytes = bytes.TrimRight(lineBytes, "\r\n")

		if len(lineBytes) == 0 {
			break
		}

		colonIdx := bytes.IndexByte(lineBytes, ':')
		if colonIdx == -1 {
			continue
		}

		key := bytes.TrimSpace(lineBytes[:colonIdx])
		value := bytes.TrimSpace(lineBytes[colonIdx+1:])

		header.headers[string(key)] = string(value)
	}

	return header, nil
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
