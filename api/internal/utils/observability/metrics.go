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
	P50Latency     float64 `json:"p50_latency_ms"`
	P95Latency     float64 `json:"p95_latency_ms"`
	P99Latency     float64 `json:"p99_latency_ms"`
}

type MetricsSnapshot struct {
	StartedAt     time.Time     `json:"started_at"`
	UptimeSeconds int64         `json:"uptime_seconds"`
	TotalRequests int64         `json:"total_requests"`
	HitRate       float64       `json:"hit_rate"`
	MissRate      float64       `json:"miss_rate"`
	Pools         []PoolMetric  `json:"pools"`
	Routes        []RouteMetric `json:"routes"`
}

type PoolMetric struct {
	Name       string  `json:"name"`
	Capacity   int64   `json:"capacity"`
	InUse      int64   `json:"in_use"`
	MaxInUse   int64   `json:"max_in_use"`
	WaitCount  int64   `json:"wait_count"`
	Saturation float64 `json:"saturation"`
}

type routeStats struct {
	requests      int64
	errors        int64
	totalDuration time.Duration
	samplesMS     [maxRouteLatencySamples]int64
	samplesCount  int
	sampleIndex   int
}

const maxRouteLatencySamples = 256

type MetricsCollector struct {
	mu           sync.RWMutex
	startedAt    time.Time
	totalRequest int64
	cacheHits    int64
	cacheMisses  int64
	routes       map[string]*routeStats
	pools        map[string]*PoolMetric
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startedAt: time.Now().UTC(),
		routes:    make(map[string]*routeStats),
		pools:     make(map[string]*PoolMetric),
	}
}

func (c *MetricsCollector) RegisterPool(name string, capacity int) {
	if capacity <= 0 || name == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.pools[name]; ok {
		return
	}
	c.pools[name] = &PoolMetric{Name: name, Capacity: int64(capacity)}
}

func (c *MetricsCollector) ObservePoolAcquire(name string, waited bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pools[name]
	if !ok {
		return
	}
	p.InUse++
	if p.InUse > p.MaxInUse {
		p.MaxInUse = p.InUse
	}
	if waited {
		p.WaitCount++
	}
	p.Saturation = saturation(p.InUse, p.Capacity)
}

func (c *MetricsCollector) ObservePoolRelease(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pools[name]
	if !ok {
		return
	}
	if p.InUse > 0 {
		p.InUse--
	}
	p.Saturation = saturation(p.InUse, p.Capacity)
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
	latencyMS := duration.Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	stats.samplesMS[stats.sampleIndex] = latencyMS
	if stats.samplesCount < maxRouteLatencySamples {
		stats.samplesCount++
	}
	stats.sampleIndex = (stats.sampleIndex + 1) % maxRouteLatencySamples
	if status >= 400 {
		stats.errors++
	}
}

func (c *MetricsCollector) ObserveCacheHit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheHits++
}

func (c *MetricsCollector) ObserveCacheMiss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheMisses++
}

func (c *MetricsCollector) Snapshot() MetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	routes := make([]RouteMetric, 0, len(c.routes))
	pools := make([]PoolMetric, 0, len(c.pools))
	for _, p := range c.pools {
		pools = append(pools, *p)
	}
	for key, stats := range c.routes {
		method, path := splitMetricKey(key)
		avg := 0.0
		if stats.requests > 0 {
			avg = float64(stats.totalDuration.Milliseconds()) / float64(stats.requests)
		}
		samples := snapshotRouteSamples(stats)
		p50 := percentile(samples, 50)
		p95 := percentile(samples, 95)
		p99 := percentile(samples, 99)
		routes = append(routes, RouteMetric{
			Method:         method,
			Path:           path,
			Requests:       stats.requests,
			Errors:         stats.errors,
			AverageLatency: avg,
			P50Latency:     p50,
			P95Latency:     p95,
			P99Latency:     p99,
		})
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].Name < pools[j].Name
	})

	return MetricsSnapshot{
		StartedAt:     c.startedAt,
		UptimeSeconds: int64(time.Since(c.startedAt).Seconds()),
		TotalRequests: c.totalRequest,
		HitRate:       cacheHitRate(c.cacheHits, c.cacheMisses),
		MissRate:      cacheMissRate(c.cacheHits, c.cacheMisses),
		Pools:         pools,
		Routes:        routes,
	}
}

func saturation(inUse, capacity int64) float64 {
	if capacity <= 0 {
		return 0
	}
	return float64(inUse) / float64(capacity)
}

func cacheHitRate(hits, misses int64) float64 {
	total := hits + misses
	if total <= 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

func cacheMissRate(hits, misses int64) float64 {
	total := hits + misses
	if total <= 0 {
		return 0
	}
	return float64(misses) / float64(total)
}

func splitMetricKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ' ' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}

func snapshotRouteSamples(stats *routeStats) []float64 {
	if stats == nil || stats.samplesCount == 0 {
		return nil
	}
	out := make([]float64, 0, stats.samplesCount)
	for i := 0; i < stats.samplesCount; i++ {
		out = append(out, float64(stats.samplesMS[i]))
	}
	return out
}

func percentile(samples []float64, p int) float64 {
	if len(samples) == 0 {
		return 0
	}
	if p <= 0 {
		p = 0
	}
	if p >= 100 {
		p = 100
	}
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)
	idx := int((float64(p) / 100.0) * float64(len(sorted)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
