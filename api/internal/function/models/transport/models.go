package transport

import (
	"net/http"

	coreservice "ollama-gateway/internal/function/core"
	"ollama-gateway/pkg/httputil"
)

type ModelsHandler struct {
	ollama *coreservice.OllamaService
}

func NewModelsHandler(ollama *coreservice.OllamaService) *ModelsHandler {
	return &ModelsHandler{ollama: ollama}
}

func (h *ModelsHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.ollama == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"models": []string{}, "offline": true})
		return
	}
	models, err := h.ollama.ListModels()
	if err != nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"models": []string{}, "offline": true})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"models": models, "offline": false})
}
