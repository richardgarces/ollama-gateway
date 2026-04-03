package transport

import (
	"net/http"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/pkg/httputil"
)

type IndexerHandler struct {
	svc domain.Indexer
}

type indexerStatusProvider interface {
	Status() map[string]interface{}
}

func NewIndexerHandler(svc domain.Indexer) *IndexerHandler {
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

func (h *IndexerHandler) Status(w http.ResponseWriter, r *http.Request) {
	if provider, ok := h.svc.(indexerStatusProvider); ok {
		httputil.WriteJSON(w, http.StatusOK, provider.Status())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"indexed_files":   0,
		"watcher_active":  false,
		"reindexing":      false,
		"last_reindex_at": "",
	})
}
