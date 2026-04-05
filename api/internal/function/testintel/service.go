package service

import (
	"regexp"
	"sort"
	"strings"
)

type Service struct{}

type AnalyzeInput struct {
	Diff string `json:"diff"`
}

type Target struct {
	Package  string `json:"package"`
	Priority int    `json:"priority"`
	Reason   string `json:"reason"`
}

type Report struct {
	ChangedFiles []string `json:"changed_files"`
	Targets      []Target `json:"targets"`
	RiskScore    int      `json:"risk_score"`
	RiskLevel    string   `json:"risk_level"`
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) AnalyzeDiff(in AnalyzeInput) Report {
	files := parseChangedFiles(in.Diff)
	targetMap := make(map[string]Target)

	addTarget := func(pkg string, priority int, reason string) {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			return
		}
		if existing, ok := targetMap[pkg]; ok {
			if priority > existing.Priority {
				existing.Priority = priority
			}
			if reason != "" && !strings.Contains(existing.Reason, reason) {
				existing.Reason += "; " + reason
			}
			targetMap[pkg] = existing
			return
		}
		targetMap[pkg] = Target{Package: pkg, Priority: priority, Reason: reason}
	}

	risk := 5
	for _, path := range files {
		normalized := strings.ToLower(path)
		risk += 8

		if strings.Contains(normalized, "internal/server/") {
			addTarget("./internal/server", 100, "cambios en enrutamiento/core de API")
			risk += 22
		}
		if strings.Contains(normalized, "internal/middleware/") || strings.Contains(normalized, "/auth") {
			addTarget("./internal/middleware", 100, "cambios en autenticacion/autorizacion")
			addTarget("./internal/server", 90, "validar efectos transversales de middleware")
			risk += 20
		}
		if strings.Contains(normalized, "internal/function/") {
			featurePkg := packageFromFeaturePath(normalized)
			if featurePkg != "" {
				addTarget(featurePkg, 85, "cambios directos en feature")
				risk += 10
			}
		}
		if strings.Contains(normalized, "pkg/") {
			addTarget("./pkg/...", 80, "cambios en libreria compartida")
			risk += 10
		}
	}

	targets := make([]Target, 0, len(targetMap))
	for _, t := range targetMap {
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Priority == targets[j].Priority {
			return targets[i].Package < targets[j].Package
		}
		return targets[i].Priority > targets[j].Priority
	})

	if len(targets) == 0 {
		targets = append(targets, Target{
			Package:  "./...",
			Priority: 50,
			Reason:   "sin mapeo específico, ejecutar suite base",
		})
		risk = 40
	}

	if risk > 100 {
		risk = 100
	}
	level := "low"
	if risk >= 70 {
		level = "high"
	} else if risk >= 40 {
		level = "medium"
	}

	return Report{
		ChangedFiles: files,
		Targets:      targets,
		RiskScore:    risk,
		RiskLevel:    level,
	}
}

func parseChangedFiles(diff string) []string {
	trimmed := strings.TrimSpace(diff)
	if trimmed == "" {
		return nil
	}
	matcher := regexp.MustCompile(`(?m)^\+\+\+\s+(?:b/)?([^\s]+)$`)
	matches := matcher.FindAllStringSubmatch(trimmed, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := strings.TrimSpace(match[1])
		if path == "" || path == "/dev/null" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func packageFromFeaturePath(path string) string {
	const marker = "internal/function/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	start := idx + len(marker)
	rest := path[start:]
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return ""
	}
	return "./internal/function/" + parts[0]
}
