package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
)

type Config struct {
	Port      string
	OllamaURL string
	QdrantURL string
	JWTSecret []byte
	RepoRoot  string
}

func Load() *Config {
	return &Config{
		Port:      getEnv("PORT", "8081"),
		OllamaURL: getEnv("OLLAMA_URL", "http://ollama:11434"),
		QdrantURL: getEnv("QDRANT_URL", "http://qdrant:6333"),
		JWTSecret: loadJWTSecret(),
		RepoRoot:  getEnv("REPO_ROOT", "."),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
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
		return []byte(s)
	}
	return decoded
}
