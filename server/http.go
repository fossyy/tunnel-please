package server

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"tunnel_pls/session"
)

func NewHTTPServer() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:80"))
	if err != nil {
		return errors.New("Error listening: " + err.Error())
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				log.Printf("Error accepting connection: %v", err)
				continue
			}

			go Handler(conn)
		}
	}()
	return nil
}

func Handler(conn net.Conn) {
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		fmt.Println("Error reading request:", err)
		return
	}
	host := strings.Split(request.Host, ".")

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

	request.Header.Set("Connection", "keep-alive")
	request.Header.Set("Keep-Alive", "timeout=60")

	go sshSession.HandleForwardedConnectionHTTP(conn, sshSession.Connection, request)
}
