package server

import (
	"bufio"
	"bytes"
	"fmt"
)

type ResponseHeader interface {
	Value(key string) string
	Set(key string, value string)
	Remove(key string)
	Finalize() []byte
}

type responseHeader struct {
	startLine []byte
	headers   map[string]string
}

type RequestHeader interface {
	Value(key string) string
	Set(key string, value string)
	Remove(key string)
	Finalize() []byte
	GetMethod() string
	GetPath() string
	GetVersion() string
}
type requestHeader struct {
	method    string
	path      string
	version   string
	startLine []byte
	headers   map[string]string
}

func NewRequestHeader(r interface{}) (RequestHeader, error) {
	switch v := r.(type) {
	case []byte:
		return parseHeadersFromBytes(v)
	case *bufio.Reader:
		return parseHeadersFromReader(v)
	default:
		return nil, fmt.Errorf("unsupported type: %T", r)
	}
}

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

func NewResponseHeader(headerData []byte) (ResponseHeader, error) {
	header := &responseHeader{
		startLine: nil,
		headers:   make(map[string]string, 16),
	}

	lineEnd := bytes.Index(headerData, []byte("\r\n"))
	if lineEnd == -1 {
		return nil, fmt.Errorf("invalid request: no CRLF found in start line")
	}

	header.startLine = headerData[:lineEnd]
	remaining := headerData[lineEnd+2:]
	setRemainingHeaders(remaining, header)

	return header, nil
}

func (resp *responseHeader) Value(key string) string {
	return resp.headers[key]
}

func (resp *responseHeader) Set(key string, value string) {
	resp.headers[key] = value
}

func (resp *responseHeader) Remove(key string) {
	delete(resp.headers, key)
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

func (resp *responseHeader) Finalize() []byte {
	return finalize(resp.startLine, resp.headers)
}

func (req *requestHeader) Value(key string) string {
	val, ok := req.headers[key]
	if !ok {
		return ""
	}
	return val
}

func (req *requestHeader) Set(key string, value string) {
	req.headers[key] = value
}

func (req *requestHeader) Remove(key string) {
	delete(req.headers, key)
}

func (req *requestHeader) GetMethod() string {
	return req.method
}

func (req *requestHeader) GetPath() string {
	return req.path
}

func (req *requestHeader) GetVersion() string {
	return req.version
}

func (req *requestHeader) Finalize() []byte {
	return finalize(req.startLine, req.headers)
}
