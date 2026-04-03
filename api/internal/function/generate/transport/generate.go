package transport

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/pkg/httputil"
)

type GenerateHandler struct {
	ragService domain.RAGEngine
}

func NewGenerateHandler(ragService domain.RAGEngine) *GenerateHandler {
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

	if req.Stream {
		if err := httputil.WriteSSEHeaders(w); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		prompt := withRequestIDPrompt(r, req.Prompt)
		if err := h.ragService.StreamGenerateWithContext(prompt, func(chunk string) error {
			return httputil.WriteSSEData(w, map[string]string{"result": chunk})
		}); err != nil {
			return
		}
		_ = httputil.WriteSSEDone(w)
		return
	}

	prompt := withRequestIDPrompt(r, req.Prompt)
	result, err := h.ragService.GenerateWithContext(prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.Response{Result: result})
}
