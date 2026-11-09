package header

import (
	"bytes"
	"fmt"
)

func NewRequest(headerData []byte) (RequestHeader, error) {
	lineEnd := bytes.Index(headerData, []byte("\r\n"))
	if lineEnd == -1 {
		return nil, fmt.Errorf("invalid request: no CRLF found in start line")
	}

	startLine := headerData[:lineEnd]
	method, path, version, err := parseStartLine(startLine)
	if err != nil {
		return nil, err
	}

	list, err := parseLines(nil, headerData[lineEnd+2:])
	if err != nil {
		return nil, err
	}

	return &requestHeader{
		method:    method,
		path:      path,
		version:   version,
		startLine: startLine,
		fields:    fields{list: list, start: startLine},
	}, nil
}

func (req *requestHeader) Method() string  { return req.method }
func (req *requestHeader) Path() string    { return req.path }
func (req *requestHeader) Version() string { return req.version }

func (req *requestHeader) Finalize() []byte {
	return req.fields.Finalize()
}
