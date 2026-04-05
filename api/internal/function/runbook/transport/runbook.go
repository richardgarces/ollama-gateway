package transport

import (
	"encoding/json"
	"net/http"
	"strings"

	runbookservice "ollama-gateway/internal/function/runbook"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *runbookservice.Service
}

type generateRunbookRequest struct {
	IncidentType string `json:"incident_type"`
	Context      string `json:"context"`
	Apply        bool   `json:"apply"`
}

func NewHandler(svc *runbookservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "runbook service no disponible")
		return
	}

	var req generateRunbookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	req.IncidentType = strings.TrimSpace(req.IncidentType)
	if req.IncidentType == "" {
		httputil.WriteError(w, http.StatusBadRequest, "incident_type es requerido")
		return
	}

	runbook, err := h.svc.GenerateRunbook(req.IncidentType, req.Context)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Apply {
		path, err := h.svc.SaveRunbook(runbook)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		runbook.Applied = true
		runbook.MarkdownPath = path
	}

	httputil.WriteJSON(w, http.StatusOK, runbook)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "runbook service no disponible")
		return
	}

	incidentType := strings.TrimSpace(r.URL.Query().Get("incident_type"))
	runbooks, err := h.svc.ListRunbooks(incidentType)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"count":    len(runbooks),
		"runbooks": runbooks,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "runbook service no disponible")
		return
	}

	incidentType := strings.TrimSpace(r.PathValue("incident_type"))
	runbook, err := h.svc.GetRunbook(incidentType)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no encontrado") {
			httputil.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, runbook)
}
