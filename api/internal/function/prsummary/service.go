package prsummary

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

type Service struct {
	logger *slog.Logger
}

type SummaryResult struct {
	SuggestedTitle     string   `json:"suggested_title"`
	Summary            string   `json:"summary"`
	Scope              string   `json:"scope"`
	Risk               string   `json:"risk"`
	AffectedComponents []string `json:"affected_components"`
	SuggestedTests     []string `json:"suggested_tests"`
	ReviewChecklist    []string `json:"review_checklist"`
}

func NewService(logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger}
}

func (s *Service) SummarizeDiff(diff string) (SummaryResult, error) {
	trimmed := strings.TrimSpace(diff)
	if trimmed == "" {
		return SummaryResult{}, fmt.Errorf("diff es requerido")
	}

	files := extractFiles(trimmed)
	components := inferComponents(files)
	risk := inferRisk(trimmed, files)
	tests := inferSuggestedTests(files, components, risk)
	checklist := buildReviewChecklist(risk, components)
	scope := inferScope(files, components)
	title := suggestTitle(components, files, risk)
	summary := buildNarrativeSummary(files, components, risk)

	return SummaryResult{
		SuggestedTitle:     title,
		Summary:            summary,
		Scope:              scope,
		Risk:               risk,
		AffectedComponents: components,
		SuggestedTests:     tests,
		ReviewChecklist:    checklist,
	}, nil
}

func extractFiles(diff string) []string {
	lines := strings.Split(diff, "\n")
	seen := map[string]struct{}{}
	files := make([]string, 0, 16)
	for _, line := range lines {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue
		}
		path := strings.TrimPrefix(parts[3], "b/")
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func inferComponents(files []string) []string {
	seen := map[string]struct{}{}
	components := make([]string, 0, 8)
	for _, f := range files {
		lf := strings.ToLower(f)
		switch {
		case strings.Contains(lf, "internal/server"):
			seen["server"] = struct{}{}
		case strings.Contains(lf, "internal/middleware"):
			seen["middleware"] = struct{}{}
		case strings.Contains(lf, "internal/function/"):
			parts := strings.Split(lf, "/")
			for i := 0; i < len(parts)-1; i++ {
				if parts[i] == "function" {
					seen["function:"+parts[i+1]] = struct{}{}
					break
				}
			}
		case strings.HasPrefix(lf, "vscode-extension/"):
			seen["vscode-extension"] = struct{}{}
		case strings.HasPrefix(lf, "docs/"):
			seen["docs"] = struct{}{}
		case strings.HasPrefix(lf, "api/pkg/") || strings.HasPrefix(lf, "pkg/"):
			seen["shared-pkg"] = struct{}{}
		default:
			seen["misc"] = struct{}{}
		}
	}

	for c := range seen {
		components = append(components, c)
	}
	sort.Strings(components)
	return components
}

func inferRisk(diff string, files []string) string {
	lower := strings.ToLower(diff)

	highSignals := 0
	mediumSignals := 0

	if len(files) >= 12 {
		highSignals++
	} else if len(files) >= 6 {
		mediumSignals++
	}

	if strings.Contains(lower, "internal/server/") || strings.Contains(lower, "server.go") {
		highSignals++
	}
	if strings.Contains(lower, "internal/middleware/") || strings.Contains(lower, "auth") || strings.Contains(lower, "jwt") {
		highSignals++
	}
	if strings.Contains(lower, "database") || strings.Contains(lower, "migration") || strings.Contains(lower, "sql") {
		mediumSignals++
	}
	if strings.Contains(lower, "security") || strings.Contains(lower, "gate") {
		highSignals++
	}
	if strings.Contains(lower, "+func") && strings.Contains(lower, "handle") {
		mediumSignals++
	}

	if highSignals >= 2 {
		return "high"
	}
	if highSignals == 1 || mediumSignals >= 2 {
		return "medium"
	}
	return "low"
}

func inferSuggestedTests(files []string, components []string, risk string) []string {
	tests := []string{"go test ./..."}

	for _, component := range components {
		switch {
		case component == "server":
			tests = append(tests, "go test ./internal/server")
		case strings.HasPrefix(component, "function:"):
			name := strings.TrimPrefix(component, "function:")
			tests = append(tests, "go test ./internal/function/"+name)
		case component == "middleware":
			tests = append(tests, "go test ./internal/middleware")
		}
	}

	if risk == "high" {
		tests = append(tests,
			"Validar flujo end-to-end de endpoints afectados con JWT habilitado.",
			"Ejecutar smoke tests post-deploy en rutas criticas.",
		)
	}

	if touchesDocsOnly(files) {
		tests = append(tests, "No se detectan cambios funcionales: validacion manual de consistencia documental.")
	}

	return uniqueSorted(tests)
}

func buildReviewChecklist(risk string, components []string) []string {
	checklist := []string{
		"Verificar que la descripcion del PR cubre alcance y motivacion del cambio.",
		"Confirmar compatibilidad hacia atras en rutas/contratos existentes.",
		"Revisar manejo de errores y mensajes de validacion para nuevas rutas.",
	}

	if containsComponent(components, "server") {
		checklist = append(checklist, "Validar registro de rutas y proteccion JWT en server.")
	}
	if containsPrefixComponent(components, "function:") {
		checklist = append(checklist, "Asegurar separacion handler/service sin logica de negocio en transporte.")
	}
	if risk == "high" {
		checklist = append(checklist,
			"Exigir evidencia de pruebas de regresion y plan de rollback.",
			"Revisar impacto operacional y observabilidad post-deploy.",
		)
	}

	return uniqueSorted(checklist)
}

func inferScope(files []string, components []string) string {
	if len(files) == 0 {
		return "alcance no determinado"
	}
	if touchesDocsOnly(files) {
		return "documentacion"
	}
	if len(components) <= 2 {
		return "acotado"
	}
	if len(files) >= 10 {
		return "amplio"
	}
	return "moderado"
}

func suggestTitle(components []string, files []string, risk string) string {
	if len(files) == 0 {
		return "chore: actualizar cambios"
	}
	primary := "componentes"
	if len(components) > 0 {
		primary = components[0]
	}
	if risk == "high" {
		return fmt.Sprintf("feat: cambios criticos en %s", primary)
	}
	if risk == "medium" {
		return fmt.Sprintf("feat: mejoras en %s", primary)
	}
	return fmt.Sprintf("chore: ajustes en %s", primary)
}

func buildNarrativeSummary(files []string, components []string, risk string) string {
	if len(files) == 0 {
		return "No se pudo inferir informacion del diff proporcionado."
	}
	return fmt.Sprintf("PR con %d archivos modificados, componentes afectados: %s. Riesgo estimado: %s.", len(files), strings.Join(components, ", "), risk)
}

func touchesDocsOnly(files []string) bool {
	if len(files) == 0 {
		return false
	}
	for _, f := range files {
		lf := strings.ToLower(strings.TrimSpace(f))
		if !strings.HasPrefix(lf, "docs/") && !strings.HasSuffix(lf, ".md") {
			return false
		}
	}
	return true
}

func containsComponent(components []string, target string) bool {
	for _, c := range components {
		if c == target {
			return true
		}
	}
	return false
}

func containsPrefixComponent(components []string, prefix string) bool {
	for _, c := range components {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func uniqueSorted(values []string) []string {
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
	sort.Strings(out)
	return out
}
