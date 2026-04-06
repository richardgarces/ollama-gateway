package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ollama-gateway/pkg/reposcope"
)

type Config struct {
	Port                   string
	OllamaURL              string
	ChatModel              string
	FIMModel               string
	EmbeddingModel         string
	OllamaKeepAlive        string
	AutoQuantizeModels     bool
	QdrantURL              string
	JWTSecret              []byte
	LogLevel               string
	LogFormat              string
	RepoRoot               string
	RepoRoots              []string
	VectorStorePath        string
	VectorStorePreferLocal bool
	// IndexerStatePath: path to persist indexer state (file)
	IndexerStatePath      string
	RateLimitRPM          int
	RateLimitUserRPM      int
	RateLimitEndpoints    map[string]int
	HTTPTimeoutSeconds    int
	HTTPMaxRetries        int
	AgentToolsDir         string
	MongoURI              string
	RemoteAPIURL          string
	RemoteAPIKey          string
	PromptLang            string
	CacheBackend          string
	RedisURL              string
	HealthCheckTimeoutMS  int
	HealthExtraChecksJSON string
	// Embedding cache settings
	EmbeddingCacheTTLSeconds int
	EmbeddingCacheMaxEntries int
	RAGCacheTTLSeconds       int
	RAGCacheMaxEntries       int
	MemoryTTLHours           int
	MemoryPruneMaxEntries    int
	CBFailureThreshold       int
	CBOpenTimeoutSeconds     int
	CBHalfOpenMaxSuccess     int
	CBOllamaThreshold        int
	CBQdrantThreshold        int
	OutboxWorkerIntervalSec  int
	OutboxBatchSize          int
	OutboxMaxAttempts        int
	OutboxRetryBackoffSec    int
	MigrationsLockTTLSeconds int
	PoolMaxOpen              int
	PoolMaxIdle              int
	PoolTimeoutSeconds       int
	MongoPoolMaxOpen         int
	MongoPoolMaxIdle         int
	MongoPoolTimeoutSeconds  int
	EmbeddingPoolSize        int
	RetrievalPoolSize        int
	OTelEnabled              bool
	OTelServiceName          string
	OTelExporterOTLPEndpoint string
	OTelExporterInsecure     bool
	OTelSamplePercent        int
}

func Load() *Config {
	cfg, err := LoadWithError()
	if err != nil {
		log.Printf("WARN: no se pudo cargar archivo de configuración, usando solo env vars: %v", err)
		return loadFromEnv()
	}
	return cfg
}

func LoadWithError() (*Config, error) {
	if _, _, err := applyConfigFileToEnv(); err != nil {
		return nil, err
	}
	return loadFromEnv(), nil
}

func loadFromEnv() *Config {
	repoRoots := parseRepoRoots()
	repoRoots = reposcope.CanonicalizeRoots(repoRoots)
	repoRoot := "."
	if len(repoRoots) > 0 {
		repoRoot = repoRoots[0]
	}
	defaultState := filepath.Join(repoRoot, ".indexer_state.json")
	defaultVectorStore := filepath.Join(repoRoot, ".vector_store.json")
	return &Config{
		Port:                     getEnv("PORT", "8081"),
		OllamaURL:                getEnv("OLLAMA_URL", "http://ollama:11434"),
		ChatModel:                getEnv("CHAT_MODEL", "local-rag"),
		FIMModel:                 getEnv("FIM_MODEL", "local-rag"),
		EmbeddingModel:           getEnv("EMBEDDING_MODEL", "nomic-embed-text:latest"),
		OllamaKeepAlive:          getEnv("OLLAMA_KEEP_ALIVE", "-1"),
		AutoQuantizeModels:       getEnvAsBool("AUTO_QUANTIZE_MODELS", true),
		QdrantURL:                getEnv("QDRANT_URL", "http://qdrant:6333"),
		JWTSecret:                loadJWTSecret(),
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
		LogFormat:                getEnv("LOG_FORMAT", "json"),
		RepoRoot:                 repoRoot,
		RepoRoots:                repoRoots,
		VectorStorePath:          getEnv("VECTOR_STORE_PATH", defaultVectorStore),
		VectorStorePreferLocal:   getEnvAsBool("VECTOR_STORE_PREFER_LOCAL", false),
		IndexerStatePath:         getEnv("INDEXER_STATE_PATH", defaultState),
		RateLimitRPM:             getEnvAsInt("RATE_LIMIT_RPM", 60),
		RateLimitUserRPM:         getEnvAsInt("RATE_LIMIT_USER_RPM", 60),
		RateLimitEndpoints:       getEnvAsIntMap("RATE_LIMIT_ENDPOINTS", map[string]int{}),
		HTTPTimeoutSeconds:       getEnvAsInt("HTTP_TIMEOUT_SECONDS", 30),
		HTTPMaxRetries:           getEnvAsInt("HTTP_MAX_RETRIES", 3),
		AgentToolsDir:            getEnv("AGENT_TOOLS_DIR", filepath.Join(repoRoot, "agent-tools")),
		MongoURI:                 getEnv("MONGO_URI", "mongodb://localhost:27017"),
		RemoteAPIURL:             getEnv("REMOTE_API_URL", ""),
		RemoteAPIKey:             getEnv("REMOTE_API_KEY", ""),
		PromptLang:               getEnv("PROMPT_LANG", "en"),
		CacheBackend:             getEnv("CACHE_BACKEND", "memory"),
		RedisURL:                 getEnv("REDIS_URL", "redis://localhost:6379/0"),
		HealthCheckTimeoutMS:     getEnvAsInt("HEALTH_CHECK_TIMEOUT_MS", 2000),
		HealthExtraChecksJSON:    getEnv("HEALTH_EXTRA_CHECKS_JSON", ""),
		EmbeddingCacheTTLSeconds: getEnvAsInt("EMBEDDING_CACHE_TTL_SECONDS", 3600),
		EmbeddingCacheMaxEntries: getEnvAsInt("EMBEDDING_CACHE_MAX_ENTRIES", 1000),
		RAGCacheTTLSeconds:       getEnvAsInt("RAG_CACHE_TTL_SECONDS", 1800),
		RAGCacheMaxEntries:       getEnvAsInt("RAG_CACHE_MAX_ENTRIES", 500),
		MemoryTTLHours:           getEnvAsInt("MEMORY_TTL_HOURS", 720),
		MemoryPruneMaxEntries:    getEnvAsInt("MEMORY_PRUNE_MAX_ENTRIES", 1500),
		CBFailureThreshold:       getEnvAsInt("CB_FAILURE_THRESHOLD", 3),
		CBOpenTimeoutSeconds:     getEnvAsInt("CB_OPEN_TIMEOUT_SECONDS", 20),
		CBHalfOpenMaxSuccess:     getEnvAsInt("CB_HALF_OPEN_MAX_SUCCESS", 1),
		CBOllamaThreshold:        getEnvAsInt("CB_OLLAMA_FAILURE_THRESHOLD", 0),
		CBQdrantThreshold:        getEnvAsInt("CB_QDRANT_FAILURE_THRESHOLD", 0),
		OutboxWorkerIntervalSec:  getEnvAsInt("OUTBOX_WORKER_INTERVAL_SEC", 3),
		OutboxBatchSize:          getEnvAsInt("OUTBOX_BATCH_SIZE", 25),
		OutboxMaxAttempts:        getEnvAsInt("OUTBOX_MAX_ATTEMPTS", 5),
		OutboxRetryBackoffSec:    getEnvAsInt("OUTBOX_RETRY_BACKOFF_SEC", 5),
		MigrationsLockTTLSeconds: getEnvAsInt("MIGRATIONS_LOCK_TTL_SECONDS", 30),
		PoolMaxOpen:              getEnvAsInt("POOL_MAX_OPEN", 64),
		PoolMaxIdle:              getEnvAsInt("POOL_MAX_IDLE", 16),
		PoolTimeoutSeconds:       getEnvAsInt("POOL_TIMEOUT_SECONDS", 30),
		MongoPoolMaxOpen:         getEnvAsInt("MONGO_POOL_MAX_OPEN", 50),
		MongoPoolMaxIdle:         getEnvAsInt("MONGO_POOL_MAX_IDLE", 10),
		MongoPoolTimeoutSeconds:  getEnvAsInt("MONGO_POOL_TIMEOUT_SECONDS", 5),
		EmbeddingPoolSize:        getEnvAsInt("EMBEDDING_POOL_SIZE", 8),
		RetrievalPoolSize:        getEnvAsInt("RETRIEVAL_POOL_SIZE", 8),
		OTelEnabled:              getEnvAsBool("OTEL_ENABLED", false),
		OTelServiceName:          getEnv("OTEL_SERVICE_NAME", "ollama-gateway"),
		OTelExporterOTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		OTelExporterInsecure:     getEnvAsBool("OTEL_EXPORTER_OTLP_INSECURE", true),
		OTelSamplePercent:        getEnvAsInt("OTEL_SAMPLE_PERCENT", 100),
	}
}

func applyConfigFileToEnv() (string, int, error) {
	path, ok := resolveConfigFilePath()
	if !ok {
		return "", 0, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", 0, fmt.Errorf("no se pudo resolver CONFIG_FILE: %w", err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return "", 0, fmt.Errorf("no se pudo leer CONFIG_FILE=%s: %w", abs, err)
	}
	vars, err := parseConfigFileVars(raw)
	if err != nil {
		return "", 0, fmt.Errorf("CONFIG_FILE inválido (%s): %w", abs, err)
	}
	applied := 0
	for k, v := range vars {
		if err := os.Setenv(k, v); err != nil {
			return "", 0, fmt.Errorf("no se pudo exportar %s desde CONFIG_FILE: %w", k, err)
		}
		applied++
	}
	return abs, applied, nil
}

func resolveConfigFilePath() (string, bool) {
	v := strings.TrimSpace(os.Getenv("CONFIG_FILE"))
	if v == "" {
		return "", false
	}
	return v, true
}

func parseConfigFileVars(raw []byte) (map[string]string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, errors.New("archivo vacío")
	}
	parsed := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, err
	}
	if len(parsed) == 0 {
		return nil, errors.New("no contiene variables")
	}
	out := make(map[string]string, len(parsed))
	for key, value := range parsed {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		s, err := rawJSONToEnvValue(value)
		if err != nil {
			return nil, fmt.Errorf("campo %q inválido: %w", k, err)
		}
		out[k] = s
	}
	return out, nil
}

func rawJSONToEnvValue(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("valor vacío")
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}
	var asBool bool
	if err := json.Unmarshal(raw, &asBool); err == nil {
		if asBool {
			return "true", nil
		}
		return "false", nil
	}
	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return asNumber.String(), nil
	}
	var asAny interface{}
	if err := json.Unmarshal(raw, &asAny); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(asAny)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func parseRepoRoots() []string {
	v := strings.TrimSpace(os.Getenv("REPO_ROOTS"))
	if v != "" {
		parts := strings.Split(v, ",")
		roots := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				roots = append(roots, trimmed)
			}
		}
		if len(roots) > 0 {
			return roots
		}
	}
	return []string{getEnv("REPO_ROOT", ".")}
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

func getEnvAsIntMap(key string, fallback map[string]int) map[string]int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed := make(map[string]int)
	if err := json.Unmarshal([]byte(v), &parsed); err != nil {
		log.Printf("WARN: %s inválido (%q), usando fallback", key, v)
		return fallback
	}
	for k, n := range parsed {
		if n <= 0 {
			delete(parsed, k)
		}
	}
	return parsed
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
