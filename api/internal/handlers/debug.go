package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type debugService interface {
	AnalyzeError(stackTrace string) (domain.DebugAnalysis, error)
	AnalyzeLog(logLines string) (domain.DebugAnalysis, error)
}

type DebugHandler struct {
	svc debugService
}

type debugErrorRequest struct {
	StackTrace string `json:"stack_trace"`
}

type debugLogRequest struct {
	Log   string `json:"log"`
	Lines int    `json:"lines"`
}

func NewDebugHandler(svc debugService) *DebugHandler {
	return &DebugHandler{svc: svc}
}

func (h *DebugHandler) AnalyzeError(w http.ResponseWriter, r *http.Request) {
	var req debugErrorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.StackTrace) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "stack_trace requerido")
		return
	}

	result, err := h.svc.AnalyzeError(req.StackTrace)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *DebugHandler) AnalyzeLog(w http.ResponseWriter, r *http.Request) {
	var req debugLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Log) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "log requerido")
		return
	}

	logData := req.Log
	if req.Lines > 0 {
		parts := strings.Split(logData, "\n")
		if req.Lines < len(parts) {
			parts = parts[len(parts)-req.Lines:]
		}
		logData = strings.Join(parts, "\n")
	}

	result, err := h.svc.AnalyzeLog(logData)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, result)
}
