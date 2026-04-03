package handlers

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/core/domain"
	"ollama-gateway/pkg/httputil"
)

type GenerateHandler struct {
	ragService interface {
		GenerateWithContext(prompt string) (string, error)
	}
}

func NewGenerateHandler(ragService interface {
	GenerateWithContext(prompt string) (string, error)
}) *GenerateHandler {
	return &GenerateHandler{ragService: ragService}
}

func (h *GenerateHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req domain.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	if req.Prompt == "" {
		httputil.WriteError(w, http.StatusBadRequest, "prompt requerido")
		return
	}

	result, err := h.ragService.GenerateWithContext(req.Prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.Response{Result: result})
}

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/core/domain"
	"ollama-gateway/internal/usecase/services"
	"ollama-gateway/pkg/httputil"
)

type GenerateHandler struct {
	ragService *services.RAGService
}

func NewGenerateHandler(ragService *services.RAGService) *GenerateHandler {
	return &GenerateHandler{ragService: ragService}
}

func (h *GenerateHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req domain.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Prompt == "" {
		httputil.WriteError(w, http.StatusBadRequest, "prompt requerido")
		return
	}

	result, err := h.ragService.GenerateWithContext(req.Prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.Response{Result: result})
}
