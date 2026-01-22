package stream

import (
	"bytes"
	"tunnel_pls/internal/http/header"
)

func (hs *http) Write(p []byte) (int, error) {
	if hs.shouldBypassBuffering(p) {
		hs.respHeader = nil
	}

	if hs.respHeader != nil {
		return hs.writer.Write(p)
	}

	hs.buf = append(hs.buf, p...)

	headerEndIdx := bytes.Index(hs.buf, DELIMITER)
	if headerEndIdx == -1 {
		return len(p), nil
	}

	return hs.processBufferedResponse(p, headerEndIdx)
}

func (hs *http) shouldBypassBuffering(p []byte) bool {
	return hs.respHeader != nil && len(hs.buf) == 0 && len(p) >= 5 && string(p[0:5]) == "HTTP/"
}

func (hs *http) processBufferedResponse(p []byte, delimiterIdx int) (int, error) {
	headerByte, bodyByte := splitHeaderAndBody(hs.buf, delimiterIdx)

	if !isHTTPHeader(headerByte) {
		return hs.writeRawBuffer()
	}

	if err := hs.processHTTPResponse(headerByte, bodyByte); err != nil {
		return 0, err
	}

	hs.buf = nil
	return len(p), nil
}

func (hs *http) writeRawBuffer() (int, error) {
	_, err := hs.writer.Write(hs.buf)
	length := len(hs.buf)
	hs.buf = nil
	if err != nil {
		return 0, err
	}
	return length, nil
}

func (hs *http) processHTTPResponse(headerByte, bodyByte []byte) error {
	resphf, err := header.NewResponse(headerByte)
	if err != nil {
		return err
	}

	if err = hs.ApplyResponseMiddlewares(resphf, bodyByte); err != nil {
		return err
	}

	hs.respHeader = resphf
	finalHeader := resphf.Finalize()

	if err = hs.writeHeaderAndBody(finalHeader, bodyByte); err != nil {
		return err
	}

	return nil
}

func (hs *http) writeHeaderAndBody(header, bodyByte []byte) error {
	if _, err := hs.writer.Write(header); err != nil {
		return err
	}

	if len(bodyByte) > 0 {
		if _, err := hs.writer.Write(bodyByte); err != nil {
			return err
		}
	}

	return nil
}
