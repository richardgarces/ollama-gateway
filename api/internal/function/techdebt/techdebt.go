package service

import (
	"bytes"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type TechDebtService struct {
	repoRoot string
	logger   *slog.Logger
}

type PriorityItem struct {
	Path            string             `json:"path"`
	Signals         PrioritySignals    `json:"signals"`
	DebtScore       float64            `json:"debtScore"`
	EffortEstimate  string             `json:"effortEstimate"`
	ExpectedValue   string             `json:"expectedValue"`
	Reasons         []string           `json:"reasons"`
	SupportingStats map[string]float64 `json:"supportingStats"`
}

type PrioritySignals struct {
	ComplexityScore float64 `json:"complexity"`
	ChurnScore      float64 `json:"churn"`
	BugHistoryScore float64 `json:"bugHistory"`
	CoverageScore   float64 `json:"lowCoverage"`
}

type TechDebtReport struct {
	GeneratedAt time.Time      `json:"generatedAt"`
	Scanned     int            `json:"scannedFiles"`
	Backlog     []PriorityItem `json:"backlog"`
}

type fileSignalInput struct {
	path             string
	relPath          string
	loc              int
	complexityTokens int
	testCoverageHint float64
}

func NewTechDebtService(repoRoot string, logger *slog.Logger) *TechDebtService {
	if logger == nil {
		logger = slog.Default()
	}
	return &TechDebtService{repoRoot: repoRoot, logger: logger}
}

func (s *TechDebtService) AnalyzeTechDebt() (TechDebtReport, error) {
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return TechDebtReport{}, fmt.Errorf("REPO_ROOT inválido")
	}

	files, err := s.collectCandidates(rootAbs)
	if err != nil {
		return TechDebtReport{}, err
	}

	items := make([]PriorityItem, 0, len(files))
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 6)

	for _, f := range files {
		f := f
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			item := s.scoreFile(rootAbs, f)
			if item.Path == "" {
				return
			}
			mu.Lock()
			items = append(items, item)
			mu.Unlock()
		}()
	}
	wg.Wait()

	sort.Slice(items, func(i, j int) bool {
		if items[i].DebtScore == items[j].DebtScore {
			return items[i].Path < items[j].Path
		}
		return items[i].DebtScore > items[j].DebtScore
	})

	if len(items) > 30 {
		items = items[:30]
	}

	return TechDebtReport{
		GeneratedAt: time.Now().UTC(),
		Scanned:     len(files),
		Backlog:     items,
	}, nil
}

func (s *TechDebtService) WriteReport(report TechDebtReport) (string, error) {
	rootAbs, err := filepath.Abs(strings.TrimSpace(s.repoRoot))
	if err != nil {
		return "", fmt.Errorf("REPO_ROOT inválido")
	}

	target := filepath.Join(rootAbs, "docs", "techdebt.md")
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("no se pudo resolver docs/techdebt.md")
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("ruta fuera de REPO_ROOT")
	}

	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return "", err
	}

	content := renderMarkdown(report)
	if err := os.WriteFile(targetAbs, []byte(content), 0o644); err != nil {
		return "", err
	}
	return targetAbs, nil
}

func (s *TechDebtService) collectCandidates(rootAbs string) ([]fileSignalInput, error) {
	pkgHasTests := make(map[string]bool)
	candidates := make([]fileSignalInput, 0, 256)

	err := filepath.Walk(rootAbs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			if strings.HasSuffix(path, "_test.go") {
				pkgHasTests[filepath.Dir(path)] = true
			}
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(rootAbs, path)
		rel = filepath.ToSlash(rel)

		loc := countLOC(data)
		if loc == 0 {
			return nil
		}

		complexityTokens := estimateComplexityTokens(data)
		candidates = append(candidates, fileSignalInput{
			path:             path,
			relPath:          rel,
			loc:              loc,
			complexityTokens: complexityTokens,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	for i := range candidates {
		d := filepath.Dir(candidates[i].path)
		if pkgHasTests[d] {
			candidates[i].testCoverageHint = 35
		} else {
			candidates[i].testCoverageHint = 85
		}
	}

	return candidates, nil
}

func (s *TechDebtService) scoreFile(rootAbs string, input fileSignalInput) PriorityItem {
	churn, bugHistory := s.loadGitSignals(rootAbs, input.relPath)

	complexityScore := clamp(float64(input.complexityTokens)*1.8+float64(input.loc)*0.08, 0, 100)
	churnScore := clamp(float64(churn)*6.0, 0, 100)
	bugScore := clamp(float64(bugHistory)*20.0, 0, 100)
	coverageScore := clamp(input.testCoverageHint, 0, 100)

	debt := 0.35*complexityScore + 0.25*churnScore + 0.25*bugScore + 0.15*coverageScore
	debt = math.Round(debt*10) / 10

	effort := estimateEffort(input.loc, complexityScore)
	value := estimateValue(debt)
	reasons := makeReasons(complexityScore, churnScore, bugScore, coverageScore)

	return PriorityItem{
		Path: input.relPath,
		Signals: PrioritySignals{
			ComplexityScore: round1(complexityScore),
			ChurnScore:      round1(churnScore),
			BugHistoryScore: round1(bugScore),
			CoverageScore:   round1(coverageScore),
		},
		DebtScore:      debt,
		EffortEstimate: effort,
		ExpectedValue:  value,
		Reasons:        reasons,
		SupportingStats: map[string]float64{
			"loc":              round1(float64(input.loc)),
			"complexityTokens": round1(float64(input.complexityTokens)),
			"churnCommits":     round1(float64(churn)),
			"bugCommits":       round1(float64(bugHistory)),
		},
	}
}

func (s *TechDebtService) loadGitSignals(rootAbs string, relPath string) (churn int, bugHistory int) {
	cmd := exec.Command("git", "log", "--pretty=%s", "--", relPath)
	cmd.Dir = rootAbs
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && strings.TrimSpace(lines[0]) == "" {
		return 0, 0
	}

	churn = len(lines)
	for _, line := range lines {
		l := strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(l, "fix") || strings.Contains(l, "bug") || strings.Contains(l, "hotfix") || strings.Contains(l, "regression") {
			bugHistory++
		}
	}
	return churn, bugHistory
}

func estimateEffort(loc int, complexityScore float64) string {
	switch {
	case loc > 450 || complexityScore > 80:
		return "high"
	case loc > 220 || complexityScore > 55:
		return "medium"
	default:
		return "low"
	}
}

func estimateValue(debt float64) string {
	switch {
	case debt >= 75:
		return "high"
	case debt >= 50:
		return "medium"
	default:
		return "low"
	}
}

func makeReasons(complexity, churn, bugs, coverage float64) []string {
	reasons := make([]string, 0, 4)
	if complexity >= 60 {
		reasons = append(reasons, "alta complejidad ciclomática estimada")
	}
	if churn >= 55 {
		reasons = append(reasons, "alto churn de commits en el historial")
	}
	if bugs >= 45 {
		reasons = append(reasons, "historial de fixes/hotfixes elevado")
	}
	if coverage >= 70 {
		reasons = append(reasons, "señal de cobertura baja (sin tests cercanos)")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "deuda técnica moderada por combinación de señales")
	}
	return reasons
}

func renderMarkdown(report TechDebtReport) string {
	var b bytes.Buffer
	b.WriteString("# Tech Debt Priorities\n\n")
	b.WriteString("Generado: ")
	b.WriteString(report.GeneratedAt.Format(time.RFC3339))
	b.WriteString("\n\n")
	b.WriteString("Archivos analizados: ")
	b.WriteString(strconv.Itoa(report.Scanned))
	b.WriteString("\n\n")
	b.WriteString("| # | Archivo | Debt Score | Effort | Value | Señales (Cmplx/Churn/Bugs/Cov) |\n")
	b.WriteString("|---|---|---:|---|---|---|\n")
	for i, item := range report.Backlog {
		b.WriteString("| ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(" | ")
		b.WriteString(item.Path)
		b.WriteString(" | ")
		b.WriteString(fmt.Sprintf("%.1f", item.DebtScore))
		b.WriteString(" | ")
		b.WriteString(item.EffortEstimate)
		b.WriteString(" | ")
		b.WriteString(item.ExpectedValue)
		b.WriteString(" | ")
		b.WriteString(fmt.Sprintf("%.1f / %.1f / %.1f / %.1f", item.Signals.ComplexityScore, item.Signals.ChurnScore, item.Signals.BugHistoryScore, item.Signals.CoverageScore))
		b.WriteString(" |\n")
	}

	b.WriteString("\n## Top Findings\n\n")
	max := 5
	if len(report.Backlog) < max {
		max = len(report.Backlog)
	}
	for i := 0; i < max; i++ {
		it := report.Backlog[i]
		b.WriteString("### ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(it.Path)
		b.WriteString("\n")
		b.WriteString("- Debt Score: ")
		b.WriteString(fmt.Sprintf("%.1f", it.DebtScore))
		b.WriteString("\n")
		b.WriteString("- Effort: ")
		b.WriteString(it.EffortEstimate)
		b.WriteString("\n")
		b.WriteString("- Value: ")
		b.WriteString(it.ExpectedValue)
		b.WriteString("\n")
		b.WriteString("- Motivos: ")
		b.WriteString(strings.Join(it.Reasons, "; "))
		b.WriteString("\n\n")
	}
	return b.String()
}

func estimateComplexityTokens(data []byte) int {
	text := string(data)
	keywords := []string{" if ", " for ", " switch ", " select ", " case ", "&&", "||", " goto ", " defer "}
	normalized := " " + strings.ReplaceAll(strings.ReplaceAll(text, "\n", " "), "\t", " ") + " "
	score := 0
	for _, kw := range keywords {
		score += strings.Count(normalized, kw)
	}
	return score
}

func countLOC(data []byte) int {
	lines := strings.Split(string(data), "\n")
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		count++
	}
	return count
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
