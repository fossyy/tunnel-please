package middleware

import (
	"net"
	"tunnel_pls/internal/http/header"
)

type ForwardedFor struct {
	addr net.Addr
}

func NewForwardedFor(addr net.Addr) *ForwardedFor {
	return &ForwardedFor{addr: addr}
}

func (ff *ForwardedFor) HandleRequest(header header.RequestHeader) error {
	host, _, err := net.SplitHostPort(ff.addr.String())
	if err != nil {
		return err
	}
	// Replace, don't keep: the parser keeps repeated fields, so a client-sent
	// X-Forwarded-For would otherwise survive next to ours.
	header.Remove("X-Forwarded-For")
	header.Set("X-Forwarded-For", host)
	return nil
}
