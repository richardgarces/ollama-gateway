package observability

import (
	"sort"
	"sync"
	"time"
)

type RouteMetric struct {
	Method         string  `json:"method"`
	Path           string  `json:"path"`
	Requests       int64   `json:"requests"`
	Errors         int64   `json:"errors"`
	AverageLatency float64 `json:"average_latency_ms"`
}

type MetricsSnapshot struct {
	StartedAt     time.Time     `json:"started_at"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	TotalRequests int64         `json:"total_requests"`
	Routes        []RouteMetric `json:"routes"`
}

type routeStats struct {
	requests      int64
	errors        int64
	totalDuration time.Duration
}

type MetricsCollector struct {
	mu           sync.RWMutex
	startedAt    time.Time
	totalRequest int64
	routes       map[string]*routeStats
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startedAt: time.Now().UTC(),
		routes:    make(map[string]*routeStats),
	}
}

func (c *MetricsCollector) Observe(method, path string, status int, duration time.Duration) {
	key := method + " " + path

	c.mu.Lock()
	defer c.mu.Unlock()

	c.totalRequest++
	stats := c.routes[key]
	if stats == nil {
		stats = &routeStats{}
		c.routes[key] = stats
	}

	stats.requests++
	stats.totalDuration += duration
	if status >= 400 {
		stats.errors++
	}
}

func (c *MetricsCollector) Snapshot() MetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	routes := make([]RouteMetric, 0, len(c.routes))
	for key, stats := range c.routes {
		method, path := splitMetricKey(key)
		avg := 0.0
		if stats.requests > 0 {
			avg = float64(stats.totalDuration.Milliseconds()) / float64(stats.requests)
		}
		routes = append(routes, RouteMetric{
			Method:         method,
			Path:           path,
			Requests:       stats.requests,
			Errors:         stats.errors,
			AverageLatency: avg,
		})
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})

	return MetricsSnapshot{
		StartedAt:     c.startedAt,
		UptimeSeconds: int64(time.Since(c.startedAt).Seconds()),
		TotalRequests: c.totalRequest,
		Routes:        routes,
	}
}

func splitMetricKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ' ' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}
