package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	releaseservice "ollama-gateway/internal/function/release"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *releaseservice.Service
}

type buildNotesRequest struct {
	FromRef string `json:"fromRef"`
	ToRef   string `json:"toRef"`
	Apply   bool   `json:"apply"`
}

func NewHandler(svc *releaseservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) BuildNotes(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "release service no disponible")
		return
	}

	var req buildNotesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}

	req.FromRef = strings.TrimSpace(req.FromRef)
	req.ToRef = strings.TrimSpace(req.ToRef)
	if req.FromRef == "" || req.ToRef == "" {
		httputil.WriteError(w, http.StatusBadRequest, "fromRef y toRef son requeridos")
		return
	}

	notes, err := h.svc.BuildReleaseNotes(req.FromRef, req.ToRef)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	applied := false
	if req.Apply {
		if err := h.svc.WriteChangelog(notes); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		applied = true
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"fromRef":         req.FromRef,
		"toRef":           req.ToRef,
		"features":        notes.Features,
		"fixes":           notes.Fixes,
		"breakingChanges": notes.BreakingChanges,
		"security":        notes.Security,
		"applied":         applied,
	})
}
