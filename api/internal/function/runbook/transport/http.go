package transport

import runbookservice "ollama-gateway/internal/function/runbook"

type HTTPHandler = Handler

func NewHTTPHandler(svc *runbookservice.Service) *Handler {
	return NewHandler(svc)
}
