package forwarder

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
	"tunnel_pls/internal/config"
	"tunnel_pls/session/slug"
	"tunnel_pls/types"

	"golang.org/x/crypto/ssh"
)

type Forwarder interface {
	SetType(tunnelType types.TunnelType)
	SetForwardedPort(port uint16)
	SetListener(listener net.Listener)
	Listener() net.Listener
	TunnelType() types.TunnelType
	ForwardedPort() uint16
	HandleConnection(dst io.ReadWriter, src ssh.Channel)
	CreateForwardedTCPIPPayload(origin net.Addr) []byte
	OpenForwardedChannel(payload []byte) (ssh.Channel, <-chan *ssh.Request, error)
	WriteBadGatewayResponse(dst io.Writer)
	Close() error
}
type forwarder struct {
	listener      net.Listener
	tunnelType    types.TunnelType
	forwardedPort uint16
	slug          slug.Slug
	conn          ssh.Conn
	bufferPool    sync.Pool
}

func New(config config.Config, slug slug.Slug, conn ssh.Conn) Forwarder {
	return &forwarder{
		listener:      nil,
		tunnelType:    types.TunnelTypeUNKNOWN,
		forwardedPort: 0,
		slug:          slug,
		conn:          conn,
		bufferPool: sync.Pool{
			New: func() interface{} {
				bufSize := config.BufferSize()
				return make([]byte, bufSize)
			},
		},
	}
}

func (f *forwarder) copyWithBuffer(dst io.Writer, src io.Reader) (written int64, err error) {
	buf := f.bufferPool.Get().([]byte)
	defer f.bufferPool.Put(buf)
	return io.CopyBuffer(dst, src, buf)
}

func (f *forwarder) OpenForwardedChannel(payload []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	type channelResult struct {
		channel ssh.Channel
		reqs    <-chan *ssh.Request
		err     error
	}
	resultChan := make(chan channelResult, 1)

	go func() {
		channel, reqs, err := f.conn.OpenChannel("forwarded-tcpip", payload)
		select {
		case resultChan <- channelResult{channel, reqs, err}:
		default:
			if channel != nil {
				err = channel.Close()
				if err != nil {
					log.Printf("Failed to close unused channel: %v", err)
					return
				}
				go ssh.DiscardRequests(reqs)
			}
		}
	}()

	select {
	case result := <-resultChan:
		return result.channel, result.reqs, result.err
	case <-time.After(5 * time.Second):
		return nil, nil, errors.New("timeout opening forwarded-tcpip channel")
	}
}

func closeWriter(w io.Writer) error {
	if cw, ok := w.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	if closer, ok := w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (f *forwarder) copyAndClose(dst io.Writer, src io.Reader, direction string) error {
	var errs []error
	_, err := f.copyWithBuffer(dst, src)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		errs = append(errs, fmt.Errorf("copy error (%s): %w", direction, err))
	}

	if err = closeWriter(dst); err != nil && !errors.Is(err, io.EOF) {
		errs = append(errs, fmt.Errorf("close stream error (%s): %w", direction, err))
	}
	return errors.Join(errs...)
}

func (f *forwarder) HandleConnection(dst io.ReadWriter, src ssh.Channel) {
	defer func() {
		_, err := io.Copy(io.Discard, src)
		if err != nil {
			log.Printf("Failed to discard connection: %v", err)
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := f.copyAndClose(dst, src, "src to dst")
		if err != nil {
			log.Println("Error during copy: ", err)
			return
		}
	}()

	go func() {
		defer wg.Done()
		err := f.copyAndClose(src, dst, "dst to src")
		if err != nil {
			log.Println("Error during copy: ", err)
			return
		}
	}()

	wg.Wait()
}

func (f *forwarder) SetType(tunnelType types.TunnelType) {
	f.tunnelType = tunnelType
}

func (f *forwarder) TunnelType() types.TunnelType {
	return f.tunnelType
}

func (f *forwarder) ForwardedPort() uint16 {
	return f.forwardedPort
}

func (f *forwarder) SetForwardedPort(port uint16) {
	f.forwardedPort = port
}

func (f *forwarder) SetListener(listener net.Listener) {
	f.listener = listener
}

func (f *forwarder) Listener() net.Listener {
	return f.listener
}

func (f *forwarder) WriteBadGatewayResponse(dst io.Writer) {
	_, err := dst.Write(types.BadGatewayResponse)
	if err != nil {
		log.Printf("failed to write Bad Gateway response: %v", err)
		return
	}
}

func (f *forwarder) Close() error {
	if f.Listener() != nil {
		return f.listener.Close()
	}
	return nil
}

func (f *forwarder) CreateForwardedTCPIPPayload(origin net.Addr) []byte {
	host, portStr, _ := net.SplitHostPort(origin.String())
	port, _ := strconv.Atoi(portStr)

	forwardPayload := struct {
		DestAddr   string
		DestPort   uint32
		OriginAddr string
		OriginPort uint32
	}{
		DestAddr:   "localhost",
		DestPort:   uint32(f.ForwardedPort()),
		OriginAddr: host,
		OriginPort: uint32(port),
	}

	return ssh.Marshal(forwardPayload)
}
