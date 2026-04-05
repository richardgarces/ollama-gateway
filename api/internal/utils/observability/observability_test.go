package observability

import (
	"testing"
	"time"
)

func TestRateLimiterCheck(t *testing.T) {
	rl := NewRateLimiter(10, 20*time.Millisecond)
	d1 := rl.Check("k", 1, true)
	if !d1.Allowed || d1.Remaining != 0 {
		t.Fatalf("expected first request allowed with remaining 0")
	}
	d2 := rl.Check("k", 1, true)
	if d2.Allowed || d2.RetryAfter <= 0 {
		t.Fatalf("expected second request rejected with retry-after")
	}
	time.Sleep(25 * time.Millisecond)
	d3 := rl.Check("k", 1, true)
	if !d3.Allowed {
		t.Fatalf("expected allowed after window reset")
	}
}

func TestMetricsCollectorSnapshot(t *testing.T) {
	m := NewMetricsCollector()
	m.Observe("GET", "/x", 200, 10*time.Millisecond)
	m.Observe("GET", "/x", 500, 20*time.Millisecond)
	m.Observe("GET", "/x", 200, 40*time.Millisecond)
	s := m.Snapshot()
	if s.TotalRequests != 3 || len(s.Routes) != 1 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if s.Routes[0].Errors != 1 {
		t.Fatalf("expected one error in route metrics")
	}
	if s.Routes[0].P50Latency <= 0 || s.Routes[0].P95Latency <= 0 || s.Routes[0].P99Latency <= 0 {
		t.Fatalf("expected percentile latencies > 0, got %+v", s.Routes[0])
	}
}

func TestLogStream(t *testing.T) {
	ls := NewLogStream(2)
	ch, unsub := ls.Subscribe()
	defer unsub()
	ls.Publish(LogEvent{Level: "info", Message: "a"})
	ls.Publish(LogEvent{Level: "warn", Message: "b"})
	ls.Publish(LogEvent{Level: "error", Message: "c"})
	recent := ls.Recent(10)
	if len(recent) != 2 || recent[0].Message != "b" || recent[1].Message != "c" {
		t.Fatalf("unexpected recent events: %+v", recent)
	}
	select {
	case <-ch:
	default:
		t.Fatalf("expected subscriber to receive events")
	}
}

func TestPrometheusObservers(t *testing.T) {
	ObservePrometheus("GET", "/metrics", 200, 5*time.Millisecond)
	ObservePrometheus("GET", "/metrics", 500, 5*time.Millisecond)
	ObserveRateLimit("", "GET /x", "allowed")
}
