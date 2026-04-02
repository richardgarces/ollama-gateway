package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ollama-gateway/internal/domain"
)

type CommitGenService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

const (
	commitGenMaxDiffChars = 120000
	commitGenGitTimeout   = 5 * time.Second
)

func NewCommitGenService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *CommitGenService {
	if logger == nil {
		logger = slog.Default()
	}
	return &CommitGenService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *CommitGenService) GenerateMessage(diff string) (string, error) {
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return "", fmt.Errorf("diff requerido")
	}

	if len(diff) > commitGenMaxDiffChars {
		diff = diff[:commitGenMaxDiffChars]
	}

	prompt := strings.Join([]string{
		"Generate a concise git commit message following Conventional Commits format for this diff.",
		"Format: type(scope): description.",
		"Types allowed: feat, fix, refactor, docs, test, chore.",
		"Include a body with bullet points if the change is complex.",
		"Return only the commit message text without markdown fences.",
		"Diff:",
		diff,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}

	msg := strings.TrimSpace(stripMarkdownFence(out))
	if msg == "" {
		return "", fmt.Errorf("no se pudo generar commit message")
	}
	return msg, nil
}

func (s *CommitGenService) GenerateFromStaged(repoRoot string) (string, error) {
	rootAbs, err := s.resolveRepoRoot(repoRoot)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), commitGenGitTimeout)
	defer cancel()

	out, err := runGitDiffCached(ctx, rootAbs)
	if err != nil {
		return "", err
	}

	diff := strings.TrimSpace(string(out))
	if diff == "" {
		return "", fmt.Errorf("no hay cambios staged")
	}

	return s.GenerateMessage(diff)
}

func runGitDiffCached(ctx context.Context, rootAbs string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", rootAbs, "diff", "--cached")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("timeout ejecutando git diff --cached")
	}
	stderr := strings.TrimSpace(string(out))
	if stderr != "" {
		return nil, fmt.Errorf("git diff --cached falló: %s", stderr)
	}
	return nil, fmt.Errorf("git diff --cached falló")
}

func (s *CommitGenService) resolveRepoRoot(input string) (string, error) {
	base, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return "", fmt.Errorf("REPO_ROOT inválido")
	}

	candidate := strings.TrimSpace(input)
	if candidate == "" {
		candidate = base
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("repoRoot inválido")
	}

	baseResolved := base
	if rb, err := filepath.EvalSymlinks(base); err == nil {
		baseResolved = rb
	}
	absResolved := abs
	if ra, err := filepath.EvalSymlinks(abs); err == nil {
		absResolved = ra
	}

	if absResolved != baseResolved && !strings.HasPrefix(absResolved, baseResolved+string(os.PathSeparator)) {
		return "", fmt.Errorf("repoRoot fuera de REPO_ROOT")
	}
	return abs, nil
}
