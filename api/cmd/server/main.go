package main

import (
	"log"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/server"
)

func main() {
	cfg := config.Load()

	srv := server.New(cfg)
	if err := srv.Start(); err != nil {
		log.Fatal("error en shutdown:", err)
	}
}
