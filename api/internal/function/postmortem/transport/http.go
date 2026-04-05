package transport

import postmortemservice "ollama-gateway/internal/function/postmortem"

type HTTPHandler = Handler

func NewHTTPHandler(svc *postmortemservice.Service) *Handler {
	return NewHandler(svc)
}
