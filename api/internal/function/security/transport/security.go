package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	securityservice "ollama-gateway/internal/function/security"
	"ollama-gateway/pkg/httputil"
)

type SecurityHandler struct {
	securityService *securityservice.SecurityService
}

type securityScanFileRequest struct {
	Path string `json:"path"`
}

func NewSecurityHandler(securityService *securityservice.SecurityService) *SecurityHandler {
	return &SecurityHandler{securityService: securityService}
}

func (h *SecurityHandler) ScanFile(w http.ResponseWriter, r *http.Request) {
	var req securityScanFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "path requerido")
		return
	}

	findings, err := h.securityService.ScanFile(req.Path)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	for _, finding := range findings {
		counts[finding.Severity]++
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"path":              req.Path,
		"findings":          findings,
		"count":             len(findings),
		"findings_by_level": counts,
	})
}

func (h *SecurityHandler) ScanRepo(w http.ResponseWriter, r *http.Request) {
	report, err := h.securityService.ScanRepo()
	if err != nil {
		h.writeServiceError(w, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, report)
}

func (h *SecurityHandler) writeServiceError(w http.ResponseWriter, err error) {
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
