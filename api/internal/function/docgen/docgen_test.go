package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripMarkdownFence(t *testing.T) {
	in := "```go\npackage a\n\nfunc X() {}\n```"
	got := stripMarkdownFence(in)
	if got != "package a\n\nfunc X() {}" {
		t.Fatalf("unexpected stripped content: %q", got)
	}
}

func TestDocGenResolveWithinRepoRejectsOutsidePath(t *testing.T) {
	repoRoot := t.TempDir()
	svc := NewDocGenService(fakeRAG{response: "ok"}, repoRoot, nil)

	outside := filepath.Join(os.TempDir(), "outside.go")
	if _, err := svc.resolveWithinRepo(outside); err == nil {
		t.Fatalf("expected error for outside path")
	}
}

func TestDocGenWriteWithBackup(t *testing.T) {
	repoRoot := t.TempDir()
	filePath := filepath.Join(repoRoot, "a.go")
	if err := os.WriteFile(filePath, []byte("package a\n"), 0o644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	svc := NewDocGenService(fakeRAG{response: "ok"}, repoRoot, nil)
	backup, err := svc.WriteWithBackup("a.go", "package a\n// doc\n")
	if err != nil {
		t.Fatalf("WriteWithBackup() error = %v", err)
	}

	backupData, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("read backup error: %v", err)
	}
	if string(backupData) != "package a\n" {
		t.Fatalf("backup content mismatch: %q", string(backupData))
	}

	current, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read updated file error: %v", err)
	}
	if string(current) != "package a\n// doc\n" {
		t.Fatalf("updated content mismatch: %q", string(current))
	}
}
