package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/handlers"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/internal/observability"
	"ollama-gateway/internal/services"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	cfg    *config.Config
	router *mux.Router
}

func New(cfg *config.Config) *Server {
	s := &Server{
		cfg:    cfg,
		router: mux.NewRouter(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	metricsCollector := observability.NewMetricsCollector()
	rateLimiter := observability.NewRateLimiter(s.cfg.RateLimitRPM, time.Minute)

	// Inicializar servicios con inyección de dependencias
	ollamaService := services.NewOllamaService(s.cfg)
	routerService := services.NewRouterService()
	agentService := services.NewAgentService(ollamaService, s.cfg.RepoRoot)
	repoService := services.NewRepoService(ollamaService, s.cfg.RepoRoot)
	qdrantService := services.NewQdrantService(s.cfg.QdrantURL, s.cfg.RepoRoot, s.cfg.VectorStorePath, s.cfg.VectorStorePreferLocal)
	ragService := services.NewRAGService(ollamaService, routerService, qdrantService)
	indexerService, _ := services.NewIndexerService(s.cfg.RepoRoot, s.cfg.IndexerStatePath, ollamaService, qdrantService)

	// Inicializar handlers
	authHandler := handlers.NewAuthHandler(s.cfg.JWTSecret)
	generateHandler := handlers.NewGenerateHandler(ragService)
	agentHandler := handlers.NewAgentHandler(agentService)
	chatHandler := handlers.NewChatHandler(ragService)
	repoHandler := handlers.NewRepoHandler(repoService)
	metricsHandler := handlers.NewMetricsHandler(metricsCollector)
	indexerHandler := handlers.NewIndexerHandler(indexerService)
	searchHandler := handlers.NewSearchHandler(ollamaService, qdrantService)
	openaiHandler := handlers.NewOpenAIHandler(ollamaService, ragService)

	// Middleware global
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.Logging)
	s.router.Use(middleware.CORS)
	s.router.Use(middleware.Metrics(metricsCollector))
	s.router.Use(middleware.RateLimit(rateLimiter, "/health", "/metrics"))

	// Rutas públicas
	s.router.HandleFunc("/health", handlers.Health).Methods("GET")
	s.router.HandleFunc("/metrics", metricsHandler.Handle).Methods("GET")
	// Prometheus scrape endpoint
	s.router.Handle("/metrics/prometheus", promhttp.Handler()).Methods("GET")
	s.router.HandleFunc("/login", authHandler.Login).Methods("POST")

	// Indexer control (internal)
	s.router.HandleFunc("/internal/index/reindex", indexerHandler.Reindex).Methods("POST")
	s.router.HandleFunc("/internal/index/start", indexerHandler.StartWatcher).Methods("POST")
	s.router.HandleFunc("/internal/index/stop", indexerHandler.StopWatcher).Methods("POST")
	s.router.HandleFunc("/internal/index/reset", indexerHandler.ResetState).Methods("POST")
	s.router.HandleFunc("/api/search", searchHandler.Handle).Methods("POST")
	// OpenAI-compatible endpoints
	s.router.HandleFunc("/openai/v1/embeddings", openaiHandler.Embeddings).Methods("POST")
	s.router.HandleFunc("/openai/v1/completions", openaiHandler.Completions).Methods("POST")
	s.router.HandleFunc("/openai/v1/chat/completions", openaiHandler.ChatCompletions).Methods("POST")

	// Rutas protegidas con JWT
	authMiddleware := middleware.NewAuthMiddleware(s.cfg.JWTSecret)
	api := s.router.PathPrefix("/api").Subrouter()
	api.Use(authMiddleware.JWT)

	api.HandleFunc("/generate", generateHandler.Handle).Methods("POST")
	api.HandleFunc("/agent", agentHandler.Handle).Methods("POST")
	api.HandleFunc("/refactor", repoHandler.Refactor).Methods("POST")
	api.HandleFunc("/analyze-repo", repoHandler.Analyze).Methods("GET")
	api.HandleFunc("/v1/chat/completions", chatHandler.Handle).Methods("POST")
}

func (s *Server) Start() error {
	srv := &http.Server{
		Addr:         ":" + s.cfg.Port,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("SaaS API en ejecución en el puerto :%s...\n", s.cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("error arrancando servidor:", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Apagando servidor...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return err
	}

	log.Println("Servidor apagado correctamente")
	return nil
}
