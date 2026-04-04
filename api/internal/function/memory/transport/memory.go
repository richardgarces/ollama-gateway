package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	memoryservice "ollama-gateway/internal/function/memory"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *memoryservice.Service
}

type saveRequest struct {
	Summary  string                 `json:"summary"`
	Detail   string                 `json:"detail"`
	Priority int                    `json:"priority"`
	Tags     []string               `json:"tags"`
	Source   string                 `json:"source"`
	Metadata map[string]interface{} `json:"metadata"`
	TTLHours int                    `json:"ttl_hours"`
}

type queryRequest struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k"`
}

func NewHandler(svc *memoryservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "memory service no disponible")
		return
	}

	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	req.Summary = strings.TrimSpace(req.Summary)
	if req.Summary == "" {
		httputil.WriteError(w, http.StatusBadRequest, "summary requerido")
		return
	}

	event, err := h.svc.SaveContext(r.Context(), memoryservice.SaveContextInput{
		Summary:  req.Summary,
		Detail:   strings.TrimSpace(req.Detail),
		Priority: req.Priority,
		Tags:     req.Tags,
		Source:   strings.TrimSpace(req.Source),
		Metadata: req.Metadata,
		TTLHours: req.TTLHours,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"saved": true,
		"event": event,
	})
}

func (h *Handler) Query(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "memory service no disponible")
		return
	}

	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		httputil.WriteError(w, http.StatusBadRequest, "query requerida")
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}

	items, err := h.svc.GetRelevantContext(r.Context(), req.Query, req.TopK)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"query":    req.Query,
		"top_k":    req.TopK,
		"contexts": items,
	})
}
