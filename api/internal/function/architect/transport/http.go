package transport

import (
	architectsvc "ollama-gateway/internal/function/architect"
)

type Handler = ArchitectHandler

func NewHandler(svc *architectsvc.Service) *Handler {
	return NewArchitectHandler(svc)
}
