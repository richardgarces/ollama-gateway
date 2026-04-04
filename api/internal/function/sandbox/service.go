package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ollama-gateway/internal/function/core/domain"
	guardrailsservice "ollama-gateway/internal/function/guardrails"
)

type PatchService interface {
	ExtractDiff(response string) []domain.UnifiedDiff
	ApplyPatch(repoRoot string, diff domain.UnifiedDiff) error
}

type PreviewResult struct {
	Valid         bool                 `json:"valid"`
	SandboxPath   string               `json:"sandbox_path,omitempty"`
	Diffs         []domain.UnifiedDiff `json:"diffs"`
	AffectedFiles []string             `json:"affected_files"`
	CompileCheck  string               `json:"compile_check"`
	Errors        []string             `json:"errors,omitempty"`
}

type ApplyResult struct {
	Applied      bool          `json:"applied"`
	AppliedCount int           `json:"applied_count"`
	Preview      PreviewResult `json:"preview"`
	Guardrails   interface{}   `json:"guardrails,omitempty"`
}

type Service struct {
	repoRoot   string
	patch      PatchService
	guardrails *guardrailsservice.Service
	logger     *slog.Logger
}

func NewService(repoRoot string, patch PatchService, guardrails *guardrailsservice.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repoRoot: repoRoot, patch: patch, guardrails: guardrails, logger: logger}
}

func (s *Service) Preview(response string) (PreviewResult, error) {
	if s == nil || s.patch == nil {
		return PreviewResult{}, errors.New("sandbox service no disponible")
	}
	if strings.TrimSpace(response) == "" {
		return PreviewResult{}, errors.New("response requerido")
	}
	absRepo, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return PreviewResult{}, fmt.Errorf("repo root invalido: %w", err)
	}
	diffs := s.patch.ExtractDiff(response)
	if len(diffs) == 0 {
		return PreviewResult{}, errors.New("no se encontraron diffs en response")
	}

	sandboxRoot, err := os.MkdirTemp("", "patch-sandbox-*")
	if err != nil {
		return PreviewResult{}, err
	}
	defer os.RemoveAll(sandboxRoot)

	if err := copyRepo(absRepo, sandboxRoot); err != nil {
		return PreviewResult{}, err
	}

	errs := make([]string, 0)
	for _, d := range diffs {
		if err := s.patch.ApplyPatch(sandboxRoot, d); err != nil {
			errs = append(errs, err.Error())
		}
	}

	affected := collectAffectedFiles(diffs)
	compileErr := validateMinimalCompilation(sandboxRoot)
	compileCheck := "ok"
	if compileErr != nil {
		compileCheck = "failed"
		errs = append(errs, compileErr.Error())
	}

	result := PreviewResult{
		Valid:         len(errs) == 0,
		Diffs:         diffs,
		AffectedFiles: affected,
		CompileCheck:  compileCheck,
		Errors:        errs,
	}
	return result, nil
}

func (s *Service) ApplyValidated(response string) (ApplyResult, error) {
	preview, err := s.Preview(response)
	if err != nil {
		return ApplyResult{}, err
	}
	if !preview.Valid {
		return ApplyResult{Applied: false, AppliedCount: 0, Preview: preview}, errors.New("patch inválido en sandbox")
	}
	if s.guardrails != nil {
		evaluation := s.guardrails.EvaluateDiffs(preview.Diffs)
		if !evaluation.Allowed {
			return ApplyResult{Applied: false, AppliedCount: 0, Preview: preview, Guardrails: evaluation}, errors.New("guardrails bloquearon el apply del patch")
		}
	}

	applied := 0
	for _, d := range preview.Diffs {
		if err := s.patch.ApplyPatch(s.repoRoot, d); err != nil {
			return ApplyResult{Applied: false, AppliedCount: applied, Preview: preview}, err
		}
		applied++
	}
	return ApplyResult{Applied: true, AppliedCount: applied, Preview: preview}, nil
}

func copyRepo(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if shouldSkipPath(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func shouldSkipPath(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, ".git/") || rel == ".git" {
		return true
	}
	if strings.HasPrefix(rel, "node_modules/") || rel == "node_modules" {
		return true
	}
	if strings.HasPrefix(rel, ".idea/") || strings.HasPrefix(rel, ".vscode/") {
		return true
	}
	if isDir && strings.HasPrefix(filepath.Base(rel), ".cache") {
		return true
	}
	return false
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func collectAffectedFiles(diffs []domain.UnifiedDiff) []string {
	set := make(map[string]struct{})
	for _, d := range diffs {
		if p := strings.TrimSpace(d.NewPath); p != "" && p != "/dev/null" {
			set[p] = struct{}{}
		}
		if p := strings.TrimSpace(d.OldPath); p != "" && p != "/dev/null" {
			set[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func validateMinimalCompilation(repoRoot string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("validación de compilación falló: %s", msg)
	}
	return nil
}
