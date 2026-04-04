package transport

import (
	"encoding/json"
	"net/http"
	"strconv"

	feedbackservice "ollama-gateway/internal/function/feedback"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *feedbackservice.Service
}

func NewHandler(svc *feedbackservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "feedback service no disponible")
		return
	}

	var req feedbackservice.SaveFeedbackInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	rec, err := h.svc.SaveFeedback(r.Context(), req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"saved":    true,
		"feedback": rec,
	})
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "feedback service no disponible")
		return
	}
	hours := 24 * 7
	if v := r.URL.Query().Get("hours"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 24*365 {
			hours = parsed
		}
	}
	summary, err := h.svc.Summary(r.Context(), hours)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, summary)
}
