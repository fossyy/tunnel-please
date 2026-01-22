package transport

import (
	"net"
)

type Transport interface {
	Listen() (net.Listener, error)
	Serve(listener net.Listener) error
}
