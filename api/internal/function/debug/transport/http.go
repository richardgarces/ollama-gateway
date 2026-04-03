package transport

import (
	debugsvc "ollama-gateway/internal/function/debug"
)

type Handler = DebugHandler

func NewHandler(svc *debugsvc.Service) *Handler {
	return NewDebugHandler(svc)
}
