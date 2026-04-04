package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/function/core/domain"
)

const defaultSuite = "v1/default"

type FixtureCase struct {
	Name              string   `json:"name"`
	Prompt            string   `json:"prompt"`
	ExpectedKeywords  []string `json:"expected_keywords"`
	ForbiddenKeywords []string `json:"forbidden_keywords"`
}

type Fixture struct {
	Suite   string        `json:"suite"`
	Version string        `json:"version"`
	Cases   []FixtureCase `json:"cases"`
}

type BenchmarkCaseResult struct {
	Name             string   `json:"name"`
	Prompt           string   `json:"prompt"`
	Output           string   `json:"output"`
	LatencyMS        int64    `json:"latency_ms"`
	AccuracyScore    float64  `json:"accuracy_score"`
	ConsistencyScore float64  `json:"consistency_score"`
	MatchedKeywords  []string `json:"matched_keywords"`
	MissingKeywords  []string `json:"missing_keywords"`
	ForbiddenHits    []string `json:"forbidden_hits"`
}

type BenchmarkSummary struct {
	TotalCases         int     `json:"total_cases"`
	AverageLatencyMS   float64 `json:"average_latency_ms"`
	AverageAccuracy    float64 `json:"average_accuracy"`
	AverageConsistency float64 `json:"average_consistency"`
	OverallScore       float64 `json:"overall_score"`
}

type BenchmarkResult struct {
	ID         string                `json:"id"`
	Suite      string                `json:"suite"`
	Version    string                `json:"version"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt time.Time             `json:"finished_at"`
	Summary    BenchmarkSummary      `json:"summary"`
	Cases      []BenchmarkCaseResult `json:"cases"`
	JSONExport string                `json:"json_export"`
	MDExport   string                `json:"markdown_export"`
}

type Service struct {
	repoRoot string
	engine   domain.RAGEngine
	logger   *slog.Logger

	mu      sync.RWMutex
	results map[string]BenchmarkResult
}

func NewService(repoRoot string, engine domain.RAGEngine, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(repoRoot) == "" {
		repoRoot = "."
	}
	return &Service{
		repoRoot: repoRoot,
		engine:   engine,
		logger:   logger,
		results:  make(map[string]BenchmarkResult),
	}
}

func (s *Service) RunBenchmark(suite string) (BenchmarkResult, error) {
	if s == nil || s.engine == nil {
		return BenchmarkResult{}, errors.New("eval service no disponible")
	}

	fixture, err := s.loadFixture(suite)
	if err != nil {
		return BenchmarkResult{}, err
	}
	if len(fixture.Cases) == 0 {
		return BenchmarkResult{}, errors.New("suite sin casos")
	}

	started := time.Now().UTC()
	results := make([]BenchmarkCaseResult, 0, len(fixture.Cases))
	var sumLatency, sumAccuracy, sumConsistency float64

	for _, tc := range fixture.Cases {
		caseRes := s.evalCase(tc)
		results = append(results, caseRes)
		sumLatency += float64(caseRes.LatencyMS)
		sumAccuracy += caseRes.AccuracyScore
		sumConsistency += caseRes.ConsistencyScore
	}

	count := float64(len(results))
	summary := BenchmarkSummary{
		TotalCases:         len(results),
		AverageLatencyMS:   safeDiv(sumLatency, count),
		AverageAccuracy:    safeDiv(sumAccuracy, count),
		AverageConsistency: safeDiv(sumConsistency, count),
	}
	summary.OverallScore = 0.5*summary.AverageAccuracy + 0.3*summary.AverageConsistency + 0.2*latencyScore(summary.AverageLatencyMS)

	res := BenchmarkResult{
		ID:         randomID(),
		Suite:      fixture.Suite,
		Version:    fixture.Version,
		StartedAt:  started,
		FinishedAt: time.Now().UTC(),
		Summary:    summary,
		Cases:      results,
	}
	res.JSONExport = mustMarshalJSON(res)
	res.MDExport = toMarkdown(res)

	s.mu.Lock()
	s.results[res.ID] = res
	s.mu.Unlock()

	return res, nil
}

func (s *Service) GetResult(id string) (BenchmarkResult, error) {
	if s == nil {
		return BenchmarkResult{}, errors.New("eval service no disponible")
	}
	clean := strings.TrimSpace(id)
	if clean == "" {
		return BenchmarkResult{}, errors.New("id requerido")
	}
	s.mu.RLock()
	res, ok := s.results[clean]
	s.mu.RUnlock()
	if !ok {
		return BenchmarkResult{}, errors.New("resultado no encontrado")
	}
	return res, nil
}

func (s *Service) loadFixture(suite string) (Fixture, error) {
	normalized := normalizeSuite(suite)
	absRepo, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return Fixture{}, fmt.Errorf("repo root inválido: %w", err)
	}
	base := filepath.Join(absRepo, "testdata", "eval")
	fixturePath := filepath.Join(base, normalized+".json")
	fixturePath, err = filepath.Abs(fixturePath)
	if err != nil {
		return Fixture{}, err
	}
	if fixturePath != base && !strings.HasPrefix(fixturePath, base+string(os.PathSeparator)) {
		return Fixture{}, errors.New("suite fuera de directorio permitido")
	}

	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return Fixture{}, fmt.Errorf("no se pudo leer suite %q: %w", normalized, err)
	}
	var fx Fixture
	if err := json.Unmarshal(data, &fx); err != nil {
		return Fixture{}, fmt.Errorf("fixture inválido: %w", err)
	}
	if strings.TrimSpace(fx.Suite) == "" {
		fx.Suite = normalized
	}
	if strings.TrimSpace(fx.Version) == "" {
		fx.Version = strings.Split(normalized, "/")[0]
	}
	return fx, nil
}

func (s *Service) evalCase(tc FixtureCase) BenchmarkCaseResult {
	prompt := strings.TrimSpace(tc.Prompt)
	if prompt == "" {
		prompt = "(empty prompt)"
	}

	start := time.Now()
	out, err := s.engine.GenerateWithContext(prompt)
	latency := time.Since(start)
	if err != nil {
		out = ""
	}

	acc, matched, missing, forbidden := accuracyScore(out, tc.ExpectedKeywords, tc.ForbiddenKeywords)
	consistency := s.consistencyScore(prompt, out)

	return BenchmarkCaseResult{
		Name:             strings.TrimSpace(tc.Name),
		Prompt:           prompt,
		Output:           strings.TrimSpace(out),
		LatencyMS:        latency.Milliseconds(),
		AccuracyScore:    acc,
		ConsistencyScore: consistency,
		MatchedKeywords:  matched,
		MissingKeywords:  missing,
		ForbiddenHits:    forbidden,
	}
}

func (s *Service) consistencyScore(prompt, firstOutput string) float64 {
	if strings.TrimSpace(firstOutput) == "" {
		return 0
	}
	start := time.Now()
	second, err := s.engine.GenerateWithContext(prompt)
	if err != nil {
		return 0
	}
	_ = start
	return jaccardSimilarity(tokenize(firstOutput), tokenize(second))
}

func normalizeSuite(suite string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(suite, "\\", "/"))
	if clean == "" {
		return defaultSuite
	}
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.TrimSuffix(clean, ".json")
	if !strings.Contains(clean, "/") {
		clean = "v1/" + clean
	}
	parts := strings.Split(clean, "/")
	safe := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." || p == ".." {
			continue
		}
		safe = append(safe, p)
	}
	if len(safe) == 0 {
		return defaultSuite
	}
	return strings.Join(safe, "/")
}

func accuracyScore(output string, expected, forbidden []string) (float64, []string, []string, []string) {
	text := strings.ToLower(strings.TrimSpace(output))
	matched := make([]string, 0)
	missing := make([]string, 0)
	forbiddenHits := make([]string, 0)

	for _, kw := range normalizeKeywords(expected) {
		if strings.Contains(text, kw) {
			matched = append(matched, kw)
		} else {
			missing = append(missing, kw)
		}
	}
	for _, kw := range normalizeKeywords(forbidden) {
		if strings.Contains(text, kw) {
			forbiddenHits = append(forbiddenHits, kw)
		}
	}

	expectedScore := 1.0
	if len(expected) > 0 {
		expectedScore = safeDiv(float64(len(matched)), float64(len(normalizeKeywords(expected))))
	}
	forbiddenScore := 1.0
	if len(forbidden) > 0 {
		forbiddenScore = 1.0 - safeDiv(float64(len(forbiddenHits)), float64(len(normalizeKeywords(forbidden))))
	}
	if forbiddenScore < 0 {
		forbiddenScore = 0
	}

	score := 0.7*expectedScore + 0.3*forbiddenScore
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	sort.Strings(matched)
	sort.Strings(missing)
	sort.Strings(forbiddenHits)
	return score, matched, missing, forbiddenHits
}

func normalizeKeywords(input []string) []string {
	set := make(map[string]struct{})
	out := make([]string, 0, len(input))
	for _, v := range input {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if _, ok := set[v]; ok {
			continue
		}
		set[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func tokenize(s string) map[string]struct{} {
	s = strings.ToLower(s)
	replacer := strings.NewReplacer(
		"\n", " ",
		"\t", " ",
		",", " ",
		".", " ",
		";", " ",
		":", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"\"", " ",
		"'", " ",
	)
	s = replacer.Replace(s)
	out := make(map[string]struct{})
	for _, part := range strings.Fields(s) {
		if len(part) < 3 {
			continue
		}
		out[part] = struct{}{}
	}
	return out
}

func jaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	union := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		union[k] = struct{}{}
	}
	for k := range b {
		union[k] = struct{}{}
		if _, ok := a[k]; ok {
			inter++
		}
	}
	return safeDiv(float64(inter), float64(len(union)))
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func latencyScore(avgMS float64) float64 {
	if avgMS <= 0 {
		return 0
	}
	if avgMS <= 400 {
		return 1
	}
	if avgMS >= 6000 {
		return 0
	}
	return 1 - ((avgMS - 400) / 5600)
}

func mustMarshalJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func toMarkdown(result BenchmarkResult) string {
	var b strings.Builder
	b.WriteString("# Prompt Benchmark Result\n\n")
	b.WriteString("- ID: " + result.ID + "\n")
	b.WriteString("- Suite: " + result.Suite + "\n")
	b.WriteString("- Version: " + result.Version + "\n")
	b.WriteString(fmt.Sprintf("- Average Accuracy: %.3f\n", result.Summary.AverageAccuracy))
	b.WriteString(fmt.Sprintf("- Average Consistency: %.3f\n", result.Summary.AverageConsistency))
	b.WriteString(fmt.Sprintf("- Average Latency (ms): %.2f\n", result.Summary.AverageLatencyMS))
	b.WriteString(fmt.Sprintf("- Overall Score: %.3f\n\n", result.Summary.OverallScore))
	b.WriteString("## Cases\n\n")
	for _, c := range result.Cases {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = "unnamed"
		}
		b.WriteString("### " + name + "\n")
		b.WriteString(fmt.Sprintf("- Accuracy: %.3f\n", c.AccuracyScore))
		b.WriteString(fmt.Sprintf("- Consistency: %.3f\n", c.ConsistencyScore))
		b.WriteString(fmt.Sprintf("- Latency (ms): %d\n", c.LatencyMS))
		if len(c.MatchedKeywords) > 0 {
			b.WriteString("- Matched Keywords: " + strings.Join(c.MatchedKeywords, ", ") + "\n")
		}
		if len(c.MissingKeywords) > 0 {
			b.WriteString("- Missing Keywords: " + strings.Join(c.MissingKeywords, ", ") + "\n")
		}
		if len(c.ForbiddenHits) > 0 {
			b.WriteString("- Forbidden Hits: " + strings.Join(c.ForbiddenHits, ", ") + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func randomID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("eval-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
