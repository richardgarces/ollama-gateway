package handlers

import (
	"net/http"

	"ollama-gateway/internal/services"
	"ollama-gateway/pkg/httputil"
)

type IndexerHandler struct {
	svc *services.IndexerService
}

func NewIndexerHandler(svc *services.IndexerService) *IndexerHandler {
	return &IndexerHandler{svc: svc}
}

func (h *IndexerHandler) Reindex(w http.ResponseWriter, r *http.Request) {
	go h.svc.IndexRepo()
	httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "reindexing"})
}

func (h *IndexerHandler) StartWatcher(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.StartWatcher(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "watcher_started"})
}

func (h *IndexerHandler) StopWatcher(w http.ResponseWriter, r *http.Request) {
	h.svc.StopWatcher()
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "watcher_stopped"})
}

func (h *IndexerHandler) ResetState(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ClearState(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "state_cleared"})
}
