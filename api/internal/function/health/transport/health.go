package transport

import (
	"net/http"
	"strings"

	"ollama-gateway/internal/config"
	healthservice "ollama-gateway/internal/function/health/service"
	"ollama-gateway/internal/function/resilience"
	"ollama-gateway/pkg/httputil"
)

type breakerStateProvider interface {
	CircuitBreakerState() resilience.Snapshot
}

type HealthHandler struct {
	svc *healthservice.Service
}

func NewHealthHandler(cfg *config.Config) *HealthHandler {
	return &HealthHandler{svc: healthservice.NewService(cfg)}
}

func NewHealthHandlerStrict(cfg *config.Config) (*HealthHandler, error) {
	svc, err := healthservice.NewServiceStrict(cfg)
	if err != nil {
		return nil, err
	}
	return &HealthHandler{svc: svc}, nil
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *HealthHandler) SetCircuitBreakers(ollama, qdrant breakerStateProvider) {
	if h == nil || h.svc == nil {
		return
	}
	h.svc.SetCircuitBreakers(ollama, qdrant)
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "health service no disponible")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, h.svc.Readiness(r.Context()))
}

func (h *HealthHandler) Backend(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "health service no disponible")
		return
	}

	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"backends": h.svc.RegisteredBackends(),
		})
		return
	}

	status, found := h.svc.CheckBackend(r.Context(), name)
	if !found {
		httputil.WriteError(w, http.StatusNotFound, "backend no registrado: "+name)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, status)
}
