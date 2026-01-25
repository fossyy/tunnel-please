package header

import (
	"bytes"
	"fmt"
)

func NewRequest(headerData []byte) (RequestHeader, error) {
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

func (req *requestHeader) Method() string {
	return req.method
}

func (req *requestHeader) Path() string {
	return req.path
}

func (req *requestHeader) Version() string {
	return req.version
}

func (req *requestHeader) Finalize() []byte {
	return finalize(req.startLine, req.headers)
}
