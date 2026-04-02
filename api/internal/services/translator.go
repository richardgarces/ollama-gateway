package services

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"ollama-gateway/internal/domain"
)

type TranslatorService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

func NewTranslatorService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *TranslatorService {
	if logger == nil {
		logger = slog.Default()
	}
	return &TranslatorService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *TranslatorService) Translate(code, fromLang, toLang string) (string, error) {
	code = strings.TrimSpace(code)
	fromLang = strings.TrimSpace(strings.ToLower(fromLang))
	toLang = strings.TrimSpace(strings.ToLower(toLang))
	if code == "" {
		return "", fmt.Errorf("code requerido")
	}
	if fromLang == "" {
		fromLang = "unknown"
	}
	if toLang == "" {
		return "", fmt.Errorf("to requerido")
	}

	prompt := fmt.Sprintf(
		"Translate this %s code to idiomatic %s. Preserve logic, add appropriate error handling for the target language, and include necessary imports. Return only code, no markdown fences.\\n\\nSource code:\\n%s",
		fromLang,
		toLang,
		code,
	)

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return stripMarkdownFence(out), nil
}

func (s *TranslatorService) TranslateFile(path, toLang string) (string, error) {
	absPath, err := s.resolveWithinRepo(path)
	if err != nil {
		return "", err
	}

	fromLang := detectLanguage(absPath)
	if fromLang == "" {
		return "", fmt.Errorf("no se pudo detectar lenguaje por extensión")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	relPath := s.relativePath(absPath)
	prompt := strings.Join([]string{
		fmt.Sprintf("Translate this %s code to idiomatic %s. Preserve logic, add appropriate error handling for the target language, and include necessary imports.", fromLang, toLang),
		"Project coherence requirement: keep compatibility with existing interfaces, types and conventions from this repository context.",
		"Return only translated code, without markdown fences.",
		"Source path:",
		relPath,
		"Source code:",
		string(data),
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return stripMarkdownFence(out), nil
}

func (s *TranslatorService) resolveWithinRepo(path string) (string, error) {
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

func (s *TranslatorService) relativePath(path string) string {
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

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".cs":
		return "csharp"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".c":
		return "c"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	default:
		return ""
	}
}
