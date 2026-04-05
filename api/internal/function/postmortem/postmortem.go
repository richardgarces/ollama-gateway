package postmortem

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

type IncidentInput struct {
	Logs       string             `json:"logs"`
	StartTime  string             `json:"start_time,omitempty"`
	EndTime    string             `json:"end_time,omitempty"`
	CommitHash string             `json:"commit_hash,omitempty"`
	Metrics    map[string]float64 `json:"metrics,omitempty"`
}

type TimelineEvent struct {
	Timestamp string `json:"timestamp,omitempty"`
	Level     string `json:"level,omitempty"`
	Summary   string `json:"summary"`
}

type IncidentReport struct {
	Timeline            []TimelineEvent `json:"timeline"`
	RootCauseHypothesis string          `json:"root_cause_hypothesis"`
	Impact              string          `json:"impact"`
	PreventiveActions   []string        `json:"preventive_actions"`
}

type PostmortemService struct {
	logger *slog.Logger
}

func NewPostmortemService(logger *slog.Logger) *PostmortemService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PostmortemService{logger: logger}
}

func (s *PostmortemService) AnalyzeIncident(input IncidentInput) (IncidentReport, error) {
	logs := strings.TrimSpace(input.Logs)
	if logs == "" {
		return IncidentReport{}, fmt.Errorf("logs requeridos")
	}

	lines := splitLogLines(logs)
	timeline := buildTimeline(lines)
	rootCause := inferRootCause(lines, input.Metrics)
	impact := inferImpact(lines, input.Metrics)
	actions := inferPreventiveActions(lines, input.Metrics, strings.TrimSpace(input.CommitHash))

	if strings.TrimSpace(input.StartTime) != "" || strings.TrimSpace(input.EndTime) != "" {
		window := strings.TrimSpace(input.StartTime) + " -> " + strings.TrimSpace(input.EndTime)
		timeline = append([]TimelineEvent{{
			Summary: "incident window: " + strings.TrimSpace(window),
			Level:   "info",
		}}, timeline...)
	}

	if commit := strings.TrimSpace(input.CommitHash); commit != "" {
		short := commit
		if len(short) > 12 {
			short = short[:12]
		}
		timeline = append([]TimelineEvent{{
			Summary: "related commit: " + short,
			Level:   "info",
		}}, timeline...)
	}

	return IncidentReport{
		Timeline:            timeline,
		RootCauseHypothesis: rootCause,
		Impact:              impact,
		PreventiveActions:   actions,
	}, nil
}

func splitLogLines(logs string) []string {
	raw := strings.Split(logs, "\n")
	lines := make([]string, 0, len(raw))
	for _, l := range raw {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}

func buildTimeline(lines []string) []TimelineEvent {
	events := make([]TimelineEvent, 0, minInt(len(lines), 10))
	for _, line := range lines {
		events = append(events, TimelineEvent{
			Timestamp: extractTimestamp(line),
			Level:     detectLevel(line),
			Summary:   summarizeLine(line),
		})
		if len(events) >= 10 {
			break
		}
	}
	if len(events) == 0 {
		events = append(events, TimelineEvent{Level: "info", Summary: "no structured log events detected"})
	}
	return events
}

func inferRootCause(lines []string, metrics map[string]float64) string {
	joined := strings.ToLower(strings.Join(lines, "\n"))
	if strings.Contains(joined, "timeout") || metricAbove(metrics, "latency_ms", 1000) {
		return "Probable saturation o dependencia lenta (timeouts/latencia elevada)."
	}
	if strings.Contains(joined, "connection refused") || strings.Contains(joined, "dial tcp") {
		return "Probable caida o indisponibilidad de servicio dependiente (network/connectivity)."
	}
	if strings.Contains(joined, "unauthorized") || strings.Contains(joined, "forbidden") || strings.Contains(joined, "jwt") {
		return "Probable fallo de autenticacion/autorizacion (credenciales token o permisos)."
	}
	if strings.Contains(joined, "panic") || strings.Contains(joined, "nil pointer") {
		return "Probable error de codigo no controlado (panic/nil pointer)."
	}
	if metricAbove(metrics, "error_rate", 0.05) || metricAbove(metrics, "errors", 0) {
		return "Probable regresion funcional reflejada en aumento de errores."
	}
	return "Causa raiz no concluyente; se requiere correlacion adicional con tracing y cambios recientes."
}

func inferImpact(lines []string, metrics map[string]float64) string {
	errCount := 0
	for _, l := range lines {
		ll := strings.ToLower(l)
		if strings.Contains(ll, "error") || strings.Contains(ll, "panic") || strings.Contains(ll, "fail") {
			errCount++
		}
	}

	parts := make([]string, 0, 3)
	if errCount > 0 {
		parts = append(parts, fmt.Sprintf("%d eventos de error relevantes en logs", errCount))
	}
	if v, ok := metricValue(metrics, "error_rate"); ok {
		parts = append(parts, fmt.Sprintf("error_rate=%.4f", v))
	}
	if v, ok := metricValue(metrics, "latency_ms"); ok {
		parts = append(parts, fmt.Sprintf("latency_ms=%.2f", v))
	}
	if len(parts) == 0 {
		return "Impacto moderado no cuantificado por falta de metricas suficientes."
	}
	return strings.Join(parts, " | ")
}

func inferPreventiveActions(lines []string, metrics map[string]float64, commitHash string) []string {
	actions := []string{
		"Agregar alertas de latencia/error por endpoint y umbrales de SLO.",
		"Incluir pruebas de regresion para el flujo afectado.",
		"Documentar runbook de mitigacion y rollback para este incidente.",
	}

	joined := strings.ToLower(strings.Join(lines, "\n"))
	if strings.Contains(joined, "timeout") || metricAbove(metrics, "latency_ms", 1000) {
		actions = append(actions, "Ajustar timeouts/retries y validar capacidad de dependencias externas.")
	}
	if strings.Contains(joined, "panic") || strings.Contains(joined, "nil pointer") {
		actions = append(actions, "Agregar validaciones defensivas y tests para nil/edge cases.")
	}
	if strings.TrimSpace(commitHash) != "" {
		actions = append(actions, "Revisar diff del commit relacionado y considerar rollback parcial controlado.")
	}

	sort.Strings(actions)
	return uniqueStrings(actions)
}

func extractTimestamp(line string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return ""
	}
	candidate := strings.TrimSpace(parts[0])
	if _, err := time.Parse(time.RFC3339, candidate); err == nil {
		return candidate
	}
	return ""
}

func detectLevel(line string) string {
	ll := strings.ToLower(line)
	switch {
	case strings.Contains(ll, " panic ") || strings.Contains(ll, "panic:"):
		return "panic"
	case strings.Contains(ll, " error ") || strings.Contains(ll, "level=error") || strings.Contains(ll, "[error]"):
		return "error"
	case strings.Contains(ll, " warn ") || strings.Contains(ll, "level=warn") || strings.Contains(ll, "[warn]"):
		return "warn"
	default:
		return "info"
	}
}

func summarizeLine(line string) string {
	if len(line) <= 220 {
		return line
	}
	return line[:220] + "..."
}

func metricAbove(metrics map[string]float64, key string, threshold float64) bool {
	v, ok := metricValue(metrics, key)
	return ok && v > threshold
}

func metricValue(metrics map[string]float64, key string) (float64, bool) {
	if metrics == nil {
		return 0, false
	}
	v, ok := metrics[key]
	return v, ok
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		t := strings.TrimSpace(v)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
