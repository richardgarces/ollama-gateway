package gate

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ollama-gateway/internal/function/core/domain"
)

type securityScanner interface {
	ScanRepo() (domain.SecurityReport, error)
}

type commandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (string, error)
}

type shellRunner struct{}

func (shellRunner) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type GateProfile struct {
	Name            string  `json:"name"`
	MinCoverage     float64 `json:"min_coverage"`
	MaxHighCritical int     `json:"max_high_critical"`
	RequireTests    bool    `json:"require_tests"`
}

type SecurityStatus struct {
	ScannedFiles   int `json:"scanned_files"`
	TotalFindings  int `json:"total_findings"`
	HighOrCritical int `json:"high_or_critical"`
}

type CoverageStatus struct {
	Percent  float64 `json:"percent"`
	MeetsMin bool    `json:"meets_min"`
	Min      float64 `json:"min"`
}

type TestsStatus struct {
	Passed bool   `json:"passed"`
	Output string `json:"output,omitempty"`
}

type GateResult struct {
	Allow           bool           `json:"allow"`
	Profile         string         `json:"profile"`
	Environment     string         `json:"environment"`
	Reasons         []string       `json:"reasons"`
	RequiredActions []string       `json:"required_actions"`
	Security        SecurityStatus `json:"security"`
	Coverage        CoverageStatus `json:"coverage"`
	Tests           TestsStatus    `json:"tests"`
	EvaluatedAtUTC  string         `json:"evaluated_at_utc"`
}

type Service struct {
	repoRoot string
	scanner  securityScanner
	runner   commandRunner
	logger   *slog.Logger
}

func NewService(repoRoot string, scanner securityScanner, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repoRoot: strings.TrimSpace(repoRoot),
		scanner:  scanner,
		runner:   shellRunner{},
		logger:   logger,
	}
}

func (s *Service) SetRunner(runner commandRunner) {
	if runner != nil {
		s.runner = runner
	}
}

func (s *Service) CheckDeployGate() (GateResult, error) {
	env := strings.TrimSpace(os.Getenv("DEPLOY_ENV"))
	profile := strings.TrimSpace(os.Getenv("DEPLOY_GATE_PROFILE"))
	return s.CheckDeployGateWith(profile, env)
}

func (s *Service) CheckDeployGateWith(profileName string, environment string) (GateResult, error) {
	if s == nil || s.scanner == nil {
		return GateResult{}, fmt.Errorf("gate service no disponible")
	}
	if s.runner == nil {
		s.runner = shellRunner{}
	}

	profile := resolveProfile(profileName, environment)
	moduleDir := detectGoModuleDir(s.repoRoot)

	securityReport, secErr := s.scanner.ScanRepo()
	testsOK, testsOutput := s.runTests(moduleDir)
	coveragePercent, coverageErr := s.runCoverage(moduleDir)

	reasons := make([]string, 0, 6)
	actions := make([]string, 0, 6)

	if secErr != nil {
		reasons = append(reasons, "No se pudo evaluar security scan del repositorio.")
		actions = append(actions, "Revisar servicio de security scan y reintentar gate.")
	} else if securityReport.HighOrCritical > profile.MaxHighCritical {
		reasons = append(reasons, fmt.Sprintf("Security findings high/critical exceden umbral (%d > %d).", securityReport.HighOrCritical, profile.MaxHighCritical))
		actions = append(actions, "Resolver findings high/critical o justificar excepciones antes de deploy.")
	}

	if coverageErr != nil {
		reasons = append(reasons, "No se pudo obtener cobertura de tests.")
		actions = append(actions, "Ejecutar suite con cobertura y validar reporte antes de desplegar.")
	} else if coveragePercent < profile.MinCoverage {
		reasons = append(reasons, fmt.Sprintf("Cobertura insuficiente (%.2f%% < %.2f%%).", coveragePercent, profile.MinCoverage))
		actions = append(actions, "Aumentar cobertura en modulos criticos hasta superar el umbral del perfil.")
	}

	if profile.RequireTests && !testsOK {
		reasons = append(reasons, "La suite de tests no esta en estado green.")
		actions = append(actions, "Corregir tests fallidos y re-ejecutar gate pre-deploy.")
	}

	allow := len(reasons) == 0
	if allow {
		actions = append(actions, "Gate aprobado: proceder con despliegue controlado y monitoreo post-deploy.")
	}

	result := GateResult{
		Allow:           allow,
		Profile:         profile.Name,
		Environment:     normalizedEnvironment(environment),
		Reasons:         uniqueStrings(reasons),
		RequiredActions: uniqueStrings(actions),
		Security: SecurityStatus{
			ScannedFiles:   securityReport.ScannedFiles,
			TotalFindings:  securityReport.TotalFindings,
			HighOrCritical: securityReport.HighOrCritical,
		},
		Coverage: CoverageStatus{
			Percent:  round2(coveragePercent),
			MeetsMin: coverageErr == nil && coveragePercent >= profile.MinCoverage,
			Min:      profile.MinCoverage,
		},
		Tests: TestsStatus{
			Passed: testsOK,
			Output: trimOutput(testsOutput),
		},
		EvaluatedAtUTC: time.Now().UTC().Format(time.RFC3339),
	}

	if secErr != nil {
		s.logger.Warn("gate security scan fallo", slog.String("error", secErr.Error()))
	}
	if coverageErr != nil {
		s.logger.Warn("gate coverage fallo", slog.String("error", coverageErr.Error()))
	}

	return result, nil
}

func (s *Service) runTests(moduleDir string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := s.runner.Run(ctx, moduleDir, "go", "test", "./...")
	if err != nil {
		return false, out
	}
	return true, out
}

func (s *Service) runCoverage(moduleDir string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := s.runner.Run(ctx, moduleDir, "go", "test", "-cover", "./...")
	if err != nil {
		return 0, fmt.Errorf("go test -cover fallo: %w", err)
	}
	value, ok := parseCoveragePercent(out)
	if !ok {
		return 0, fmt.Errorf("no se pudo parsear coverage")
	}
	return value, nil
}

func detectGoModuleDir(repoRoot string) string {
	root := strings.TrimSpace(repoRoot)
	if root == "" {
		root = "."
	}
	if info, err := os.Stat(filepath.Join(root, "api", "go.mod")); err == nil && !info.IsDir() {
		return filepath.Join(root, "api")
	}
	return root
}

func resolveProfile(profileName string, environment string) GateProfile {
	name := strings.ToLower(strings.TrimSpace(profileName))
	if name == "" {
		name = defaultProfileByEnvironment(environment)
	}
	if name == "relaxed" {
		return GateProfile{Name: "relaxed", MinCoverage: 60, MaxHighCritical: 2, RequireTests: true}
	}
	return GateProfile{Name: "strict", MinCoverage: 80, MaxHighCritical: 0, RequireTests: true}
}

func defaultProfileByEnvironment(environment string) string {
	env := normalizedEnvironment(environment)
	if env == "prod" || env == "production" {
		return "strict"
	}
	return "relaxed"
}

func normalizedEnvironment(environment string) string {
	env := strings.ToLower(strings.TrimSpace(environment))
	if env == "" {
		return "dev"
	}
	return env
}

var coverageRe = regexp.MustCompile(`coverage:\s*([0-9]+(?:\.[0-9]+)?)%\s+of\s+statements`)

func parseCoveragePercent(output string) (float64, bool) {
	matches := coverageRe.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return 0, false
	}
	v := strings.TrimSpace(last[1])
	value, err := strconvParseFloat(v)
	if err != nil {
		return 0, false
	}
	return value, true
}

func strconvParseFloat(value string) (float64, error) {
	var whole float64
	parts := strings.SplitN(value, ".", 2)
	for _, ch := range parts[0] {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("valor invalido")
		}
		whole = whole*10 + float64(ch-'0')
	}
	if len(parts) == 1 {
		return whole, nil
	}
	frac := 0.0
	base := 10.0
	for _, ch := range parts[1] {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("valor invalido")
		}
		frac += float64(ch-'0') / base
		base *= 10
	}
	return whole + frac, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func trimOutput(output string) string {
	clean := strings.TrimSpace(output)
	if len(clean) <= 1600 {
		return clean
	}
	return clean[:1600] + "..."
}

func round2(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}
