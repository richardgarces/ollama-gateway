package transport

import (
	docgensvc "ollama-gateway/internal/function/docgen"
)

type Handler = DocGenHandler

func NewHandler(svc *docgensvc.Service) *Handler {
	return NewDocGenHandler(svc)
}
