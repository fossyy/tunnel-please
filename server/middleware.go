package server

import (
	"net"
)

type RequestMiddleware interface {
	HandleRequest(header RequestHeader) error
}

type ResponseMiddleware interface {
	HandleResponse(header ResponseHeader, body []byte) error
}

type TunnelFingerprint struct{}

func NewTunnelFingerprint() *TunnelFingerprint {
	return &TunnelFingerprint{}
}

func (h *TunnelFingerprint) HandleResponse(header ResponseHeader, body []byte) error {
	header.Set("Server", "Tunnel Please")
	return nil
}

type ForwardedFor struct {
	addr net.Addr
}

func NewForwardedFor(addr net.Addr) *ForwardedFor {
	return &ForwardedFor{addr: addr}
}

func (ff *ForwardedFor) HandleRequest(header RequestHeader) error {
	host, _, err := net.SplitHostPort(ff.addr.String())
	if err != nil {
		return err
	}
	header.Set("X-Forwarded-For", host)
	return nil
}
