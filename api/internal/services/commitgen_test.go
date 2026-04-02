package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitGenGenerateMessage(t *testing.T) {
	svc := NewCommitGenService(fakeRAG{response: "feat(api): add endpoint"}, t.TempDir(), nil)
	out, err := svc.GenerateMessage("diff --git a/a.go b/a.go")
	if err != nil {
		t.Fatalf("GenerateMessage() error = %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected non-empty commit message")
	}
}

func TestCommitGenGenerateFromStagedRejectsOutsideRepoRoot(t *testing.T) {
	svc := NewCommitGenService(fakeRAG{response: "feat(core): x"}, t.TempDir(), nil)
	if _, err := svc.GenerateFromStaged(os.TempDir()); err == nil {
		t.Fatalf("expected error for path outside REPO_ROOT")
	}
}

func TestCommitGenGenerateFromStaged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no disponible en entorno de test")
	}

	repoRoot := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")

	file := filepath.Join(repoRoot, "main.go")
	if err := os.WriteFile(file, []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write file error: %v", err)
	}
	run("add", "main.go")

	svc := NewCommitGenService(fakeRAG{response: "feat(go): add main entrypoint"}, repoRoot, nil)
	out, err := svc.GenerateFromStaged("")
	if err != nil {
		t.Fatalf("GenerateFromStaged() error = %v", err)
	}
	if !strings.Contains(out, "feat") {
		t.Fatalf("unexpected commit message: %q", out)
	}
}
