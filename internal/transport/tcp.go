package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

type tcp struct {
	port      uint16
	forwarder Forwarder
}

type Forwarder interface {
	OpenForwardedChannel(ctx context.Context, origin net.Addr) (ssh.Channel, <-chan *ssh.Request, error)
	HandleConnection(dst io.ReadWriter, src ssh.Channel)
}

func NewTCPServer(port uint16, forwarder Forwarder) Transport {
	return &tcp{
		port:      port,
		forwarder: forwarder,
	}
}

func (tt *tcp) Listen() (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", tt.port))
}

func (tt *tcp) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}
		go tt.handleTcp(conn)
	}
}

func (tt *tcp) handleTcp(conn net.Conn) {
	defer func() {
		err := conn.Close()
		if err != nil {
			log.Printf("Failed to close connection: %v", err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	channel, reqs, err := tt.forwarder.OpenForwardedChannel(ctx, conn.RemoteAddr())
	if err != nil {
		log.Printf("Failed to open forwarded-tcpip channel: %v", err)
		return
	}

	go ssh.DiscardRequests(reqs)
	tt.forwarder.HandleConnection(conn, channel)
}
