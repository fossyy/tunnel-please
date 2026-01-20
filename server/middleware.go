package server

import (
	"net"
	"tunnel_pls/internal/httpheader"
)

type RequestMiddleware interface {
	HandleRequest(header httpheader.RequestHeader) error
}

type ResponseMiddleware interface {
	HandleResponse(header httpheader.ResponseHeader, body []byte) error
}

type TunnelFingerprint struct{}

func NewTunnelFingerprint() *TunnelFingerprint {
	return &TunnelFingerprint{}
}

func (h *TunnelFingerprint) HandleResponse(header httpheader.ResponseHeader, body []byte) error {
	header.Set("Server", "Tunnel Please")
	return nil
}

type ForwardedFor struct {
	addr net.Addr
}

func NewForwardedFor(addr net.Addr) *ForwardedFor {
	return &ForwardedFor{addr: addr}
}

func (ff *ForwardedFor) HandleRequest(header httpheader.RequestHeader) error {
	host, _, err := net.SplitHostPort(ff.addr.String())
	if err != nil {
		return err
	}
	header.Set("X-Forwarded-For", host)
	return nil
}
