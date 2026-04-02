package services

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ollama-gateway/internal/domain"
)

type DocGenService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

func NewDocGenService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *DocGenService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DocGenService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *DocGenService) GenerateDocForFile(path string) (string, error) {
	absPath, err := s.resolveWithinRepo(path)
	if err != nil {
		return "", err
	}
	if strings.ToLower(filepath.Ext(absPath)) != ".go" {
		return "", fmt.Errorf("solo se permiten archivos .go")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	rel := s.relativePath(absPath)
	prompt := strings.Join([]string{
		"Generate Go doc comments for all exported functions, types and methods in this file.",
		"Return the complete file with comments added.",
		"Respect existing project conventions and formatting style.",
		"Do not include markdown fences.",
		"File path:",
		rel,
		"Source code:",
		string(data),
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return cleanupGeneratedCode(out), nil
}

func (s *DocGenService) GenerateREADME(repoRoot string) (string, error) {
	root, err := s.resolveRepoRoot(repoRoot)
	if err != nil {
		return "", err
	}

	files, err := s.pickMainFiles(root, 20)
	if err != nil {
		return "", err
	}

	var snippets []string
	for _, p := range files {
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			continue
		}
		rel := s.relativePath(p)
		snippets = append(snippets, "FILE: "+rel+"\n"+truncate(string(data), 4000))
	}

	prompt := strings.Join([]string{
		"Generate a complete README.md for this project.",
		"The README must include sections: description, installation, usage, API endpoints, configuration, architecture.",
		"Use concise and practical language.",
		"Respect existing repository conventions.",
		"Return only markdown content for README.md.",
		"Project context (sampled files):",
		strings.Join(snippets, "\n\n---\n\n"),
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripMarkdownFence(out)), nil
}

func (s *DocGenService) WriteWithBackup(path string, content string) (string, error) {
	absPath, err := s.resolveWithinRepo(path)
	if err != nil {
		return "", err
	}

	original, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	backupPath := absPath + ".bak"
	if err := os.WriteFile(backupPath, original, 0o644); err != nil {
		return "", err
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(absPath); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(absPath, []byte(content), mode); err != nil {
		return "", err
	}
	return backupPath, nil
}

func (s *DocGenService) resolveRepoRoot(input string) (string, error) {
	base, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return "", fmt.Errorf("REPO_ROOT inválido")
	}
	candidate := strings.TrimSpace(input)
	if candidate == "" {
		return base, nil
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("repoRoot inválido")
	}
	if abs != base && !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("repoRoot fuera de REPO_ROOT")
	}
	return abs, nil
}

func (s *DocGenService) resolveWithinRepo(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path requerido")
	}

	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return "", fmt.Errorf("REPO_ROOT inválido")
	}

	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	absPath, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("path inválido")
	}

	if absPath != rootAbs && !strings.HasPrefix(absPath, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path fuera de REPO_ROOT")
	}
	return absPath, nil
}

func (s *DocGenService) relativePath(path string) string {
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(rootAbs, path)
	if err != nil {
		return path
	}
	return rel
}

func (s *DocGenService) pickMainFiles(root string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	candidates := make([]string, 0, limit)
	priority := make([]string, 0, limit)

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}

		name := strings.ToLower(info.Name())
		ext := strings.ToLower(filepath.Ext(path))
		if name == "readme.md" || name == "makefile" || name == "dockerfile" || name == "go.mod" ||
			strings.HasSuffix(name, ".env") || ext == ".go" || ext == ".md" || ext == ".yml" || ext == ".yaml" {
			candidates = append(candidates, path)
			if name == "readme.md" || name == "go.mod" || strings.HasSuffix(path, string(os.PathSeparator)+"cmd"+string(os.PathSeparator)+"server"+string(os.PathSeparator)+"main.go") || strings.HasSuffix(path, string(os.PathSeparator)+"internal"+string(os.PathSeparator)+"server"+string(os.PathSeparator)+"server.go") {
				priority = append(priority, path)
			}
		}
		return nil
	})

	sort.Strings(priority)
	sort.Strings(candidates)
	seen := make(map[string]struct{})
	out := make([]string, 0, limit)

	appendUnique := func(items []string) {
		for _, p := range items {
			if len(out) >= limit {
				return
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}

	appendUnique(priority)
	appendUnique(candidates)
	return out, nil
}

func cleanupGeneratedCode(raw string) string {
	return strings.TrimSpace(stripMarkdownFence(raw))
}

func stripMarkdownFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return strings.Trim(text, "`")
	}
	start := 1
	end := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			end = i
			break
		}
	}
	if end <= start {
		return text
	}
	return strings.Join(lines[start:end], "\n")
}

func truncate(v string, max int) string {
	if max <= 0 || len(v) <= max {
		return v
	}
	return v[:max]
}
