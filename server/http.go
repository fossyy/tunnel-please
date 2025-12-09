package server

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strings"
	"tunnel_pls/session"
	"tunnel_pls/utils"
)

type Interaction interface {
	SendMessage(message string)
}
type CustomWriter struct {
	RemoteAddr  net.Addr
	writer      io.Writer
	reader      io.Reader
	headerBuf   []byte
	buf         []byte
	respHeader  *ResponseHeaderFactory
	reqHeader   *RequestHeaderFactory
	interaction Interaction
	respMW      []ResponseMiddleware
	reqStartMW  []RequestMiddleware
	reqEndMW    []RequestMiddleware
}

func (cw *CustomWriter) SetInteraction(interaction Interaction) {
	cw.interaction = interaction
}

func (cw *CustomWriter) Read(p []byte) (int, error) {
	tmp := make([]byte, len(p))
	read, err := cw.reader.Read(tmp)
	if read == 0 && err != nil {
		return 0, err
	}

	tmp = tmp[:read]

	idx := bytes.Index(tmp, DELIMITER)
	if idx == -1 {
		copy(p, tmp)
		if err != nil {
			return read, err
		}
		return read, nil
	}

	header := tmp[:idx+len(DELIMITER)]
	body := tmp[idx+len(DELIMITER):]

	if !isHTTPHeader(header) {
		copy(p, tmp)
		return read, nil
	}

	for _, m := range cw.reqEndMW {
		err = m.HandleRequest(cw.reqHeader)
		if err != nil {
			log.Printf("Error when applying request middleware: %v", err)
			return 0, err
		}
	}

	headerReader := bufio.NewReader(bytes.NewReader(header))
	reqhf, err := NewRequestHeaderFactory(headerReader)
	if err != nil {
		return 0, err
	}

	for _, m := range cw.reqStartMW {
		if mwErr := m.HandleRequest(reqhf); mwErr != nil {
			log.Printf("Error when applying request middleware: %v", mwErr)
			return 0, mwErr
		}
	}

	cw.reqHeader = reqhf
	finalHeader := reqhf.Finalize()

	combined := append(finalHeader, body...)

	n := copy(p, combined)

	return n, nil
}

func NewCustomWriter(writer io.Writer, reader io.Reader, remoteAddr net.Addr) *CustomWriter {
	return &CustomWriter{
		RemoteAddr:  remoteAddr,
		writer:      writer,
		reader:      reader,
		buf:         make([]byte, 0, 4096),
		interaction: nil,
	}
}

var DELIMITER = []byte{0x0D, 0x0A, 0x0D, 0x0A}
var requestLine = regexp.MustCompile(`^(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH|TRACE|CONNECT) \S+ HTTP/\d\.\d$`)
var responseLine = regexp.MustCompile(`^HTTP/\d\.\d \d{3} .+`)

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

func (cw *CustomWriter) Write(p []byte) (int, error) {
	if cw.respHeader != nil && len(cw.buf) == 0 && len(p) >= 5 && string(p[0:5]) == "HTTP/" {
		cw.respHeader = nil
	}

	if cw.respHeader != nil {
		n, err := cw.writer.Write(p)
		if err != nil {
			return n, err
		}
		return n, nil
	}

	cw.buf = append(cw.buf, p...)

	idx := bytes.Index(cw.buf, DELIMITER)
	if idx == -1 {
		return len(p), nil
	}

	header := cw.buf[:idx+len(DELIMITER)]
	body := cw.buf[idx+len(DELIMITER):]

	if !isHTTPHeader(header) {
		_, err := cw.writer.Write(cw.buf)
		cw.buf = nil
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}

	resphf := NewResponseHeaderFactory(header)
	for _, m := range cw.respMW {
		err := m.HandleResponse(resphf, body)
		if err != nil {
			log.Printf("Cannot apply middleware: %s\n", err)
			return 0, err
		}
	}
	header = resphf.Finalize()
	cw.respHeader = resphf

	_, err := cw.writer.Write(header)
	if err != nil {
		return 0, err
	}
	if len(body) > 0 {
		_, err = cw.writer.Write(body)
		if err != nil {
			return 0, err
		}
	}
	cw.buf = nil
	return len(p), nil
}

func (cw *CustomWriter) AddInteraction(interaction Interaction) {
	cw.interaction = interaction
}

var redirectTLS = false

func NewHTTPServer() error {
	listener, err := net.Listen("tcp", ":80")
	if err != nil {
		return errors.New("Error listening: " + err.Error())
	}
	if utils.Getenv("tls_enabled") == "true" && utils.Getenv("tls_redirect") == "true" {
		redirectTLS = true
	}
	go func() {
		for {
			var conn net.Conn
			conn, err = listener.Accept()
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
	defer func() {
		err := conn.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("Error closing connection: %v", err)
			return
		}
		return
	}()

	dstReader := bufio.NewReader(conn)
	reqhf, err := NewRequestHeaderFactory(dstReader)
	if err != nil {
		log.Printf("Error creating request header: %v", err)
		return
	}

	host := strings.Split(reqhf.Get("Host"), ".")
	if len(host) < 1 {
		_, err := conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		if err != nil {
			log.Println("Failed to write 400 Bad Request:", err)
			return
		}
		return
	}

	slug := host[0]

	if redirectTLS {
		_, err = conn.Write([]byte("HTTP/1.1 301 Moved Permanently\r\n" +
			fmt.Sprintf("Location: https://%s.%s/\r\n", slug, utils.Getenv("domain")) +
			"Content-Length: 0\r\n" +
			"Connection: close\r\n" +
			"\r\n"))
		if err != nil {
			log.Println("Failed to write 301 Moved Permanently:", err)
			return
		}
		return
	}

	if slug == "ping" {
		_, err = conn.Write([]byte(
			"HTTP/1.1 200 OK\r\n" +
				"Content-Length: 0\r\n" +
				"Connection: close\r\n" +
				"Access-Control-Allow-Origin: *\r\n" +
				"Access-Control-Allow-Methods: GET, HEAD, OPTIONS\r\n" +
				"Access-Control-Allow-Headers: *\r\n" +
				"\r\n",
		))
		if err != nil {
			log.Println("Failed to write 200 OK:", err)
			return
		}
		return
	}

	sshSession, ok := session.Clients[slug]
	if !ok {
		_, err = conn.Write([]byte("HTTP/1.1 301 Moved Permanently\r\n" +
			fmt.Sprintf("Location: https://tunnl.live/tunnel-not-found?slug=%s\r\n", slug) +
			"Content-Length: 0\r\n" +
			"Connection: close\r\n" +
			"\r\n"))
		if err != nil {
			log.Println("Failed to write 301 Moved Permanently:", err)
			return
		}
		return
	}
	cw := NewCustomWriter(conn, dstReader, conn.RemoteAddr())
	cw.SetInteraction(sshSession.Interaction)
	forwardRequest(cw, reqhf, sshSession)
	return
}

func forwardRequest(cw *CustomWriter, initialRequest *RequestHeaderFactory, sshSession *session.SSHSession) {
	payload := sshSession.Forwarder.CreateForwardedTCPIPPayload(cw.RemoteAddr)
	channel, reqs, err := sshSession.Lifecycle.GetConnection().OpenChannel("forwarded-tcpip", payload)
	if err != nil {
		log.Printf("Failed to open forwarded-tcpip channel: %v", err)
		return
	}

	go func() {
		for req := range reqs {
			err := req.Reply(false, nil)
			if err != nil {
				log.Printf("Failed to reply to request: %v", err)
				return
			}
		}
	}()

	fingerprintMiddleware := NewTunnelFingerprint()
	forwardedForMiddleware := NewForwardedFor(cw.RemoteAddr)

	cw.respMW = append(cw.respMW, fingerprintMiddleware)
	cw.reqStartMW = append(cw.reqStartMW, forwardedForMiddleware)
	cw.reqEndMW = nil
	cw.reqHeader = initialRequest

	for _, m := range cw.reqStartMW {
		err = m.HandleRequest(cw.reqHeader)
		if err != nil {
			log.Printf("Error handling request: %v", err)
			return
		}
	}

	_, err = channel.Write(initialRequest.Finalize())
	if err != nil {
		log.Printf("Failed to forward request: %v", err)
		return
	}

	sshSession.Forwarder.HandleConnection(cw, channel, cw.RemoteAddr)
	return
}
