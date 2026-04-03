package transport

import (
	cicdsvc "ollama-gateway/internal/function/cicd"
)

type Handler = CICDHandler

func NewHandler(svc *cicdsvc.Service) *Handler {
	return NewCICDHandler(svc)
}
