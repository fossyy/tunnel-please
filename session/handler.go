package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	portUtil "tunnel_pls/internal/port"
	"tunnel_pls/internal/random"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

var blockedReservedPorts = []uint16{1080, 1433, 1521, 1900, 2049, 3306, 3389, 5432, 5900, 6379, 8080, 8443, 9000, 9200, 27017}

func (s *SSHSession) HandleGlobalRequest(GlobalRequest <-chan *ssh.Request) {
	for req := range GlobalRequest {
		switch req.Type {
		case "shell", "pty-req":
			err := req.Reply(true, nil)
			if err != nil {
				log.Println("Failed to reply to request:", err)
				return
			}
		case "window-change":
			p := req.Payload
			if len(p) < 16 {
				log.Println("invalid window-change payload")
				err := req.Reply(false, nil)
				if err != nil {
					log.Println("Failed to reply to request:", err)
					return
				}
				return
			}
			cols := binary.BigEndian.Uint32(p[0:4])
			rows := binary.BigEndian.Uint32(p[4:8])

			s.interaction.SetWH(int(cols), int(rows))

			err := req.Reply(true, nil)
			if err != nil {
				log.Println("Failed to reply to request:", err)
				return
			}
		default:
			log.Println("Unknown request type:", req.Type)
			err := req.Reply(false, nil)
			if err != nil {
				log.Println("Failed to reply to request:", err)
				return
			}
		}
	}
}

func (s *SSHSession) HandleTCPIPForward(req *ssh.Request) {
	log.Println("Port forwarding request detected")

	reader := bytes.NewReader(req.Payload)

	addr, err := readSSHString(reader)
	if err != nil {
		log.Println("Failed to read address from payload:", err)
		err := req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		err = s.lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return
	}

	var rawPortToBind uint32
	if err := binary.Read(reader, binary.BigEndian, &rawPortToBind); err != nil {
		log.Println("Failed to read port from payload:", err)
		err := req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		err = s.lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return
	}

	if rawPortToBind > 65535 {
		log.Printf("Port %d is larger than allowed port of 65535", rawPortToBind)
		err := req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		err = s.lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return
	}

	portToBind := uint16(rawPortToBind)
	if isBlockedPort(portToBind) {
		log.Printf("Port %d is blocked or restricted", portToBind)
		err := req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		err = s.lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return
	}

	if portToBind == 80 || portToBind == 443 {
		s.HandleHTTPForward(req, portToBind)
		return
	}
	if portToBind == 0 {
		unassign, success := portUtil.Default.GetUnassignedPort()
		portToBind = unassign
		if !success {
			log.Println("No available port")
			err := req.Reply(false, nil)
			if err != nil {
				log.Println("Failed to reply to request:", err)
				return
			}
			err = s.lifecycle.Close()
			if err != nil {
				log.Printf("failed to close session: %v", err)
			}
			return
		}
	} else if isUse, isExist := portUtil.Default.GetPortStatus(portToBind); isExist && isUse {
		log.Printf("Port %d is already in use or restricted", portToBind)
		err := req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		err = s.lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return
	}
	err = portUtil.Default.SetPortStatus(portToBind, true)
	if err != nil {
		log.Println("Failed to set port status:", err)
		return
	}

	s.HandleTCPForward(req, addr, portToBind)
}

func (s *SSHSession) HandleHTTPForward(req *ssh.Request, portToBind uint16) {
	slug := random.GenerateRandomString(20)

	if !s.registry.Register(slug, s) {
		log.Printf("Failed to register client with slug: %s", slug)
		err := req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
		}
		return
	}

	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.BigEndian, uint32(portToBind))
	if err != nil {
		log.Println("Failed to write port to buffer:", err)
		s.registry.Remove(slug)
		err = req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
		}
		return
	}
	log.Printf("HTTP forwarding approved on port: %d", portToBind)

	err = req.Reply(true, buf.Bytes())
	if err != nil {
		log.Println("Failed to reply to request:", err)
		s.registry.Remove(slug)
		err = req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
		}
		return
	}

	s.forwarder.SetType(types.HTTP)
	s.forwarder.SetForwardedPort(portToBind)
	s.slugManager.Set(slug)
	s.lifecycle.SetStatus(types.RUNNING)
	s.interaction.Start()
}

func (s *SSHSession) HandleTCPForward(req *ssh.Request, addr string, portToBind uint16) {
	log.Printf("Requested forwarding on %s:%d", addr, portToBind)
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", portToBind))
	if err != nil {
		log.Printf("Port %d is already in use or restricted", portToBind)
		if setErr := portUtil.Default.SetPortStatus(portToBind, false); setErr != nil {
			log.Printf("Failed to reset port status: %v", setErr)
		}
		err = req.Reply(false, nil)
		if err != nil {
			log.Println("Failed to reply to request:", err)
			return
		}
		err = s.lifecycle.Close()
		if err != nil {
			log.Printf("failed to close session: %v", err)
		}
		return
	}

	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, uint32(portToBind))
	if err != nil {
		log.Println("Failed to write port to buffer:", err)
		if setErr := portUtil.Default.SetPortStatus(portToBind, false); setErr != nil {
			log.Printf("Failed to reset port status: %v", setErr)
		}
		err = listener.Close()
		if err != nil {
			log.Printf("Failed to close listener: %s", err)
			return
		}
		return
	}

	log.Printf("TCP forwarding approved on port: %d", portToBind)
	err = req.Reply(true, buf.Bytes())
	if err != nil {
		log.Println("Failed to reply to request:", err)
		if setErr := portUtil.Default.SetPortStatus(portToBind, false); setErr != nil {
			log.Printf("Failed to reset port status: %v", setErr)
		}
		err = listener.Close()
		if err != nil {
			log.Printf("Failed to close listener: %s", err)
			return
		}
		return
	}

	s.forwarder.SetType(types.TCP)
	s.forwarder.SetListener(listener)
	s.forwarder.SetForwardedPort(portToBind)
	s.lifecycle.SetStatus(types.RUNNING)
	go s.forwarder.AcceptTCPConnections()
	s.interaction.Start()
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

func isBlockedPort(port uint16) bool {
	if port == 80 || port == 443 {
		return false
	}
	if port < 1024 && port != 0 {
		return true
	}
	for _, p := range blockedReservedPorts {
		if p == port {
			return true
		}
	}
	return false
}
