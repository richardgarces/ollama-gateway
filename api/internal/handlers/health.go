package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/pkg/httputil"
)

type HealthHandler struct {
	ollamaURL string
	qdrantURL string
	client    *http.Client
}

type dependencyStatus struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

type dependenciesStatus struct {
	Ollama dependencyStatus `json:"ollama"`
	Qdrant dependencyStatus `json:"qdrant"`
}

type readinessResponse struct {
	Status       string             `json:"status"`
	Dependencies dependenciesStatus `json:"dependencies"`
}

func NewHealthHandler(cfg *config.Config) *HealthHandler {
	h := &HealthHandler{
		client: &http.Client{Timeout: 2 * time.Second},
	}
	if cfg != nil {
		h.ollamaURL = cfg.OllamaURL
		h.qdrantURL = cfg.QdrantURL
	}
	return h
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	deps := dependenciesStatus{}

	// TODO: agregar ping de MongoDB con timeout de 2s cuando se integre esa dependencia.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		deps.Ollama = h.checkDependency(r.Context(), h.ollamaURL)
	}()

	go func() {
		defer wg.Done()
		deps.Qdrant = h.checkDependency(r.Context(), h.qdrantURL)
	}()

	wg.Wait()

	healthyCount := 0
	if deps.Ollama.Status == "healthy" {
		healthyCount++
	}
	if deps.Qdrant.Status == "healthy" {
		healthyCount++
	}

	status := "degraded"
	switch healthyCount {
	case 2:
		status = "healthy"
	case 0:
		status = "unhealthy"
	}

	httputil.WriteJSON(w, http.StatusOK, readinessResponse{
		Status:       status,
		Dependencies: deps,
	})
}

func (h *HealthHandler) checkDependency(parentCtx context.Context, rawURL string) dependencyStatus {
	if strings.TrimSpace(rawURL) == "" {
		return dependencyStatus{Status: "unhealthy", LatencyMS: 0}
	}

	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel()

	target := strings.TrimRight(rawURL, "/") + "/"
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return dependencyStatus{Status: "unhealthy", LatencyMS: time.Since(start).Milliseconds()}
	}

	resp, err := h.client.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return dependencyStatus{Status: "unhealthy", LatencyMS: latency}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return dependencyStatus{Status: "healthy", LatencyMS: latency}
	}

	return dependencyStatus{Status: "unhealthy", LatencyMS: latency}
}
