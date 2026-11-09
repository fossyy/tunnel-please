package header

type ResponseHeader interface {
	Value(key string) string
	Set(key string, value string)
	Remove(key string)
	Finalize() []byte
}

type RequestHeader interface {
	Count(key string) int
	Value(key string) string
	Set(key string, value string)
	Remove(key string)
	Finalize() []byte
	Method() string
	Path() string
	Version() string
}
