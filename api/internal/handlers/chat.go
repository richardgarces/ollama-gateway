package handlers

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type ChatHandler struct {
	ragService domain.RAGEngine
}

func NewChatHandler(ragService domain.RAGEngine) *ChatHandler {
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

	if req.Stream {
		if err := httputil.WriteSSEHeaders(w); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.ragService.StreamGenerateWithContext(prompt, func(chunk string) error {
			return httputil.WriteSSEData(w, map[string]interface{}{
				"choices": []map[string]interface{}{{
					"delta": map[string]string{"role": "assistant", "content": chunk},
				}},
			})
		}); err != nil {
			return
		}
		_ = httputil.WriteSSEDone(w)
		return
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
