package services

import (
	"testing"
)

func TestParseArchReportWithStringRecommendations(t *testing.T) {
	raw := `{"score_1_10":8,"strengths":["clean layers"],"weaknesses":["tight coupling"],"recommendations":["extract interface"],"dependency_graph":{"a.go":["b.go"]}}`
	report, err := parseArchReport(raw)
	if err != nil {
		t.Fatalf("parseArchReport() error = %v", err)
	}
	if report.Score1To10 != 8 {
		t.Fatalf("expected score 8, got %d", report.Score1To10)
	}
	if len(report.Recommendations) != 1 {
		t.Fatalf("expected one recommendation")
	}
	if report.Recommendations[0].Title == "" {
		t.Fatalf("expected recommendation title populated")
	}
}

func TestBuildArchitectureContextLimit(t *testing.T) {
	svc := NewArchitectService(fakeRAG{response: "{}"}, ".", nil, nil)
	files := []archCandidate{
		{path: "a.go", size: 100, complexity: 2},
	}
	ctx := svc.buildArchitectureContext(files, 50)
	if len(ctx) > 120 { // includes header + snippet cap
		t.Fatalf("expected bounded context, got len=%d", len(ctx))
	}
}
