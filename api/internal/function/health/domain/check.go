package domain

import "ollama-gateway/internal/function/resilience"

type CheckType string

const (
	CheckTypeHTTP CheckType = "http"
	CheckTypeTCP  CheckType = "tcp"
)

type BackendCheckConfig struct {
	Name      string    `json:"name"`
	Type      CheckType `json:"type"`
	Target    string    `json:"target"`
	Path      string    `json:"path,omitempty"`
	TimeoutMS int       `json:"timeout_ms,omitempty"`
	Required  bool      `json:"required"`
}

type DependencyStatus struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
	Type      string `json:"type"`
	Target    string `json:"target"`
	Required  bool   `json:"required"`
	Error     string `json:"error,omitempty"`
}

type ReadinessResponse struct {
	Status       string                         `json:"status"`
	Dependencies map[string]DependencyStatus    `json:"dependencies"`
	Breakers     map[string]resilience.Snapshot `json:"breakers"`
}
