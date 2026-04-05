package perf

import (
	"testing"
	"time"

	"ollama-gateway/internal/utils/observability"
)

type fakeCollector struct {
	snapshot observability.MetricsSnapshot
}

func (f fakeCollector) Snapshot() observability.MetricsSnapshot {
	return f.snapshot
}

func TestAnalyzeEndpointsRanking(t *testing.T) {
	svc := NewService(fakeCollector{snapshot: observability.MetricsSnapshot{
		StartedAt: time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC),
		Routes: []observability.RouteMetric{
			{Method: "GET", Path: "/api/a", Requests: 500, Errors: 2, AverageLatency: 120, P50Latency: 90, P95Latency: 600, P99Latency: 1200},
			{Method: "POST", Path: "/api/b", Requests: 300, Errors: 30, AverageLatency: 700, P50Latency: 400, P95Latency: 1800, P99Latency: 2500},
			{Method: "GET", Path: "/api/c", Requests: 20, Errors: 0, AverageLatency: 40, P50Latency: 30, P95Latency: 50, P99Latency: 60},
		},
	}})

	result, err := svc.AnalyzeEndpoints()
	if err != nil {
		t.Fatalf("AnalyzeEndpoints() error = %v", err)
	}
	if result.TotalEndpoints != 3 {
		t.Fatalf("expected 3 endpoints, got %d", result.TotalEndpoints)
	}
	if len(result.CriticalRanking) != 3 {
		t.Fatalf("expected ranking entries")
	}
	if result.CriticalRanking[0].Path != "/api/b" {
		t.Fatalf("expected /api/b as top critical endpoint, got %s", result.CriticalRanking[0].Path)
	}
	if result.CriticalRanking[0].ImpactScore <= result.CriticalRanking[1].ImpactScore {
		t.Fatalf("expected descending impact score")
	}
	if len(result.CriticalRanking[0].Recommendations) == 0 {
		t.Fatalf("expected recommendations for top endpoint")
	}
}

func TestAnalyzeEndpointsRequiresCollector(t *testing.T) {
	_, err := (&Service{}).AnalyzeEndpoints()
	if err == nil {
		t.Fatalf("expected error when collector is nil")
	}
}
