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
	"ollama-gateway/internal/usecase/services"

	"github.com/gorilla/mux"
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
	// Inicializar servicios con inyección de dependencias
	ollamaService := services.NewOllamaService(s.cfg.OllamaURL)
	routerService := services.NewRouterService()
	ragService := services.NewRAGService(ollamaService, routerService, s.cfg.QdrantURL)
	agentService := services.NewAgentService(ollamaService, s.cfg.RepoRoot)
	repoService := services.NewRepoService(ollamaService, s.cfg.RepoRoot)

	// Inicializar handlers
	authHandler := handlers.NewAuthHandler(s.cfg.JWTSecret)
	generateHandler := handlers.NewGenerateHandler(ragService)
	agentHandler := handlers.NewAgentHandler(agentService)
	chatHandler := handlers.NewChatHandler(ragService)
	repoHandler := handlers.NewRepoHandler(repoService)

	// Middleware global
	s.router.Use(middleware.Logging)
	s.router.Use(middleware.CORS)

	// Rutas públicas
	s.router.HandleFunc("/health", handlers.Health).Methods("GET")
	s.router.HandleFunc("/login", authHandler.Login).Methods("POST")

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
