package transport

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/function/resilience"
	"ollama-gateway/pkg/httputil"
)

type HealthHandler struct {
	ollamaURL string
	qdrantURL string
	mongoURI  string
	redisURL  string
	client    *http.Client
	ollamaCB  breakerStateProvider
	qdrantCB  breakerStateProvider
}

type breakerStateProvider interface {
	CircuitBreakerState() resilience.Snapshot
}

type dependencyStatus struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

type dependenciesStatus struct {
	Ollama dependencyStatus `json:"ollama"`
	Qdrant dependencyStatus `json:"qdrant"`
	Mongo  dependencyStatus `json:"mongo"`
	Redis  dependencyStatus `json:"redis"`
}

type readinessResponse struct {
	Status       string             `json:"status"`
	Dependencies dependenciesStatus `json:"dependencies"`
	Breakers     breakersStatus     `json:"breakers"`
}

type breakersStatus struct {
	Ollama resilience.Snapshot `json:"ollama"`
	Qdrant resilience.Snapshot `json:"qdrant"`
}

func NewHealthHandler(cfg *config.Config) *HealthHandler {
	h := &HealthHandler{
		client: &http.Client{Timeout: 2 * time.Second},
	}
	if cfg != nil {
		h.ollamaURL = cfg.OllamaURL
		h.qdrantURL = cfg.QdrantURL
		h.mongoURI = cfg.MongoURI
		h.redisURL = cfg.RedisURL
	}
	return h
}

func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h *HealthHandler) SetCircuitBreakers(ollama, qdrant breakerStateProvider) {
	if h == nil {
		return
	}
	h.ollamaCB = ollama
	h.qdrantCB = qdrant
}

func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	deps := dependenciesStatus{}

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		deps.Ollama = h.checkDependency(r.Context(), h.ollamaURL)
	}()

	go func() {
		defer wg.Done()
		deps.Qdrant = h.checkDependency(r.Context(), h.qdrantURL)
	}()

	go func() {
		defer wg.Done()
		deps.Mongo = h.checkTCPDependency(r.Context(), h.mongoURI, "mongodb")
	}()

	go func() {
		defer wg.Done()
		deps.Redis = h.checkTCPDependency(r.Context(), h.redisURL, "redis")
	}()

	wg.Wait()

	healthyCount := 0
	if deps.Ollama.Status == "healthy" {
		healthyCount++
	}
	if deps.Qdrant.Status == "healthy" {
		healthyCount++
	}
	if deps.Mongo.Status == "healthy" {
		healthyCount++
	}
	if deps.Redis.Status == "healthy" {
		healthyCount++
	}

	status := "degraded"
	switch healthyCount {
	case 4:
		status = "healthy"
	case 0:
		status = "unhealthy"
	}

	breakers := breakersStatus{
		Ollama: resilience.Snapshot{Name: "ollama", State: resilience.StateClosed},
		Qdrant: resilience.Snapshot{Name: "qdrant", State: resilience.StateClosed},
	}
	if h.ollamaCB != nil {
		breakers.Ollama = h.ollamaCB.CircuitBreakerState()
	}
	if h.qdrantCB != nil {
		breakers.Qdrant = h.qdrantCB.CircuitBreakerState()
	}

	httputil.WriteJSON(w, http.StatusOK, readinessResponse{
		Status:       status,
		Dependencies: deps,
		Breakers:     breakers,
	})
}

func (h *HealthHandler) checkTCPDependency(parentCtx context.Context, rawURL, schemeHint string) dependencyStatus {
	hostPort := hostPortFromRaw(rawURL, schemeHint)
	if hostPort == "" {
		return dependencyStatus{Status: "unhealthy", LatencyMS: 0}
	}
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel()
	start := time.Now()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return dependencyStatus{Status: "unhealthy", LatencyMS: latency}
	}
	_ = conn.Close()
	return dependencyStatus{Status: "healthy", LatencyMS: latency}
}

func hostPortFromRaw(rawURL, schemeHint string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = schemeHint + "://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "mongodb" || parsed.Scheme == "mongodb+srv" {
		hosts := strings.Split(parsed.Host, ",")
		first := strings.TrimSpace(hosts[0])
		if first == "" {
			return ""
		}
		if strings.Contains(first, ":") {
			return first
		}
		return first + ":27017"
	}
	host := parsed.Host
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") {
		return host
	}
	switch parsed.Scheme {
	case "redis":
		return host + ":6379"
	case "http":
		return host + ":80"
	case "https":
		return host + ":443"
	default:
		return host
	}
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
