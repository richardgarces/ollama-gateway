package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"ollama-gateway/pkg/httputil"
)

type docGenService interface {
	GenerateDocForFile(path string) (string, error)
	GenerateREADME(repoRoot string) (string, error)
	WriteWithBackup(path string, content string) (string, error)
}

type DocGenHandler struct {
	svc docGenService
}

type docGenFileRequest struct {
	Path  string `json:"path"`
	Apply bool   `json:"apply"`
}

type docGenReadmeRequest struct {
	RepoRoot string `json:"repo_root"`
	Apply    bool   `json:"apply"`
}

func NewDocGenHandler(svc docGenService) *DocGenHandler {
	return &DocGenHandler{svc: svc}
}

func (h *DocGenHandler) GenerateFileDoc(w http.ResponseWriter, r *http.Request) {
	var req docGenFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}

	content, err := h.svc.GenerateDocForFile(req.Path)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	backup := ""
	if req.Apply {
		backup, err = h.svc.WriteWithBackup(req.Path, content)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"path":        req.Path,
		"content":     content,
		"applied":     req.Apply,
		"backup_path": backup,
	})
}

func (h *DocGenHandler) GenerateREADME(w http.ResponseWriter, r *http.Request) {
	var req docGenReadmeRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
			return
		}
	}

	content, err := h.svc.GenerateREADME(req.RepoRoot)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	backup := ""
	if req.Apply {
		writePath := "README.md"
		if strings.TrimSpace(req.RepoRoot) != "" {
			writePath = strings.TrimRight(req.RepoRoot, "/") + "/README.md"
		}
		backup, err = h.svc.WriteWithBackup(writePath, content)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"content":     content,
		"applied":     req.Apply,
		"backup_path": backup,
	})
}

func (h *DocGenHandler) writeServiceError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(strings.ToLower(msg), "path") || strings.Contains(msg, "REPO_ROOT") || strings.Contains(strings.ToLower(msg), "repo"):
		httputil.WriteError(w, http.StatusBadRequest, msg)
	case errors.Is(err, os.ErrNotExist):
		httputil.WriteError(w, http.StatusNotFound, "archivo no encontrado")
	default:
		httputil.WriteError(w, http.StatusInternalServerError, msg)
	}
}
