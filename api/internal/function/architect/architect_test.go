package service

import (
	"os"
	"path/filepath"
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

func TestDetectPatternRefactors(t *testing.T) {
	repoRoot := t.TempDir()
	file := filepath.Join(repoRoot, "internal", "sample", "sample.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	content := "package sample\n\nimport \"fmt\"\n\nfunc A() {\n"
	for i := 0; i < 10; i++ {
		content += "if err := do(); err != nil { return }\n"
	}
	content += "fmt.Println(\"debug\")\n}\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	svc := NewArchitectService(fakeRAG{response: "{}"}, repoRoot, nil, nil)
	report, err := svc.DetectPatternRefactors("internal/sample/sample.go")
	if err != nil {
		t.Fatalf("DetectPatternRefactors() error = %v", err)
	}
	if len(report.Suggestions) == 0 {
		t.Fatalf("expected suggestions")
	}
	if report.RiskScore <= 0 {
		t.Fatalf("expected risk score > 0")
	}
}
