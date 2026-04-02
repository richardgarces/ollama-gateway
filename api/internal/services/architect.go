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

type architectIndexer interface {
	IndexRepo() error
}

type ArchitectService struct {
	rag      domain.RAGEngine
	repoRoot string
	indexer  architectIndexer
	logger   *slog.Logger
}

type archCandidate struct {
	path       string
	size       int64
	complexity int
	score      int64
}

func NewArchitectService(rag domain.RAGEngine, repoRoot string, indexer architectIndexer, logger *slog.Logger) *ArchitectService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ArchitectService{rag: rag, repoRoot: repoRoot, indexer: indexer, logger: logger}
}

func (s *ArchitectService) AnalyzeProject() (domain.ArchReport, error) {
	if s.indexer != nil {
		if err := s.indexer.IndexRepo(); err != nil {
			s.logger.Warn("indexación previa de arquitectura falló", slog.String("error", err.Error()))
		}
	}

	files, err := s.selectTopFiles(20)
	if err != nil {
		return domain.ArchReport{}, err
	}
	if len(files) == 0 {
		return domain.ArchReport{}, fmt.Errorf("no se encontraron archivos para analizar")
	}

	context := s.buildArchitectureContext(files, 26000)
	prompt := strings.Join([]string{
		"Analyze this Go project architecture. Evaluate: SOLID compliance, dependency direction, separation of concerns, error handling patterns, cyclomatic complexity indicators. Return JSON: {score_1_10, strengths[], weaknesses[], recommendations[], dependency_graph:{}}.",
		"Recommendations should be concrete and actionable.",
		"Return ONLY valid JSON.",
		"Project context:",
		context,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return domain.ArchReport{}, err
	}
	return parseArchReport(out)
}

func (s *ArchitectService) SuggestRefactor(path string) (string, error) {
	absPath, err := s.resolveWithinRepo(path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}

	rel := s.relativePath(absPath)
	prompt := strings.Join([]string{
		"You are a senior Go architect.",
		"Analyze this file and suggest concrete refactorings with code examples.",
		"Focus on architecture, maintainability, dependency direction, error handling and testability.",
		"Return plain text markdown.",
		"File:",
		rel,
		"Source code:",
		string(data),
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripMarkdownFence(out)), nil
}

func parseArchReport(raw string) (domain.ArchReport, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return domain.ArchReport{}, fmt.Errorf("respuesta vacía del análisis")
	}

	jsonObj := extractJSONObjectArch(text)
	if jsonObj == "" {
		jsonObj = text
	}

	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(jsonObj), &generic); err != nil {
		return domain.ArchReport{}, fmt.Errorf("respuesta de arquitectura no es JSON válido")
	}

	report := domain.ArchReport{
		Strengths:       []string{},
		Weaknesses:      []string{},
		Recommendations: []domain.Recommendation{},
		DependencyGraph: map[string][]string{},
	}
	report.Score1To10 = asInt(generic["score_1_10"])
	report.Strengths = asStringSlice(generic["strengths"])
	report.Weaknesses = asStringSlice(generic["weaknesses"])
	report.Recommendations = asRecommendations(generic["recommendations"])
	report.DependencyGraph = asDependencyGraph(generic["dependency_graph"])

	if report.Score1To10 < 1 {
		report.Score1To10 = 1
	}
	if report.Score1To10 > 10 {
		report.Score1To10 = 10
	}
	return report, nil
}

func asInt(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return 0
		}
		var out int
		_, _ = fmt.Sscanf(t, "%d", &out)
		return out
	default:
		return 0
	}
}

func asStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func asRecommendations(v interface{}) []domain.Recommendation {
	arr, ok := v.([]interface{})
	if !ok {
		return []domain.Recommendation{}
	}
	out := make([]domain.Recommendation, 0, len(arr))
	for _, item := range arr {
		switch t := item.(type) {
		case string:
			t = strings.TrimSpace(t)
			if t != "" {
				out = append(out, domain.Recommendation{Title: t, Detail: t, Priority: "medium"})
			}
		case map[string]interface{}:
			r := domain.Recommendation{
				Title:    strings.TrimSpace(asString(t["title"])),
				Detail:   strings.TrimSpace(asString(t["detail"])),
				Priority: strings.TrimSpace(strings.ToLower(asString(t["priority"]))),
			}
			if r.Title == "" {
				r.Title = strings.TrimSpace(asString(t["name"]))
			}
			if r.Detail == "" {
				r.Detail = strings.TrimSpace(asString(t["description"]))
			}
			if r.Priority == "" {
				r.Priority = "medium"
			}
			if r.Title != "" || r.Detail != "" {
				out = append(out, r)
			}
		}
	}
	return out
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asDependencyGraph(v interface{}) map[string][]string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(m))
	for k, val := range m {
		out[k] = asStringSlice(val)
	}
	return out
}

func extractJSONObjectArch(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return text[start : end+1]
}

func (s *ArchitectService) selectTopFiles(limit int) ([]archCandidate, error) {
	if limit <= 0 {
		limit = 20
	}
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return nil, fmt.Errorf("REPO_ROOT inválido")
	}

	candidates := make([]archCandidate, 0, limit*2)
	err = filepath.Walk(rootAbs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isArchitectureFile(path) {
			return nil
		}
		complexity := estimateComplexity(path)
		score := int64(complexity*3000) + info.Size()
		candidates = append(candidates, archCandidate{path: path, size: info.Size(), complexity: complexity, score: score})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].path < candidates[j].path
		}
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func isArchitectureFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))
	if name == "go.mod" || name == "makefile" || name == "dockerfile" || name == "readme.md" {
		return true
	}
	switch ext {
	case ".go", ".md", ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func estimateComplexity(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	text := strings.ToLower(string(data))
	keywords := []string{" if ", " for ", " switch ", " case ", "&&", "||", " go ", " select ", " defer "}
	score := 0
	for _, k := range keywords {
		score += strings.Count(text, k)
	}
	return score
}

func (s *ArchitectService) buildArchitectureContext(files []archCandidate, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 26000
	}
	remaining := maxBytes
	var b strings.Builder
	for _, f := range files {
		if remaining <= 0 {
			break
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		rel := s.relativePath(f.path)
		header := fmt.Sprintf("FILE: %s (size=%d, complexity=%d)\n", rel, f.size, f.complexity)
		b.WriteString(header)
		snippet := string(data)
		if len(snippet) > remaining {
			snippet = snippet[:remaining]
		}
		b.WriteString(snippet)
		b.WriteString("\n\n")
		remaining = maxBytes - b.Len()
	}
	return b.String()
}

func (s *ArchitectService) resolveWithinRepo(path string) (string, error) {
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

func (s *ArchitectService) relativePath(path string) string {
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
