package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type architectService interface {
	AnalyzeProject() (domain.ArchReport, error)
	SuggestRefactor(path string) (string, error)
}

type ArchitectHandler struct {
	svc architectService
}

type archRefactorRequest struct {
	Path string `json:"path"`
}

func NewArchitectHandler(svc architectService) *ArchitectHandler {
	return &ArchitectHandler{svc: svc}
}

func (h *ArchitectHandler) AnalyzeProject(w http.ResponseWriter, r *http.Request) {
	report, err := h.svc.AnalyzeProject()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

func (h *ArchitectHandler) SuggestRefactor(w http.ResponseWriter, r *http.Request) {
	var req archRefactorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}

	result, err := h.svc.SuggestRefactor(req.Path)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(strings.ToLower(msg), "path") || strings.Contains(msg, "REPO_ROOT"):
			httputil.WriteError(w, http.StatusBadRequest, msg)
		case errors.Is(err, os.ErrNotExist):
			httputil.WriteError(w, http.StatusNotFound, "archivo no encontrado")
		default:
			httputil.WriteError(w, http.StatusInternalServerError, msg)
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"refactor_suggestion": result})
}
