package session

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"
	"tunnel_pls/utils"
)

type UserConnection struct {
	Reader io.Reader
	Writer net.Conn
}

func (s *Session) handleGlobalRequest() {
	for {
		select {
		case req := <-s.GlobalRequest:
			if req == nil {
				return
			}
			if req.Type == "tcpip-forward" {
				s.handleTCPIPForward(req)
				continue
			} else {
				req.Reply(false, nil)
			}
		case <-s.Done:
			break
		}
	}
}

func (s *Session) handleTCPIPForward(req *ssh.Request) {
	log.Println("Port forwarding request detected")

	reader := bytes.NewReader(req.Payload)

	addr, err := readSSHString(reader)
	if err != nil {
		log.Println("Failed to read address from payload:", err)
		req.Reply(false, nil)
		return
	}

	var portToBind uint32

	if err := binary.Read(reader, binary.BigEndian, &portToBind); err != nil {
		log.Println("Failed to read port from payload:", err)
		req.Reply(false, nil)
		return
	}

	if portToBind == 80 || portToBind == 443 {
		s.TunnelType = HTTP
		s.ForwardedPort = uint16(portToBind)
		var slug string
		for {
			slug = utils.GenerateRandomString(32)
			if _, ok := Clients[slug]; ok {
				return
			}
			break
		}
		Clients[slug] = s
		s.Slug = slug
		buf := new(bytes.Buffer)
		binary.Write(buf, binary.BigEndian, uint32(80))
		log.Printf("Forwarding approved on port: %d", 80)
		if utils.Getenv("tls_enabled") == "true" {
			s.ConnChannels[0].Write([]byte(fmt.Sprintf("Forwarding your traffic to https://%s.%s \r\n", slug, utils.Getenv("domain"))))
		} else {
			s.ConnChannels[0].Write([]byte(fmt.Sprintf("Forwarding your traffic to http://%s.%s \r\n", slug, utils.Getenv("domain"))))
		}
		req.Reply(true, buf.Bytes())

	} else {
		s.TunnelType = TCP
		log.Printf("Requested forwarding on %s:%d", addr, portToBind)

		listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", portToBind))
		if err != nil {
			log.Printf("Failed to bind to port %d: %v", portToBind, err)
			req.Reply(false, nil)
			return
		}
		s.Listener = listener
		s.ConnChannels[0].Write([]byte(fmt.Sprintf("Forwarding your traffic to %s:%d \r\n", utils.Getenv("domain"), portToBind)))
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

				go s.HandleForwardedConnection(UserConnection{
					Reader: nil,
					Writer: conn,
				}, s.Connection, portToBind)
			}
		}()

		buf := new(bytes.Buffer)
		binary.Write(buf, binary.BigEndian, uint32(portToBind))

		log.Printf("Forwarding approved on port: %d", portToBind)
		req.Reply(true, buf.Bytes())
	}

}

func (s *Session) HandleSessionChannel(newChannel ssh.NewChannel) {
	connection, requests, err := newChannel.Accept()
	s.ConnChannels = append(s.ConnChannels, connection)
	if err != nil {
		log.Printf("Could not accept channel: %s", err)
		return
	}
	go func() {
		var commandBuffer bytes.Buffer
		buf := make([]byte, 1)
		for {
			n, err := connection.Read(buf)
			if n > 0 {
				char := buf[0]
				connection.Write(buf[:n])
				if char == 8 || char == 127 {
					if commandBuffer.Len() > 0 {
						commandBuffer.Truncate(commandBuffer.Len() - 1)
						connection.Write([]byte("\b \b"))
					}
					continue
				}

				if char == '/' {
					commandBuffer.Reset()
					commandBuffer.WriteByte(char)
					continue
				}

				if commandBuffer.Len() > 0 {
					if char == 13 {
						command := commandBuffer.String()
						fmt.Println("User entered command:", command, "<>")

						if command == "/bye" {
							fmt.Println("Closing connection...")
							s.Close()
							break
						} else if command == "/help" {
							connection.Write([]byte("Available commands: /bye, /help, /clear"))

						} else if command == "/clear" {
							connection.Write([]byte("\033[H\033[2J"))
						} else {
							connection.Write([]byte("Unknown command"))
						}

						commandBuffer.Reset()
						continue
					}

					commandBuffer.WriteByte(char)
					continue
				}
			}

			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading from client: %s", err)
				}
				break
			}
		}
	}()

	go func() {
		asciiArt := []string{
			` _______                     _   _____  _      `,
			`|__   __|                   | | |  __ \| |    `,
			`   | |_   _ _ __  _ __   ___| | | |__) | |___ `,
			`   | | | | | '_ \| '_ \ / _ \ | |  ___/| / __|`,
			`   | | |_| | | | | | | |  __/ | | |    | \__ \`,
			`   |_|\__,_|_| |_|_| |_|\___|_| |_|    |_|___/`,
			``,
			`       "Tunnel Pls" - Project by Bagas`,
			`           https://fossy.my.id`,
			``,
			`        Welcome to Tunnel! Available commands:`,
			`        - '/bye'   : Exit the tunnel`,
			`        - '/help'  : Show this help message`,
			`        - '/clear' : Clear the current line`,
		}

		connection.Write([]byte("\033[H\033[2J"))

		for _, line := range asciiArt {
			connection.Write([]byte("\r\n" + line))
		}

		connection.Write([]byte("\r\n\r\n"))
		go s.handleGlobalRequest()

		for req := range requests {
			switch req.Type {
			case "shell", "pty-req", "window-change":
				req.Reply(true, nil)
			default:
				fmt.Println("Unknown request type of : ", req.Type)
				req.Reply(false, nil)
			}
		}
	}()
}

func (s *Session) HandleForwardedConnection(conn UserConnection, sshConn *ssh.ServerConn, port uint32) {
	defer conn.Writer.Close()
	log.Printf("Handling new forwarded connection from %s", conn.Writer.RemoteAddr())
	host, originPort := ParseAddr(conn.Writer.RemoteAddr().String())
	payload := createForwardedTCPIPPayload(host, originPort, port)
	channel, reqs, err := sshConn.OpenChannel("forwarded-tcpip", payload)
	go func() {
		for req := range reqs {
			req.Reply(false, nil)
		}
	}()
	if err != nil {
		log.Printf("Failed to open forwarded-tcpip channel: %v", err)
		return
	}
	defer channel.Close()
	if conn.Reader == nil {
		conn.Reader = bufio.NewReader(conn.Writer)
	}
	go io.Copy(channel, conn.Reader)
	reader := bufio.NewReader(channel)
	_, err = reader.Peek(1)
	if err == io.EOF {
		fmt.Println("error babi")
	}
	io.Copy(conn.Writer, reader)
}

func (s *Session) HandleForwardedConnectionHTTP(conn net.Conn, sshConn *ssh.ServerConn, request *http.Request) {
	defer conn.Close()
	fmt.Println(request)
	channelPayload := createForwardedTCPIPPayload(request.Host, 80, 80)
	channel, reqs, err := sshConn.OpenChannel("forwarded-tcpip", channelPayload)
	go func() {
		for req := range reqs {
			req.Reply(false, nil)
		}
	}()

	var requestBuffer bytes.Buffer
	if err := request.Write(&requestBuffer); err != nil {
		fmt.Println("Error serializing request:", err)
		channel.Close()
		conn.Close()
		return
	}
	channel.Write(requestBuffer.Bytes())

	reader := bufio.NewReader(channel)
	_, err = reader.Peek(1)
	if err == io.EOF {
		io.Copy(conn, bytes.NewReader([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 11\r\nContent-Type: text/plain\r\n\r\nBad Gateway")))
		s.ConnChannels[0].Write([]byte("Could not forward request to the tunnel addr\r\n"))
		return
	} else {
		s.ConnChannels[0].Write([]byte(fmt.Sprintf("\033[32m %s -- [%s] \"%s %s %s\" 	\r\n \033[0m", request.Host, time.Now().Format("02/Jan/2006 15:04:05"), request.Method, request.RequestURI, request.Proto)))
		io.Copy(conn, reader)
	}
}

//TODO: Implement HTTPS forwarding
//func (s *Session) GetForwardedConnectionTLS(host string, sshConn *ssh.ServerConn, payload []byte, originPort, port uint32, path, method, proto string) (*http.Response, error) {
//	channelPayload := createForwardedTCPIPPayload(host, originPort, port)
//	channel, reqs, err := sshConn.OpenChannel("forwarded-tcpip", channelPayload)
//	if err != nil {
//		return nil, err
//	}
//	defer channel.Close()
//
//	initalPayload := bytes.NewReader(payload)
//	io.Copy(channel, initalPayload)
//
//	go func() {
//		for req := range reqs {
//			req.Reply(false, nil)
//		}
//	}()
//
//	reader := bufio.NewReader(channel)
//	_, err = reader.Peek(1)
//	if err == io.EOF {
//		return nil, err
//	} else {
//		s.ConnChannels[0].Write([]byte(fmt.Sprintf("\033[32m %s -- [%s] \"%s %s %s\" 	\r\n \033[0m", host, time.Now().Format("02/Jan/2006 15:04:05"), method, path, proto)))
//		response, err := http.ReadResponse(reader, nil)
//		if err != nil {
//			return nil, err
//		}
//		return response, err
//	}
//}

func writeSSHString(buffer *bytes.Buffer, str string) {
	binary.Write(buffer, binary.BigEndian, uint32(len(str)))
	buffer.WriteString(str)
}

func ParseAddr(addr string) (string, uint32) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("Failed to parse origin address: %s from address %s", err.Error(), addr)
		return "0.0.0.0", uint32(0)
	}
	port, _ := strconv.Atoi(portStr)
	return host, uint32(port)
}

func createForwardedTCPIPPayload(host string, originPort, port uint32) []byte {
	var buf bytes.Buffer

	writeSSHString(&buf, "localhost")
	binary.Write(&buf, binary.BigEndian, uint32(port))
	writeSSHString(&buf, host)
	binary.Write(&buf, binary.BigEndian, uint32(originPort))

	return buf.Bytes()
}

func readSSHString(reader *bytes.Reader) (string, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return "", err
	}
	strBytes := make([]byte, length)
	if _, err := reader.Read(strBytes); err != nil {
		return "", err
	}
	return string(strBytes), nil
}
