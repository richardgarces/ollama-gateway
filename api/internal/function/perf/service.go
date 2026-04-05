package perf

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"ollama-gateway/internal/utils/observability"
)

type metricsCollector interface {
	Snapshot() observability.MetricsSnapshot
}

type Service struct {
	collector metricsCollector
}

type EndpointPerf struct {
	Method          string   `json:"method"`
	Path            string   `json:"path"`
	Requests        int64    `json:"requests"`
	P50LatencyMS    float64  `json:"p50_latency_ms"`
	P95LatencyMS    float64  `json:"p95_latency_ms"`
	P99LatencyMS    float64  `json:"p99_latency_ms"`
	ErrorRate       float64  `json:"error_rate"`
	ImpactScore     float64  `json:"impact_score"`
	Recommendations []string `json:"recommendations"`
}

type AnalyzeResult struct {
	GeneratedAtUTC  string         `json:"generated_at_utc"`
	TotalEndpoints  int            `json:"total_endpoints"`
	CriticalRanking []EndpointPerf `json:"critical_ranking"`
}

func NewService(collector metricsCollector) *Service {
	return &Service{collector: collector}
}

func (s *Service) AnalyzeEndpoints() (AnalyzeResult, error) {
	if s == nil || s.collector == nil {
		return AnalyzeResult{}, fmt.Errorf("metrics collector no disponible")
	}

	snap := s.collector.Snapshot()
	out := make([]EndpointPerf, 0, len(snap.Routes))

	for _, route := range snap.Routes {
		if route.Requests <= 0 {
			continue
		}

		errorRate := float64(route.Errors) / float64(route.Requests)
		p50 := fallbackLatency(route.P50Latency, route.AverageLatency)
		p95 := fallbackLatency(route.P95Latency, route.AverageLatency)
		p99 := fallbackLatency(route.P99Latency, route.AverageLatency)
		impact := computeImpactScore(p95, errorRate, route.Requests)

		out = append(out, EndpointPerf{
			Method:          route.Method,
			Path:            route.Path,
			Requests:        route.Requests,
			P50LatencyMS:    p50,
			P95LatencyMS:    p95,
			P99LatencyMS:    p99,
			ErrorRate:       round(errorRate, 4),
			ImpactScore:     round(impact, 2),
			Recommendations: buildRecommendations(p95, p99, errorRate, route.Requests),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ImpactScore == out[j].ImpactScore {
			if out[i].P95LatencyMS == out[j].P95LatencyMS {
				return out[i].ErrorRate > out[j].ErrorRate
			}
			return out[i].P95LatencyMS > out[j].P95LatencyMS
		}
		return out[i].ImpactScore > out[j].ImpactScore
	})

	return AnalyzeResult{
		GeneratedAtUTC:  snap.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		TotalEndpoints:  len(out),
		CriticalRanking: out,
	}, nil
}

func computeImpactScore(p95 float64, errorRate float64, requests int64) float64 {
	latencyScore := clamp01(p95 / 2000.0)
	errorScore := clamp01(errorRate / 0.05)
	volumeScore := clamp01(math.Log10(float64(requests)+1.0) / 4.0)
	return (0.5 * latencyScore * 100.0) + (0.35 * errorScore * 100.0) + (0.15 * volumeScore * 100.0)
}

func buildRecommendations(p95 float64, p99 float64, errorRate float64, requests int64) []string {
	recs := []string{}
	if p95 > 800 {
		recs = append(recs, "Investigar consultas/llamadas lentas y aplicar caching selectivo en respuestas frecuentes.")
	}
	if p99 > 1500 {
		recs = append(recs, "Aplicar timeouts y retries acotados en dependencias para reducir cola de latencias extremas.")
	}
	if errorRate > 0.02 {
		recs = append(recs, "Priorizar analisis de errores 5xx y agregar circuit breaker o fallback en integraciones inestables.")
	}
	if requests > 200 {
		recs = append(recs, "Este endpoint tiene alto volumen: evaluar optimizaciones de mayor impacto primero.")
	}
	if len(recs) == 0 {
		recs = append(recs, "Endpoint estable: mantener observabilidad y revisar tendencias semanales.")
	}
	return unique(recs)
}

func fallbackLatency(value, fallback float64) float64 {
	if value > 0 {
		return round(value, 2)
	}
	if fallback > 0 {
		return round(fallback, 2)
	}
	return 0
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func round(value float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(value*factor) / factor
}
