package transport

import perfservice "ollama-gateway/internal/function/perf"

type HTTPHandler = Handler

func NewHTTPHandler(svc *perfservice.Service) *Handler {
	return NewHandler(svc)
}
