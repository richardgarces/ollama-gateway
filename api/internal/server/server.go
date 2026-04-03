package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"ollama-gateway/internal/config"
	agentservice "ollama-gateway/internal/function/agent"
	agenttransport "ollama-gateway/internal/function/agent/transport"
	apiexplorertransport "ollama-gateway/internal/function/api_explorer/transport"
	architectservice "ollama-gateway/internal/function/architect"
	architecttransport "ollama-gateway/internal/function/architect/transport"
	authtransport "ollama-gateway/internal/function/auth/transport"
	chattransport "ollama-gateway/internal/function/chat/transport"
	cicdservice "ollama-gateway/internal/function/cicd"
	cicdtransport "ollama-gateway/internal/function/cicd/transport"
	commitgenservice "ollama-gateway/internal/function/commitgen"
	commitgentransport "ollama-gateway/internal/function/commitgen/transport"
	coreservice "ollama-gateway/internal/function/core"
	"ollama-gateway/internal/function/core/domain"
	dashboardtransport "ollama-gateway/internal/function/dashboard/transport"
	debugservice "ollama-gateway/internal/function/debug"
	debugtransport "ollama-gateway/internal/function/debug/transport"
	docgenservice "ollama-gateway/internal/function/docgen"
	docgentransport "ollama-gateway/internal/function/docgen/transport"
	generatetransport "ollama-gateway/internal/function/generate/transport"
	healthtransport "ollama-gateway/internal/function/health/transport"
	indexerservice "ollama-gateway/internal/function/indexer"
	indexertransport "ollama-gateway/internal/function/indexer/transport"
	metricstransport "ollama-gateway/internal/function/metrics/transport"
	modelstransport "ollama-gateway/internal/function/models/transport"
	openaitransport "ollama-gateway/internal/function/openai/transport"
	patchservice "ollama-gateway/internal/function/patch"
	patchtransport "ollama-gateway/internal/function/patch/transport"
	profileservice "ollama-gateway/internal/function/profile"
	profiletransport "ollama-gateway/internal/function/profile/transport"
	reposervice "ollama-gateway/internal/function/repo"
	repotransport "ollama-gateway/internal/function/repo/transport"
	reviewservice "ollama-gateway/internal/function/review"
	reviewtransport "ollama-gateway/internal/function/review/transport"
	searchtransport "ollama-gateway/internal/function/search/transport"
	securityservice "ollama-gateway/internal/function/security"
	securitytransport "ollama-gateway/internal/function/security/transport"
	sessionservice "ollama-gateway/internal/function/session"
	sessiontransport "ollama-gateway/internal/function/session/transport"
	sqlgenservice "ollama-gateway/internal/function/sqlgen"
	sqlgentransport "ollama-gateway/internal/function/sqlgen/transport"
	testgenservice "ollama-gateway/internal/function/testgen"
	testgentransport "ollama-gateway/internal/function/testgen/transport"
	translatorservice "ollama-gateway/internal/function/translator"
	translatortransport "ollama-gateway/internal/function/translator/transport"
	wstransport "ollama-gateway/internal/function/ws/transport"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/internal/utils/observability"
	"ollama-gateway/pkg/cache"

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
}

type RouteDefinition = domain.RouteDefinition

func GetRouteDefinitions() []RouteDefinition {
	return []RouteDefinition{
		{Method: "GET", Path: "/health", Description: "Liveness probe", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/health/liveness", Description: "Liveness detail", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/health/readiness", Description: "Readiness probe", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/metrics", Description: "Metricas JSON internas", ExampleBody: "", Protected: false},
		{Method: "GET", Path: "/api/models", Description: "Modelos disponibles de Ollama", ExampleBody: "", Protected: false},
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
		{Method: "POST", Path: "/api/search", Description: "Busqueda semantica", ExampleBody: "{\n  \"query\": \"auth middleware\",\n  \"top_k\": 5\n}", Protected: false},
		{Method: "POST", Path: "/openai/v1/embeddings", Description: "OpenAI compatible embeddings", ExampleBody: "{\n  \"model\": \"nomic-embed-text\",\n  \"input\": \"hola\"\n}", Protected: false},
		{Method: "POST", Path: "/openai/v1/completions", Description: "OpenAI compatible completions", ExampleBody: "{\n  \"model\": \"llama3\",\n  \"prompt\": \"Hello\"\n}", Protected: false},
		{Method: "POST", Path: "/openai/v1/chat/completions", Description: "OpenAI compatible chat completions", ExampleBody: "{\n  \"model\": \"llama3\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"hola\"}]\n}", Protected: false},
		{Method: "GET", Path: "/ws/chat", Description: "WebSocket chat", ExampleBody: "", Protected: false},
		{Method: "POST", Path: "/api/generate", Description: "Generacion simple", ExampleBody: "{\n  \"prompt\": \"Resume este texto\",\n  \"stream\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/agent", Description: "Ejecucion de agente", ExampleBody: "{\n  \"input\": \"Analiza el repo\"\n}", Protected: true},
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
		{Method: "POST", Path: "/api/sql/query", Description: "Generar query SQL", ExampleBody: "{\n  \"description\": \"listar usuarios activos\",\n  \"dialect\": \"postgres\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sql/migration", Description: "Generar migracion SQL", ExampleBody: "{\n  \"description\": \"crear tabla sessions\",\n  \"dialect\": \"postgres\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sql/explain", Description: "Explicar query SQL", ExampleBody: "{\n  \"sql\": \"SELECT * FROM users WHERE id = 1\"\n}", Protected: true},
		{Method: "POST", Path: "/api/cicd/generate", Description: "Generar pipeline CI/CD", ExampleBody: "{\n  \"platform\": \"github-actions\",\n  \"apply\": false\n}", Protected: true},
		{Method: "POST", Path: "/api/cicd/optimize", Description: "Optimizar pipeline CI/CD", ExampleBody: "{\n  \"platform\": \"gitlab-ci\",\n  \"pipeline\": \"stages: [test]\"\n}", Protected: true},
		{Method: "POST", Path: "/api/commit/message", Description: "Generar commit message desde diff", ExampleBody: "{\n  \"diff\": \"diff --git ...\"\n}", Protected: true},
		{Method: "POST", Path: "/api/commit/staged", Description: "Generar commit message desde staged", ExampleBody: "{\n  \"repo_root\": \".\"\n}", Protected: true},
		{Method: "GET", Path: "/api/architect/analyze", Description: "Analisis de arquitectura", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/architect/refactor", Description: "Sugerencia de refactor", ExampleBody: "{\n  \"path\": \"api/internal/function/core/router.go\"\n}", Protected: true},
		{Method: "POST", Path: "/api/sessions", Description: "Crear sesion compartida", ExampleBody: "{}", Protected: true},
		{Method: "POST", Path: "/api/sessions/{id}/join", Description: "Unirse a sesion", ExampleBody: "{}", Protected: true},
		{Method: "GET", Path: "/api/sessions/{id}/messages", Description: "Obtener mensajes de sesion", ExampleBody: "", Protected: true},
		{Method: "POST", Path: "/api/sessions/{id}/chat", Description: "Enviar chat a sesion", ExampleBody: "{\n  \"message\": \"hola equipo\"\n}", Protected: true},
		{Method: "POST", Path: "/api/security/scan/file", Description: "Escanear seguridad de archivo", ExampleBody: "{\n  \"path\": \"api/internal/server/server.go\"\n}", Protected: true},
		{Method: "POST", Path: "/api/security/scan/repo", Description: "Escanear seguridad del repo", ExampleBody: "{}", Protected: true},
		{Method: "POST", Path: "/api/v1/chat/completions", Description: "Chat completions interno", ExampleBody: "{\n  \"model\": \"llama3\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"hola\"}]\n}", Protected: true},
		{Method: "GET", Path: "/api/profile", Description: "Obtener perfil", ExampleBody: "", Protected: true},
		{Method: "PUT", Path: "/api/profile", Description: "Actualizar perfil", ExampleBody: "{\n  \"default_model\": \"llama3\"\n}", Protected: true},
		{Method: "POST", Path: "/api/patch", Description: "Aplicar patch generado", ExampleBody: "{\n  \"response\": \"*** Begin Patch ...\",\n  \"apply\": true\n}", Protected: true},
		{Method: "GET", Path: "/api/patch/preview", Description: "Previsualizar patch", ExampleBody: "", Protected: true},
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
	repoRoots := s.cfg.RepoRoots
	if len(repoRoots) == 0 {
		repoRoots = []string{s.cfg.RepoRoot}
	}

	// Inicializar servicios con inyección de dependencias
	ollamaService := coreservice.NewOllamaService(s.cfg, logger, s.cache)
	routerService := coreservice.NewRouterService(s.cfg, ollamaService, logger)
	toolRegistry := coreservice.NewToolRegistry(s.cfg.AgentToolsDir, s.cfg.RepoRoot, logger)
	agentService := agentservice.NewService(ollamaService, logger, toolRegistry)
	conversationService, err := coreservice.NewConversationService(s.cfg.MongoURI, logger)
	if err != nil {
		logger.Warn("conversation service no disponible; se continuará sin persistencia", slog.String("error", err.Error()))
	} else {
		s.conversationService = conversationService
	}
	profileService, err := profileservice.NewMongoService(s.cfg.MongoURI, logger)
	if err != nil {
		logger.Warn("profile service no disponible; se continuará sin perfiles", slog.String("error", err.Error()))
	} else {
		s.profileService = profileService
	}
	patchService := patchservice.NewService(logger)
	repoService := reposervice.NewService(ollamaService, s.cfg.RepoRoot, logger)
	qdrantService := coreservice.NewQdrantService(
		s.cfg.QdrantURL,
		s.cfg.RepoRoot,
		s.cfg.VectorStorePath,
		s.cfg.VectorStorePreferLocal,
		s.cfg.HTTPTimeoutSeconds,
		s.cfg.HTTPMaxRetries,
		logger,
	)
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
	reviewService := reviewservice.NewService(ragService, s.cfg.RepoRoot, logger)
	docGenService := docgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	debugService := debugservice.NewService(ragService, s.cfg.RepoRoot, logger)
	translatorService := translatorservice.NewService(ragService, s.cfg.RepoRoot, logger)
	testGenService := testgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	sqlGenService := sqlgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	cicdService := cicdservice.NewService(ragService, s.cfg.RepoRoot, logger)
	commitGenService := commitgenservice.NewService(ragService, s.cfg.RepoRoot, logger)
	sessionService := sessionservice.NewService()
	securityService := securityservice.NewService(ragService, s.cfg.RepoRoot, logger)
	indexerService, _ := indexerservice.NewService(repoRoots, s.cfg.IndexerStatePath, ollamaService, qdrantService, logger)
	indexerService.SetOnContentChange(ragService.InvalidateResponseCache)
	indexerService.SetOnFileIndexed(func(path string) {
		go func(filePath string) {
			findings, err := securityService.ScanFile(filePath)
			if err != nil {
				logger.Debug("security scan en indexer falló", slog.String("path", filePath), slog.String("error", err.Error()))
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
		}(path)
	})
	architectService := architectservice.NewService(ragService, s.cfg.RepoRoot, indexerService, logger)

	var ollamaClient domain.OllamaClient = ollamaService
	var vectorStore domain.VectorStore = qdrantService
	var ragEngine domain.RAGEngine = ragService
	var indexer domain.Indexer = indexerService
	var agentRunner domain.AgentRunner = agentService
	s.indexer = indexer

	// Inicializar handlers
	authHandler := authtransport.NewHandler(s.cfg.JWTSecret)
	generateHandler := generatetransport.NewHandler(ragEngine)
	agentHandler := agenttransport.NewHandler(agentRunner)
	chatHandler := chattransport.NewHandler(ragEngine)
	repoHandler := repotransport.NewHandler(repoService)
	reviewHandler := reviewtransport.NewHandler(reviewService)
	docGenHandler := docgentransport.NewHandler(docGenService)
	debugHandler := debugtransport.NewHandler(debugService)
	translatorHandler := translatortransport.NewHandler(translatorService)
	testGenHandler := testgentransport.NewHandler(testGenService)
	sqlGenHandler := sqlgentransport.NewHandler(sqlGenService)
	cicdHandler := cicdtransport.NewHandler(cicdService)
	commitGenHandler := commitgentransport.NewHandler(commitGenService)
	sessionHandler := sessiontransport.NewHandler(sessionService, ragEngine)
	securityHandler := securitytransport.NewHandler(securityService)
	architectHandler := architecttransport.NewHandler(architectService)
	profileHandler := profiletransport.NewHandler(s.profileService)
	patchHandler := patchtransport.NewHandler(s.cfg.RepoRoot, patchService)
	metricsHandler := metricstransport.NewHandler(metricsCollector)
	modelsHandler := modelstransport.NewHandler(ollamaService)
	indexerHandler := indexertransport.NewHandler(indexer)
	dashboardHandler := dashboardtransport.NewHandler(s.cfg, metricsCollector, indexerService, logStream)
	searchHandler := searchtransport.NewHandler(ollamaClient, vectorStore, repoRoots)
	openaiHandler := openaitransport.NewHandler(ollamaClient, ragEngine, s.conversationService, s.profileService)
	wsHandler := wstransport.NewHandler(ragEngine, s.cfg.JWTSecret)
	apiExplorerHandler := apiexplorertransport.NewHandler(GetRouteDefinitions())
	healthHandler := healthtransport.NewHandler(s.cfg)
	authMiddleware := middleware.NewAuthMiddleware(s.cfg.JWTSecret)
	localhostOnly := middleware.LocalhostOnly

	mux := http.NewServeMux()

	// Rutas públicas
	mux.HandleFunc("GET /health", healthHandler.Liveness)
	mux.HandleFunc("GET /health/liveness", healthHandler.Liveness)
	mux.HandleFunc("GET /health/readiness", healthHandler.Readiness)
	mux.HandleFunc("GET /metrics", metricsHandler.Handle)
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

	// Indexer control (internal, solo localhost)
	mux.Handle("GET /internal/index/status", localhostOnly(http.HandlerFunc(indexerHandler.Status)))
	mux.Handle("POST /internal/index/reindex", localhostOnly(http.HandlerFunc(indexerHandler.Reindex)))
	mux.Handle("POST /internal/index/start", localhostOnly(http.HandlerFunc(indexerHandler.StartWatcher)))
	mux.Handle("POST /internal/index/stop", localhostOnly(http.HandlerFunc(indexerHandler.StopWatcher)))
	mux.Handle("POST /internal/index/reset", localhostOnly(http.HandlerFunc(indexerHandler.ResetState)))
	mux.HandleFunc("POST /api/search", searchHandler.Handle)
	// OpenAI-compatible endpoints
	mux.HandleFunc("POST /openai/v1/embeddings", openaiHandler.Embeddings)
	mux.HandleFunc("POST /openai/v1/completions", openaiHandler.Completions)
	mux.HandleFunc("POST /openai/v1/chat/completions", openaiHandler.ChatCompletions)
	mux.HandleFunc("GET /ws/chat", wsHandler.Handle)

	// Rutas protegidas con JWT
	mux.Handle("POST /api/generate", authMiddleware.JWT(http.HandlerFunc(generateHandler.Handle)))
	mux.Handle("POST /api/agent", authMiddleware.JWT(http.HandlerFunc(agentHandler.Handle)))
	mux.Handle("POST /api/refactor", authMiddleware.JWT(http.HandlerFunc(repoHandler.Refactor)))
	mux.Handle("GET /api/analyze-repo", authMiddleware.JWT(http.HandlerFunc(repoHandler.Analyze)))
	mux.Handle("POST /api/review/diff", authMiddleware.JWT(http.HandlerFunc(reviewHandler.ReviewDiff)))
	mux.Handle("POST /api/review/file", authMiddleware.JWT(http.HandlerFunc(reviewHandler.ReviewFile)))
	mux.Handle("POST /api/docs/file", authMiddleware.JWT(http.HandlerFunc(docGenHandler.GenerateFileDoc)))
	mux.Handle("POST /api/docs/readme", authMiddleware.JWT(http.HandlerFunc(docGenHandler.GenerateREADME)))
	mux.Handle("POST /api/debug/error", authMiddleware.JWT(http.HandlerFunc(debugHandler.AnalyzeError)))
	mux.Handle("POST /api/debug/log", authMiddleware.JWT(http.HandlerFunc(debugHandler.AnalyzeLog)))
	mux.Handle("POST /api/translate", authMiddleware.JWT(http.HandlerFunc(translatorHandler.Translate)))
	mux.Handle("POST /api/translate/file", authMiddleware.JWT(http.HandlerFunc(translatorHandler.TranslateFile)))
	mux.Handle("POST /api/testgen", authMiddleware.JWT(http.HandlerFunc(testGenHandler.Generate)))
	mux.Handle("POST /api/testgen/file", authMiddleware.JWT(http.HandlerFunc(testGenHandler.GenerateForFile)))
	mux.Handle("POST /api/sql/query", authMiddleware.JWT(http.HandlerFunc(sqlGenHandler.GenerateQuery)))
	mux.Handle("POST /api/sql/migration", authMiddleware.JWT(http.HandlerFunc(sqlGenHandler.GenerateMigration)))
	mux.Handle("POST /api/sql/explain", authMiddleware.JWT(http.HandlerFunc(sqlGenHandler.ExplainQuery)))
	mux.Handle("POST /api/cicd/generate", authMiddleware.JWT(http.HandlerFunc(cicdHandler.GeneratePipeline)))
	mux.Handle("POST /api/cicd/optimize", authMiddleware.JWT(http.HandlerFunc(cicdHandler.OptimizePipeline)))
	mux.Handle("POST /api/commit/message", authMiddleware.JWT(http.HandlerFunc(commitGenHandler.Message)))
	mux.Handle("POST /api/commit/staged", authMiddleware.JWT(http.HandlerFunc(commitGenHandler.Staged)))
	mux.Handle("GET /api/architect/analyze", authMiddleware.JWT(http.HandlerFunc(architectHandler.AnalyzeProject)))
	mux.Handle("POST /api/architect/refactor", authMiddleware.JWT(http.HandlerFunc(architectHandler.SuggestRefactor)))
	mux.Handle("POST /api/sessions", authMiddleware.JWT(http.HandlerFunc(sessionHandler.Create)))
	mux.Handle("POST /api/sessions/{id}/join", authMiddleware.JWT(http.HandlerFunc(sessionHandler.Join)))
	mux.Handle("GET /api/sessions/{id}/messages", authMiddleware.JWT(http.HandlerFunc(sessionHandler.GetMessages)))
	mux.Handle("POST /api/sessions/{id}/chat", authMiddleware.JWT(http.HandlerFunc(sessionHandler.Chat)))
	mux.Handle("POST /api/security/scan/file", authMiddleware.JWT(http.HandlerFunc(securityHandler.ScanFile)))
	mux.Handle("POST /api/security/scan/repo", authMiddleware.JWT(http.HandlerFunc(securityHandler.ScanRepo)))
	mux.Handle("POST /api/v1/chat/completions", authMiddleware.JWT(http.HandlerFunc(chatHandler.Handle)))
	mux.Handle("GET /api/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Get)))
	mux.Handle("PUT /api/profile", authMiddleware.JWT(http.HandlerFunc(profileHandler.Put)))
	mux.Handle("POST /api/patch", authMiddleware.JWT(http.HandlerFunc(patchHandler.Apply)))
	mux.Handle("GET /api/patch/preview", authMiddleware.JWT(http.HandlerFunc(patchHandler.Preview)))

	s.router = chain(
		mux,
		middleware.RequestID,
		middleware.LoggingWithStream(logStream),
		middleware.CORS,
		middleware.Compress,
		middleware.Metrics(metricsCollector),
		middleware.RateLimit(rateLimiter, s.cfg, "/health", "/health/liveness", "/health/readiness", "/metrics", "/ws/chat"),
	)
}

func (s *Server) Start() error {
	s.httpServer = &http.Server{
		Addr:         ":" + s.cfg.Port,
		Handler:      s.router,
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
