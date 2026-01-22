package middleware

import (
	"tunnel_pls/internal/http/header"
)

type RequestMiddleware interface {
	HandleRequest(header header.RequestHeader) error
}

type ResponseMiddleware interface {
	HandleResponse(header header.ResponseHeader, body []byte) error
}
