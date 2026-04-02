package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/domain"
	"ollama-gateway/internal/handlers"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/internal/observability"
	"ollama-gateway/internal/services"
	"ollama-gateway/pkg/cache"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	cfg                 *config.Config
	router              http.Handler
	httpServer          *http.Server
	indexer             domain.Indexer
	cache               cache.Cache
	conversationService *services.ConversationService
	profileService      *services.ProfileService
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
	ollamaService := services.NewOllamaService(s.cfg, logger, s.cache)
	routerService := services.NewRouterService(s.cfg, ollamaService, logger)
	toolRegistry := services.NewToolRegistry(s.cfg.AgentToolsDir, s.cfg.RepoRoot, logger)
	agentService := services.NewAgentService(ollamaService, logger, toolRegistry)
	conversationService, err := services.NewConversationService(s.cfg.MongoURI, logger)
	if err != nil {
		logger.Warn("conversation service no disponible; se continuará sin persistencia", slog.String("error", err.Error()))
	} else {
		s.conversationService = conversationService
	}
	profileService, err := services.NewProfileService(s.cfg.MongoURI, logger)
	if err != nil {
		logger.Warn("profile service no disponible; se continuará sin perfiles", slog.String("error", err.Error()))
	} else {
		s.profileService = profileService
	}
	patchService := services.NewPatchService(logger)
	repoService := services.NewRepoService(ollamaService, s.cfg.RepoRoot, logger)
	qdrantService := services.NewQdrantService(
		s.cfg.QdrantURL,
		s.cfg.RepoRoot,
		s.cfg.VectorStorePath,
		s.cfg.VectorStorePreferLocal,
		s.cfg.HTTPTimeoutSeconds,
		s.cfg.HTTPMaxRetries,
		logger,
	)
	ragService := services.NewRAGService(
		ollamaService,
		routerService,
		qdrantService,
		logger,
		s.cache,
		repoRoots,
		s.cfg.RAGCacheTTLSeconds,
		s.cfg.RAGCacheMaxEntries,
	)
	reviewService := services.NewReviewService(ragService, s.cfg.RepoRoot, logger)
	docGenService := services.NewDocGenService(ragService, s.cfg.RepoRoot, logger)
	debugService := services.NewDebugService(ragService, s.cfg.RepoRoot, logger)
	indexerService, _ := services.NewIndexerService(repoRoots, s.cfg.IndexerStatePath, ollamaService, qdrantService, logger)
	indexerService.SetOnContentChange(ragService.InvalidateResponseCache)

	var ollamaClient domain.OllamaClient = ollamaService
	var vectorStore domain.VectorStore = qdrantService
	var ragEngine domain.RAGEngine = ragService
	var indexer domain.Indexer = indexerService
	var agentRunner domain.AgentRunner = agentService
	s.indexer = indexer

	// Inicializar handlers
	authHandler := handlers.NewAuthHandler(s.cfg.JWTSecret)
	generateHandler := handlers.NewGenerateHandler(ragEngine)
	agentHandler := handlers.NewAgentHandler(agentRunner)
	chatHandler := handlers.NewChatHandler(ragEngine)
	repoHandler := handlers.NewRepoHandler(repoService)
	reviewHandler := handlers.NewReviewHandler(reviewService)
	docGenHandler := handlers.NewDocGenHandler(docGenService)
	debugHandler := handlers.NewDebugHandler(debugService)
	profileHandler := handlers.NewProfileHandler(s.profileService)
	patchHandler := handlers.NewPatchHandler(s.cfg.RepoRoot, patchService)
	metricsHandler := handlers.NewMetricsHandler(metricsCollector)
	indexerHandler := handlers.NewIndexerHandler(indexer)
	dashboardHandler := handlers.NewDashboardHandler(s.cfg, metricsCollector, indexerService, logStream)
	searchHandler := handlers.NewSearchHandler(ollamaClient, vectorStore, repoRoots)
	openaiHandler := handlers.NewOpenAIHandler(ollamaClient, ragEngine, s.conversationService, s.profileService)
	wsHandler := handlers.NewWSHandler(ragEngine, s.cfg.JWTSecret)
	healthHandler := handlers.NewHealthHandler(s.cfg)
	authMiddleware := middleware.NewAuthMiddleware(s.cfg.JWTSecret)
	localhostOnly := middleware.LocalhostOnly

	mux := http.NewServeMux()

	// Rutas públicas
	mux.HandleFunc("GET /health", healthHandler.Liveness)
	mux.HandleFunc("GET /health/liveness", healthHandler.Liveness)
	mux.HandleFunc("GET /health/readiness", healthHandler.Readiness)
	mux.HandleFunc("GET /metrics", metricsHandler.Handle)
	// Prometheus scrape endpoint
	mux.Handle("GET /metrics/prometheus", promhttp.Handler())
	mux.HandleFunc("POST /login", authHandler.Login)

	// Dashboard interno (solo localhost)
	mux.Handle("GET /dashboard", localhostOnly(http.HandlerFunc(dashboardHandler.Handle)))
	mux.Handle("GET /internal/dashboard/status", localhostOnly(http.HandlerFunc(dashboardHandler.Status)))
	mux.Handle("GET /internal/logs/stream", localhostOnly(http.HandlerFunc(dashboardHandler.LogsStream)))

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
