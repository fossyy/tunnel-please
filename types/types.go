package types

type Status string

const (
	INITIALIZING Status = "INITIALIZING"
	RUNNING      Status = "RUNNING"
	SETUP        Status = "SETUP"
)

type TunnelType string

const (
	HTTP TunnelType = "HTTP"
	TCP  TunnelType = "TCP"
)
