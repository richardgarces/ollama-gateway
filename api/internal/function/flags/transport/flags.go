package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	flagsservice "ollama-gateway/internal/function/flags"
	"ollama-gateway/pkg/httputil"
)

type Handler struct {
	svc *flagsservice.Service
}

type upsertRequest struct {
	Tenant            string  `json:"tenant"`
	Feature           string  `json:"feature"`
	Enabled           bool    `json:"enabled"`
	RolloutPercentage int     `json:"rollout_percentage"`
	StartAt           *string `json:"start_at,omitempty"`
	EndAt             *string `json:"end_at,omitempty"`
	Description       string  `json:"description,omitempty"`
}

func NewHandler(svc *flagsservice.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "flags service no disponible")
		return
	}
	var req upsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	input, err := toInput(req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.Feature) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "feature requerido")
		return
	}
	created, err := h.svc.Create(r.Context(), input)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "flags service no disponible")
		return
	}
	tenant := strings.TrimSpace(r.URL.Query().Get("tenant"))
	flags, err := h.svc.List(r.Context(), tenant)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"flags": flags, "count": len(flags)})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "flags service no disponible")
		return
	}
	feature := strings.TrimSpace(r.PathValue("feature"))
	tenant := strings.TrimSpace(r.URL.Query().Get("tenant"))
	flag, err := h.svc.Get(r.Context(), tenant, feature)
	if err != nil {
		httputil.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	enabled, enabledErr := h.svc.IsEnabledWithContext(r.Context(), flag.Tenant, flag.Feature)
	if enabledErr == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"flag": flag, "enabled_now": enabled})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{"flag": flag})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "flags service no disponible")
		return
	}
	feature := strings.TrimSpace(r.PathValue("feature"))
	var req upsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "body invalido: "+err.Error())
		return
	}
	input, err := toInput(req)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	tenant := strings.TrimSpace(input.Tenant)
	if tenant == "" {
		tenant = strings.TrimSpace(r.URL.Query().Get("tenant"))
	}
	updated, err := h.svc.Update(r.Context(), tenant, feature, input)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, updated)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "flags service no disponible")
		return
	}
	feature := strings.TrimSpace(r.PathValue("feature"))
	tenant := strings.TrimSpace(r.URL.Query().Get("tenant"))
	if err := h.svc.Delete(r.Context(), tenant, feature); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted", "feature": strings.ToLower(feature), "tenant": strings.ToLower(strings.TrimSpace(defaultIfEmpty(tenant, "default")))})
}

func toInput(req upsertRequest) (flagsservice.UpsertInput, error) {
	startAt, err := parseOptionalTime(req.StartAt)
	if err != nil {
		return flagsservice.UpsertInput{}, err
	}
	endAt, err := parseOptionalTime(req.EndAt)
	if err != nil {
		return flagsservice.UpsertInput{}, err
	}
	return flagsservice.UpsertInput{
		Tenant:            strings.TrimSpace(req.Tenant),
		Feature:           strings.TrimSpace(req.Feature),
		Enabled:           req.Enabled,
		RolloutPercentage: req.RolloutPercentage,
		StartAt:           startAt,
		EndAt:             endAt,
		Description:       strings.TrimSpace(req.Description),
	}, nil
}

func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func withTimeoutCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}
