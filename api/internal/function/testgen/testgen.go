package service

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ollama-gateway/internal/function/core/domain"
)

type TestGenService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

func NewTestGenService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *TestGenService {
	if logger == nil {
		logger = slog.Default()
	}
	return &TestGenService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *TestGenService) GenerateTests(code, lang string) (string, error) {
	code = strings.TrimSpace(code)
	lang = strings.TrimSpace(strings.ToLower(lang))
	if code == "" {
		return "", fmt.Errorf("code requerido")
	}
	if lang == "" {
		return "", fmt.Errorf("lang requerido")
	}

	prompt := fmt.Sprintf(
		"Generate comprehensive unit tests for this %s code. Use subtests (t.Run) for Go. Include edge cases, error paths, and table-driven tests. Use testify/assert if the code imports it. Return only test code, without markdown fences.\\n\\nSource code:\\n%s",
		lang,
		code,
	)

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return stripMarkdownFence(out), nil
}

func (s *TestGenService) GenerateTestsForFile(path string) (string, error) {
	absPath, err := s.resolveWithinRepo(path)
	if err != nil {
		return "", err
	}

	lang := detectLanguage(absPath)
	if lang == "" {
		return "", fmt.Errorf("no se pudo detectar lenguaje por extensión")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	style := s.collectTestStyleContext(lang, absPath, 4)
	rel := s.relativePath(absPath)
	prompt := strings.Join([]string{
		fmt.Sprintf("Generate comprehensive unit tests for this %s code.", lang),
		"Use subtests (t.Run) for Go. Include edge cases, error paths, and table-driven tests.",
		"Use testify/assert if the code imports it.",
		"Return only test code, without markdown fences.",
		"Keep consistency with existing repository testing style.",
		"Source file:",
		rel,
		"Source code:",
		string(data),
		"Existing test style snippets:",
		style,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return stripMarkdownFence(out), nil
}

func (s *TestGenService) WriteTestsForFile(sourcePath, testCode string) (string, string, error) {
	absSource, err := s.resolveWithinRepo(sourcePath)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(testCode) == "" {
		return "", "", fmt.Errorf("test_code vacío")
	}

	targetPath, err := deriveTestPath(absSource)
	if err != nil {
		return "", "", err
	}

	backupPath := ""
	if existing, readErr := os.ReadFile(targetPath); readErr == nil {
		backupPath = targetPath + ".bak"
		if err := os.WriteFile(backupPath, existing, 0o644); err != nil {
			return "", "", err
		}
	}

	if err := os.WriteFile(targetPath, []byte(testCode), 0o644); err != nil {
		return "", "", err
	}
	return targetPath, backupPath, nil
}

func (s *TestGenService) resolveWithinRepo(path string) (string, error) {
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

func (s *TestGenService) relativePath(path string) string {
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(rootAbs, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func (s *TestGenService) collectTestStyleContext(lang, sourcePath string, limit int) string {
	if limit <= 0 {
		limit = 4
	}
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return ""
	}

	candidates := make([]string, 0, limit)
	_ = filepath.Walk(rootAbs, func(path string, info os.FileInfo, err error) error {
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
		if path == sourcePath {
			return nil
		}
		if isTestFileForLanguage(path, lang) {
			candidates = append(candidates, path)
		}
		return nil
	})

	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	var b strings.Builder
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		b.WriteString("FILE: ")
		b.WriteString(s.relativePath(p))
		b.WriteString("\n")
		snippet := string(data)
		if len(snippet) > 1800 {
			snippet = snippet[:1800]
		}
		b.WriteString(snippet)
		b.WriteString("\n\n")
	}
	return b.String()
}

func isTestFileForLanguage(path, lang string) bool {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))
	switch lang {
	case "go":
		return strings.HasSuffix(name, "_test.go")
	case "python":
		return strings.HasPrefix(name, "test_") && ext == ".py"
	case "javascript", "typescript":
		return strings.Contains(name, ".test.") || strings.Contains(name, ".spec.")
	default:
		return strings.Contains(name, "test")
	}
}

func deriveTestPath(sourcePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(sourcePath))
	base := strings.TrimSuffix(sourcePath, ext)
	name := filepath.Base(base)
	dir := filepath.Dir(sourcePath)

	switch ext {
	case ".go":
		return base + "_test.go", nil
	case ".py":
		return filepath.Join(dir, "test_"+name+".py"), nil
	case ".js", ".ts", ".java", ".rs", ".rb", ".php", ".c", ".cpp", ".cc", ".cxx", ".cs":
		return base + ".test" + ext, nil
	default:
		return "", fmt.Errorf("no se puede derivar archivo test para extensión %s", ext)
	}
}
