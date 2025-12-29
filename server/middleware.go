package server

import (
	"net"
)

type RequestMiddleware interface {
	HandleRequest(header RequestHeaderManager) error
}

type ResponseMiddleware interface {
	HandleResponse(header ResponseHeaderManager, body []byte) error
}

type TunnelFingerprint struct{}

func NewTunnelFingerprint() *TunnelFingerprint {
	return &TunnelFingerprint{}
}

func (h *TunnelFingerprint) HandleResponse(header ResponseHeaderManager, body []byte) error {
	header.Set("Server", "Tunnel Please")
	return nil
}

type ForwardedFor struct {
	addr net.Addr
}

func NewForwardedFor(addr net.Addr) *ForwardedFor {
	return &ForwardedFor{addr: addr}
}

func (ff *ForwardedFor) HandleRequest(header RequestHeaderManager) error {
	host, _, err := net.SplitHostPort(ff.addr.String())
	if err != nil {
		return err
	}
	header.Set("X-Forwarded-For", host)
	return nil
}
