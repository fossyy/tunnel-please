package header

import (
	"bytes"
	"fmt"
)

func NewResponse(headerData []byte) (ResponseHeader, error) {
	lineEnd := bytes.Index(headerData, []byte("\r\n"))
	if lineEnd == -1 {
		return nil, fmt.Errorf("invalid response: no CRLF found in start line")
	}

	startLine := headerData[:lineEnd]
	list, err := parseLines(nil, headerData[lineEnd+2:])
	if err != nil {
		return nil, err
	}

	return &responseHeader{fields: fields{list: list, start: startLine}}, nil
}
