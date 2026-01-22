package transport

import (
	"net"
)

type Transport interface {
	Listen() (net.Listener, error)
	Serve(listener net.Listener) error
}

type HTTP interface {
	Handler(conn net.Conn, isTLS bool)
}
