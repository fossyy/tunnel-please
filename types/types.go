package types

import "time"

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

type Detail struct {
	ForwardingType string    `json:"forwarding_type,omitempty"`
	Slug           string    `json:"slug,omitempty"`
	UserID         string    `json:"user_id,omitempty"`
	Active         bool      `json:"active,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
}

var BadGatewayResponse = []byte("HTTP/1.1 502 Bad Gateway\r\n" +
	"Content-Length: 11\r\n" +
	"Content-Type: text/plain\r\n\r\n" +
	"Bad Gateway")
