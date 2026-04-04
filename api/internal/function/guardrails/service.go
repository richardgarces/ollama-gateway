package service

import (
	"log/slog"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"ollama-gateway/internal/function/core/domain"
)

type Rule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
	Severity    string `json:"severity"`
}

type Finding struct {
	RuleID      string `json:"rule_id"`
	Severity    string `json:"severity"`
	Path        string `json:"path,omitempty"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation"`
}

type Evaluation struct {
	Allowed      bool      `json:"allowed"`
	Findings     []Finding `json:"findings"`
	BlockedCount int       `json:"blocked_count"`
}

type Service struct {
	logger *slog.Logger
	rules  []Rule
}

func NewService(logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger, rules: defaultRules()}
}

func (s *Service) Rules() []Rule {
	if s == nil {
		return nil
	}
	out := make([]Rule, len(s.rules))
	copy(out, s.rules)
	return out
}

func (s *Service) EvaluateDiffs(diffs []domain.UnifiedDiff) Evaluation {
	findings := make([]Finding, 0)
	for _, diff := range diffs {
		path := normalizedPath(diff.NewPath)
		if path == "" || path == "/dev/null" {
			path = normalizedPath(diff.OldPath)
		}
		if sensitivePath(path) {
			findings = append(findings, Finding{
				RuleID:      "sensitive-path",
				Severity:    "high",
				Path:        path,
				Evidence:    "El patch modifica un path sensible",
				Remediation: "Evita modificar secretos/configuración sensible o mueve el cambio a un path seguro.",
			})
		}

		for _, line := range addedLines(diff) {
			if looksLikeSecret(line) {
				findings = append(findings, Finding{
					RuleID:      "secret-detected",
					Severity:    "critical",
					Path:        path,
					Evidence:    truncateEvidence(line),
					Remediation: "Elimina secretos del patch y usa variables de entorno o vault.",
				})
			}
			if dangerousCommand(line) {
				findings = append(findings, Finding{
					RuleID:      "dangerous-command",
					Severity:    "high",
					Path:        path,
					Evidence:    truncateEvidence(line),
					Remediation: "Reemplaza el comando por una alternativa segura o añade validación estricta.",
				})
			}
		}
	}

	findings = dedupeFindings(findings)
	allowed := len(findings) == 0
	return Evaluation{Allowed: allowed, Findings: findings, BlockedCount: len(findings)}
}

func defaultRules() []Rule {
	return []Rule{
		{
			ID:          "sensitive-path",
			Name:        "Bloqueo por path sensible",
			Description: "Impide cambios en archivos/directorios sensibles como secretos, llaves y configuración crítica.",
			Remediation: "Limita cambios a código de aplicación y evita tocar credenciales o configuración sensible.",
			Severity:    "high",
		},
		{
			ID:          "secret-detected",
			Name:        "Detección de secretos",
			Description: "Detecta posibles tokens, claves privadas y credenciales en líneas añadidas.",
			Remediation: "Sustituye secretos por variables de entorno, secretos de CI o gestores de secretos.",
			Severity:    "critical",
		},
		{
			ID:          "dangerous-command",
			Name:        "Comandos peligrosos",
			Description: "Bloquea comandos destructivos o de ejecución remota en scripts y contenido añadido.",
			Remediation: "Evita comandos destructivos y usa flujos auditables con validación explícita.",
			Severity:    "high",
		},
	}
}

func sensitivePath(path string) bool {
	if path == "" {
		return false
	}
	p := strings.ToLower(filepath.ToSlash(path))
	sensitiveMarkers := []string{
		".env",
		".env.local",
		".env.production",
		"id_rsa",
		"id_ed25519",
		".ssh/",
		"secrets",
		"credentials",
		"kubeconfig",
		"authorized_keys",
		"known_hosts",
	}
	for _, marker := range sensitiveMarkers {
		if strings.Contains(p, marker) {
			return true
		}
	}
	return false
}

func addedLines(diff domain.UnifiedDiff) []string {
	lines := make([]string, 0, 16)
	for _, h := range diff.Hunks {
		for _, line := range h.Lines {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "+")))
			}
		}
	}
	return lines
}

var secretRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)api[_-]?key\s*[:=]\s*["']?[a-z0-9_\-]{12,}`),
	regexp.MustCompile(`(?i)secret\s*[:=]\s*["']?[a-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)password\s*[:=]\s*["'][^"']{6,}["']`),
	regexp.MustCompile(`(?i)token\s*[:=]\s*["']?[a-z0-9_\-]{12,}`),
	regexp.MustCompile(`(?i)-----begin (rsa|ec|openssh|dsa|pgp) private key-----`),
	regexp.MustCompile(`(?i)ghp_[a-z0-9]{20,}`),
	regexp.MustCompile(`(?i)sk-[a-z0-9]{20,}`),
}

func looksLikeSecret(line string) bool {
	if line == "" {
		return false
	}
	for _, re := range secretRegexes {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

var dangerousCommandRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-rf\s+/`),
	regexp.MustCompile(`(?i)\bcurl\b[^|\n]*\|\s*(sh|bash)`),
	regexp.MustCompile(`(?i)\bwget\b[^|\n]*\|\s*(sh|bash)`),
	regexp.MustCompile(`(?i)\bsudo\s+`),
	regexp.MustCompile(`(?i)\bchmod\s+777\b`),
	regexp.MustCompile(`(?i)\bmkfs\b`),
	regexp.MustCompile(`(?i)\bdd\s+if=`),
}

func dangerousCommand(line string) bool {
	for _, re := range dangerousCommandRegexes {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func truncateEvidence(v string) string {
	const maxLen = 180
	v = strings.TrimSpace(v)
	if len(v) <= maxLen {
		return v
	}
	return v[:maxLen] + "..."
}

func normalizedPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimPrefix(p, "a/")
	p = strings.TrimPrefix(p, "b/")
	return filepath.ToSlash(p)
}

func dedupeFindings(in []Finding) []Finding {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		key := f.RuleID + "|" + f.Path + "|" + f.Evidence
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity == out[j].Severity {
			if out[i].RuleID == out[j].RuleID {
				return out[i].Path < out[j].Path
			}
			return out[i].RuleID < out[j].RuleID
		}
		return severityWeight(out[i].Severity) > severityWeight(out[j].Severity)
	})
	return out
}

func severityWeight(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "critical":
		return 3
	case "high":
		return 2
	case "medium":
		return 1
	default:
		return 0
	}
}
