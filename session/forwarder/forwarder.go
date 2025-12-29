package forwarder

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"
	"tunnel_pls/utils"

	"golang.org/x/crypto/ssh"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		bufSize := utils.GetBufferSize()
		return make([]byte, bufSize)
	},
}

func copyWithBuffer(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)
	return io.CopyBuffer(dst, src, buf)
}

type Forwarder struct {
	listener      net.Listener
	tunnelType    types.TunnelType
	forwardedPort uint16
	slugManager   slug.Manager
	lifecycle     Lifecycle
}

func NewForwarder(slugManager slug.Manager) *Forwarder {
	return &Forwarder{
		listener:      nil,
		tunnelType:    "",
		forwardedPort: 0,
		slugManager:   slugManager,
		lifecycle:     nil,
	}
}

type Lifecycle interface {
	GetConnection() ssh.Conn
}

type ForwardingController interface {
	AcceptTCPConnections()
	SetType(tunnelType types.TunnelType)
	GetTunnelType() types.TunnelType
	GetForwardedPort() uint16
	SetForwardedPort(port uint16)
	SetListener(listener net.Listener)
	GetListener() net.Listener
	Close() error
	HandleConnection(dst io.ReadWriter, src ssh.Channel, remoteAddr net.Addr)
	SetLifecycle(lifecycle Lifecycle)
	CreateForwardedTCPIPPayload(origin net.Addr) []byte
	WriteBadGatewayResponse(dst io.Writer)
}

func (f *Forwarder) SetLifecycle(lifecycle Lifecycle) {
	f.lifecycle = lifecycle
}

func (f *Forwarder) AcceptTCPConnections() {
	for {
		conn, err := f.GetListener().Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("Error accepting connection: %v", err)
			continue
		}

		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			log.Printf("Failed to set connection deadline: %v", err)
			if closeErr := conn.Close(); closeErr != nil {
				log.Printf("Failed to close connection: %v", closeErr)
			}
			continue
		}

		payload := f.CreateForwardedTCPIPPayload(conn.RemoteAddr())

		type channelResult struct {
			channel ssh.Channel
			reqs    <-chan *ssh.Request
			err     error
		}
		resultChan := make(chan channelResult, 1)

		go func() {
			channel, reqs, err := f.lifecycle.GetConnection().OpenChannel("forwarded-tcpip", payload)
			resultChan <- channelResult{channel, reqs, err}
		}()

		select {
		case result := <-resultChan:
			if result.err != nil {
				log.Printf("Failed to open forwarded-tcpip channel: %v", result.err)
				if closeErr := conn.Close(); closeErr != nil {
					log.Printf("Failed to close connection: %v", closeErr)
				}
				continue
			}

			if err := conn.SetDeadline(time.Time{}); err != nil {
				log.Printf("Failed to clear connection deadline: %v", err)
			}

			go ssh.DiscardRequests(result.reqs)
			go f.HandleConnection(conn, result.channel, conn.RemoteAddr())

		case <-time.After(5 * time.Second):
			log.Printf("Timeout opening forwarded-tcpip channel")
			if closeErr := conn.Close(); closeErr != nil {
				log.Printf("Failed to close connection: %v", closeErr)
			}
		}
	}
}

func (f *Forwarder) HandleConnection(dst io.ReadWriter, src ssh.Channel, remoteAddr net.Addr) {
	defer func() {
		_, err := io.Copy(io.Discard, src)
		if err != nil {
			log.Printf("Failed to discard connection: %v", err)
		}

		err = src.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			log.Printf("Error closing source channel: %v", err)
		}

		if closer, ok := dst.(io.Closer); ok {
			err = closer.Close()
			if err != nil && !errors.Is(err, io.EOF) {
				log.Printf("Error closing destination connection: %v", err)
			}
		}
	}()

	log.Printf("Handling new forwarded connection from %s", remoteAddr)

	done := make(chan struct{}, 2)

	go func() {
		_, err := copyWithBuffer(src, dst)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			log.Printf("Error copying from conn.Reader to channel: %v", err)
		}
		done <- struct{}{}
	}()

	go func() {
		_, err := copyWithBuffer(dst, src)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			log.Printf("Error copying from channel to conn.Writer: %v", err)
		}
		done <- struct{}{}
	}()

	<-done
}

func (f *Forwarder) SetType(tunnelType types.TunnelType) {
	f.tunnelType = tunnelType
}

func (f *Forwarder) GetTunnelType() types.TunnelType {
	return f.tunnelType
}

func (f *Forwarder) GetForwardedPort() uint16 {
	return f.forwardedPort
}

func (f *Forwarder) SetForwardedPort(port uint16) {
	f.forwardedPort = port
}

func (f *Forwarder) SetListener(listener net.Listener) {
	f.listener = listener
}

func (f *Forwarder) GetListener() net.Listener {
	return f.listener
}

func (f *Forwarder) WriteBadGatewayResponse(dst io.Writer) {
	_, err := dst.Write(types.BadGatewayResponse)
	if err != nil {
		log.Printf("failed to write Bad Gateway response: %v", err)
		return
	}
}

func (f *Forwarder) Close() error {
	if f.GetListener() != nil {
		return f.listener.Close()
	}
	return nil
}

func (f *Forwarder) CreateForwardedTCPIPPayload(origin net.Addr) []byte {
	var buf bytes.Buffer

	host, originPort := parseAddr(origin.String())

	writeSSHString(&buf, "localhost")
	err := binary.Write(&buf, binary.BigEndian, uint32(f.GetForwardedPort()))
	if err != nil {
		log.Printf("Failed to write string to buffer: %v", err)
		return nil
	}

	writeSSHString(&buf, host)
	err = binary.Write(&buf, binary.BigEndian, uint32(originPort))
	if err != nil {
		log.Printf("Failed to write string to buffer: %v", err)
		return nil
	}

	return buf.Bytes()
}

func parseAddr(addr string) (string, uint16) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("Failed to parse origin address: %s from address %s", err.Error(), addr)
		return "0.0.0.0", uint16(0)
	}
	port, _ := strconv.Atoi(portStr)
	return host, uint16(port)
}

func writeSSHString(buffer *bytes.Buffer, str string) {
	err := binary.Write(buffer, binary.BigEndian, uint32(len(str)))
	if err != nil {
		log.Printf("Failed to write string to buffer: %v", err)
		return
	}
	buffer.WriteString(str)
}
