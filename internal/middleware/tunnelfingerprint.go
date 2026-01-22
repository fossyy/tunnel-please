package middleware

import (
	"tunnel_pls/internal/http/header"
)

type TunnelFingerprint struct{}

func NewTunnelFingerprint() *TunnelFingerprint {
	return &TunnelFingerprint{}
}

func (h *TunnelFingerprint) HandleResponse(header header.ResponseHeader, body []byte) error {
	header.Set("Server", "Tunnel Please")
	return nil
}
