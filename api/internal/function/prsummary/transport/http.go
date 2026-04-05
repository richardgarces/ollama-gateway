package transport

import prsummaryservice "ollama-gateway/internal/function/prsummary"

type HTTPHandler = Handler

func NewHTTPHandler(svc *prsummaryservice.Service) *Handler {
	return NewHandler(svc)
}
