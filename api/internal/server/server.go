package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ollama-gateway/internal/config"
	agentservice "ollama-gateway/internal/function/agent"
	agenttransport "ollama-gateway/internal/function/agent/transport"
	apiexplorertransport "ollama-gateway/internal/function/api_explorer/transport"
	architectservice "ollama-gateway/internal/function/architect"
	architecttransport "ollama-gateway/internal/function/architect/transport"
	auditservice "ollama-gateway/internal/function/audit"
	audittransport "ollama-gateway/internal/function/audit/transport"
	authtransport "ollama-gateway/internal/function/auth/transport"
	ragcacheservice "ollama-gateway/internal/function/cache"
	chattransport "ollama-gateway/internal/function/chat/transport"
	cicdservice "ollama-gateway/internal/function/cicd"
	cicdtransport "ollama-gateway/internal/function/cicd/transport"
	commitgenservice "ollama-gateway/internal/function/commitgen"
	commitgentransport "ollama-gateway/internal/function/commitgen/transport"
	completeservice "ollama-gateway/internal/function/complete"
	completetransport "ollama-gateway/internal/function/complete/transport"
	contextservice "ollama-gateway/internal/function/context"
	contexttransport "ollama-gateway/internal/function/context/transport"
	coreservice "ollama-gateway/internal/function/core"
	"ollama-gateway/internal/function/core/domain"
	dashboardtransport "ollama-gateway/internal/function/dashboard/transport"
	debugservice "ollama-gateway/internal/function/debug"
	debugtransport "ollama-gateway/internal/function/debug/transport"
	docgenservice "ollama-gateway/internal/function/docgen"
	docgentransport "ollama-gateway/internal/function/docgen/transport"
	evalservice "ollama-gateway/internal/function/eval"
	evaltransport "ollama-gateway/internal/function/eval/transport"
	eventservice "ollama-gateway/internal/function/events"
	feedbackservice "ollama-gateway/internal/function/feedback"
	feedbacktransport "ollama-gateway/internal/function/feedback/transport"
	flagsservice "ollama-gateway/internal/function/flags"
	flagstransport "ollama-gateway/internal/function/flags/transport"
	gateservice "ollama-gateway/internal/function/gate"
	gatetransport "ollama-gateway/internal/function/gate/transport"
	generatetransport "ollama-gateway/internal/function/generate/transport"
	guardrailsservice "ollama-gateway/internal/function/guardrails"
	guardrailstransport "ollama-gateway/internal/function/guardrails/transport"
	healthtransport "ollama-gateway/internal/function/health/transport"
	indexerservice "ollama-gateway/internal/function/indexer"
	indexertransport "ollama-gateway/internal/function/indexer/transport"
	jobsservice "ollama-gateway/internal/function/jobs"
	jobstransport "ollama-gateway/internal/function/jobs/transport"
	memoryservice "ollama-gateway/internal/function/memory"
	memorytransport "ollama-gateway/internal/function/memory/transport"
	metricstransport "ollama-gateway/internal/function/metrics/transport"
	modelrecommenderservice "ollama-gateway/internal/function/model_recommender"
	modelrecommendertransport "ollama-gateway/internal/function/model_recommender/transport"
	modelstransport "ollama-gateway/internal/function/models/transport"
	onboardingservice "ollama-gateway/internal/function/onboarding"
	onboardingtransport "ollama-gateway/internal/function/onboarding/transport"
	openaitransport "ollama-gateway/internal/function/openai/transport"
	"ollama-gateway/internal/function/ostools"
	ostoolstransport "ollama-gateway/internal/function/ostools/transport"
	outboxservice "ollama-gateway/internal/function/outbox"
	outboxtransport "ollama-gateway/internal/function/outbox/transport"
	patchservice "ollama-gateway/internal/function/patch"
	patchtransport "ollama-gateway/internal/function/patch/transport"
	perfservice "ollama-gateway/internal/function/perf"
	perftransport "ollama-gateway/internal/function/perf/transport"
	plannerservice "ollama-gateway/internal/function/planner"
	plannertransport "ollama-gateway/internal/function/planner/transport"
	postmortemservice "ollama-gateway/internal/function/postmortem"
	postmortemtransport "ollama-gateway/internal/function/postmortem/transport"
	profileservice "ollama-gateway/internal/function/profile"
	profiletransport "ollama-gateway/internal/function/profile/transport"
	prsummaryservice "ollama-gateway/internal/function/prsummary"
	prsummarytransport "ollama-gateway/internal/function/prsummary/transport"
	releaseservice "ollama-gateway/internal/function/release"
	releasetransport "ollama-gateway/internal/function/release/transport"
	reposervice "ollama-gateway/internal/function/repo"
	repotransport "ollama-gateway/internal/function/repo/transport"
	reviewservice "ollama-gateway/internal/function/review"
	reviewtransport "ollama-gateway/internal/function/review/transport"
	runbookservice "ollama-gateway/internal/function/runbook"
	runbooktransport "ollama-gateway/internal/function/runbook/transport"
	runtimeconfigservice "ollama-gateway/internal/function/runtime_config"
	runtimeconfigtransport "ollama-gateway/internal/function/runtime_config/transport"
	sandboxservice "ollama-gateway/internal/function/sandbox"
	sandboxtransport "ollama-gateway/internal/function/sandbox/transport"
	searchservice "ollama-gateway/internal/function/search"
	searchtransport "ollama-gateway/internal/function/search/transport"
	securityservice "ollama-gateway/internal/function/security"
	securitytransport "ollama-gateway/internal/function/security/transport"
	sessionservice "ollama-gateway/internal/function/session"
	sessiontransport "ollama-gateway/internal/function/session/transport"
	sqlgenservice "ollama-gateway/internal/function/sqlgen"
	sqlgentransport "ollama-gateway/internal/function/sqlgen/transport"
	sqlreviewservice "ollama-gateway/internal/function/sqlreview"
	sqlreviewtransport "ollama-gateway/internal/function/sqlreview/transport"
	techdebtservice "ollama-gateway/internal/function/techdebt"
	techdebttransport "ollama-gateway/internal/function/techdebt/transport"
	testgenservice "ollama-gateway/internal/function/testgen"
	testgentransport "ollama-gateway/internal/function/testgen/transport"
	testintelservice "ollama-gateway/internal/function/testintel"
	testinteltransport "ollama-gateway/internal/function/testintel/transport"
	translatorservice "ollama-gateway/internal/function/translator"
	translatortransport "ollama-gateway/internal/function/translator/transport"
	wstransport "ollama-gateway/internal/function/ws/transport"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/internal/utils/observability"
	"ollama-gateway/pkg/cache"
	"ollama-gateway/pkg/httputil"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	cfg                 *config.Config
	router              http.Handler
	httpServer          *http.Server
	indexer             domain.Indexer
	cache               cache.Cache
	conversationService *coreservice.ConversationService
	profileService      *profileservice.Service
	feedbackService     *feedbackservice.Service
	flagsService        *flagsservice.Service
	outboxService       *outboxservice.Service
	jobsService         *jobsservice.Service
	eventBus            *eventservice.Bus
	eventCancel         context.CancelFunc
}

type RouteDefinition = domain.RouteDefinition

func GetRouteDefinitions() []RouteDefinition {
	return []RouteDefinition{
		{Method: "GET", Path: "/health", Description: "Liveness probe", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/health/liveness", Description: "Liveness detail", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/health/readiness", Description: "Readiness probe", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/health/readiness/{name}", Description: "Health check de un backend individual", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/metrics", Description: "Metricas JSON internas", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/metrics/value", Description: "Metricas de valor de negocio derivadas", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/metrics/traces/features", Description: "Trazas agregadas por feature con breakdown de etapas", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/api/tenant/context", Description: "Contexto tenant resuelto para la request actual", ExampleBody: "", Protected: true},
		{Method: "GET", Path: "/api/models", Description: "Modelos disponibles de Ollama", ExampleBody: "", Protected: false},
		{Method: "POST", Path: "/api/models/recommend", Description: "(Legacy) Recomendar modelo; responde con headers de deprecación", ExampleBody: "{\n  \"task_type\": \"code\",\n  \"sla_latency_ms\": 2000,\n  \"token_budget\": 3000\n}", Protected: true},
		{Method: "POST", Path: "/api/v1/models/recommend", Description: "Recomendar modelo (v1)", ExampleBody: "{\n  \"task_type\": \"code\",\n  \"sla_latency_ms\": 2000,\n  \"token_budget\": 3000\n}", Protected: true},
		{Method: "POST", Path: "/api/v2/models/recommend", Description: "Recomendar modelo (v2, acepta aliases de campos deprecados)", ExampleBody: "{\n  \"task_type\": \"code\",\n  \"sla_latency_ms\": 2000,\n  \"token_budget\": 3000\n}", Protected: true},
		{Method: "GET", Path: "/metrics/prometheus", Description: "Metricas Prometheus", ExampleBody: "", Protected: false},
		{Method: "POST", Path: "/login", Description: "Autenticacion JWT", ExampleBody: "{\n  \"username\": \"admin\",\n  \"password\": \"admin\"\n}", Protected: false},
		{Method: "GET", Path: "/dashboard", Description: "Dashboard de monitoreo", ExampleBody: "", Protected: false, LocalhostOnly: true},
		{Method: "GET", Path: "/internal/dashboard/status", Description: "Estado de dashboard", ExampleBody: "", Protected: false, LocalhostOnly: true},
		{Method: "GET", Path: "/internal/logs/stream", Description: "Stream SSE de logs", ExampleBody: "", Protected: false, LocalhostOnly: true, SSE: true},
		{Method: "GET", Path: "/internal/index/status", Description: "Estado del indexer", ExampleBody: "", Protected: false, LocalhostOnly: true},
		{Method: "POST", Path: "/internal/index/reindex", Description: "Reindexar repositorio", ExampleBody: "{}", Protected: false, LocalhostOnly: true},
		{Method: "POST", Path: "/internal/index/start", Description: "Iniciar watcher de indexer", ExampleBody: "{}", Protected: false, LocalhostOnly: true},
		{Method: "POST", Path: "/internal/index/stop", Description: "Detener watcher de indexer", ExampleBody: "{}", Protected: false, LocalhostOnly: true},
		{Method: "POST", Path: "/internal/index/reset", Description: "Resetear estado indexer", ExampleBody: "{}", Protected: false, LocalhostOnly: true},
		{Method: "GET", Path: "/api-docs", Description: "SPA API Explorer embebido", ExampleBody: "", Protected: false, LocalhostOnly: true},
		{Method: "GET", Path: "/internal/api-docs/routes", Description: "Definiciones de rutas para API explorer", ExampleBody: "", Protected: false, LocalhostOnly: true},
		{Method: "GET", Path: "/api/spec", Description: "Especificacion OpenAPI publica", ExampleBody: "", Protected: false},
		{Method: "POST", Path: "/api/search", Description: "(Legacy) Busqueda semantica; responde con headers de deprecación", ExampleBody: "{\n  \"query\": \"auth middleware\",\n  \"top\": 5\n}", Protected: false},
		{Method: "POST", Path: "/api/v1/search", Description: "Busqueda semantica (v1)", ExampleBody: "{\n  \"query\": \"auth middleware\",\n  \"top\": 5\n}", Protected: false},
		{Method: "POST", Path: "/api/v2/search", Description: "Busqueda semantica (v2, acepta aliases top_k/k)", ExampleBody: "{\n  \"query\": \"auth middleware\",\n  \"top\": 5\n}", Protected: false},
		{Method: "POST", Path: "/openai/v1/embeddings", Description: "OpenAI compatible embeddings", ExampleBody: "{\n  \"model\": \"nomic-embed-text:latest\",\n  \"input\": \"hola\"\n}", Protected: false},
		{Method: "POST", Path: "/openai/v1/completions", Description: "OpenAI compatible completions", ExampleBody: "{\n  \"model\": \"llama3\",\n  \"prompt\": \"Hello\"\n}", Protected: false},
		{Method: "POST", Path: "/complete", Description: "Code completion FIM with cursor context (<PRE><SUF><MID>)", ExampleBody: "{\n  \"model\": \"qwen2.5-coder\",\n  \"prefix\": \"func main() {\\n\",\n  \"suffix\": \"\\n}\",\n  \"language\": \"go\"\n}", Protected: false},
		{Method: "POST", Path: "/openai/v1/chat/completions", Description: "OpenAI compatible chat completions", ExampleBody: "{\n  \"model\": \"llama3\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"hola\"}]\n}", Protected: false},
		{Method: "GET", Path: "/ws/chat", Description: "WebSocket chat", ExampleBody: "", Protected: false},
		{Method: "POST", Path: "/api/generate", Description: "(Legacy) Generacion simple; responde con headers de deprecación", ExampleBody: "{\n  \"prompt\": \"Resume este texto\",\n  \"stream\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/v1/generate", Description: "Generacion simple (v1)", ExampleBody: "{\n  \"prompt\": \"Resume este texto\",\n  \"stream\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/v2/generate", Description: "Generacion simple (v2, acepta aliases query/input)", ExampleBody: "{\n  \"prompt\": \"Resume este texto\",\n  \"stream\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/agent", Description: "Ejecucion de agente", ExampleBody: "{\n  \"input\": \"Analiza el repo\"\n}", Protected: true},
		{Method: "POST", Path: "/api/agent/plan", Description: "Ejecutar plan multi-step para agente", ExampleBody: "{\n  \"steps\": [\n    {\"id\":\"step-1\",\"input\":\"analiza error\",\"retry_limit\":2,\"backoff_ms\":400},\n    {\"id\":\"step-2\",\"input\":\"sugiere fix\"}\n  ]\n}", Protected: true},
		{Method: "POST", Path: "/api/agent/autopilot", Description: "Agente autopilot con loop iterativo (SSE)", ExampleBody: "{\n  \"task\": \"Analiza la estructura del repo y sugiere mejoras\",\n  \"model\": \"qwen2.5:7b\"\n}", Protected: true},
		{Method: "POST", Path: "/api/flags", Description: "Crear feature flag", ExampleBody: "{\n  \"tenant\": \"default\",\n  \"feature\": \"postmortem\",\n  \"enabled\": true,\n  \"rollout_percentage\": 100\n}", Protected: true},
		{Method: "GET", Path: "/api/flags", Description: "Listar feature flags", ExampleBody: "", Protected: true},
		{Method: "GET", Path: "/api/flags/{feature}", Description: "Obtener feature flag por feature", ExampleBody: "", Protected: true},
		{Method: "PUT", Path: "/api/flags/{feature}", Description: "Actualizar feature flag", ExampleBody: "{\n  \"tenant\": \"default\",\n  \"enabled\": false,\n  \"rollout_percentage\": 0\n}", Protected: true},
		{Method: "DELETE", Path: "/api/flags/{feature}", Description: "Eliminar feature flag", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/postmortem/analyze", Description: "Analizar incidente y generar reporte postmortem", ExampleBody: "{\n  \"logs\": \"ERROR timeout contacting qdrant\",\n  \"start_time\": \"2026-04-04T10:00:00Z\",\n  \"end_time\": \"2026-04-04T10:10:00Z\",\n  \"commit_hash\": \"abc123def456\",\n  \"metrics\": {\"latency_ms\": 1832, \"error_rate\": 0.17}\n}", Protected: true},
		{Method: "POST", Path: "/api/runbooks/generate", Description: "Generar runbook operativo desde tipo de incidente y contexto", ExampleBody: "{\n  \"incident_type\": \"db-lock-timeout\",\n  \"context\": \"timeouts despues de migracion\",\n  \"apply\": false\n}", Protected: true},
		{Method: "GET", Path: "/api/runbooks", Description: "Listar playbooks/runbooks guardados", ExampleBody: "", Protected: true},
		{Method: "GET", Path: "/api/runbooks/{incident_type}", Description: "Obtener playbook/runbook por tipo de incidente", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/refactor", Description: "Refactor de archivo", ExampleBody: "{\n  \"path\": \"api/internal/server/server.go\",\n  \"prompt\": \"extrae helper\"\n}", Protected: true},
		{Method: "GET", Path: "/api/analyze-repo", Description: "Analisis de repositorio", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/review/diff", Description: "Code review de diff", ExampleBody: "{\n  \"diff\": \"diff --git ...\"\n}", Protected: true},
		{Method: "POST", Path: "/api/review/file", Description: "Code review de archivo", ExampleBody: "{\n  \"path\": \"api/internal/chat/transport/chat.go\"\n}", Protected: true},
		{Method: "POST", Path: "/api/docs/file", Description: "Generar docs para archivo", ExampleBody: "{\n  \"path\": \"api/internal/function/repo/repo.go\",\n  \"apply\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/docs/readme", Description: "Generar README", ExampleBody: "{\n  \"apply\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/debug/error", Description: "Analizar stack trace", ExampleBody: "{\n  \"stack_trace\": \"panic: ...\"\n}", Protected: true},
		{Method: "POST", Path: "/api/debug/log", Description: "Analizar logs", ExampleBody: "{\n  \"log\": \"error line\",\n  \"lines\": 200\n}", Protected: true},
		{Method: "POST", Path: "/api/translate", Description: "Traducir codigo", ExampleBody: "{\n  \"code\": \"print('hi')\",\n  \"from\": \"python\",\n  \"to\": \"go\"\n}", Protected: true},
		{Method: "POST", Path: "/api/translate/file", Description: "Traducir archivo", ExampleBody: "{\n  \"path\": \"api/internal/domain/models.go\",\n  \"to\": \"typescript\"\n}", Protected: true},
		{Method: "POST", Path: "/api/testgen", Description: "Generar tests desde codigo", ExampleBody: "{\n  \"lang\": \"go\",\n  \"code\": \"func Add(a,b int) int { return a+b }\"\n}", Protected: true},
		{Method: "POST", Path: "/api/testgen/file", Description: "Generar tests para archivo", ExampleBody: "{\n  \"path\": \"api/internal/function/repo/repo.go\",\n  \"apply\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/testintel/prioritize", Description: "Priorizar paquetes de tests y riesgo de merge a partir de diff", ExampleBody: "{\n  \"diff\": \"diff --git ...\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sql/query", Description: "Generar query SQL", ExampleBody: "{\n  \"description\": \"listar usuarios activos\",\n  \"dialect\": \"postgres\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sql/migration", Description: "Generar migracion SQL", ExampleBody: "{\n  \"description\": \"crear tabla sessions\",\n  \"dialect\": \"postgres\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sql/explain", Description: "Explicar query SQL", ExampleBody: "{\n  \"sql\": \"SELECT * FROM users WHERE id = 1\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sql/review", Description: "Revisar riesgo de migracion SQL", ExampleBody: "{\n  \"sql\": \"ALTER TABLE users ADD COLUMN status text\",\n  \"dialect\": \"postgres\"\n}", Protected: true},
		{Method: "GET", Path: "/api/perf/endpoints", Description: "Analizar ranking de endpoints criticos por latencia y error rate", ExampleBody: "", Protected: true},
		{Method: "GET", Path: "/api/gate/deploy", Description: "Validar gate pre-deploy con seguridad, cobertura y estado de tests", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/onboarding/guide", Description: "Generar guia de onboarding por rol y exportarla opcionalmente", ExampleBody: "{\n  \"role\": \"backend\",\n  \"apply\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/pr/summary", Description: "Generar resumen ejecutivo de PR a partir de diff", ExampleBody: "{\n  \"diff\": \"diff --git ...\"\n}", Protected: true},
		{Method: "POST", Path: "/api/cicd/generate", Description: "Generar pipeline CI/CD", ExampleBody: "{\n  \"platform\": \"github-actions\",\n  \"apply\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/cicd/optimize", Description: "Optimizar pipeline CI/CD", ExampleBody: "{\n  \"platform\": \"gitlab-ci\",\n  \"pipeline\": \"stages: [test]\"\n}", Protected: true},
		{Method: "POST", Path: "/api/commit/message", Description: "Generar commit message desde diff", ExampleBody: "{\n  \"diff\": \"diff --git ...\"\n}", Protected: true},
		{Method: "POST", Path: "/api/commit/staged", Description: "Generar commit message desde staged", ExampleBody: "{\n  \"repo_root\": \".\"\n}", Protected: true},
		{Method: "POST", Path: "/api/release/notes", Description: "Generar release notes entre dos referencias git", ExampleBody: "{\n  \"fromRef\": \"v1.0.0\",\n  \"toRef\": \"v1.1.0\",\n  \"apply\": false\n}", Protected: true},
		{Method: "GET", Path: "/api/audit/events", Description: "Consultar eventos de auditoria (JSON/CSV)", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/jobs", Description: "Crear job asíncrono (security.scan_repo, docs.readme, architect.analyze, indexer.reindex)", ExampleBody: "{\n  \"type\": \"security.scan_repo\",\n  \"params\": {}\n}", Protected: true},
		{Method: "GET", Path: "/api/jobs/{id}", Description: "Consultar estado de job", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/jobs/{id}/cancel", Description: "Cancelar job pendiente o en ejecución", ExampleBody: "{}", Protected: true},
		{Method: "GET", Path: "/api/jobs/{id}/result", Description: "Obtener resultado final de job", ExampleBody: "", Protected: true},
		{Method: "GET", Path: "/api/architect/analyze", Description: "Analisis de arquitectura", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/architect/refactor", Description: "Sugerencia de refactor", ExampleBody: "{\n  \"path\": \"api/internal/function/core/router.go\"\n}", Protected: true},
		{Method: "POST", Path: "/api/architect/patterns", Description: "Detectar patrones de refactor y riesgo por archivo", ExampleBody: "{\n  \"path\": \"api/internal/server/server.go\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sessions", Description: "Crear sesion compartida", ExampleBody: "{}", Protected: true},
		{Method: "POST", Path: "/api/sessions/{id}/join", Description: "Unirse a sesion", ExampleBody: "{}", Protected: true},
		{Method: "GET", Path: "/api/sessions/{id}/messages", Description: "Obtener mensajes de sesion", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/sessions/{id}/chat", Description: "Enviar chat a sesion", ExampleBody: "{\n  \"message\": \"hola equipo\"\n}", Protected: true},
		{Method: "PATCH", Path: "/api/sessions/{id}/participants/{user}/role", Description: "Actualizar rol de participante (owner/editor/viewer/moderator)", ExampleBody: "{\n  \"role\": \"editor\"\n}", Protected: true},
		{Method: "POST", Path: "/api/security/scan/file", Description: "Escanear seguridad de archivo", ExampleBody: "{\n  \"path\": \"api/internal/server/server.go\"\n}", Protected: true},
		{Method: "POST", Path: "/api/security/scan/repo", Description: "Escanear seguridad del repo", ExampleBody: "{}", Protected: true},
		{Method: "POST", Path: "/api/security/scan/secrets", Description: "Escanear secretos hardcodeados en el repo", ExampleBody: "{}", Protected: true},
		{Method: "POST", Path: "/api/security/policy/evaluate", Description: "Evaluar policy para acciones de alto impacto (apply/deploy/merge)", ExampleBody: "{\n  \"action\": \"cicd:apply\"\n}", Protected: true},
		{Method: "GET", Path: "/api/techdebt/priorities", Description: "Priorizar deuda técnica por señales de riesgo", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/memory/save", Description: "Guardar evento en memoria semántica persistente", ExampleBody: "{\n  \"summary\": \"se resolvió bug de auth\",\n  \"priority\": 8,\n  \"tags\": [\"auth\", \"fix\"]\n}", Protected: true},
		{Method: "POST", Path: "/api/memory/query", Description: "Consultar contexto histórico relevante", ExampleBody: "{\n  \"query\": \"error auth token\",\n  \"top_k\": 5\n}", Protected: true},
		{Method: "POST", Path: "/api/context/resolve", Description: "Resolver archivos de contexto por grafo de imports y prompt", ExampleBody: "{\n  \"file_path\": \"internal/server/server.go\",\n  \"prompt\": \"ruta de auth\",\n  \"top_k\": 8,\n  \"max_depth\": 2\n}", Protected: true},
		{Method: "POST", Path: "/api/feedback", Description: "Guardar feedback de calidad de respuesta", ExampleBody: "{\n  \"rating\": 4,\n  \"comment\": \"buena respuesta\",\n  \"request_id\": \"req-123\",\n  \"model\": \"qwen2.5:7b\",\n  \"metadata\": {\"task\": \"review\"}\n}", Protected: true},
		{Method: "GET", Path: "/api/feedback/summary", Description: "Resumen agregado de feedback por modelo", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/admin/outbox/retry", Description: "Reintento manual de eventos en dead-letter de outbox", ExampleBody: "{\n  \"id\": \"6611b7e8c6d3ef2c71f0a9b3\"\n}", Protected: true},
		{Method: "POST", Path: "/api/admin/config/reload", Description: "Recargar configuración en runtime sin reiniciar proceso", ExampleBody: "{}", Protected: true},
		{Method: "POST", Path: "/api/eval/run", Description: "Ejecutar benchmark de prompts por suite versionada", ExampleBody: "{\n  \"suite\": \"v1/default\"\n}", Protected: true},
		{Method: "GET", Path: "/api/eval/results/{id}", Description: "Obtener resultado de benchmark y exportes JSON/Markdown", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/v1/chat/completions", Description: "Chat completions interno", ExampleBody: "{\n  \"model\": \"llama3\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"hola\"}]\n}", Protected: true},
		{Method: "POST", Path: "/api/v2/chat/completions", Description: "Chat completions interno (v2)", ExampleBody: "{\n  \"model\": \"llama3\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"hola\"}]\n}", Protected: true},
		{Method: "GET", Path: "/api/profile", Description: "(Legacy) Obtener perfil", ExampleBody: "", Protected: true},
		{Method: "PUT", Path: "/api/profile", Description: "(Legacy) Actualizar perfil", ExampleBody: "{\n  \"default_model\": \"llama3\"\n}", Protected: true},
		{Method: "GET", Path: "/api/v1/profile", Description: "Obtener perfil (v1)", ExampleBody: "", Protected: true},
		{Method: "PUT", Path: "/api/v1/profile", Description: "Actualizar perfil (v1)", ExampleBody: "{\n  \"default_model\": \"llama3\"\n}", Protected: true},
		{Method: "GET", Path: "/api/v2/profile", Description: "Obtener perfil (v2)", ExampleBody: "", Protected: true},
		{Method: "PUT", Path: "/api/v2/profile", Description: "Actualizar perfil (v2)", ExampleBody: "{\n  \"default_model\": \"llama3\"\n}", Protected: true},
		{Method: "POST", Path: "/api/patch", Description: "Aplicar patch generado", ExampleBody: "{\n  \"response\": \"*** Begin Patch ...\",\n  \"apply\": true\n}", Protected: true},
		{Method: "GET", Path: "/api/patch/preview", Description: "Previsualizar patch", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/patch/sandbox/preview", Description: "Validar patch en sandbox aislado sin tocar repo real", ExampleBody: "{\n  \"response\": \"*** Begin Patch ...\"\n}", Protected: true},
		{Method: "POST", Path: "/api/patch/sandbox/apply", Description: "Aplicar patch real solo si la validación en sandbox es exitosa", ExampleBody: "{\n  \"response\": \"*** Begin Patch ...\"\n}", Protected: true},
		{Method: "GET", Path: "/api/guardrails/rules", Description: "Listar reglas de guardrails para apply de patch", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/os/exec", Description: "Ejecutar comando del sistema en el repo root", ExampleBody: "{\n  \"command\": \"go\",\n  \"args\": [\"version\"]\n}", Protected: true},
		{Method: "POST", Path: "/api/os/read", Description: "Leer contenido de un archivo", ExampleBody: "{\n  \"path\": \"go.mod\"\n}", Protected: true},
		{Method: "POST", Path: "/api/os/write", Description: "Escribir contenido a un archivo", ExampleBody: "{\n  \"path\": \"tmp/test.txt\",\n  \"content\": \"hello world\"\n}", Protected: true},
		{Method: "POST", Path: "/api/os/delete", Description: "Eliminar un archivo", ExampleBody: "{\n  \"path\": \"tmp/test.txt\"\n}", Protected: true},
		{Method: "POST", Path: "/api/os/list", Description: "Listar contenido de un directorio", ExampleBody: "{\n  \"path\": \".\"\n}", Protected: true},
		{Method: "POST", Path: "/api/os/exists", Description: "Verificar si un archivo o directorio existe", ExampleBody: "{\n  \"path\": \"go.mod\"\n}", Protected: true},
	}
}

func New(cfg *config.Config, cacheBackend cache.Cache) *Server {
	s := &Server{
		cfg:   cfg,
		cache: cacheBackend,
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	logger := slog.Default()
	metricsCollector := observability.NewMetricsCollector()
	logStream := observability.NewLogStream(500)
	rateLimiter := observability.NewRateLimiter(s.cfg.RateLimitRPM, time.Minute)
	eventCtx, eventCancel := context.WithCancel(context.Background())
	eventBus := eventservice.NewBus(eventCtx, eventservice.Options{BufferSize: 512, Workers: 2}, logger)
	s.eventBus = eventBus
	s.eventCancel = eventCancel
	repoRoots := s.cfg.RepoRoots
	if len(repoRoots) == 0 {
		repoRoots = []string{s.cfg.RepoRoot}
	}

	// Inicializar servicios con inyección de dependencias
	ollamaService := coreservice.NewOllamaService(s.cfg, logger, s.cache)
	routerService := coreservice.NewRouterService(s.cfg, ollamaService, logger)
	toolRegistry := coreservice.NewToolRegistry(s.cfg.AgentToolsDir, s.cfg.RepoRoot, logger)

	// Web search (Ollama cloud)
	webSearchService := searchservice.NewWebSearchService(s.cfg.OllamaAPIKey, logger)
	if webSearchService.Enabled() {
		webSearchTool := searchservice.NewWebSearchTool(webSearchService)
		toolRegistry.RegisterTool("web_search", "Busca información actualizada en la web. Args: {\"query\": \"tu consulta\"}", webSearchTool)
		logger.Info("web_search tool registrada (OLLAMA_API_KEY presente)")
	} else {
		logger.Info("web_search tool deshabilitada (OLLAMA_API_KEY vacía)")
	}

	agentService := agentservice.NewService(ollamaService, logger, toolRegistry)
	autopilotService := agentservice.NewAutopilotService(ollamaService, logger, toolRegistry)
	conversationService, err := coreservice.NewConversationServiceWithPool(
		s.cfg.MongoURI,
		s.cfg.MongoPoolMaxOpen,
		s.cfg.MongoPoolMaxIdle,
		s.cfg.MongoPoolTimeoutSeconds,
		logger,
	)
	if err != nil {
		sqlitePath := filepath.Join(s.cfg.RepoRoot, ".chat_history.sqlite")
		conversationService, err = coreservice.NewConversationServiceSQLite(sqlitePath, logger)
		if err != nil {
			logger.Warn("conversation service no disponible; se continuará sin persistencia", slog.String("error", err.Error()))
		} else {
			logger.Warn("mongo no disponible; se usará sqlite para persistencia de conversación", slog.String("path", sqlitePath))
			s.conversationService = conversationService
		}
	} else {
		s.conversationService = conversationService
	}
	profileService, err := profileservice.NewMongoServiceWithPool(
		s.cfg.MongoURI,
		s.cfg.MongoPoolMaxOpen,
		s.cfg.MongoPoolMaxIdle,
		s.cfg.MongoPoolTimeoutSeconds,
		logger,
	)
	if err != nil {
		logger.Warn("profile service no disponible; se continuará sin perfiles", slog.String("error", err.Error()))
	} else {
		s.profileService = profileService
	}
	feedbackService, err := feedbackservice.NewServiceWithPool(
		s.cfg.MongoURI,
		s.cfg.MongoPoolMaxOpen,
		s.cfg.MongoPoolMaxIdle,
		s.cfg.MongoPoolTimeoutSeconds,
		logger,
	)
	if err != nil {
		logger.Warn("feedback service no disponible; se continuará sin feedback loop", slog.String("error", err.Error()))
	} else {
		s.feedbackService = feedbackService
		routerService.SetFeedbackProvider(feedbackService)
	}
	outboxSvc, err := outboxservice.NewServiceWithPool(
		s.cfg.MongoURI,
		s.cfg.MongoPoolMaxOpen,
		s.cfg.MongoPoolMaxIdle,
		s.cfg.MongoPoolTimeoutSeconds,
		eventBus,
		time.Duration(s.cfg.OutboxWorkerIntervalSec)*time.Second,
		s.cfg.OutboxBatchSize,
		s.cfg.OutboxMaxAttempts,
		time.Duration(s.cfg.OutboxRetryBackoffSec)*time.Second,
		logger,
	)
	if err != nil {
		logger.Warn("outbox service no disponible; worker deshabilitado", slog.String("error", err.Error()))
	} else {
		s.outboxService = outboxSvc
		outboxSvc.Start(context.Background())
	}
	flagsSvc, err := flagsservice.NewServiceWithPool(
		s.cfg.MongoURI,
		s.cfg.MongoPoolMaxOpen,
		s.cfg.MongoPoolMaxIdle,
		s.cfg.MongoPoolTimeoutSeconds,
		30*time.Second,
		logger,
	)
	if err != nil {
		logger.Warn("flags service no disponible; feature flags deshabilitadas", slog.String("error", err.Error()))
	} else {
		s.flagsService = flagsSvc
	}
	modelRecommenderService := modelrecommenderservice.NewService(logger)
	if feedbackService != nil {
		modelRecommenderService.SetFeedbackProvider(feedbackService)
	}
	routerService.SetModelHintProvider(modelRecommenderService)
	guardrailsService := guardrailsservice.NewService(logger)
	patchService := patchservice.NewService(logger)
	sandboxPatchService := sandboxservice.NewService(s.cfg.RepoRoot, patchService, guardrailsService, logger)
	repoService := reposervice.NewService(ollamaService, s.cfg.RepoRoot, logger)
	qdrantService := coreservice.NewQdrantService(
		s.cfg.QdrantURL,
		s.cfg.RepoRoot,
		s.cfg.VectorStorePath,
		s.cfg.VectorStorePreferLocal,
		s.cfg.HTTPTimeoutSeconds,
		s.cfg.HTTPMaxRetries,
		thresholdOrFallback(s.cfg.CBQdrantThreshold, s.cfg.CBFailureThreshold),
		s.cfg.CBOpenTimeoutSeconds,
		s.cfg.CBHalfOpenMaxSuccess,
		s.cfg.PoolMaxOpen,
		s.cfg.PoolMaxIdle,
		s.cfg.PoolTimeoutSeconds,
		logger,
	)
	ollamaService.SetPoolObserver(metricsCollector)
	ragService := coreservice.NewRAGService(
		ollamaService,
		routerService,
		qdrantService,
		logger,
		s.cache,
		repoRoots,
		s.cfg.PromptLang,
		s.cfg.RAGCacheTTLSeconds,
		s.cfg.RAGCacheMaxEntries,
	)
	ragService.SetRetrievalPool(s.cfg.RetrievalPoolSize, metricsCollector)
	distributedRAGCache := ragcacheservice.NewService(
		s.cache,
		time.Duration(s.cfg.RAGCacheTTLSeconds)*time.Second,
		"rag-distributed",
		logger,
		metricsCollector,
	)
	ragService.SetDistributedCache(distributedRAGCache)
	contextService := contextservice.NewService(repoRoots, logger)
	ragService.SetContextResolver(contextService)
	evalService := evalservice.NewService(s.cfg.RepoRoot, ragService, logger)
	memoryService, err := memoryservice.NewServiceWithPool(
		s.cfg.MongoURI,
		s.cfg.MongoPoolMaxOpen,
		s.cfg.MongoPoolMaxIdle,
		s.cfg.MongoPoolTimeoutSeconds,
		ollamaService,
		qdrantService,
		s.cfg.RepoRoot,
		s.cfg.MemoryTTLHours,
		s.cfg.MemoryPruneMaxEntries,
		logger,
	)
	if err != nil {
		logger.Warn("memory service no disponible; se continuará sin memoria semántica", slog.String("error", err.Error()))
	} else {
		ragService.SetMemoryProvider(memoryService)
	}
	reviewService := reviewservice.NewService(ragService, s.cfg.RepoRoot, logger)
	docGenService := docgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	debugService := debugservice.NewService(ragService, s.cfg.RepoRoot, logger)
	translatorService := translatorservice.NewService(ragService, s.cfg.RepoRoot, logger)
	testGenService := testgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	sqlGenService := sqlgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	sqlReviewService := sqlreviewservice.NewService(logger)
	perfService := perfservice.NewService(metricsCollector)
	cicdService := cicdservice.NewService(ragService, s.cfg.RepoRoot, logger)
	commitGenService := commitgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	releaseService := releaseservice.NewService(s.cfg.RepoRoot)
	runbookService := runbookservice.NewService(s.cfg.RepoRoot, logger)
	onboardingService := onboardingservice.NewService(s.cfg.RepoRoot, logger)
	prSummaryService := prsummaryservice.NewService(logger)
	sessionService := sessionservice.NewService(eventBus)
	securityService := securityservice.NewService(ragService, s.cfg.RepoRoot, logger)
	gateService := gateservice.NewService(s.cfg.RepoRoot, securityService, logger)
	techDebtService := techdebtservice.NewService(s.cfg.RepoRoot, logger)
	indexerService, _ := indexerservice.NewService(repoRoots, s.cfg.IndexerStatePath, ollamaService, qdrantService, logger)
	indexerService.SetEventPublisher(eventBus)
	indexerService.SetOnContentChange(func() {
		ragService.InvalidateResponseCache()
		for _, root := range repoRoots {
			_ = distributedRAGCache.InvalidateRepo(context.Background(), root)
		}
	})
	eventBus.Subscribe(eventservice.EventFileIndexed, func(ctx context.Context, event eventservice.Event) {
		ev, ok := event.(eventservice.FileIndexed)
		if !ok {
			return
		}
		if ev.RepoRoot != "" {
			_ = distributedRAGCache.InvalidateRepo(context.Background(), ev.RepoRoot)
		}
		findings, err := securityService.ScanFile(ev.Path)
		if err != nil {
			logger.Debug("security scan en indexer falló", slog.String("path", ev.Path), slog.String("error", err.Error()))
			return
		}
		for _, finding := range findings {
			if securityservice.IsHighSeverity(finding.Severity) {
				logger.Warn("security finding detectado",
					slog.String("path", finding.Path),
					slog.String("severity", finding.Severity),
					slog.String("category", finding.Category),
					slog.Int("line", finding.Line),
					slog.String("description", finding.Description),
				)
			}
		}
	})
	eventBus.Subscribe(eventservice.EventSessionCreated, func(ctx context.Context, event eventservice.Event) {
		ev, ok := event.(eventservice.SessionCreated)
		if !ok {
			return
		}
		logger.Info("session creada", slog.String("session_id", ev.SessionID), slog.String("owner_id", ev.OwnerID))
	})
	eventBus.Subscribe(eventservice.EventRequestCompleted, func(ctx context.Context, event eventservice.Event) {
		ev, ok := event.(eventservice.RequestCompleted)
		if !ok {
			return
		}
		logger.Debug("request completado",
			slog.String("request_id", ev.RequestID),
			slog.String("path", ev.Path),
			slog.Int("status", ev.StatusCode),
			slog.Int64("duration_ms", ev.DurationMS),
		)
	})
	architectService := architectservice.NewService(ragService, s.cfg.RepoRoot, indexerService, logger)
	jobsService := jobsservice.NewService(jobsservice.Dependencies{
		Security:  securityService,
		DocGen:    docGenService,
		Architect: architectService,
		Indexer:   indexerService,
		Logger:    logger,
		Workers:   3,
		QueueSize: 256,
	})
	s.jobsService = jobsService
	plannerService := plannerservice.NewService(agentService, logger)
	postmortemService := postmortemservice.NewService(logger)

	var ollamaClient domain.OllamaClient = ollamaService
	var vectorStore domain.VectorStore = qdrantService
	var ragEngine domain.RAGEngine = ragService
	var indexer domain.Indexer = indexerService
	var agentRunner domain.AgentRunner = agentService
	s.indexer = indexer

	// Inicializar handlers
	authHandler := authtransport.NewHandler(s.cfg.JWTSecret)
	generateHandler := generatetransport.NewHandler(ragEngine)
	generateHandler.SetEventPublisher(eventBus)
	agentHandler := agenttransport.NewHandler(agentRunner)
	autopilotHandler := agenttransport.NewAutopilotHandler(autopilotService)
	plannerHandler := plannertransport.NewHandler(plannerService)
	flagsHandler := flagstransport.NewHandler(flagsSvc)
	postmortemHandler := postmortemtransport.NewHandler(postmortemService)
	chatHandler := chattransport.NewHandler(ragEngine)
	repoHandler := repotransport.NewHandler(repoService)
	reviewHandler := reviewtransport.NewHandler(reviewService)
	docGenHandler := docgentransport.NewHandler(docGenService)
	debugHandler := debugtransport.NewHandler(debugService)
	translatorHandler := translatortransport.NewHandler(translatorService)
	testGenHandler := testgentransport.NewHandler(testGenService)
	sqlGenHandler := sqlgentransport.NewHandler(sqlGenService)
	sqlReviewHandler := sqlreviewtransport.NewHandler(sqlReviewService)
	perfHandler := perftransport.NewHandler(perfService)
	gateHandler := gatetransport.NewHandler(gateService)
	cicdHandler := cicdtransport.NewHandler(cicdService)
	commitGenHandler := commitgentransport.NewHandler(commitGenService)
	releaseHandler := releasetransport.NewHandler(releaseService)
	runbookHandler := runbooktransport.NewHandler(runbookService)
	onboardingHandler := onboardingtransport.NewHandler(onboardingService)
	prSummaryHandler := prsummarytransport.NewHandler(prSummaryService)
	sessionHandler := sessiontransport.NewHandler(sessionService, ragEngine)
	securityHandler := securitytransport.NewHandler(securityService)
	jobsHandler := jobstransport.NewHandler(jobsService)
	techDebtHandler := techdebttransport.NewHandler(techDebtService)
	testIntelHandler := testinteltransport.NewHandler(testintelservice.NewService())
	architectHandler := architecttransport.NewHandler(architectService)
	profileHandler := profiletransport.NewHandler(s.profileService)
	patchHandler := patchtransport.NewHandler(s.cfg.RepoRoot, patchService, guardrailsService)
	sandboxHandler := sandboxtransport.NewHandler(sandboxPatchService)
	guardrailsHandler := guardrailstransport.NewHandler(guardrailsService)
	ostoolsService, ostoolsErr := ostools.NewService(s.cfg.RepoRoot, logger)
	if ostoolsErr != nil {
		logger.Warn("ostools service no disponible", slog.String("error", ostoolsErr.Error()))
	}
	ostoolsHandler := ostoolstransport.NewHandler(ostoolsService)
	metricsHandler := metricstransport.NewHandler(metricsCollector)
	contextHandler := contexttransport.NewHandler(contextService)
	memoryHandler := memorytransport.NewHandler(memoryService)
	feedbackHandler := feedbacktransport.NewHandlerWithMetrics(feedbackService, metricsCollector)
	outboxHandler := outboxtransport.NewHandler(outboxSvc)
	runtimeConfigHandler := runtimeconfigtransport.NewHandler(runtimeconfigservice.NewService(s.cfg))
	modelRecommenderHandler := modelrecommendertransport.NewHandler(modelRecommenderService)
	evalHandler := evaltransport.NewHandler(evalService)
	modelsHandler := modelstransport.NewHandler(ollamaService)
	indexerHandler := indexertransport.NewHandler(indexer)
	dashboardHandler := dashboardtransport.NewHandler(s.cfg, metricsCollector, indexerService, logStream)
	searchHandler := searchtransport.NewHandler(ollamaClient, vectorStore, repoRoots)
	openaiHandler := openaitransport.NewHandler(ollamaClient, ragEngine, s.conversationService, s.profileService)
	if webSearchService.Enabled() {
		openaiHandler.SetWebSearcher(webSearchService)
	}
	completeService := completeservice.NewService(ollamaService, s.cfg.FIMModel)
	completeHandler := completetransport.NewHandler(completeService)
	wsHandler := wstransport.NewHandler(ragEngine, s.cfg.JWTSecret)
	apiExplorerHandler := apiexplorertransport.NewHandler(GetRouteDefinitions())
	healthHandler := healthtransport.NewHandler(s.cfg)
	healthHandler.SetCircuitBreakers(ollamaService, qdrantService)
	authMiddleware := middleware.NewAuthMiddleware(s.cfg.JWTSecret, s.cfg.JWTEnabled)
	auditSvc := auditservice.NewService(5000)
	middleware.SetAuditRecorder(auditSvc)
	auditHandler := audittransport.NewHandler(auditSvc)
	localhostOnly := middleware.LocalhostOnly
	legacyDeprecationDate := "2026-12-31"
	legacy := func(next http.Handler, successorPath string) http.Handler {
		return middleware.WithDeprecationHeaders(next, successorPath, legacyDeprecationDate)
	}
	v2Generate := middleware.WithJSONFieldAliases(http.HandlerFunc(generateHandler.Handle), map[string]string{
		"query": "prompt",
		"input": "prompt",
	})
	v2Search := middleware.WithJSONFieldAliases(http.HandlerFunc(searchHandler.Handle), map[string]string{
		"q":     "query",
		"top_k": "top",
		"k":     "top",
	})
	v2ModelRecommend := middleware.WithJSONFieldAliases(http.HandlerFunc(modelRecommenderHandler.Recommend), map[string]string{
		"task":          "task_type",
		"sla_ms":        "sla_latency_ms",
		"budget_tokens": "token_budget",
	})
	v2InternalChat := middleware.WithJSONFieldAliases(http.HandlerFunc(chatHandler.Handle), map[string]string{
		"conversation_id": "session_id",
	})
	featureGate := func(feature string, handler http.Handler) http.Handler {
		return middleware.RequireFeatureFlag(flagsSvc, feature)(handler)
	}
	scopeGate := func(scope string, handler http.Handler) http.Handler {
		return middleware.RequireScope(scope)(handler)
	}

	mux := http.NewServeMux()

	// Rutas públicas
	mux.HandleFunc("GET /health", healthHandler.Liveness)
	mux.HandleFunc("GET /health/liveness", healthHandler.Liveness)
	mux.HandleFunc("GET /health/readiness", healthHandler.Readiness)
	mux.HandleFunc("GET /health/readiness/{name}", healthHandler.Backend)
	mux.HandleFunc("GET /metrics", metricsHandler.Handle)
	mux.HandleFunc("GET /metrics/value", metricsHandler.Value)
	mux.HandleFunc("GET /metrics/traces/features", metricsHandler.TraceFeatures)
	mux.HandleFunc("GET /api/models", modelsHandler.List)
	// Prometheus scrape endpoint
	mux.Handle("GET /metrics/prometheus", promhttp.Handler())
	mux.HandleFunc("POST /login", authHandler.Login)

	// Dashboard interno (solo localhost)
	mux.Handle("GET /dashboard", localhostOnly(http.HandlerFunc(dashboardHandler.Handle)))
	mux.Handle("GET /api-docs", localhostOnly(http.HandlerFunc(apiExplorerHandler.Handle)))
	mux.Handle("GET /internal/dashboard/status", localhostOnly(http.HandlerFunc(dashboardHandler.Status)))
	mux.Handle("GET /internal/logs/stream", localhostOnly(http.HandlerFunc(dashboardHandler.LogsStream)))
	mux.Handle("GET /internal/api-docs/routes", localhostOnly(http.HandlerFunc(apiExplorerHandler.Routes)))
	mux.HandleFunc("GET /api/spec", func(w http.ResponseWriter, r *http.Request) {
		for _, candidate := range []string{
			filepath.Join(s.cfg.RepoRoot, "api", "docs", "openapi.json"),
			filepath.Join(s.cfg.RepoRoot, "docs", "openapi.json"),
		} {
			path, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				if errors.Is(readErr, os.ErrNotExist) {
					continue
				}
				http.Error(w, `{"error":"no se pudo leer api spec"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		http.Error(w, `{"error":"api spec no encontrada"}`, http.StatusNotFound)
	})

	// Indexer control (internal, solo localhost — sin JWT, protegido por localhostOnly)
	mux.Handle("GET /internal/index/status", localhostOnly(http.HandlerFunc(indexerHandler.Status)))
	mux.Handle("POST /internal/index/reindex", localhostOnly(http.HandlerFunc(indexerHandler.Reindex)))
	mux.Handle("POST /internal/index/start", localhostOnly(http.HandlerFunc(indexerHandler.StartWatcher)))
	mux.Handle("POST /internal/index/stop", localhostOnly(http.HandlerFunc(indexerHandler.StopWatcher)))
	mux.Handle("POST /internal/index/reset", localhostOnly(http.HandlerFunc(indexerHandler.ResetState)))
	mux.Handle("POST /api/search", legacy(http.HandlerFunc(searchHandler.Handle), "/api/v2/search"))
	mux.HandleFunc("POST /api/v1/search", searchHandler.Handle)
	mux.Handle("POST /api/v2/search", v2Search)
	// OpenAI-compatible endpoints
	mux.HandleFunc("POST /openai/v1/embeddings", openaiHandler.Embeddings)
	mux.HandleFunc("POST /openai/v1/completions", openaiHandler.Completions)
	mux.HandleFunc("POST /complete", completeHandler.Complete)
	mux.HandleFunc("POST /openai/v1/chat/completions", openaiHandler.ChatCompletions)
	mux.HandleFunc("GET /ws/chat", wsHandler.Handle)

	// Rutas protegidas con JWT
	mux.Handle("POST /api/generate", authMiddleware.JWT(legacy(http.HandlerFunc(generateHandler.Handle), "/api/v2/generate")))
	mux.Handle("POST /api/v1/generate", authMiddleware.JWT(http.HandlerFunc(generateHandler.Handle)))
	mux.Handle("POST /api/v2/generate", authMiddleware.JWT(v2Generate))
	mux.Handle("POST /api/agent", authMiddleware.JWT(http.HandlerFunc(agentHandler.Handle)))
	mux.Handle("GET /api/tenant/context", authMiddleware.JWT(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"tenant": middleware.TenantFromContext(r.Context())})
	})))
	mux.Handle("POST /api/agent/plan", authMiddleware.JWT(http.HandlerFunc(plannerHandler.ExecutePlan)))
	mux.Handle("POST /api/agent/autopilot", authMiddleware.JWT(http.HandlerFunc(autopilotHandler.Handle)))
	mux.Handle("POST /api/flags", authMiddleware.JWT(http.HandlerFunc(flagsHandler.Create)))
	mux.Handle("GET /api/flags", authMiddleware.JWT(http.HandlerFunc(flagsHandler.List)))
	mux.Handle("GET /api/flags/{feature}", authMiddleware.JWT(http.HandlerFunc(flagsHandler.Get)))
	mux.Handle("PUT /api/flags/{feature}", authMiddleware.JWT(http.HandlerFunc(flagsHandler.Update)))
	mux.Handle("DELETE /api/flags/{feature}", authMiddleware.JWT(http.HandlerFunc(flagsHandler.Delete)))
	mux.Handle("POST /api/postmortem/analyze", authMiddleware.JWT(featureGate("postmortem", http.HandlerFunc(postmortemHandler.Analyze))))
	mux.Handle("POST /api/runbooks/generate", authMiddleware.JWT(featureGate("runbooks", http.HandlerFunc(runbookHandler.Generate))))
	mux.Handle("GET /api/runbooks", authMiddleware.JWT(featureGate("runbooks", http.HandlerFunc(runbookHandler.List))))
	mux.Handle("GET /api/runbooks/{incident_type}", authMiddleware.JWT(featureGate("runbooks", http.HandlerFunc(runbookHandler.Get))))
	mux.Handle("POST /api/refactor", authMiddleware.JWT(http.HandlerFunc(repoHandler.Refactor)))
	mux.Handle("GET /api/analyze-repo", authMiddleware.JWT(http.HandlerFunc(repoHandler.Analyze)))
	mux.Handle("POST /api/review/diff", authMiddleware.JWT(http.HandlerFunc(reviewHandler.ReviewDiff)))
	mux.Handle("POST /api/review/file", authMiddleware.JWT(http.HandlerFunc(reviewHandler.ReviewFile)))
	mux.Handle("POST /api/docs/file", authMiddleware.JWT(scopeGate("docs:apply", http.HandlerFunc(docGenHandler.GenerateFileDoc))))
	mux.Handle("POST /api/docs/readme", authMiddleware.JWT(scopeGate("docs:apply", http.HandlerFunc(docGenHandler.GenerateREADME))))
	mux.Handle("POST /api/debug/error", authMiddleware.JWT(http.HandlerFunc(debugHandler.AnalyzeError)))
	mux.Handle("POST /api/debug/log", authMiddleware.JWT(http.HandlerFunc(debugHandler.AnalyzeLog)))
	mux.Handle("POST /api/translate", authMiddleware.JWT(http.HandlerFunc(translatorHandler.Translate)))
	mux.Handle("POST /api/translate/file", authMiddleware.JWT(http.HandlerFunc(translatorHandler.TranslateFile)))
	mux.Handle("POST /api/testgen", authMiddleware.JWT(http.HandlerFunc(testGenHandler.Generate)))
	mux.Handle("POST /api/testgen/file", authMiddleware.JWT(http.HandlerFunc(testGenHandler.GenerateForFile)))
	mux.Handle("POST /api/testintel/prioritize", authMiddleware.JWT(scopeGate("test:analyze", http.HandlerFunc(testIntelHandler.Prioritize))))
	mux.Handle("POST /api/sql/query", authMiddleware.JWT(http.HandlerFunc(sqlGenHandler.GenerateQuery)))
	mux.Handle("POST /api/sql/migration", authMiddleware.JWT(http.HandlerFunc(sqlGenHandler.GenerateMigration)))
	mux.Handle("POST /api/sql/explain", authMiddleware.JWT(http.HandlerFunc(sqlGenHandler.ExplainQuery)))
	mux.Handle("POST /api/sql/review", authMiddleware.JWT(featureGate("sql_review", http.HandlerFunc(sqlReviewHandler.Review))))
	mux.Handle("GET /api/perf/endpoints", authMiddleware.JWT(featureGate("perf_endpoints", http.HandlerFunc(perfHandler.AnalyzeEndpoints))))
	mux.Handle("GET /api/gate/deploy", authMiddleware.JWT(featureGate("deploy_gate", http.HandlerFunc(gateHandler.CheckDeployGate))))
	mux.Handle("POST /api/onboarding/guide", authMiddleware.JWT(featureGate("onboarding_guide", http.HandlerFunc(onboardingHandler.GenerateGuide))))
	mux.Handle("POST /api/pr/summary", authMiddleware.JWT(featureGate("pr_summary", http.HandlerFunc(prSummaryHandler.Summarize))))
	mux.Handle("POST /api/cicd/generate", authMiddleware.JWT(scopeGate("cicd:apply", http.HandlerFunc(cicdHandler.GeneratePipeline))))
	mux.Handle("POST /api/cicd/optimize", authMiddleware.JWT(scopeGate("cicd:apply", http.HandlerFunc(cicdHandler.OptimizePipeline))))
	mux.Handle("POST /api/commit/message", authMiddleware.JWT(http.HandlerFunc(commitGenHandler.Message)))
	mux.Handle("POST /api/commit/staged", authMiddleware.JWT(http.HandlerFunc(commitGenHandler.Staged)))
	mux.Handle("POST /api/release/notes", authMiddleware.JWT(http.HandlerFunc(releaseHandler.BuildNotes)))
	mux.Handle("GET /api/audit/events", authMiddleware.JWT(scopeGate("audit:read", http.HandlerFunc(auditHandler.List))))
	mux.Handle("POST /api/jobs", authMiddleware.JWT(scopeGate("jobs:manage", http.HandlerFunc(jobsHandler.Create))))
	mux.Handle("GET /api/jobs/{id}", authMiddleware.JWT(scopeGate("jobs:read", http.HandlerFunc(jobsHandler.Get))))
	mux.Handle("POST /api/jobs/{id}/cancel", authMiddleware.JWT(scopeGate("jobs:manage", http.HandlerFunc(jobsHandler.Cancel))))
	mux.Handle("GET /api/jobs/{id}/result", authMiddleware.JWT(scopeGate("jobs:read", http.HandlerFunc(jobsHandler.Result))))
	mux.Handle("GET /api/architect/analyze", authMiddleware.JWT(http.HandlerFunc(architectHandler.AnalyzeProject)))
	mux.Handle("POST /api/architect/refactor", authMiddleware.JWT(http.HandlerFunc(architectHandler.SuggestRefactor)))
	mux.Handle("POST /api/architect/patterns", authMiddleware.JWT(http.HandlerFunc(architectHandler.DetectPatterns)))
	mux.Handle("POST /api/sessions", authMiddleware.JWT(http.HandlerFunc(sessionHandler.Create)))
	mux.Handle("POST /api/sessions/{id}/join", authMiddleware.JWT(http.HandlerFunc(sessionHandler.Join)))
	mux.Handle("GET /api/sessions/{id}/messages", authMiddleware.JWT(http.HandlerFunc(sessionHandler.GetMessages)))
	mux.Handle("POST /api/sessions/{id}/chat", authMiddleware.JWT(http.HandlerFunc(sessionHandler.Chat)))
	mux.Handle("PATCH /api/sessions/{id}/participants/{user}/role", authMiddleware.JWT(http.HandlerFunc(sessionHandler.UpdateRole)))
	mux.Handle("POST /api/security/scan/file", authMiddleware.JWT(scopeGate("security:scan", http.HandlerFunc(securityHandler.ScanFile))))
	mux.Handle("POST /api/security/scan/repo", authMiddleware.JWT(scopeGate("security:scan", http.HandlerFunc(securityHandler.ScanRepo))))
	mux.Handle("POST /api/security/scan/secrets", authMiddleware.JWT(scopeGate("security:scan", http.HandlerFunc(securityHandler.ScanSecretsRepo))))
	mux.Handle("POST /api/security/policy/evaluate", authMiddleware.JWT(scopeGate("policy:enforce", http.HandlerFunc(securityHandler.EvaluatePolicy))))
	mux.Handle("GET /api/techdebt/priorities", authMiddleware.JWT(http.HandlerFunc(techDebtHandler.Priorities)))
	mux.Handle("POST /api/memory/save", authMiddleware.JWT(http.HandlerFunc(memoryHandler.Save)))
	mux.Handle("POST /api/memory/query", authMiddleware.JWT(http.HandlerFunc(memoryHandler.Query)))
	mux.Handle("POST /api/context/resolve", authMiddleware.JWT(http.HandlerFunc(contextHandler.Resolve)))
	mux.Handle("POST /api/feedback", authMiddleware.JWT(http.HandlerFunc(feedbackHandler.Save)))
	mux.Handle("GET /api/feedback/summary", authMiddleware.JWT(http.HandlerFunc(feedbackHandler.Summary)))
	mux.Handle("POST /api/admin/outbox/retry", authMiddleware.JWT(http.HandlerFunc(outboxHandler.Retry)))
	mux.Handle("POST /api/admin/config/reload", authMiddleware.JWT(http.HandlerFunc(runtimeConfigHandler.Reload)))
	mux.Handle("POST /api/eval/run", authMiddleware.JWT(http.HandlerFunc(evalHandler.Run)))
	mux.Handle("GET /api/eval/results/{id}", authMiddleware.JWT(http.HandlerFunc(evalHandler.GetResult)))
	mux.Handle("POST /api/models/recommend", authMiddleware.JWT(legacy(http.HandlerFunc(modelRecommenderHandler.Recommend), "/api/v2/models/recommend")))
	mux.Handle("POST /api/v1/models/recommend", authMiddleware.JWT(http.HandlerFunc(modelRecommenderHandler.Recommend)))
	mux.Handle("POST /api/v2/models/recommend", authMiddleware.JWT(v2ModelRecommend))
	mux.Handle("POST /api/v1/chat/completions", authMiddleware.JWT(http.HandlerFunc(chatHandler.Handle)))
	mux.Handle("POST /api/v2/chat/completions", authMiddleware.JWT(v2InternalChat))
	mux.Handle("GET /api/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Get)))
	mux.Handle("GET /api/v1/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Get)))
	mux.Handle("GET /api/v2/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Get)))
	mux.Handle("PUT /api/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Put)))
	mux.Handle("PUT /api/v1/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Put)))
	mux.Handle("PUT /api/v2/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Put)))
	mux.Handle("POST /api/patch", authMiddleware.JWT(scopeGate("patch:apply", http.HandlerFunc(patchHandler.Apply))))
	mux.Handle("GET /api/patch/preview", authMiddleware.JWT(http.HandlerFunc(patchHandler.Preview)))
	mux.Handle("POST /api/patch/sandbox/preview", authMiddleware.JWT(http.HandlerFunc(sandboxHandler.Preview)))
	mux.Handle("POST /api/patch/sandbox/apply", authMiddleware.JWT(scopeGate("patch:apply", http.HandlerFunc(sandboxHandler.Apply))))
	mux.Handle("GET /api/guardrails/rules", authMiddleware.JWT(http.HandlerFunc(guardrailsHandler.Rules)))
	mux.Handle("POST /api/os/exec", authMiddleware.JWT(scopeGate("os:exec", http.HandlerFunc(ostoolsHandler.Exec))))
	mux.Handle("POST /api/os/read", authMiddleware.JWT(http.HandlerFunc(ostoolsHandler.ReadFile)))
	mux.Handle("POST /api/os/write", authMiddleware.JWT(scopeGate("os:write", http.HandlerFunc(ostoolsHandler.WriteFile))))
	mux.Handle("POST /api/os/delete", authMiddleware.JWT(scopeGate("os:write", http.HandlerFunc(ostoolsHandler.DeleteFile))))
	mux.Handle("POST /api/os/list", authMiddleware.JWT(http.HandlerFunc(ostoolsHandler.ListDir)))
	mux.Handle("POST /api/os/exists", authMiddleware.JWT(http.HandlerFunc(ostoolsHandler.FileExists)))

	s.router = chain(
		mux,
		middleware.RequestID,
		middleware.Tenant,
		middleware.Trace,
		middleware.LoggingWithStream(logStream),
		middleware.CORS,
		middleware.Compress,
		middleware.Metrics(metricsCollector),
		middleware.RateLimit(rateLimiter, s.cfg, "/health", "/health/liveness", "/health/readiness", "/metrics", "/metrics/value", "/metrics/traces/features", "/ws/chat"),
	)
}

func (s *Server) Start() error {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.Any("/*any", gin.WrapH(s.router))

	s.httpServer = &http.Server{
		Addr:         ":" + s.cfg.Port,
		Handler:      g,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("servidor iniciado", slog.String("port", s.cfg.Port))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.indexer != nil {
		s.indexer.StopWatcher()
	}
	if s.conversationService != nil {
		if err := s.conversationService.Disconnect(ctx); err != nil {
			slog.Warn("error al cerrar conexión de MongoDB", slog.String("error", err.Error()))
		}
	}
	if s.profileService != nil {
		if err := s.profileService.Disconnect(ctx); err != nil {
			slog.Warn("error al cerrar perfil de MongoDB", slog.String("error", err.Error()))
		}
	}
	if s.feedbackService != nil {
		if err := s.feedbackService.Disconnect(ctx); err != nil {
			slog.Warn("error al cerrar feedback de MongoDB", slog.String("error", err.Error()))
		}
	}
	if s.flagsService != nil {
		if err := s.flagsService.Disconnect(ctx); err != nil {
			slog.Warn("error al cerrar flags de MongoDB", slog.String("error", err.Error()))
		}
	}
	if s.outboxService != nil {
		if err := s.outboxService.Shutdown(ctx); err != nil {
			slog.Warn("error al cerrar worker de outbox", slog.String("error", err.Error()))
		}
	}
	if s.jobsService != nil {
		if err := s.jobsService.Shutdown(ctx); err != nil {
			slog.Warn("error al cerrar worker de jobs", slog.String("error", err.Error()))
		}
	}
	if s.eventCancel != nil {
		s.eventCancel()
	}
	if s.eventBus != nil {
		s.eventBus.Shutdown()
	}
	// No hay buffer de métricas pendiente para flush explícito en la implementación actual.
	slog.Info("flush métricas completado")

	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	wrapped := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		wrapped = middlewares[i](wrapped)
	}
	return wrapped
}

func thresholdOrFallback(value, fallback int) int {
	if value > 0 {
		return value
	}
	if fallback > 0 {
		return fallback
	}
	return 3
}
