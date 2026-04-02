package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	if got := detectLanguage("a.go"); got != "go" {
		t.Fatalf("expected go, got %s", got)
	}
	if got := detectLanguage("b.ts"); got != "typescript" {
		t.Fatalf("expected typescript, got %s", got)
	}
	if got := detectLanguage("c.unknown"); got != "" {
		t.Fatalf("expected empty language, got %s", got)
	}
}

func TestTranslateFileRejectsOutsideRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	svc := NewTranslatorService(fakeRAG{response: "translated"}, repoRoot, nil)
	outside := filepath.Join(os.TempDir(), "outside.go")
	if _, err := svc.TranslateFile(outside, "python"); err == nil {
		t.Fatalf("expected error for path outside repo root")
	}
}

func TestTranslateFileReadsInsideRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	in := filepath.Join(repoRoot, "service.go")
	if err := os.WriteFile(in, []byte("package main\nfunc A(){}\n"), 0o644); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	svc := NewTranslatorService(fakeRAG{response: "```python\ndef a():\n    pass\n```"}, repoRoot, nil)
	out, err := svc.TranslateFile("service.go", "python")
	if err != nil {
		t.Fatalf("TranslateFile() error = %v", err)
	}
	if out == "" || out == "```python\ndef a():\n    pass\n```" {
		t.Fatalf("expected cleaned translated content")
	}
}
