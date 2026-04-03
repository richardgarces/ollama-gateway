package transport

import coreservice "ollama-gateway/internal/function/core"

func NewHandler(ollama *coreservice.OllamaService) *ModelsHandler {
	return NewModelsHandler(ollama)
}
