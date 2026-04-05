package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	jobsservice "ollama-gateway/internal/function/jobs"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *jobsservice.Service
}

type createJobRequest struct {
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params,omitempty"`
}

func NewHandler(svc *jobsservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "jobs service no disponible")
		return
	}

	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body JSON inválido")
		return
	}

	job, err := h.svc.Create(jobsservice.CreateInput{
		Type:        jobsservice.JobType(strings.TrimSpace(req.Type)),
		Params:      req.Params,
		RequestedBy: middleware.UserIDFromContext(r.Context()),
	})
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"job": job,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "jobs service no disponible")
		return
	}

	jobID := strings.TrimSpace(r.PathValue("id"))
	job, err := h.svc.Get(jobID)
	if err != nil {
		if errors.Is(err, jobsservice.ErrJobNotFound) {
			httputil.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"job": job})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "jobs service no disponible")
		return
	}

	jobID := strings.TrimSpace(r.PathValue("id"))
	job, err := h.svc.Cancel(jobID)
	if err != nil {
		if errors.Is(err, jobsservice.ErrJobNotFound) {
			httputil.WriteError(w, http.StatusNotFound, err.Error())
			return
		}
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"job": job})
}

func (h *Handler) Result(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "jobs service no disponible")
		return
	}

	jobID := strings.TrimSpace(r.PathValue("id"))
	result, status, err := h.svc.GetResult(jobID)
	if err != nil {
		switch {
		case errors.Is(err, jobsservice.ErrJobNotFound):
			httputil.WriteError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, jobsservice.ErrJobResultPending):
			httputil.WriteJSON(w, http.StatusConflict, map[string]interface{}{
				"status": status,
				"error":  err.Error(),
			})
		case errors.Is(err, jobsservice.ErrJobCanceled):
			httputil.WriteJSON(w, http.StatusConflict, map[string]interface{}{
				"status": status,
				"error":  err.Error(),
			})
		default:
			httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]interface{}{
				"status": status,
				"error":  err.Error(),
			})
		}
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": status,
		"result": result,
	})
}
