package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/pkg/httputil"
)

type TestGenHandler struct {
	svc domain.TestGenService
}

type testGenRequest struct {
	Code string `json:"code"`
	Lang string `json:"lang"`
}

type testGenFileRequest struct {
	Path  string `json:"path"`
	Apply bool   `json:"apply"`
}

func NewTestGenHandler(svc domain.TestGenService) *TestGenHandler {
	return &TestGenHandler{svc: svc}
}

func (h *TestGenHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req testGenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "code requerido")
		return
	}
	if strings.TrimSpace(req.Lang) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "lang requerido")
		return
	}

	tests, err := h.svc.GenerateTests(req.Code, req.Lang)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"test_code": tests})
}

func (h *TestGenHandler) GenerateForFile(w http.ResponseWriter, r *http.Request) {
	var req testGenFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}

	tests, err := h.svc.GenerateTestsForFile(req.Path)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	appliedPath := ""
	backupPath := ""
	if req.Apply {
		appliedPath, backupPath, err = h.svc.WriteTestsForFile(req.Path, tests)
		if err != nil {
			h.writeServiceError(w, err)
			return
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"test_code":    tests,
		"applied":      req.Apply,
		"applied_path": appliedPath,
		"backup_path":  backupPath,
	})
}

func (h *TestGenHandler) writeServiceError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(strings.ToLower(msg), "path") || strings.Contains(msg, "REPO_ROOT"):
		httputil.WriteError(w, http.StatusBadRequest, msg)
	case errors.Is(err, os.ErrNotExist):
		httputil.WriteError(w, http.StatusNotFound, "archivo no encontrado")
	default:
		httputil.WriteError(w, http.StatusInternalServerError, msg)
	}
}
