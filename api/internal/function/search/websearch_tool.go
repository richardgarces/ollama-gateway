package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	coreservice "ollama-gateway/internal/function/core"
)

// WebSearchTool wraps WebSearchService as a Tool for the agent ToolRegistry.
type WebSearchTool struct {
	service *WebSearchService
}

func NewWebSearchTool(service *WebSearchService) *WebSearchTool {
	return &WebSearchTool{service: service}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Run(args map[string]string) (string, error) {
	query := strings.TrimSpace(args["query"])
	if query == "" {
		return "", fmt.Errorf("query requerido")
	}
	if !t.service.Enabled() {
		return "", fmt.Errorf("web search no disponible: OLLAMA_API_KEY no configurada")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	results, err := t.service.Search(ctx, query)
	if err != nil {
		return "", fmt.Errorf("error en web search: %w", err)
	}

	return t.service.FormatResults(results), nil
}

// Ensure WebSearchTool satisfies coreservice.Tool interface.
var _ coreservice.Tool = (*WebSearchTool)(nil)
