package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"ollama-gateway/internal/domain"
)

var stackFilePattern = regexp.MustCompile(`[A-Za-z0-9_./\\-]+\.go`)

type DebugService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

func NewDebugService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *DebugService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DebugService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *DebugService) AnalyzeError(stackTrace string) (domain.DebugAnalysis, error) {
	stackTrace = strings.TrimSpace(stackTrace)
	if stackTrace == "" {
		return domain.DebugAnalysis{}, fmt.Errorf("stack_trace requerido")
	}

	files := s.extractRelatedFiles(stackTrace)
	ctx := s.collectFileContext(files, 3500)

	prompt := strings.Join([]string{
		"You are a debugging expert.",
		"Analyze this Go stack trace. Return JSON: {root_cause, explanation, suggested_fixes: [], related_files: []}",
		"Return ONLY valid JSON.",
		"Stack trace:",
		stackTrace,
		"Related files inferred from stack trace:",
		strings.Join(files, "\n"),
		"File snippets:",
		ctx,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return domain.DebugAnalysis{}, err
	}

	analysis, err := parseDebugAnalysis(out)
	if err != nil {
		return domain.DebugAnalysis{}, err
	}
	if len(analysis.RelatedFiles) == 0 && len(files) > 0 {
		analysis.RelatedFiles = files
	}
	return analysis, nil
}

func (s *DebugService) AnalyzeLog(logLines string) (domain.DebugAnalysis, error) {
	logLines = strings.TrimSpace(logLines)
	if logLines == "" {
		return domain.DebugAnalysis{}, fmt.Errorf("log requerido")
	}

	files := s.extractRelatedFiles(logLines)
	ctx := s.collectFileContext(files, 3000)

	prompt := strings.Join([]string{
		"You are a debugging expert.",
		"Analyze these server logs for probable root cause and actionable fixes.",
		"Return JSON: {root_cause, explanation, suggested_fixes: [], related_files: []}",
		"Return ONLY valid JSON.",
		"Logs:",
		logLines,
		"Potential related files:",
		strings.Join(files, "\n"),
		"File snippets:",
		ctx,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return domain.DebugAnalysis{}, err
	}

	analysis, err := parseDebugAnalysis(out)
	if err != nil {
		return domain.DebugAnalysis{}, err
	}
	if len(analysis.RelatedFiles) == 0 && len(files) > 0 {
		analysis.RelatedFiles = files
	}
	return analysis, nil
}

func parseDebugAnalysis(raw string) (domain.DebugAnalysis, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return domain.DebugAnalysis{}, fmt.Errorf("respuesta vacía del analizador")
	}

	var out domain.DebugAnalysis
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		jsonObj := extractJSONObject(text)
		if jsonObj == "" {
			return domain.DebugAnalysis{}, fmt.Errorf("respuesta del debugger no es JSON válido")
		}
		if err := json.Unmarshal([]byte(jsonObj), &out); err != nil {
			return domain.DebugAnalysis{}, fmt.Errorf("respuesta del debugger no es JSON válido")
		}
	}

	out.RootCause = strings.TrimSpace(out.RootCause)
	out.Explanation = strings.TrimSpace(out.Explanation)
	for i := range out.SuggestedFixes {
		out.SuggestedFixes[i] = strings.TrimSpace(out.SuggestedFixes[i])
	}
	for i := range out.RelatedFiles {
		out.RelatedFiles[i] = strings.TrimSpace(out.RelatedFiles[i])
	}
	return out, nil
}

func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func (s *DebugService) extractRelatedFiles(text string) []string {
	matches := stackFilePattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	files := make([]string, 0, len(matches))
	for _, m := range matches {
		clean := strings.TrimSpace(strings.ReplaceAll(m, "\\", "/"))
		if clean == "" || strings.HasSuffix(clean, "_test.go") {
			continue
		}
		abs, rel, err := s.normalizeFilePath(clean)
		if err != nil {
			continue
		}
		key := abs
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		files = append(files, rel)
	}
	sort.Strings(files)
	return files
}

func (s *DebugService) normalizeFilePath(path string) (string, string, error) {
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return "", "", err
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(rootAbs, filepath.FromSlash(candidate))
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", "", err
	}
	if abs != rootAbs && !strings.HasPrefix(abs, rootAbs+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("outside repo root")
	}

	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		return "", "", err
	}
	return abs, filepath.ToSlash(rel), nil
}

func (s *DebugService) collectFileContext(files []string, maxBytes int) string {
	if len(files) == 0 {
		return ""
	}
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return ""
	}

	if maxBytes <= 0 {
		maxBytes = 3000
	}
	var b strings.Builder
	remaining := maxBytes

	for _, rel := range files {
		if remaining <= 0 {
			break
		}
		abs := filepath.Join(rootAbs, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		snippet := string(data)
		if len(snippet) > remaining {
			snippet = snippet[:remaining]
		}
		b.WriteString("FILE: ")
		b.WriteString(rel)
		b.WriteString("\n")
		b.WriteString(snippet)
		b.WriteString("\n\n")
		remaining = maxBytes - b.Len()
	}
	return b.String()
}
