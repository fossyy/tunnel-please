package header

import (
	"bytes"
	"fmt"
)

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
