package header

import (
	"bytes"
	"fmt"
)

func NewResponse(headerData []byte) (ResponseHeader, error) {
	header := &responseHeader{
		startLine: nil,
		headers:   make(map[string]string, 16),
	}

	lineEnd := bytes.Index(headerData, []byte("\r\n"))
	if lineEnd == -1 {
		return nil, fmt.Errorf("invalid response: no CRLF found in start line")
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

func (resp *responseHeader) Finalize() []byte {
	return finalize(resp.startLine, resp.headers)
}
