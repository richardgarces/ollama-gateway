package services

import (
	"testing"
)

func TestParseDebugAnalysisExtractsJSONObject(t *testing.T) {
	raw := "analysis:\n{\"root_cause\":\"nil pointer\",\"explanation\":\"x\",\"suggested_fixes\":[\"check nil\"],\"related_files\":[\"a/b.go\"]}\n-- end"
	got, err := parseDebugAnalysis(raw)
	if err != nil {
		t.Fatalf("parseDebugAnalysis() error = %v", err)
	}
	if got.RootCause != "nil pointer" {
		t.Fatalf("unexpected root cause: %s", got.RootCause)
	}
	if len(got.SuggestedFixes) != 1 {
		t.Fatalf("expected one suggested fix")
	}
}

func TestExtractRelatedFiles(t *testing.T) {
	svc := NewDebugService(fakeRAG{response: "{}"}, ".", nil)
	files := svc.extractRelatedFiles("panic at internal/services/rag.go:20 and cmd/server/main.go:10")
	if len(files) == 0 {
		t.Fatalf("expected at least one related file")
	}
}
