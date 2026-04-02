package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ollama-gateway/pkg/httputil"
)

type commitGenService interface {
	GenerateMessage(diff string) (string, error)
	GenerateFromStaged(repoRoot string) (string, error)
}

type CommitGenHandler struct {
	svc commitGenService
}

type commitMessageRequest struct {
	Diff string `json:"diff"`
}

type commitStagedRequest struct {
	RepoRoot string `json:"repo_root"`
}

func NewCommitGenHandler(svc commitGenService) *CommitGenHandler {
	return &CommitGenHandler{svc: svc}
}

func (h *CommitGenHandler) Message(w http.ResponseWriter, r *http.Request) {
	var req commitMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Diff) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "diff requerido")
		return
	}

	message, err := h.svc.GenerateMessage(req.Diff)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": message,
		"warning": "Mensaje generado por IA. Revísalo antes de confirmar el commit.",
	})
}

func (h *CommitGenHandler) Staged(w http.ResponseWriter, r *http.Request) {
	var req commitStagedRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
			return
		}
	}

	message, err := h.svc.GenerateFromStaged(req.RepoRoot)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message": message,
		"warning": "Mensaje generado por IA. Revísalo antes de confirmar el commit.",
	})
}

func (h *CommitGenHandler) writeServiceError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(strings.ToLower(msg), "diff") || strings.Contains(strings.ToLower(msg), "repo") || strings.Contains(strings.ToLower(msg), "path") || strings.Contains(strings.ToLower(msg), "staged") || strings.Contains(strings.ToLower(msg), "timeout"):
		httputil.WriteError(w, http.StatusBadRequest, msg)
	default:
		httputil.WriteError(w, http.StatusInternalServerError, msg)
	}
}
