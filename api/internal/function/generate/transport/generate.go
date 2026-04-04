package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"ollama-gateway/internal/function/core/domain"
	eventservice "ollama-gateway/internal/function/events"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/pkg/httputil"
)

type GenerateHandler struct {
	ragService domain.RAGEngine
	events     eventservice.Publisher
}

func NewGenerateHandler(ragService domain.RAGEngine) *GenerateHandler {
	return &GenerateHandler{ragService: ragService}
}

func (h *GenerateHandler) SetEventPublisher(p eventservice.Publisher) {
	if h == nil {
		return
	}
	h.events = p
}

func (h *GenerateHandler) Handle(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now().UTC()
	statusCode := http.StatusOK
	defer func() {
		h.publishRequestCompleted(r, statusCode, startedAt)
	}()

	var req domain.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = http.StatusBadRequest
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	if req.Prompt == "" {
		statusCode = http.StatusBadRequest
		httputil.WriteError(w, http.StatusBadRequest, "prompt requerido")
		return
	}

	if req.Stream {
		if err := httputil.WriteSSEHeaders(w); err != nil {
			statusCode = http.StatusInternalServerError
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		prompt := withRequestIDPrompt(r, req.Prompt)
		if err := h.ragService.StreamGenerateWithContext(prompt, func(chunk string) error {
			return httputil.WriteSSEData(w, map[string]string{"result": chunk})
		}); err != nil {
			statusCode = http.StatusInternalServerError
			return
		}
		_ = httputil.WriteSSEDone(w)
		statusCode = http.StatusOK
		return
	}

	prompt := withRequestIDPrompt(r, req.Prompt)
	result, err := h.ragService.GenerateWithContext(prompt)
	if err != nil {
		statusCode = http.StatusInternalServerError
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.Response{Result: result})
	statusCode = http.StatusOK
}

func (h *GenerateHandler) publishRequestCompleted(r *http.Request, statusCode int, startedAt time.Time) {
	if h == nil || h.events == nil || r == nil {
		return
	}
	_ = h.events.Publish(context.Background(), eventservice.RequestCompleted{
		RequestID:  middleware.RequestIDFromContext(r.Context()),
		Path:       r.URL.Path,
		StatusCode: statusCode,
		DurationMS: time.Since(startedAt).Milliseconds(),
		At:         time.Now().UTC(),
	})
}
