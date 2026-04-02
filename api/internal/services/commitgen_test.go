package services

import (
	"context"
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

func TestCommitGenGenerateMessageRejectsEmptyDiff(t *testing.T) {
	svc := NewCommitGenService(fakeRAG{response: "feat(api): add endpoint"}, t.TempDir(), nil)
	if _, err := svc.GenerateMessage("   "); err == nil {
		t.Fatalf("expected error for empty diff")
	}
}

func TestCommitGenGenerateMessageTruncatesLargeDiff(t *testing.T) {
	var capturedPrompt string
	rag := fakeRAGRecorder{onPrompt: func(prompt string) { capturedPrompt = prompt }, response: "fix(core): update"}
	svc := NewCommitGenService(rag, t.TempDir(), nil)
	big := strings.Repeat("a", commitGenMaxDiffChars+123)
	if _, err := svc.GenerateMessage(big); err != nil {
		t.Fatalf("GenerateMessage error: %v", err)
	}
	if len(capturedPrompt) == 0 {
		t.Fatalf("expected captured prompt")
	}
	if strings.Count(capturedPrompt, "a") < commitGenMaxDiffChars {
		t.Fatalf("expected large diff included in prompt")
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

func TestRunGitDiffCachedFailure(t *testing.T) {
	ctx := context.Background()
	if _, err := runGitDiffCached(ctx, "/path/does-not-exist"); err == nil {
		t.Fatalf("expected git diff helper to fail on invalid repo")
	}
}

type fakeRAGRecorder struct {
	onPrompt func(string)
	response string
}

func (f fakeRAGRecorder) GenerateWithContext(prompt string) (string, error) {
	if f.onPrompt != nil {
		f.onPrompt(prompt)
	}
	return f.response, nil
}

func (f fakeRAGRecorder) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	return nil
}
