package server

import (
	"bytes"
	"io"
	"log"
	"net"
	"regexp"
	"tunnel_pls/internal/httpheader"
)

type HTTPWriter interface {
	io.ReadWriteCloser
	CloseWrite() error
	RemoteAddr() net.Addr
	UseResponseMiddleware(mw ResponseMiddleware)
	UseRequestMiddleware(mw RequestMiddleware)
	SetRequestHeader(header httpheader.RequestHeader)
	RequestMiddlewares() []RequestMiddleware
	ResponseMiddlewares() []ResponseMiddleware
	ApplyResponseMiddlewares(resphf httpheader.ResponseHeader, body []byte) error
	ApplyRequestMiddlewares(reqhf httpheader.RequestHeader) error
}

type httpWriter struct {
	remoteAddr net.Addr
	writer     io.Writer
	reader     io.Reader
	headerBuf  []byte
	buf        []byte
	respHeader httpheader.ResponseHeader
	reqHeader  httpheader.RequestHeader
	respMW     []ResponseMiddleware
	reqMW      []RequestMiddleware
}

var DELIMITER = []byte{0x0D, 0x0A, 0x0D, 0x0A}
var requestLine = regexp.MustCompile(`^(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH|TRACE|CONNECT) \S+ HTTP/\d\.\d$`)
var responseLine = regexp.MustCompile(`^HTTP/\d\.\d \d{3} .+`)

func (hw *httpWriter) RemoteAddr() net.Addr {
	return hw.remoteAddr
}

func (hw *httpWriter) UseResponseMiddleware(mw ResponseMiddleware) {
	hw.respMW = append(hw.respMW, mw)
}

func (hw *httpWriter) UseRequestMiddleware(mw RequestMiddleware) {
	hw.reqMW = append(hw.reqMW, mw)
}

func (hw *httpWriter) SetRequestHeader(header httpheader.RequestHeader) {
	hw.reqHeader = header
}

func (hw *httpWriter) RequestMiddlewares() []RequestMiddleware {
	return hw.reqMW
}

func (hw *httpWriter) ResponseMiddlewares() []ResponseMiddleware {
	return hw.respMW
}
func (hw *httpWriter) Close() error {
	return hw.writer.(io.Closer).Close()
}

func (hw *httpWriter) CloseWrite() error {
	if closer, ok := hw.writer.(interface{ CloseWrite() error }); ok {
		return closer.CloseWrite()
	}
	return hw.Close()
}

func (hw *httpWriter) Read(p []byte) (int, error) {
	tmp := make([]byte, len(p))
	read, err := hw.reader.Read(tmp)
	if read == 0 && err != nil {
		return 0, err
	}

	tmp = tmp[:read]

	headerEndIdx := bytes.Index(tmp, DELIMITER)
	if headerEndIdx == -1 {
		return hw.handleNoDelimiter(p, tmp, err)
	}

	header, body := hw.splitHeaderAndBody(tmp, headerEndIdx)

	if !isHTTPHeader(header) {
		copy(p, tmp)
		return read, nil
	}

	return hw.processHTTPRequest(p, header, body)
}

func (hw *httpWriter) handleNoDelimiter(p, tmp []byte, err error) (int, error) {
	copy(p, tmp)
	return len(tmp), err
}

func (hw *httpWriter) splitHeaderAndBody(data []byte, delimiterIdx int) ([]byte, []byte) {
	header := data[:delimiterIdx+len(DELIMITER)]
	body := data[delimiterIdx+len(DELIMITER):]
	return header, body
}

func (hw *httpWriter) processHTTPRequest(p, header, body []byte) (int, error) {
	reqhf, err := httpheader.NewRequestHeader(header)
	if err != nil {
		return 0, err
	}

	if err = hw.ApplyRequestMiddlewares(reqhf); err != nil {
		return 0, err
	}

	hw.reqHeader = reqhf
	combined := append(reqhf.Finalize(), body...)
	return copy(p, combined), nil
}

func (hw *httpWriter) ApplyRequestMiddlewares(reqhf httpheader.RequestHeader) error {
	for _, m := range hw.RequestMiddlewares() {
		if err := m.HandleRequest(reqhf); err != nil {
			log.Printf("Error when applying request middleware: %v", err)
			return err
		}
	}
	return nil
}

func (hw *httpWriter) Write(p []byte) (int, error) {
	if hw.shouldBypassBuffering(p) {
		hw.respHeader = nil
	}

	if hw.respHeader != nil {
		return hw.writer.Write(p)
	}

	hw.buf = append(hw.buf, p...)

	headerEndIdx := bytes.Index(hw.buf, DELIMITER)
	if headerEndIdx == -1 {
		return len(p), nil
	}

	return hw.processBufferedResponse(p, headerEndIdx)
}

func (hw *httpWriter) shouldBypassBuffering(p []byte) bool {
	return hw.respHeader != nil && len(hw.buf) == 0 && len(p) >= 5 && string(p[0:5]) == "HTTP/"
}

func (hw *httpWriter) processBufferedResponse(p []byte, delimiterIdx int) (int, error) {
	header, body := hw.splitHeaderAndBody(hw.buf, delimiterIdx)

	if !isHTTPHeader(header) {
		return hw.writeRawBuffer()
	}

	if err := hw.processHTTPResponse(header, body); err != nil {
		return 0, err
	}

	hw.buf = nil
	return len(p), nil
}

func (hw *httpWriter) writeRawBuffer() (int, error) {
	_, err := hw.writer.Write(hw.buf)
	length := len(hw.buf)
	hw.buf = nil
	if err != nil {
		return 0, err
	}
	return length, nil
}

func (hw *httpWriter) processHTTPResponse(header, body []byte) error {
	resphf, err := httpheader.NewResponseHeader(header)
	if err != nil {
		return err
	}

	if err = hw.ApplyResponseMiddlewares(resphf, body); err != nil {
		return err
	}

	hw.respHeader = resphf
	finalHeader := resphf.Finalize()

	if err = hw.writeHeaderAndBody(finalHeader, body); err != nil {
		return err
	}

	return nil
}

func (hw *httpWriter) ApplyResponseMiddlewares(resphf httpheader.ResponseHeader, body []byte) error {
	for _, m := range hw.ResponseMiddlewares() {
		if err := m.HandleResponse(resphf, body); err != nil {
			log.Printf("Cannot apply middleware: %s\n", err)
			return err
		}
	}
	return nil
}

func (hw *httpWriter) writeHeaderAndBody(header, body []byte) error {
	if _, err := hw.writer.Write(header); err != nil {
		return err
	}

	if len(body) > 0 {
		if _, err := hw.writer.Write(body); err != nil {
			return err
		}
	}

	return nil
}

func NewHTTPWriter(writer io.Writer, reader io.Reader, remoteAddr net.Addr) HTTPWriter {
	return &httpWriter{
		remoteAddr: remoteAddr,
		writer:     writer,
		reader:     reader,
		buf:        make([]byte, 0, 4096),
	}
}

func isHTTPHeader(buf []byte) bool {
	lines := bytes.Split(buf, []byte("\r\n"))

	startLine := string(lines[0])
	if !requestLine.MatchString(startLine) && !responseLine.MatchString(startLine) {
		return false
	}

	for _, line := range lines[1:] {
		if len(line) == 0 {
			break
		}
		colonIdx := bytes.IndexByte(line, ':')
		if colonIdx <= 0 {
			return false
		}
	}
	return true
}
