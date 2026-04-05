package postmortem

import "testing"

func TestAnalyzeIncidentSuccess(t *testing.T) {
	svc := NewPostmortemService(nil)
	report, err := svc.AnalyzeIncident(IncidentInput{
		Logs:       "2026-04-04T10:00:00Z level=error timeout contacting qdrant",
		CommitHash: "abc123def456",
		Metrics: map[string]float64{
			"latency_ms": 1735,
			"error_rate": 0.12,
		},
	})
	if err != nil {
		t.Fatalf("AnalyzeIncident() error = %v", err)
	}
	if len(report.Timeline) == 0 {
		t.Fatalf("expected timeline events")
	}
	if report.RootCauseHypothesis == "" {
		t.Fatalf("expected root cause hypothesis")
	}
	if report.Impact == "" {
		t.Fatalf("expected impact summary")
	}
	if len(report.PreventiveActions) == 0 {
		t.Fatalf("expected preventive actions")
	}
}

func TestAnalyzeIncidentRequiresLogs(t *testing.T) {
	svc := NewPostmortemService(nil)
	_, err := svc.AnalyzeIncident(IncidentInput{})
	if err == nil {
		t.Fatalf("expected error for missing logs")
	}
}
