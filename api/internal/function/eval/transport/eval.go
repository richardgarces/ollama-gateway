package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	evalservice "ollama-gateway/internal/function/eval"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *evalservice.Service
}

type runRequest struct {
	Suite string `json:"suite"`
}

func NewHandler(svc *evalservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Run(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "eval service no disponible")
		return
	}

	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	result, err := h.svc.RunBenchmark(req.Suite)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) GetResult(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "eval service no disponible")
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "id requerido")
		return
	}

	result, err := h.svc.GetResult(id)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "markdown" || format == "md" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(result.MDExport))
		return
	}
	if format == "json_export" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(result.JSONExport))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}
