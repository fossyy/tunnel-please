package types

type Status int

const (
	INITIALIZING Status = iota
	RUNNING
)

type Mode int

const (
	INTERACTIVE Mode = iota
	HEADLESS
)

type TunnelType int

const (
	UNKNOWN TunnelType = iota
	HTTP
	TCP
)

type SessionKey struct {
	Id   string
	Type TunnelType
}

var BadGatewayResponse = []byte("HTTP/1.1 502 Bad Gateway\r\n" +
	"Content-Length: 11\r\n" +
	"Content-Type: text/plain\r\n\r\n" +
	"Bad Gateway")
