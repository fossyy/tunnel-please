package random

import (
	"crypto/rand"
	"fmt"
	"io"
)

var (
	ErrInvalidLength = fmt.Errorf("invalid length")
)

type Random interface {
	String(length int) (string, error)
}

type random struct {
	reader io.Reader
}

func New() Random {
	return &random{reader: rand.Reader}
}

func (ran *random) String(length int) (string, error) {
	if length < 0 {
		return "", ErrInvalidLength
	}
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)

	if _, err := ran.reader.Read(b); err != nil {
		return "", err
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}

	return string(b), nil
}
