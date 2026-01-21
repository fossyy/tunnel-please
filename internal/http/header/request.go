package header

import (
	"bufio"
	"fmt"
)

func NewRequest(r interface{}) (RequestHeader, error) {
	switch v := r.(type) {
	case []byte:
		return parseHeadersFromBytes(v)
	case *bufio.Reader:
		return parseHeadersFromReader(v)
	default:
		return nil, fmt.Errorf("unsupported type: %T", r)
	}
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
