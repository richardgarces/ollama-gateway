package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Port                   string
	OllamaURL              string
	QdrantURL              string
	JWTSecret              []byte
	RepoRoot               string
	VectorStorePath        string
	VectorStorePreferLocal bool
	// IndexerStatePath: path to persist indexer state (file)
	IndexerStatePath string
	RateLimitRPM     int
	// Embedding cache settings
	EmbeddingCacheTTLSeconds int
	EmbeddingCacheMaxEntries int
}

func Load() *Config {
	repoRoot := getEnv("REPO_ROOT", ".")
	defaultState := filepath.Join(repoRoot, ".indexer_state.json")
	defaultVectorStore := filepath.Join(repoRoot, ".vector_store.json")
	return &Config{
		Port:                     getEnv("PORT", "8081"),
		OllamaURL:                getEnv("OLLAMA_URL", "http://ollama:11434"),
		QdrantURL:                getEnv("QDRANT_URL", "http://qdrant:6333"),
		JWTSecret:                loadJWTSecret(),
		RepoRoot:                 repoRoot,
		VectorStorePath:          getEnv("VECTOR_STORE_PATH", defaultVectorStore),
		VectorStorePreferLocal:   getEnvAsBool("VECTOR_STORE_PREFER_LOCAL", false),
		IndexerStatePath:         getEnv("INDEXER_STATE_PATH", defaultState),
		RateLimitRPM:             getEnvAsInt("RATE_LIMIT_RPM", 60),
		EmbeddingCacheTTLSeconds: getEnvAsInt("EMBEDDING_CACHE_TTL_SECONDS", 3600),
		EmbeddingCacheMaxEntries: getEnvAsInt("EMBEDDING_CACHE_MAX_ENTRIES", 1000),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("WARN: %s inválido (%q), usando %d", key, v, fallback)
		return fallback
	}
	if parsed < 0 {
		log.Printf("WARN: %s no puede ser negativo (%d), usando %d", key, parsed, fallback)
		return fallback
	}
	return parsed
}

func getEnvAsBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		log.Printf("WARN: %s inválido (%q), usando %t", key, v, fallback)
		return fallback
	}
}

func loadJWTSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		log.Println("WARN: JWT_SECRET no configurado, generando uno aleatorio (se perderá al reiniciar)")
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatal("no se pudo generar JWT_SECRET aleatorio:", err)
		}
		return b
	}
	decoded, err := hex.DecodeString(s)
	if err != nil {
		if len(s) < 32 {
			log.Printf("WARN: JWT_SECRET no es hexadecimal y es corto (%d chars): %s", len(s), fmt.Sprintf("%q", s))
		}
		return []byte(s)
	}
	return decoded
}
