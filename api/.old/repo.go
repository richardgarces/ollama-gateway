package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages requerido")
		return
	}

	prompt := ""
	for _, m := range req.Messages {
		prompt += m.Role + ": " + m.Content + "\n"
	}

	result, err := generateWithRAG(prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]interface{}{
		"choices": []map[string]interface{}{
			{
				"message": map[string]string{
					"role":    "assistant",
					"content": result,
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func refactorHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Validar que el path esté dentro del directorio permitido
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		http.Error(w, `{"error":"ruta inválida"}`, http.StatusBadRequest)
		return
	}
	allowedRoot, _ := filepath.Abs(cfg.RepoRoot)
	if !strings.HasPrefix(absPath, allowedRoot+string(os.PathSeparator)) && absPath != allowedRoot {
		http.Error(w, `{"error":"acceso denegado: fuera del directorio permitido"}`, http.StatusForbidden)
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "archivo no encontrado: "+err.Error())
		return
	}

	prompt := "Eres un senior Go developer. Mejora este código:\n" + string(data)

	resp, err := callOllama("deepseek-coder:6.7b", prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"refactor": resp})
}

func analyzeRepoHandler(w http.ResponseWriter, r *http.Request) {
	var files []string
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})

	results := []string{}
	for _, f := range files {
		if len(results) > 5 { // Limitar para prueba
			break
		}
		data, _ := os.ReadFile(f)
		prompt := "Revisa este file y dime algo sobre su arquitectura y mejoras:\n" + string(data)
		resp, _ := callOllama("qwen2.5:7b", prompt)
		results = append(results, resp)
	}

	prompt := "Resume estos análisis:\n" + strings.Join(results, "\n")
	final, _ := callOllama("qwen2.5:7b", prompt)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"analysis": final})
}
