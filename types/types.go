package types

type Status string

const (
	INITIALIZING Status = "INITIALIZING"
	RUNNING      Status = "RUNNING"
	SETUP        Status = "SETUP"
)

type TunnelType string

const (
	UNKNOWN TunnelType = "UNKNOWN"
	HTTP    TunnelType = "HTTP"
	TCP     TunnelType = "TCP"
)

type SessionKey struct {
	Id   string
	Type TunnelType
}

var BadGatewayResponse = []byte("HTTP/1.1 502 Bad Gateway\r\n" +
	"Content-Length: 11\r\n" +
	"Content-Type: text/plain\r\n\r\n" +
	"Bad Gateway")
