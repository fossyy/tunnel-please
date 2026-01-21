package stream

import (
	"io"
	"log"
	"net"
	"regexp"
	"tunnel_pls/internal/http/header"
	"tunnel_pls/internal/middleware"
)

var DELIMITER = []byte{0x0D, 0x0A, 0x0D, 0x0A}
var requestLine = regexp.MustCompile(`^(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH|TRACE|CONNECT) \S+ HTTP/\d\.\d$`)
var responseLine = regexp.MustCompile(`^HTTP/\d\.\d \d{3} .+`)

type HTTP interface {
	io.ReadWriteCloser
	CloseWrite() error
	RemoteAddr() net.Addr
	UseResponseMiddleware(mw middleware.ResponseMiddleware)
	UseRequestMiddleware(mw middleware.RequestMiddleware)
	SetRequestHeader(header header.RequestHeader)
	RequestMiddlewares() []middleware.RequestMiddleware
	ResponseMiddlewares() []middleware.ResponseMiddleware
	ApplyResponseMiddlewares(resphf header.ResponseHeader, body []byte) error
	ApplyRequestMiddlewares(reqhf header.RequestHeader) error
}

type http struct {
	remoteAddr net.Addr
	writer     io.Writer
	reader     io.Reader
	headerBuf  []byte
	buf        []byte
	respHeader header.ResponseHeader
	reqHeader  header.RequestHeader
	respMW     []middleware.ResponseMiddleware
	reqMW      []middleware.RequestMiddleware
}

func New(writer io.Writer, reader io.Reader, remoteAddr net.Addr) HTTP {
	return &http{
		remoteAddr: remoteAddr,
		writer:     writer,
		reader:     reader,
		buf:        make([]byte, 0, 4096),
	}
}

func (hs *http) RemoteAddr() net.Addr {
	return hs.remoteAddr
}

func (hs *http) UseResponseMiddleware(mw middleware.ResponseMiddleware) {
	hs.respMW = append(hs.respMW, mw)
}

func (hs *http) UseRequestMiddleware(mw middleware.RequestMiddleware) {
	hs.reqMW = append(hs.reqMW, mw)
}

func (hs *http) SetRequestHeader(header header.RequestHeader) {
	hs.reqHeader = header
}

func (hs *http) RequestMiddlewares() []middleware.RequestMiddleware {
	return hs.reqMW
}

func (hs *http) ResponseMiddlewares() []middleware.ResponseMiddleware {
	return hs.respMW
}

func (hs *http) Close() error {
	return hs.writer.(io.Closer).Close()
}

func (hs *http) CloseWrite() error {
	if closer, ok := hs.writer.(interface{ CloseWrite() error }); ok {
		return closer.CloseWrite()
	}
	return hs.Close()
}

func (hs *http) ApplyRequestMiddlewares(reqhf header.RequestHeader) error {
	for _, m := range hs.RequestMiddlewares() {
		if err := m.HandleRequest(reqhf); err != nil {
			log.Printf("Error when applying request middleware: %v", err)
			return err
		}
	}
	return nil
}

func (hs *http) ApplyResponseMiddlewares(resphf header.ResponseHeader, bodyByte []byte) error {
	for _, m := range hs.ResponseMiddlewares() {
		if err := m.HandleResponse(resphf, bodyByte); err != nil {
			log.Printf("Cannot apply middleware: %s\n", err)
			return err
		}
	}
	return nil
}
