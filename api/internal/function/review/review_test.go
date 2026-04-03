package service

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeRAG struct {
	response string
}

func (f fakeRAG) GenerateWithContext(prompt string) (string, error) {
	return f.response, nil
}

func (f fakeRAG) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	return nil
}

func TestParseReviewCommentsExtractsJSONFromText(t *testing.T) {
	raw := "analysis before\n[{\"file\":\"a.go\",\"line\":10,\"severity\":\"HIGH\",\"comment\":\"check nil\"}]\nanalysis after"
	comments, err := parseReviewComments(raw)
	if err != nil {
		t.Fatalf("parseReviewComments() error = %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Severity != "high" {
		t.Fatalf("expected normalized severity high, got %s", comments[0].Severity)
	}
}

func TestReviewFileRejectsPathOutsideRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	outside := filepath.Join(os.TempDir(), "outside-review-file.go")
	_ = os.WriteFile(outside, []byte("package main"), 0o644)
	defer os.Remove(outside)

	svc := NewReviewService(fakeRAG{response: "[]"}, repoRoot, nil)
	if _, err := svc.ReviewFile(outside); err == nil {
		t.Fatalf("expected error for path outside repo root")
	}
}

func TestReviewFileReadsWithinRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	file := filepath.Join(repoRoot, "internal", "x.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	svc := NewReviewService(fakeRAG{response: `[{"line":1,"severity":"low","comment":"ok"}]`}, repoRoot, nil)
	comments, err := svc.ReviewFile("internal/x.go")
	if err != nil {
		t.Fatalf("ReviewFile() error = %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].File != filepath.Join("internal", "x.go") {
		t.Fatalf("expected relative file field, got %s", comments[0].File)
	}
}
