package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"ollama-gateway/pkg/httputil"
)

type cicdService interface {
	GeneratePipeline(platform, repoRoot string) (string, error)
	OptimizePipeline(existing string, platform string) (string, error)
	ApplyPipeline(platform, repoRoot, content string) (string, string, error)
}

type CICDHandler struct {
	svc cicdService
}

type cicdGenerateRequest struct {
	Platform string `json:"platform"`
	RepoRoot string `json:"repo_root"`
	Apply    bool   `json:"apply"`
}

type cicdOptimizeRequest struct {
	Pipeline string `json:"pipeline"`
	Platform string `json:"platform"`
}

func NewCICDHandler(svc cicdService) *CICDHandler {
	return &CICDHandler{svc: svc}
}

func (h *CICDHandler) GeneratePipeline(w http.ResponseWriter, r *http.Request) {
	var req cicdGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Platform) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "platform requerido")
		return
	}

	pipeline, err := h.svc.GeneratePipeline(req.Platform, req.RepoRoot)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	appliedPath := ""
	backupPath := ""
	if req.Apply {
		appliedPath, backupPath, err = h.svc.ApplyPipeline(req.Platform, req.RepoRoot, pipeline)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"platform":     req.Platform,
		"pipeline":     pipeline,
		"applied":      req.Apply,
		"applied_path": appliedPath,
		"backup_path":  backupPath,
		"warning":      "Pipeline generado por IA. Revísalo manualmente antes de usarlo en producción.",
	})
}

func (h *CICDHandler) OptimizePipeline(w http.ResponseWriter, r *http.Request) {
	var req cicdOptimizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Platform) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "platform requerido")
		return
	}
	if strings.TrimSpace(req.Pipeline) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "pipeline requerido")
		return
	}

	optimized, err := h.svc.OptimizePipeline(req.Pipeline, req.Platform)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"platform":           req.Platform,
		"optimized_pipeline": optimized,
		"warning":            "Pipeline optimizado por IA. Revísalo manualmente antes de usarlo en producción.",
	})
}

func (h *CICDHandler) writeServiceError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(strings.ToLower(msg), "platform") || strings.Contains(strings.ToLower(msg), "pipeline") || strings.Contains(strings.ToLower(msg), "repo") || strings.Contains(strings.ToLower(msg), "path"):
		httputil.WriteError(w, http.StatusBadRequest, msg)
	case errors.Is(err, os.ErrNotExist):
		httputil.WriteError(w, http.StatusNotFound, "archivo no encontrado")
	default:
		httputil.WriteError(w, http.StatusInternalServerError, msg)
	}
}
