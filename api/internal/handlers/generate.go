package handlers

import (
	"encoding/json"
	"net/http"

	"ollama-gateway/internal/domain"
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

	if req.Stream {
		streamer, ok := h.ragService.(interface {
			StreamGenerateWithContext(prompt string, onChunk func(string) error) error
		})
		if !ok {
			httputil.WriteError(w, http.StatusNotImplemented, "streaming no soportado")
			return
		}
		if err := httputil.WriteSSEHeaders(w); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := streamer.StreamGenerateWithContext(req.Prompt, func(chunk string) error {
			return httputil.WriteSSEData(w, map[string]string{"result": chunk})
		}); err != nil {
			return
		}
		_ = httputil.WriteSSEDone(w)
		return
	}

	result, err := h.ragService.GenerateWithContext(req.Prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.Response{Result: result})
}
