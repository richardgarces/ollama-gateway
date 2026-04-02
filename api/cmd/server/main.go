package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/server"
	"ollama-gateway/pkg/cache"
)

func main() {
	cfg := config.Load()
	logger := config.SetupLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	var cacheBackend cache.Cache
	switch strings.ToLower(cfg.CacheBackend) {
	case "redis":
		redisCache, err := cache.NewRedis(cfg.RedisURL)
		if err != nil {
			slog.Error("no se pudo inicializar cache redis", slog.Any("error", err))
			os.Exit(1)
		}
		defer redisCache.Close()
		cacheBackend = redisCache
	default:
		cacheBackend = cache.NewMemory()
	}

	srv := server.New(cfg, cacheBackend)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			slog.Error("error arrancando servidor", slog.Any("error", err))
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			slog.Error("graceful shutdown falló", slog.Any("error", err))
			os.Exit(1)
		}

		if err := <-errCh; err != nil {
			slog.Error("error en cierre de servidor", slog.Any("error", err))
			os.Exit(1)
		}

		slog.Info("server stopped")
	}
}
