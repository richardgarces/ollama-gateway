package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

type Request struct {
	Prompt string `json:"prompt"`
}

type Response struct {
	Result string `json:"result"`
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func generateHandler(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt requerido")
		return
	}

	result, err := generateWithRAG(req.Prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, Response{Result: result})
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/health", health).Methods("GET")

	r.Use(loggingMiddleware)
	r.Use(corsMiddleware)

	api := r.PathPrefix("/api").Subrouter()
	api.Use(jwtMiddleware)
	api.HandleFunc("/generate", generateHandler).Methods("POST")
	api.HandleFunc("/agent", agentHandler).Methods("POST")
	api.HandleFunc("/refactor", refactorHandler).Methods("POST")
	api.HandleFunc("/analyze-repo", analyzeRepoHandler).Methods("GET")
	api.HandleFunc("/v1/chat/completions", chatHandler).Methods("POST")

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("SaaS API en ejecución en el puerto :%s...\n", cfg.Port)
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
		log.Fatal("error en shutdown:", err)
	}
	log.Println("Servidor apagado correctamente")
}
