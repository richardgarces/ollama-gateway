package transport

import (
	translatorsvc "ollama-gateway/internal/function/translator"
)

type Handler = TranslatorHandler

func NewHandler(svc *translatorsvc.Service) *Handler {
	return NewTranslatorHandler(svc)
}
