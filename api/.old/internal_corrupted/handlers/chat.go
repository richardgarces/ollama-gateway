package handlers

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/core/domain"
	"ollama-gateway/pkg/httputil"
)

type ChatHandler struct {
	ragService interface {
		GenerateWithContext(prompt string) (string, error)
	}
}

func NewChatHandler(ragService interface {
	GenerateWithContext(prompt string) (string, error)
}) *ChatHandler {
	return &ChatHandler{ragService: ragService}
}

func (h *ChatHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req domain.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	if len(req.Messages) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "messages requerido")
		return
	}

	prompt := ""
	for _, m := range req.Messages {
		prompt += m.Role + ": " + m.Content + "\n"
	}

	result, err := h.ragService.GenerateWithContext(prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := domain.ChatResponse{
		Choices: []domain.Choice{
			{
				Message: domain.Message{
					Role:    "assistant",
					Content: result,
				},
			},
		},
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/core/domain"
	"ollama-gateway/internal/usecase/services"
	"ollama-gateway/pkg/httputil"
)

type ChatHandler struct {
	ragService *services.RAGService
}

func NewChatHandler(ragService *services.RAGService) *ChatHandler {
	return &ChatHandler{ragService: ragService}
}

func (h *ChatHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req domain.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if len(req.Messages) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "messages requerido")
		return
	}

	prompt := ""
	for _, m := range req.Messages {
		prompt += m.Role + ": " + m.Content + "\n"
	}

	result, err := h.ragService.GenerateWithContext(prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := domain.ChatResponse{
		Choices: []domain.ChatChoice{
			{
				Message: domain.ChatMessage{
					Role:    "assistant",
					Content: result,
				},
			},
		},
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}
