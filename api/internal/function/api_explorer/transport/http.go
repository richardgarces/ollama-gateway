package transport

import (
	"ollama-gateway/internal/function/core/domain"
)

type Handler = APIExplorerHandler

func NewHandler(routes []domain.RouteDefinition) *Handler {
	return NewAPIExplorerHandler(routes)
}
