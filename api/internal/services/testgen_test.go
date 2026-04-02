package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveTestPath(t *testing.T) {
	got, err := deriveTestPath("/tmp/service.go")
	if err != nil {
		t.Fatalf("deriveTestPath() error = %v", err)
	}
	if got != "/tmp/service_test.go" {
		t.Fatalf("unexpected path: %s", got)
	}
}

func TestWriteTestsForFileCreatesTarget(t *testing.T) {
	repoRoot := t.TempDir()
	src := filepath.Join(repoRoot, "service.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write source error: %v", err)
	}

	svc := NewTestGenService(fakeRAG{response: "package main\n"}, repoRoot, nil)
	applied, backup, err := svc.WriteTestsForFile("service.go", "package main\nfunc TestX(t *testing.T) {}\n")
	if err != nil {
		t.Fatalf("WriteTestsForFile() error = %v", err)
	}
	if backup != "" {
		t.Fatalf("expected no backup on first write")
	}
	if filepath.Base(applied) != "service_test.go" {
		t.Fatalf("unexpected applied path: %s", applied)
	}
}

func TestGenerateTestsForFileRejectsOutsideRepo(t *testing.T) {
	repoRoot := t.TempDir()
	svc := NewTestGenService(fakeRAG{response: "ok"}, repoRoot, nil)
	outside := filepath.Join(os.TempDir(), "outside.go")
	if _, err := svc.GenerateTestsForFile(outside); err == nil {
		t.Fatalf("expected error for outside path")
	}
}
