package transport

import gateservice "ollama-gateway/internal/function/gate"

type HTTPHandler = Handler

func NewHTTPHandler(svc *gateservice.Service) *Handler {
	return NewHandler(svc)
}
