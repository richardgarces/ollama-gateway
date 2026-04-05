package runbook

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type Service struct {
	repoRoot string
	logger   *slog.Logger
}

type Runbook struct {
	IncidentType           string   `json:"incident_type"`
	Title                  string   `json:"title"`
	DiagnosisSteps         []string `json:"diagnosis_steps"`
	MitigationSteps        []string `json:"mitigation_steps"`
	RollbackSteps          []string `json:"rollback_steps"`
	PostFixValidationSteps []string `json:"post_fix_validation_steps"`
	Recommendations        []string `json:"recommendations"`
	Applied                bool     `json:"applied"`
	MarkdownPath           string   `json:"markdown_path,omitempty"`
	Markdown               string   `json:"markdown,omitempty"`
}

type RunbookSummary struct {
	IncidentType string `json:"incident_type"`
	Title        string `json:"title"`
	MarkdownPath string `json:"markdown_path"`
}

func NewService(repoRoot string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repoRoot: strings.TrimSpace(repoRoot), logger: logger}
}

func (s *Service) GenerateRunbook(incidentType string, context string) (Runbook, error) {
	incidentType = strings.TrimSpace(incidentType)
	if incidentType == "" {
		return Runbook{}, fmt.Errorf("incident_type es requerido")
	}

	signals := strings.ToLower(incidentType + "\n" + strings.TrimSpace(context))

	diagnosis := []string{
		"Confirmar el alcance: servicios, endpoints y ventanas horarias afectadas.",
		"Recopilar logs, metricas y ultimos cambios desplegados antes del incidente.",
		"Comparar comportamiento con baseline previo para identificar regresiones.",
	}
	if containsAny(signals, "timeout", "latency", "p95", "p99", "slow") {
		diagnosis = append(diagnosis, "Revisar saturation de dependencias y colas (timeouts, latencia alta).")
	}
	if containsAny(signals, "db", "database", "sql", "migration", "lock") {
		diagnosis = append(diagnosis, "Inspeccionar locks, planes de ejecucion y tiempos de transaccion en base de datos.")
	}
	if containsAny(signals, "auth", "jwt", "token", "forbidden", "unauthorized") {
		diagnosis = append(diagnosis, "Validar expiracion y permisos de credenciales/token en servicios implicados.")
	}

	mitigation := []string{
		"Aplicar mitigacion de menor riesgo primero (feature flag, degrade mode, rate limit).",
		"Aislar componente sospechoso y reducir blast radius con rollback parcial si procede.",
		"Escalar comunicacion de estado a stakeholders con ETA y plan inmediato.",
	}
	if containsAny(signals, "db", "sql", "migration", "lock") {
		mitigation = append(mitigation, "Pausar migraciones concurrentes y evaluar ejecucion fuera de horas pico.")
	}

	rollback := []string{
		"Verificar que existe plan de rollback probado en staging y ejecutar smoke test previo.",
		"Definir punto de no retorno y criterio claro para revertir sin perdida de datos.",
		"Comprobar idempotencia del rollback y de scripts de recuperacion antes de aplicar.",
		"Ejecutar rollback controlado, monitorear error rate/latencia y confirmar recuperacion.",
	}

	postFix := []string{
		"Validar metricas clave post-fix (p50/p95/p99, error rate, throughput) por al menos una ventana estable.",
		"Ejecutar pruebas funcionales criticas y canary checks sobre rutas afectadas.",
		"Confirmar que no hay regresiones en servicios dependientes y cerrar incidente con evidencia.",
	}

	recommendations := []string{
		"Agregar alerta temprana para desviaciones de SLO en el flujo afectado.",
		"Automatizar checklist de pre-deploy con gate de rollback e idempotencia.",
		"Mantener este runbook versionado y revisarlo tras cada incidente real.",
	}

	return Runbook{
		IncidentType:           incidentType,
		Title:                  "Runbook - " + incidentType,
		DiagnosisSteps:         diagnosis,
		MitigationSteps:        mitigation,
		RollbackSteps:          rollback,
		PostFixValidationSteps: postFix,
		Recommendations:        recommendations,
		Applied:                false,
	}, nil
}

func (s *Service) SaveRunbook(runbook Runbook) (string, error) {
	targetAbs, relPath, err := s.resolveRunbookPath(runbook.IncidentType)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return "", fmt.Errorf("no se pudo crear directorio destino: %w", err)
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(targetAbs); statErr == nil {
		mode = info.Mode().Perm()
	}

	if err := os.WriteFile(targetAbs, []byte(RenderMarkdown(runbook)), mode); err != nil {
		return "", fmt.Errorf("no se pudo escribir runbook: %w", err)
	}

	return relPath, nil
}

func (s *Service) ListRunbooks(incidentType string) ([]RunbookSummary, error) {
	rootAbs, err := filepath.Abs(strings.TrimSpace(s.repoRoot))
	if err != nil {
		return nil, fmt.Errorf("REPO_ROOT invalido: %w", err)
	}
	docsDir := filepath.Join(rootAbs, "docs", "runbooks")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunbookSummary{}, nil
		}
		return nil, fmt.Errorf("no se pudo listar runbooks: %w", err)
	}

	filter := sanitizeIncidentType(incidentType)
	out := make([]RunbookSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.ToLower(filepath.Ext(name)) != ".md" {
			continue
		}
		incident := strings.TrimSuffix(name, filepath.Ext(name))
		if filter != "" && incident != filter {
			continue
		}
		title, titleErr := s.readRunbookTitle(filepath.Join(docsDir, name))
		if titleErr != nil {
			title = "Runbook - " + incident
		}
		out = append(out, RunbookSummary{
			IncidentType: incident,
			Title:        title,
			MarkdownPath: filepath.ToSlash(filepath.Join("docs", "runbooks", name)),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].IncidentType < out[j].IncidentType
	})
	return out, nil
}

func (s *Service) GetRunbook(incidentType string) (Runbook, error) {
	incident := sanitizeIncidentType(incidentType)
	if incident == "" {
		return Runbook{}, fmt.Errorf("incident_type invalido")
	}

	rootAbs, err := filepath.Abs(strings.TrimSpace(s.repoRoot))
	if err != nil {
		return Runbook{}, fmt.Errorf("REPO_ROOT invalido: %w", err)
	}
	path := filepath.Join(rootAbs, "docs", "runbooks", incident+".md")
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return Runbook{}, fmt.Errorf("no se pudo resolver runbook: %w", err)
	}
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+string(os.PathSeparator)) {
		return Runbook{}, fmt.Errorf("ruta fuera de REPO_ROOT")
	}

	content, err := os.ReadFile(pathAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return Runbook{}, fmt.Errorf("runbook no encontrado")
		}
		return Runbook{}, fmt.Errorf("no se pudo leer runbook: %w", err)
	}

	title := "Runbook - " + incident
	if parsed := parseTitleFromMarkdown(string(content)); parsed != "" {
		title = parsed
	}

	return Runbook{
		IncidentType: incident,
		Title:        title,
		Applied:      true,
		MarkdownPath: filepath.ToSlash(filepath.Join("docs", "runbooks", incident+".md")),
		Markdown:     string(content),
	}, nil
}

func (s *Service) readRunbookTitle(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	title := parseTitleFromMarkdown(string(content))
	if title == "" {
		return "", fmt.Errorf("titulo no encontrado")
	}
	return title, nil
}

func parseTitleFromMarkdown(markdown string) string {
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

func (s *Service) resolveRunbookPath(incidentType string) (string, string, error) {
	rootAbs, err := filepath.Abs(strings.TrimSpace(s.repoRoot))
	if err != nil {
		return "", "", fmt.Errorf("REPO_ROOT invalido: %w", err)
	}

	safeType := sanitizeIncidentType(incidentType)
	if safeType == "" {
		return "", "", fmt.Errorf("incident_type invalido")
	}

	target := filepath.Join(rootAbs, "docs", "runbooks", safeType+".md")
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("no se pudo resolver ruta destino: %w", err)
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("ruta fuera de REPO_ROOT: %s", targetAbs)
	}

	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", "", fmt.Errorf("no se pudo calcular ruta relativa: %w", err)
	}
	return targetAbs, filepath.ToSlash(rel), nil
}

func RenderMarkdown(runbook Runbook) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(strings.TrimSpace(runbook.Title))
	b.WriteString("\n\n")
	b.WriteString("Tipo de incidente: `")
	b.WriteString(strings.TrimSpace(runbook.IncidentType))
	b.WriteString("`\n\n")
	writeSection(&b, "Diagnostico", runbook.DiagnosisSteps)
	writeSection(&b, "Mitigacion", runbook.MitigationSteps)
	writeSection(&b, "Rollback", runbook.RollbackSteps)
	writeSection(&b, "Validacion Post-Fix", runbook.PostFixValidationSteps)
	writeSection(&b, "Recomendaciones", runbook.Recommendations)
	return b.String()
}

func writeSection(builder *strings.Builder, title string, steps []string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n")
	for _, step := range steps {
		trimmed := strings.TrimSpace(step)
		if trimmed == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(trimmed)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func sanitizeIncidentType(raw string) string {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r) || r == '/' || r == '\\':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	return out
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
