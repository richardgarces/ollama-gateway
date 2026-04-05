package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/config"
	healthdomain "ollama-gateway/internal/function/health/domain"
	"ollama-gateway/internal/function/resilience"
)

type breakerStateProvider interface {
	CircuitBreakerState() resilience.Snapshot
}

type Service struct {
	checks         []healthdomain.BackendCheckConfig
	defaultTimeout time.Duration
	client         *http.Client
	ollamaCB       breakerStateProvider
	qdrantCB       breakerStateProvider
}

func NewService(cfg *config.Config) *Service {
	checks, timeout := buildChecks(cfg)
	return &Service{
		checks:         checks,
		defaultTimeout: timeout,
		client:         &http.Client{Timeout: timeout},
	}
}

func NewServiceStrict(cfg *config.Config) (*Service, error) {
	checks, timeout, err := buildChecksStrict(cfg)
	if err != nil {
		return nil, err
	}
	return &Service{
		checks:         checks,
		defaultTimeout: timeout,
		client:         &http.Client{Timeout: timeout},
	}, nil
}

func (s *Service) SetCircuitBreakers(ollama, qdrant breakerStateProvider) {
	if s == nil {
		return
	}
	s.ollamaCB = ollama
	s.qdrantCB = qdrant
}

func (s *Service) Readiness(ctx context.Context) healthdomain.ReadinessResponse {
	deps := make(map[string]healthdomain.DependencyStatus, len(s.checks))
	if s == nil {
		return healthdomain.ReadinessResponse{Status: "unhealthy", Dependencies: deps, Breakers: map[string]resilience.Snapshot{}}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, check := range s.checks {
		check := check
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := s.runCheck(ctx, check)
			mu.Lock()
			deps[check.Name] = status
			mu.Unlock()
		}()
	}
	wg.Wait()

	return healthdomain.ReadinessResponse{
		Status:       aggregateStatus(s.checks, deps),
		Dependencies: deps,
		Breakers:     s.breakersSnapshot(),
	}
}

func (s *Service) CheckBackend(ctx context.Context, name string) (healthdomain.DependencyStatus, bool) {
	if s == nil {
		return healthdomain.DependencyStatus{}, false
	}
	for _, check := range s.checks {
		if check.Name == name {
			return s.runCheck(ctx, check), true
		}
	}
	return healthdomain.DependencyStatus{}, false
}

func (s *Service) RegisteredBackends() []string {
	if s == nil {
		return nil
	}
	names := make([]string, len(s.checks))
	for i, c := range s.checks {
		names[i] = c.Name
	}
	return names
}

func buildChecks(cfg *config.Config) ([]healthdomain.BackendCheckConfig, time.Duration) {
	checks, timeout, _ := buildChecksStrict(cfg)
	return checks, timeout
}

func buildChecksStrict(cfg *config.Config) ([]healthdomain.BackendCheckConfig, time.Duration, error) {
	timeout := 2 * time.Second
	checks := make([]healthdomain.BackendCheckConfig, 0, 8)
	if cfg == nil {
		return checks, timeout, nil
	}

	if cfg.HealthCheckTimeoutMS > 0 {
		timeout = time.Duration(cfg.HealthCheckTimeoutMS) * time.Millisecond
	}

	checks = appendIfTarget(checks, healthdomain.BackendCheckConfig{
		Name:     "ollama",
		Type:     healthdomain.CheckTypeHTTP,
		Target:   strings.TrimSpace(cfg.OllamaURL),
		Path:     "/",
		Required: true,
	})
	checks = appendIfTarget(checks, healthdomain.BackendCheckConfig{
		Name:     "qdrant",
		Type:     healthdomain.CheckTypeHTTP,
		Target:   strings.TrimSpace(cfg.QdrantURL),
		Path:     "/",
		Required: true,
	})
	checks = appendIfTarget(checks, healthdomain.BackendCheckConfig{
		Name:     "mongo",
		Type:     healthdomain.CheckTypeTCP,
		Target:   strings.TrimSpace(cfg.MongoURI),
		Required: true,
	})
	checks = appendIfTarget(checks, healthdomain.BackendCheckConfig{
		Name:     "redis",
		Type:     healthdomain.CheckTypeTCP,
		Target:   strings.TrimSpace(cfg.RedisURL),
		Required: true,
	})

	extra, err := parseExtraChecksStrict(cfg.HealthExtraChecksJSON)
	if err != nil {
		return nil, timeout, err
	}
	if len(extra) > 0 {
		checks = append(checks, extra...)
	}

	sort.SliceStable(checks, func(i, j int) bool {
		return checks[i].Name < checks[j].Name
	})
	return checks, timeout, nil
}

func parseExtraChecks(raw string) []healthdomain.BackendCheckConfig {
	result, _ := parseExtraChecksStrict(raw)
	return result
}

func parseExtraChecksStrict(raw string) ([]healthdomain.BackendCheckConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var checks []healthdomain.BackendCheckConfig
	if err := json.Unmarshal([]byte(trimmed), &checks); err != nil {
		return nil, fmt.Errorf("HEALTH_EXTRA_CHECKS_JSON: json inválido: %w", err)
	}
	out := make([]healthdomain.BackendCheckConfig, 0, len(checks))
	seen := make(map[string]struct{}, len(checks))
	for i, check := range checks {
		name := strings.TrimSpace(strings.ToLower(check.Name))
		target := strings.TrimSpace(check.Target)
		if name == "" {
			return nil, fmt.Errorf("HEALTH_EXTRA_CHECKS_JSON[%d]: campo 'name' vacío", i)
		}
		if target == "" {
			return nil, fmt.Errorf("HEALTH_EXTRA_CHECKS_JSON[%d] (%s): campo 'target' vacío", i, name)
		}
		if check.Type != "" && check.Type != healthdomain.CheckTypeHTTP && check.Type != healthdomain.CheckTypeTCP {
			return nil, fmt.Errorf("HEALTH_EXTRA_CHECKS_JSON[%d] (%s): type '%s' no soportado (usar http o tcp)", i, name, check.Type)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("HEALTH_EXTRA_CHECKS_JSON[%d]: nombre duplicado '%s'", i, name)
		}
		seen[name] = struct{}{}
		check.Name = name
		check.Target = target
		if check.Type == "" {
			check.Type = healthdomain.CheckTypeHTTP
		}
		if check.Path == "" && check.Type == healthdomain.CheckTypeHTTP {
			check.Path = "/"
		}
		out = append(out, check)
	}
	return out, nil
}

func appendIfTarget(checks []healthdomain.BackendCheckConfig, check healthdomain.BackendCheckConfig) []healthdomain.BackendCheckConfig {
	if strings.TrimSpace(check.Target) == "" {
		return checks
	}
	return append(checks, check)
}

func (s *Service) runCheck(parentCtx context.Context, check healthdomain.BackendCheckConfig) healthdomain.DependencyStatus {
	status := healthdomain.DependencyStatus{
		Status:   "unhealthy",
		Type:     string(check.Type),
		Target:   check.Target,
		Required: check.Required,
	}

	timeout := s.defaultTimeout
	if check.TimeoutMS > 0 {
		timeout = time.Duration(check.TimeoutMS) * time.Millisecond
	}

	switch check.Type {
	case healthdomain.CheckTypeHTTP:
		return s.checkHTTP(parentCtx, check, timeout)
	case healthdomain.CheckTypeTCP:
		return s.checkTCP(parentCtx, check, timeout)
	default:
		status.Error = "unsupported check type"
		return status
	}
}

func (s *Service) checkHTTP(parentCtx context.Context, check healthdomain.BackendCheckConfig, timeout time.Duration) healthdomain.DependencyStatus {
	status := healthdomain.DependencyStatus{
		Status:   "unhealthy",
		Type:     string(check.Type),
		Target:   check.Target,
		Required: check.Required,
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	path := check.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	target := strings.TrimRight(check.Target, "/") + path
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		status.LatencyMS = time.Since(start).Milliseconds()
		status.Error = err.Error()
		return status
	}

	resp, err := s.client.Do(req)
	status.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
		status.Status = "healthy"
		return status
	}
	status.Error = "unexpected status code"
	return status
}

func (s *Service) checkTCP(parentCtx context.Context, check healthdomain.BackendCheckConfig, timeout time.Duration) healthdomain.DependencyStatus {
	status := healthdomain.DependencyStatus{
		Status:   "unhealthy",
		Type:     string(check.Type),
		Target:   check.Target,
		Required: check.Required,
	}
	hostPort := hostPortFromRaw(check.Target)
	if hostPort == "" {
		status.Error = "invalid target"
		return status
	}

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()
	start := time.Now()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	status.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	_ = conn.Close()
	status.Status = "healthy"
	return status
}

func hostPortFromRaw(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "tcp://" + trimmed
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

func aggregateStatus(checks []healthdomain.BackendCheckConfig, deps map[string]healthdomain.DependencyStatus) string {
	required := make([]string, 0, len(checks))
	for _, check := range checks {
		if check.Required {
			required = append(required, check.Name)
		}
	}
	if len(required) == 0 {
		for _, check := range checks {
			required = append(required, check.Name)
		}
	}
	if len(required) == 0 {
		return "healthy"
	}

	healthy := 0
	for _, name := range required {
		if dep, ok := deps[name]; ok && dep.Status == "healthy" {
			healthy++
		}
	}

	switch {
	case healthy == len(required):
		return "healthy"
	case healthy == 0:
		return "unhealthy"
	default:
		return "degraded"
	}
}

func (s *Service) breakersSnapshot() map[string]resilience.Snapshot {
	breakers := map[string]resilience.Snapshot{}
	if s == nil {
		return breakers
	}

	if s.ollamaCB != nil {
		breakers["ollama"] = s.ollamaCB.CircuitBreakerState()
	} else {
		breakers["ollama"] = resilience.Snapshot{Name: "ollama", State: resilience.StateClosed}
	}

	if s.qdrantCB != nil {
		breakers["qdrant"] = s.qdrantCB.CircuitBreakerState()
	} else {
		breakers["qdrant"] = resilience.Snapshot{Name: "qdrant", State: resilience.StateClosed}
	}

	return breakers
}
