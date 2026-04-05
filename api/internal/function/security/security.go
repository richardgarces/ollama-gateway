package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/function/core/domain"
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

type SecretFinding struct {
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	Description string `json:"description"`
	Snippet     string `json:"snippet,omitempty"`
}

type SecretScanReport struct {
	ScannedFiles int             `json:"scanned_files"`
	TotalFinding int             `json:"total_findings"`
	ByLevel      map[string]int  `json:"findings_by_level"`
	Findings     []SecretFinding `json:"findings"`
	GeneratedAt  time.Time       `json:"generated_at"`
}

type PolicyDecision struct {
	Action              string   `json:"action"`
	Allowed             bool     `json:"allowed"`
	Reasons             []string `json:"reasons"`
	SecurityFindings    int      `json:"security_findings"`
	SecretFindings      int      `json:"secret_findings"`
	HighOrCriticalCount int      `json:"high_or_critical_count"`
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

func (s *SecurityService) ScanSecretsRepo() (SecretScanReport, error) {
	files, err := s.selectTopSecurityFiles(48)
	if err != nil {
		return SecretScanReport{}, err
	}

	type secretPattern struct {
		kind        string
		severity    string
		description string
		re          *regexp.Regexp
	}
	patterns := []secretPattern{
		{kind: "aws_access_key", severity: "critical", description: "AWS access key detectada", re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
		{kind: "github_pat", severity: "high", description: "GitHub token detectado", re: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{20,}`)},
		{kind: "generic_secret", severity: "high", description: "Posible secreto hardcodeado", re: regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*["'][^"']{8,}["']`)},
	}

	findings := make([]SecretFinding, 0, 64)
	counts := map[string]int{"critical": 0, "high": 0, "medium": 0, "low": 0}

	for _, candidate := range files {
		if strings.HasSuffix(strings.ToLower(candidate.path), ".md") {
			continue
		}
		f, openErr := os.Open(candidate.path)
		if openErr != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()
			for _, p := range patterns {
				if p.re.MatchString(line) {
					snippet := strings.TrimSpace(line)
					if len(snippet) > 180 {
						snippet = snippet[:180] + "..."
					}
					findings = append(findings, SecretFinding{
						Severity:    p.severity,
						Kind:        p.kind,
						Path:        s.relativePath(candidate.path),
						Line:        lineNumber,
						Description: p.description,
						Snippet:     snippet,
					})
					counts[p.severity]++
				}
			}
		}
		_ = f.Close()
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity
		}
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})

	return SecretScanReport{
		ScannedFiles: len(files),
		TotalFinding: len(findings),
		ByLevel:      counts,
		Findings:     findings,
		GeneratedAt:  time.Now().UTC(),
	}, nil
}

func (s *SecurityService) EvaluatePolicy(action string) (PolicyDecision, error) {
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "" {
		return PolicyDecision{}, fmt.Errorf("action es requerida")
	}

	securityReport, err := s.ScanRepo()
	if err != nil {
		return PolicyDecision{}, err
	}
	secretReport, err := s.ScanSecretsRepo()
	if err != nil {
		return PolicyDecision{}, err
	}

	reasons := []string{}
	allowed := true
	highOrCritical := securityReport.HighOrCritical + secretReport.ByLevel["high"] + secretReport.ByLevel["critical"]

	if strings.HasSuffix(action, ":apply") || strings.Contains(action, "deploy") || strings.Contains(action, "merge") {
		if securityReport.HighOrCritical > 0 {
			allowed = false
			reasons = append(reasons, "findings de seguridad high/critical detectados")
		}
		if secretReport.ByLevel["critical"] > 0 || secretReport.ByLevel["high"] > 0 {
			allowed = false
			reasons = append(reasons, "se detectaron secretos potenciales en el repositorio")
		}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "policy checks sin bloqueos para la acción solicitada")
	}

	return PolicyDecision{
		Action:              action,
		Allowed:             allowed,
		Reasons:             reasons,
		SecurityFindings:    securityReport.TotalFindings,
		SecretFindings:      secretReport.TotalFinding,
		HighOrCriticalCount: highOrCritical,
	}, nil
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
	if (strings.Contains(full, "/internal/") && strings.Contains(full, "/transport/")) || strings.Contains(full, "/internal/middleware/") || strings.Contains(full, "/internal/function/") {
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
