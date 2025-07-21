package server

import (
	"bufio"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"strings"
	"tunnel_pls/session"
	"tunnel_pls/utils"
)

func NewHTTPSServer() error {
	cert, err := tls.LoadX509KeyPair(utils.Getenv("cert_loc"), utils.Getenv("key_loc"))
	if err != nil {
		return err
	}

	config := &tls.Config{Certificates: []tls.Certificate{cert}}
	ln, err := tls.Listen("tcp", ":443", config)
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					log.Println("https server closed")
				}
				log.Printf("Error accepting connection: %v", err)
				continue
			}

			go HandlerTLS(conn)
		}
	}()
	return nil
}

func HandlerTLS(conn net.Conn) {
	reader := bufio.NewReader(conn)
	headers, err := peekUntilHeaders(reader, 8192)
	if err != nil {
		log.Println("Failed to peek headers:", err)
		return
	}

	host := strings.Split(parseHostFromHeader(headers), ".")
	if len(host) < 1 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		conn.Close()
		return
	}

	if len(host) < 1 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		conn.Close()
		return
	}
	slug := host[0]

	sshSession, ok := session.Clients[slug]
	if !ok {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		conn.Close()
		return
	}

	sshSession.HandleForwardedConnection(session.UserConnection{
		Reader: reader,
		Writer: conn,
	}, sshSession.Connection)
	return
}
