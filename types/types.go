package types

import "time"

type SessionStatus int

const (
	SessionStatusINITIALIZING SessionStatus = iota
	SessionStatusRUNNING
)

type InteractiveMode int

const (
	InteractiveModeINTERACTIVE InteractiveMode = iota + 1
	InteractiveModeHEADLESS
)

type TunnelType int

const (
	TunnelTypeUNKNOWN TunnelType = iota
	TunnelTypeHTTP
	TunnelTypeTCP
)

type ServerMode int

const (
	ServerModeSTANDALONE = iota + 1
	ServerModeNODE
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
