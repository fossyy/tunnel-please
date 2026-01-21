package stream

import (
	"bytes"
	"tunnel_pls/internal/http/header"
)

func (hs *http) Read(p []byte) (int, error) {
	tmp := make([]byte, len(p))
	read, err := hs.reader.Read(tmp)
	if read == 0 && err != nil {
		return 0, err
	}

	tmp = tmp[:read]

	headerEndIdx := bytes.Index(tmp, DELIMITER)
	if headerEndIdx == -1 {
		return handleNoDelimiter(p, tmp, err)
	}

	headerByte, bodyByte := splitHeaderAndBody(tmp, headerEndIdx)

	if !isHTTPHeader(headerByte) {
		copy(p, tmp)
		return read, nil
	}

	return hs.processHTTPRequest(p, headerByte, bodyByte)
}

func (hs *http) processHTTPRequest(p, headerByte, bodyByte []byte) (int, error) {
	reqhf, err := header.NewRequest(headerByte)
	if err != nil {
		return 0, err
	}

	if err = hs.ApplyRequestMiddlewares(reqhf); err != nil {
		return 0, err
	}

	hs.reqHeader = reqhf
	combined := append(reqhf.Finalize(), bodyByte...)
	return copy(p, combined), nil
}

func handleNoDelimiter(p, tmp []byte, err error) (int, error) {
	copy(p, tmp)
	return len(tmp), err
}
