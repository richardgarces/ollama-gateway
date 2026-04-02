package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type RepoHandler struct {
	repoService interface {
		Refactor(absPath string) (string, error)
		AnalyzeRepo() (string, error)
	}
}

func NewRepoHandler(repoService interface {
	Refactor(absPath string) (string, error)
	AnalyzeRepo() (string, error)
}) *RepoHandler {
	return &RepoHandler{repoService: repoService}
}

func (h *RepoHandler) Refactor(w http.ResponseWriter, r *http.Request) {
	var req domain.RefactorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	if req.Path == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}

	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "ruta inválida")
		return
	}

	result, err := h.repoService.Refactor(absPath)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"refactor": result})
}

func (h *RepoHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	result, err := h.repoService.AnalyzeRepo()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"analysis": result})
}
	"encoding/json"
	"net/http"
	"path/filepath"

	"ollama-gateway/internal/domain"
	"ollama-gateway/internal/services"
	"ollama-gateway/pkg/httputil"
)

type RepoHandler struct {
	repoService *services.RepoService
}

func NewRepoHandler(repoService *services.RepoService) *RepoHandler {
	return &RepoHandler{repoService: repoService}
}

func (h *RepoHandler) Refactor(w http.ResponseWriter, r *http.Request) {
	var req domain.RefactorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	result, err := h.repoService.Refactor(req.Path)
	if err != nil {
		if err == filepath.ErrBadPattern {
			httputil.WriteError(w, http.StatusForbidden, "acceso denegado: fuera del directorio permitido")
		} else {
			httputil.WriteError(w, http.StatusNotFound, "archivo no encontrado: "+err.Error())
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.RefactorResponse{Refactor: result})
}

func (h *RepoHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	result, err := h.repoService.AnalyzeRepo()
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, domain.AnalysisResponse{Analysis: result})
}
