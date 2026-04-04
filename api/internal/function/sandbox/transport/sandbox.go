package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	sandboxservice "ollama-gateway/internal/function/sandbox"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *sandboxservice.Service
}

type sandboxRequest struct {
	Response string `json:"response"`
}

func NewHandler(svc *sandboxservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "sandbox service no disponible")
		return
	}
	var req sandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	req.Response = strings.TrimSpace(req.Response)
	if req.Response == "" {
		httputil.WriteError(w, http.StatusBadRequest, "response requerido")
		return
	}

	res, err := h.svc.Preview(req.Response)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusOK
	if !res.Valid {
		status = http.StatusUnprocessableEntity
	}
	httputil.WriteJSON(w, status, res)
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "sandbox service no disponible")
		return
	}
	var req sandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	req.Response = strings.TrimSpace(req.Response)
	if req.Response == "" {
		httputil.WriteError(w, http.StatusBadRequest, "response requerido")
		return
	}

	res, err := h.svc.ApplyValidated(req.Response)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "inválido en sandbox") || strings.Contains(err.Error(), "guardrails bloquearon") {
			status = http.StatusUnprocessableEntity
		}
		httputil.WriteJSON(w, status, map[string]interface{}{
			"error":      err.Error(),
			"applied":    false,
			"preview":    res.Preview,
			"guardrails": res.Guardrails,
		})
		return
	}

	httputil.WriteJSON(w, http.StatusOK, res)
}
