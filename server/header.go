package server

import (
	"bufio"
	"bytes"
	"fmt"
)

type HeaderManager interface {
	Get(key string) []byte
	Set(key string, value []byte)
	Remove(key string)
	Finalize() []byte
}

type ResponseHeaderManager interface {
	Get(key string) string
	Set(key string, value string)
	Remove(key string)
	Finalize() []byte
}

type RequestHeaderManager interface {
	Get(key string) string
	Set(key string, value string)
	Remove(key string)
	Finalize() []byte
	GetMethod() string
	GetPath() string
	GetVersion() string
}

type responseHeaderFactory struct {
	startLine []byte
	headers   map[string]string
}

type requestHeaderFactory struct {
	method    string
	path      string
	version   string
	startLine []byte
	headers   map[string]string
}

func NewRequestHeaderFactory(r interface{}) (RequestHeaderManager, error) {
	switch v := r.(type) {
	case []byte:
		return parseHeadersFromBytes(v)
	case *bufio.Reader:
		return parseHeadersFromReader(v)
	default:
		return nil, fmt.Errorf("unsupported type: %T", r)
	}
}

func parseHeadersFromBytes(headerData []byte) (RequestHeaderManager, error) {
	header := &requestHeaderFactory{
		headers: make(map[string]string, 16),
	}

	lineEnd := bytes.IndexByte(headerData, '\n')
	if lineEnd == -1 {
		return nil, fmt.Errorf("invalid request: no newline found")
	}

	startLine := bytes.TrimRight(headerData[:lineEnd], "\r\n")
	header.startLine = make([]byte, len(startLine))
	copy(header.startLine, startLine)

	parts := bytes.Split(startLine, []byte{' '})
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid request line")
	}

	header.method = string(parts[0])
	header.path = string(parts[1])
	header.version = string(parts[2])

	remaining := headerData[lineEnd+1:]

	for len(remaining) > 0 {
		lineEnd = bytes.IndexByte(remaining, '\n')
		if lineEnd == -1 {
			lineEnd = len(remaining)
		}

		line := bytes.TrimRight(remaining[:lineEnd], "\r\n")

		if len(line) == 0 {
			break
		}

		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx != -1 {
			key := bytes.TrimSpace(line[:colonIdx])
			value := bytes.TrimSpace(line[colonIdx+1:])
			header.headers[string(key)] = string(value)
		}

		if lineEnd == len(remaining) {
			break
		}
		remaining = remaining[lineEnd+1:]
	}

	return header, nil
}

func parseHeadersFromReader(br *bufio.Reader) (RequestHeaderManager, error) {
	header := &requestHeaderFactory{
		headers: make(map[string]string, 16),
	}

	startLineBytes, err := br.ReadSlice('\n')
	if err != nil {
		if err == bufio.ErrBufferFull {
			var startLine string
			startLine, err = br.ReadString('\n')
			if err != nil {
				return nil, err
			}
			startLineBytes = []byte(startLine)
		} else {
			return nil, err
		}
	}

	startLineBytes = bytes.TrimRight(startLineBytes, "\r\n")
	header.startLine = make([]byte, len(startLineBytes))
	copy(header.startLine, startLineBytes)

	parts := bytes.Split(startLineBytes, []byte{' '})
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid request line")
	}

	header.method = string(parts[0])
	header.path = string(parts[1])
	header.version = string(parts[2])

	for {
		lineBytes, err := br.ReadSlice('\n')
		if err != nil {
			if err == bufio.ErrBufferFull {
				var line string
				line, err = br.ReadString('\n')
				if err != nil {
					return nil, err
				}
				lineBytes = []byte(line)
			} else {
				return nil, err
			}
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

func NewResponseHeaderFactory(startLine []byte) ResponseHeaderManager {
	header := &responseHeaderFactory{
		startLine: nil,
		headers:   make(map[string]string),
	}
	lines := bytes.Split(startLine, []byte("\r\n"))
	if len(lines) == 0 {
		return header
	}
	header.startLine = lines[0]
	for _, h := range lines[1:] {
		if len(h) == 0 {
			continue
		}

		parts := bytes.SplitN(h, []byte(":"), 2)
		if len(parts) < 2 {
			continue
		}

		key := parts[0]
		val := bytes.TrimSpace(parts[1])
		header.headers[string(key)] = string(val)
	}
	return header
}

func (resp *responseHeaderFactory) Get(key string) string {
	return resp.headers[key]
}

func (resp *responseHeaderFactory) Set(key string, value string) {
	resp.headers[key] = value
}

func (resp *responseHeaderFactory) Remove(key string) {
	delete(resp.headers, key)
}

func (resp *responseHeaderFactory) Finalize() []byte {
	var buf bytes.Buffer

	buf.Write(resp.startLine)
	buf.WriteString("\r\n")

	for key, val := range resp.headers {
		buf.WriteString(key)
		buf.WriteString(": ")
		buf.WriteString(val)
		buf.WriteString("\r\n")
	}

	buf.WriteString("\r\n")
	return buf.Bytes()
}

func (req *requestHeaderFactory) Get(key string) string {
	val, ok := req.headers[key]
	if !ok {
		return ""
	}
	return val
}

func (req *requestHeaderFactory) Set(key string, value string) {
	req.headers[key] = value
}

func (req *requestHeaderFactory) Remove(key string) {
	delete(req.headers, key)
}

func (req *requestHeaderFactory) GetMethod() string {
	return req.method
}

func (req *requestHeaderFactory) GetPath() string {
	return req.path
}

func (req *requestHeaderFactory) GetVersion() string {
	return req.version
}

func (req *requestHeaderFactory) Finalize() []byte {
	var buf bytes.Buffer

	buf.Write(req.startLine)
	buf.WriteString("\r\n")

	for key, val := range req.headers {
		buf.WriteString(key)
		buf.WriteString(": ")
		buf.WriteString(val)
		buf.WriteString("\r\n")
	}

	buf.WriteString("\r\n")
	return buf.Bytes()
}
