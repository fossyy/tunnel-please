package httpheader

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
