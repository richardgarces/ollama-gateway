package sqlreview

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
)

type Service struct {
	logger *slog.Logger
}

type Finding struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Message        string `json:"message"`
	Evidence       string `json:"evidence,omitempty"`
	Recommendation string `json:"recommendation"`
}

type ReviewResult struct {
	Dialect                string    `json:"dialect"`
	Findings               []Finding `json:"findings"`
	GlobalRisk             string    `json:"global_risk"`
	RolloutRecommendations []string  `json:"rollout_recommendations"`
	RollbackChecks         []string  `json:"rollback_checks"`
	IdempotencyChecks      []string  `json:"idempotency_checks"`
}

func NewService(logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger}
}

func (s *Service) ReviewMigration(sql string, dialect string) (ReviewResult, error) {
	dialect = strings.TrimSpace(strings.ToLower(dialect))
	if dialect == "" {
		return ReviewResult{}, fmt.Errorf("dialect es requerido")
	}
	body := strings.TrimSpace(sql)
	if body == "" {
		return ReviewResult{}, fmt.Errorf("sql es requerido")
	}

	findings := make([]Finding, 0, 8)
	lower := strings.ToLower(body)
	analysisSQL := stripSQLComments(lower)

	findings = append(findings, detectLongLocks(analysisSQL)...)
	findings = append(findings, detectDropWithoutBackup(analysisSQL)...)
	findings = append(findings, detectCostlyAlter(analysisSQL)...)

	rollbackChecks := evaluateRollbackChecks(lower)
	idempotencyChecks := evaluateIdempotencyChecks(analysisSQL)

	globalRisk := computeGlobalRisk(findings)
	rolloutRecommendations := buildRolloutRecommendations(globalRisk, findings)

	return ReviewResult{
		Dialect:                dialect,
		Findings:               findings,
		GlobalRisk:             globalRisk,
		RolloutRecommendations: rolloutRecommendations,
		RollbackChecks:         rollbackChecks,
		IdempotencyChecks:      idempotencyChecks,
	}, nil
}

func detectLongLocks(sql string) []Finding {
	findings := make([]Finding, 0, 4)

	if strings.Contains(sql, "lock table") {
		findings = append(findings, Finding{
			Code:           "long_lock",
			Severity:       "high",
			Message:        "Uso explicito de LOCK TABLE puede generar bloqueo prolongado.",
			Evidence:       "LOCK TABLE",
			Recommendation: "Usar estrategias online y reducir ventana de lock con rollout por lotes.",
		})
	}

	if strings.Contains(sql, "alter table") {
		findings = append(findings, Finding{
			Code:           "alter_lock_risk",
			Severity:       "medium",
			Message:        "ALTER TABLE puede tomar lock de escritura dependiendo del motor.",
			Evidence:       "ALTER TABLE",
			Recommendation: "Ejecutar en ventana controlada, medir tiempo en staging y preparar rollback.",
		})
	}

	if strings.Contains(sql, "create index") && !strings.Contains(sql, "concurrently") {
		findings = append(findings, Finding{
			Code:           "index_lock_risk",
			Severity:       "medium",
			Message:        "CREATE INDEX sin modo online puede bloquear escrituras.",
			Evidence:       "CREATE INDEX",
			Recommendation: "Preferir creacion online (ej. CONCURRENTLY) o estrategia shadow index.",
		})
	}

	if hasUpdateOrDeleteWithoutWhere(sql) {
		findings = append(findings, Finding{
			Code:           "full_table_write",
			Severity:       "high",
			Message:        "UPDATE/DELETE sin WHERE puede bloquear y afectar toda la tabla.",
			Evidence:       "UPDATE/DELETE sin WHERE",
			Recommendation: "Dividir en batches por rango y validar cardinalidad antes de ejecutar.",
		})
	}

	return findings
}

func detectDropWithoutBackup(sql string) []Finding {
	findings := make([]Finding, 0, 3)
	hasBackupSignal := strings.Contains(sql, "backup") || strings.Contains(sql, "snapshot") || strings.Contains(sql, "restore point")

	if strings.Contains(sql, "drop table") || strings.Contains(sql, "drop column") || strings.Contains(sql, "truncate table") {
		severity := "high"
		recommendation := "Adjuntar plan de backup/snapshot y validacion de restore antes del deploy."
		if hasBackupSignal {
			severity = "medium"
			recommendation = "Validar restauracion del backup en entorno de ensayo antes de producir."
		}
		findings = append(findings, Finding{
			Code:           "destructive_change",
			Severity:       severity,
			Message:        "Operacion destructiva detectada (DROP/TRUNCATE).",
			Evidence:       "DROP/TRUNCATE",
			Recommendation: recommendation,
		})
	}

	return findings
}

func detectCostlyAlter(sql string) []Finding {
	findings := make([]Finding, 0, 3)

	if alterTypeRE.MatchString(sql) {
		findings = append(findings, Finding{
			Code:           "alter_type_rewrite",
			Severity:       "high",
			Message:        "ALTER COLUMN TYPE puede reescribir tabla completa en motores comunes.",
			Evidence:       "ALTER COLUMN ... TYPE",
			Recommendation: "Aplicar estrategia expand-contract con columna temporal y backfill por lotes.",
		})
	}

	if addNotNullDefaultRE.MatchString(sql) {
		findings = append(findings, Finding{
			Code:           "add_not_null_default",
			Severity:       "medium",
			Message:        "ADD COLUMN NOT NULL con DEFAULT puede ser costoso en tablas grandes.",
			Evidence:       "ADD COLUMN ... NOT NULL ... DEFAULT",
			Recommendation: "Agregar nullable, backfill por lotes y luego constraint NOT NULL.",
		})
	}

	return findings
}

func evaluateRollbackChecks(sql string) []string {
	checks := []string{}
	if strings.Contains(sql, "-- rollback") || strings.Contains(sql, "-- down") || strings.Contains(sql, "/* rollback") {
		checks = append(checks, "Plan de rollback declarado en la migracion.")
	} else {
		checks = append(checks, "Falta bloque explicito de rollback (agregar pasos DOWN o script de reversa).")
	}

	if strings.Contains(sql, "drop table") || strings.Contains(sql, "drop column") || strings.Contains(sql, "truncate table") {
		checks = append(checks, "Cambio destructivo requiere backup verificado y procedimiento de restore probado.")
	} else {
		checks = append(checks, "No se detectaron operaciones destructivas de alto impacto para rollback de datos.")
	}

	return checks
}

func evaluateIdempotencyChecks(sql string) []string {
	checks := make([]string, 0, 4)
	if strings.Contains(sql, "create table") && !strings.Contains(sql, "if not exists") {
		checks = append(checks, "CREATE TABLE sin IF NOT EXISTS: no idempotente.")
	}
	if strings.Contains(sql, "create index") && !strings.Contains(sql, "if not exists") {
		checks = append(checks, "CREATE INDEX sin IF NOT EXISTS: no idempotente.")
	}
	if strings.Contains(sql, "drop table") && !strings.Contains(sql, "if exists") {
		checks = append(checks, "DROP TABLE sin IF EXISTS: no idempotente.")
	}
	if strings.Contains(sql, "drop column") && !strings.Contains(sql, "if exists") {
		checks = append(checks, "DROP COLUMN sin IF EXISTS: no idempotente.")
	}

	if len(checks) == 0 {
		checks = append(checks, "No se detectaron problemas obvios de idempotencia en patrones comunes.")
	}
	return checks
}

func computeGlobalRisk(findings []Finding) string {
	high := 0
	medium := 0
	for _, f := range findings {
		switch strings.ToLower(strings.TrimSpace(f.Severity)) {
		case "high":
			high++
		case "medium":
			medium++
		}
	}
	if high > 0 {
		return "high"
	}
	if medium >= 2 {
		return "medium"
	}
	if medium == 1 || len(findings) > 0 {
		return "medium"
	}
	return "low"
}

func buildRolloutRecommendations(globalRisk string, findings []Finding) []string {
	recommendations := []string{
		"Ejecutar primero en entorno staging con volumen representativo y monitoreo activo.",
		"Aprobar rollout por fases (canary) y definir criterio de abort inmediato.",
	}

	if globalRisk == "high" {
		recommendations = append(recommendations,
			"Exigir backup validado y simulacion de restore antes de produccion.",
			"Programar ventana de mantenimiento y comunicacion preventiva a equipos afectados.",
		)
	}

	for _, f := range findings {
		if f.Code == "full_table_write" {
			recommendations = append(recommendations, "Reescribir cambios masivos en batches para minimizar lock y replication lag.")
			break
		}
	}

	return uniqueSorted(recommendations)
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
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

func hasUpdateOrDeleteWithoutWhere(sql string) bool {
	statements := strings.Split(sql, ";")
	for _, stmt := range statements {
		line := strings.TrimSpace(stmt)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "--") {
			continue
		}
		if strings.Contains(line, "update ") || strings.HasPrefix(line, "update") || strings.Contains(line, "delete from") {
			if !strings.Contains(line, " where ") {
				return true
			}
		}
	}
	return false
}

func stripSQLComments(sql string) string {
	if strings.TrimSpace(sql) == "" {
		return sql
	}

	withoutBlockComments := blockCommentRE.ReplaceAllString(sql, " ")
	lines := strings.Split(withoutBlockComments, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		clean = append(clean, line)
	}
	return strings.Join(clean, "\n")
}

var (
	alterTypeRE         = regexp.MustCompile(`(?is)\balter\s+table\b[^;]*\balter\s+column\b[^;]*\btype\b`)
	addNotNullDefaultRE = regexp.MustCompile(`(?is)\balter\s+table\b[^;]*\badd\s+column\b[^;]*\bnot\s+null\b[^;]*\bdefault\b`)
	blockCommentRE      = regexp.MustCompile(`(?s)/\*.*?\*/`)
)
