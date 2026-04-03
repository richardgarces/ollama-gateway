package transport

import (
	securitysvc "ollama-gateway/internal/function/security"
)

type Handler = SecurityHandler

func NewHandler(securityService *securitysvc.Service) *Handler {
	return NewSecurityHandler(securityService)
}
