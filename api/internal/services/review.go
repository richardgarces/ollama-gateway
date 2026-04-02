package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ollama-gateway/internal/domain"
)

type ReviewService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

func NewReviewService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *ReviewService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReviewService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *ReviewService) ReviewDiff(diff string) ([]domain.ReviewComment, error) {
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return nil, fmt.Errorf("diff requerido")
	}

	prompt := strings.Join([]string{
		"You are a senior code reviewer.",
		"Analyze this unified diff and return ONLY a JSON array of objects with fields:",
		"{file, line, severity, comment}.",
		"Severity must be one of: low, medium, high, critical.",
		"Do not include markdown, explanations, or code fences.",
		"Unified diff:",
		diff,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return nil, err
	}

	return parseReviewComments(out)
}

func (s *ReviewService) ReviewFile(path string) ([]domain.ReviewComment, error) {
	absPath, err := s.resolveWithinRepo(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	relPath := absPath
	if rootAbs, rootErr := filepath.Abs(s.repoRoot); rootErr == nil {
		if rel, relErr := filepath.Rel(rootAbs, absPath); relErr == nil {
			relPath = rel
		}
	}

	prompt := strings.Join([]string{
		"You are a senior code reviewer.",
		"Review this source file and return ONLY a JSON array of objects with fields:",
		"{file, line, severity, comment}.",
		"Severity must be one of: low, medium, high, critical.",
		"Focus on correctness, security, regressions, maintainability, and tests.",
		"Do not include markdown, explanations, or code fences.",
		"File:",
		relPath,
		"Content:",
		string(data),
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return nil, err
	}

	comments, err := parseReviewComments(out)
	if err != nil {
		return nil, err
	}

	for i := range comments {
		if strings.TrimSpace(comments[i].File) == "" {
			comments[i].File = relPath
		}
	}
	return comments, nil
}

func (s *ReviewService) resolveWithinRepo(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path requerido")
	}

	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return "", fmt.Errorf("repo root inválido")
	}

	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, candidate)
	}
	absPath, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("path inválido")
	}

	rootResolved := rootAbs
	if resolvedRoot, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootResolved = resolvedRoot
	}
	pathResolved := absPath
	if resolvedPath, err := filepath.EvalSymlinks(absPath); err == nil {
		pathResolved = resolvedPath
	}

	if pathResolved != rootResolved && !strings.HasPrefix(pathResolved, rootResolved+string(os.PathSeparator)) {
		return "", fmt.Errorf("path fuera de REPO_ROOT")
	}

	return absPath, nil
}

func parseReviewComments(raw string) ([]domain.ReviewComment, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return []domain.ReviewComment{}, nil
	}

	var comments []domain.ReviewComment
	if err := json.Unmarshal([]byte(text), &comments); err != nil {
		jsonArray := extractJSONArray(text)
		if jsonArray == "" {
			return nil, fmt.Errorf("respuesta del reviewer no es JSON válido")
		}
		if err := json.Unmarshal([]byte(jsonArray), &comments); err != nil {
			return nil, fmt.Errorf("respuesta del reviewer no es JSON válido")
		}
	}

	for i := range comments {
		comments[i].File = strings.TrimSpace(comments[i].File)
		comments[i].Comment = strings.TrimSpace(comments[i].Comment)
		comments[i].Severity = normalizeSeverity(comments[i].Severity)
		if comments[i].Line < 0 {
			comments[i].Line = 0
		}
	}

	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].File == comments[j].File {
			return comments[i].Line < comments[j].Line
		}
		return comments[i].File < comments[j].File
	})
	return comments, nil
}

func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func normalizeSeverity(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "low", "medium", "high", "critical":
		return v
	default:
		return "medium"
	}
}

func BuildReviewSummary(comments []domain.ReviewComment) string {
	if len(comments) == 0 {
		return "Sin observaciones"
	}
	counts := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}
	for _, c := range comments {
		counts[normalizeSeverity(c.Severity)]++
	}
	return fmt.Sprintf("%d comentarios (critical=%d, high=%d, medium=%d, low=%d)",
		len(comments), counts["critical"], counts["high"], counts["medium"], counts["low"])
}
