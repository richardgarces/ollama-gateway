package onboarding

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Service struct {
	repoRoot string
	logger   *slog.Logger
}

type Guide struct {
	Role       string   `json:"role"`
	Title      string   `json:"title"`
	SetupSteps []string `json:"setup_steps"`
	Commands   []string `json:"commands"`
	KeyPaths   []string `json:"key_paths"`
	Tips       []string `json:"tips"`
	Applied    bool     `json:"applied"`
	OutputPath string   `json:"output_path,omitempty"`
}

func NewService(repoRoot string, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repoRoot: strings.TrimSpace(repoRoot), logger: logger}
}

func (s *Service) GenerateGuide(role string) (Guide, error) {
	normalizedRole := normalizeRole(role)
	if normalizedRole == "" {
		return Guide{}, fmt.Errorf("role invalido; soportados: backend, frontend, devops, qa")
	}

	guide := Guide{
		Role:    normalizedRole,
		Title:   "Onboarding " + strings.ToUpper(normalizedRole[:1]) + normalizedRole[1:],
		Applied: false,
	}

	commonSetup := []string{
		"Clonar repositorio y entrar al directorio api para flujos backend.",
		"Configurar variables de entorno base (REPO_ROOT, OLLAMA_URL, QDRANT_URL, JWT_SECRET).",
		"Levantar dependencias locales con docker compose segun necesidad (ollama/qdrant/mongo).",
	}
	commonCommands := []string{
		"cd api",
		"go test ./...",
		"make test-coverage",
	}
	commonPaths := []string{
		"api/internal/server/server.go",
		"api/internal/function/",
		"api/pkg/httputil/response.go",
		"docs/",
	}

	guide.SetupSteps = append(guide.SetupSteps, commonSetup...)
	guide.Commands = append(guide.Commands, commonCommands...)
	guide.KeyPaths = append(guide.KeyPaths, commonPaths...)

	switch normalizedRole {
	case "backend":
		guide.SetupSteps = append(guide.SetupSteps,
			"Revisar arquitectura limpia y convenciones de handlers/services/domain antes de implementar cambios.",
			"Validar nuevas rutas protegidas JWT y uso de helpers de respuesta en cada endpoint.",
		)
		guide.Commands = append(guide.Commands,
			"go test ./internal/server ./internal/function/...",
			"gofmt -w ./internal ./pkg",
		)
		guide.KeyPaths = append(guide.KeyPaths,
			"api/internal/function/core/domain/",
			"api/internal/middleware/",
			"api/internal/config/config.go",
		)
		guide.Tips = append(guide.Tips,
			"Prioriza inyeccion de dependencias y evita estado global.",
			"Para escritura de archivos, valida siempre que el destino quede dentro de REPO_ROOT.",
		)
	case "frontend":
		guide.SetupSteps = append(guide.SetupSteps,
			"Instalar dependencias de la extension VS Code y revisar comandos registrados.",
			"Probar interacciones clave: chat, slash commands y validaciones de modelos.",
		)
		guide.Commands = append(guide.Commands,
			"cd vscode-extension && npm install",
			"cd vscode-extension && npm run lint",
		)
		guide.KeyPaths = append(guide.KeyPaths,
			"vscode-extension/extension.js",
			"vscode-extension/package.json",
		)
		guide.Tips = append(guide.Tips,
			"Mantén consistencia de UX con estados de carga/error en comandos largos.",
			"Asegura compatibilidad con flujos locales sin degradar comandos existentes.",
		)
	case "devops":
		guide.SetupSteps = append(guide.SetupSteps,
			"Validar stack local en docker compose y salud de servicios dependientes.",
			"Configurar observabilidad base (logs, metricas, health checks) antes de pruebas de carga.",
		)
		guide.Commands = append(guide.Commands,
			"docker compose -f docker-compose.yml up -d",
			"curl -s http://localhost:8081/health/readiness",
		)
		guide.KeyPaths = append(guide.KeyPaths,
			"docker-compose.yml",
			"docker-compose.api.yml",
			"api/internal/utils/observability/",
		)
		guide.Tips = append(guide.Tips,
			"Automatiza validaciones pre-deploy con el endpoint /api/gate/deploy.",
			"Controla costos de modelos locales ajustando cache y politicas de pool.",
		)
	case "qa":
		guide.SetupSteps = append(guide.SetupSteps,
			"Preparar datos de prueba y escenarios de regresion para endpoints protegidos y legacy.",
			"Validar contratos de respuesta JSON y codigos HTTP en rutas criticas.",
		)
		guide.Commands = append(guide.Commands,
			"go test ./... -run Test",
			"cd api && go test ./internal/server -run TestGetRouteDefinitions",
		)
		guide.KeyPaths = append(guide.KeyPaths,
			"api/internal/server/server_test.go",
			"api/integration_test.go",
			"api/main_test.go",
		)
		guide.Tips = append(guide.Tips,
			"Enfoca pruebas en rutas /api/* autenticadas y cambios de compatibilidad de versiones.",
			"Incluye casos de error y validaciones de mensajes para endpoints nuevos.",
		)
	}

	guide.SetupSteps = uniqueStrings(guide.SetupSteps)
	guide.Commands = uniqueStrings(guide.Commands)
	guide.KeyPaths = uniqueStrings(guide.KeyPaths)
	guide.Tips = uniqueStrings(guide.Tips)

	return guide, nil
}

func (s *Service) SaveGuide(guide Guide) (string, error) {
	targetAbs, relPath, err := s.resolveGuidePath(guide.Role)
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

	if err := os.WriteFile(targetAbs, []byte(RenderMarkdown(guide)), mode); err != nil {
		return "", fmt.Errorf("no se pudo escribir guia: %w", err)
	}

	return relPath, nil
}

func (s *Service) resolveGuidePath(role string) (string, string, error) {
	rootAbs, err := filepath.Abs(strings.TrimSpace(s.repoRoot))
	if err != nil {
		return "", "", fmt.Errorf("REPO_ROOT invalido: %w", err)
	}

	normalizedRole := normalizeRole(role)
	if normalizedRole == "" {
		return "", "", fmt.Errorf("role invalido")
	}

	target := filepath.Join(rootAbs, "docs", "onboarding", normalizedRole+".md")
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

func RenderMarkdown(guide Guide) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(strings.TrimSpace(guide.Title))
	b.WriteString("\n\n")
	b.WriteString("Rol: `")
	b.WriteString(guide.Role)
	b.WriteString("`\n\n")
	writeSection(&b, "Setup", guide.SetupSteps)
	writeSection(&b, "Comandos utiles", guide.Commands)
	writeSection(&b, "Rutas clave", guide.KeyPaths)
	writeSection(&b, "Tips", guide.Tips)
	return b.String()
}

func writeSection(builder *strings.Builder, title string, items []string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n")
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(trimmed)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func normalizeRole(role string) string {
	value := strings.ToLower(strings.TrimSpace(role))
	switch value {
	case "backend", "frontend", "devops", "qa":
		return value
	default:
		return ""
	}
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
