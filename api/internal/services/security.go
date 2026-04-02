package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/domain"
)

const securitySystemPrompt = "You are a security auditor. Scan this code for OWASP Top 10 vulnerabilities: injection, broken auth, sensitive data exposure, XXE, broken access control, misconfig, XSS, insecure deserialization, known vulnerabilities, insufficient logging. Return JSON: [{severity, category, line, description, fix}]."

type SecurityService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

type securityFileCandidate struct {
	path       string
	size       int64
	complexity int
	score      int64
}

func NewSecurityService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *SecurityService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SecurityService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *SecurityService) ScanFile(path string) ([]domain.SecurityFinding, error) {
	absPath, err := s.resolveWithinRepo(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	relPath := s.relativePath(absPath)
	prompt := strings.Join([]string{
		securitySystemPrompt,
		"Return ONLY valid JSON array.",
		"Do not include markdown fences.",
		"If there are no findings, return [].",
		"File:",
		relPath,
		"Code:",
		string(data),
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return nil, err
	}

	findings, err := parseSecurityFindings(out)
	if err != nil {
		return nil, err
	}
	for i := range findings {
		if strings.TrimSpace(findings[i].Path) == "" {
			findings[i].Path = relPath
		}
	}
	return findings, nil
}

func (s *SecurityService) ScanRepo() (domain.SecurityReport, error) {
	files, err := s.selectTopSecurityFiles(24)
	if err != nil {
		return domain.SecurityReport{}, err
	}

	findings := make([]domain.SecurityFinding, 0, 64)
	fileErrors := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	for _, file := range files {
		f := file
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fileFindings, scanErr := s.ScanFile(f.path)
			mu.Lock()
			defer mu.Unlock()
			if scanErr != nil {
				fileErrors[s.relativePath(f.path)] = scanErr.Error()
				return
			}
			findings = append(findings, fileFindings...)
		}()
	}

	wg.Wait()
	sortSecurityFindings(findings)

	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}
	highOrCritical := 0
	for _, finding := range findings {
		sev := normalizeFindingSeverity(finding.Severity)
		counts[sev]++
		if IsHighSeverity(sev) {
			highOrCritical++
		}
	}

	report := domain.SecurityReport{
		ScannedFiles:    len(files),
		TotalFindings:   len(findings),
		HighOrCritical:  highOrCritical,
		FindingsByLevel: counts,
		Findings:        findings,
		GeneratedAt:     time.Now().UTC(),
	}
	if len(fileErrors) > 0 {
		report.FileErrors = fileErrors
		s.logger.Warn("scan de seguridad parcial", slog.Int("file_errors", len(fileErrors)))
	}
	return report, nil
}

func IsHighSeverity(severity string) bool {
	sev := normalizeFindingSeverity(severity)
	return sev == "high" || sev == "critical"
}

func parseSecurityFindings(raw string) ([]domain.SecurityFinding, error) {
	text := strings.TrimSpace(stripMarkdownFence(raw))
	if text == "" {
		return []domain.SecurityFinding{}, nil
	}

	var findings []domain.SecurityFinding
	if err := json.Unmarshal([]byte(text), &findings); err != nil {
		jsonArray := extractJSONArraySecurity(text)
		if jsonArray == "" {
			return nil, fmt.Errorf("respuesta del security scanner no es JSON válido")
		}
		if err := json.Unmarshal([]byte(jsonArray), &findings); err != nil {
			return nil, fmt.Errorf("respuesta del security scanner no es JSON válido")
		}
	}

	for i := range findings {
		findings[i].Severity = normalizeFindingSeverity(findings[i].Severity)
		findings[i].Category = strings.TrimSpace(strings.ToLower(findings[i].Category))
		findings[i].Description = strings.TrimSpace(findings[i].Description)
		findings[i].Fix = strings.TrimSpace(findings[i].Fix)
		findings[i].Path = strings.TrimSpace(findings[i].Path)
		if findings[i].Line < 0 {
			findings[i].Line = 0
		}
	}
	return findings, nil
}

func extractJSONArraySecurity(s string) string {
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}

func normalizeFindingSeverity(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	switch v {
	case "low", "medium", "high", "critical":
		return v
	default:
		return "medium"
	}
}

func sortSecurityFindings(findings []domain.SecurityFinding) {
	severityRank := map[string]int{
		"critical": 0,
		"high":     1,
		"medium":   2,
		"low":      3,
	}
	sort.SliceStable(findings, func(i, j int) bool {
		si := normalizeFindingSeverity(findings[i].Severity)
		sj := normalizeFindingSeverity(findings[j].Severity)
		if severityRank[si] != severityRank[sj] {
			return severityRank[si] < severityRank[sj]
		}
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
}

func (s *SecurityService) selectTopSecurityFiles(limit int) ([]securityFileCandidate, error) {
	if limit <= 0 {
		limit = 24
	}
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return nil, fmt.Errorf("REPO_ROOT inválido")
	}

	candidates := make([]securityFileCandidate, 0, limit*3)
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
		if !isSecurityRelevantFile(path) {
			return nil
		}
		complexity := estimateSecurityComplexity(path)
		priority := securityPriority(path)
		score := int64(priority*1000000) + int64(complexity*4000) + info.Size()
		candidates = append(candidates, securityFileCandidate{path: path, size: info.Size(), complexity: complexity, score: score})
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

func isSecurityRelevantFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))
	if name == "dockerfile" || name == "docker-compose.yml" || name == "docker-compose.yaml" || name == ".env" || name == ".env.example" {
		return true
	}
	switch ext {
	case ".go", ".ts", ".js", ".py", ".java", ".json", ".yaml", ".yml", ".xml", ".md", ".sql":
		return true
	default:
		return false
	}
}

func securityPriority(path string) int {
	name := strings.ToLower(filepath.Base(path))
	full := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(full, "/internal/handlers/") || strings.Contains(full, "/internal/middleware/") || strings.Contains(full, "/internal/services/") {
		return 10
	}
	switch name {
	case "main.go", "server.go", "auth.go", "middleware.go", "dockerfile", "docker-compose.yml", "docker-compose.yaml", "go.mod":
		return 12
	case "makefile", "readme.md":
		return 6
	default:
		return 4
	}
}

func estimateSecurityComplexity(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	text := strings.ToLower(string(data))
	keywords := []string{"auth", "jwt", "token", "password", "secret", "exec", "sql", "query", "header", "cookie", "cors", "permission", "role", "admin"}
	score := 0
	for _, k := range keywords {
		score += strings.Count(text, k)
	}
	return score
}

func (s *SecurityService) resolveWithinRepo(path string) (string, error) {
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

func (s *SecurityService) relativePath(path string) string {
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(rootAbs, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
