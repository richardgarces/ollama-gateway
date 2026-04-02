package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// safeReadFile lee un archivo solo si está dentro del directorio permitido.
func safeReadFile(input string) string {
	absPath, err := filepath.Abs(input)
	if err != nil {
		return "ruta inválida"
	}
	allowedRoot, _ := filepath.Abs(cfg.RepoRoot)
	if !strings.HasPrefix(absPath, allowedRoot+string(os.PathSeparator)) && absPath != allowedRoot {
		return "acceso denegado: fuera del directorio permitido"
	}
	data, e := os.ReadFile(absPath)
	if e != nil {
		return e.Error()
	}
	return string(data)
}

func runAgent(prompt string) string {
	toolsDef := `
Herramientas:
1. get_time -> devuelve hora
2. read_file -> lee archivo del proyecto

Formato JSON obligado:
{"action": "...", "input": "...", "response": "..."}
`
	fullPrompt := prompt + "\n" + toolsDef

	model := "qwen2.5:7b"
	resp, err := callOllama(model, fullPrompt)
	if err != nil {
		return err.Error()
	}

	var ar struct {
		Action   string `json:"action"`
		Input    string `json:"input"`
		Response string `json:"response"`
	}
	json.Unmarshal([]byte(resp), &ar)

	switch ar.Action {
	case "get_time":
		return time.Now().Format(time.RFC3339)
	case "read_file":
		return safeReadFile(ar.Input)
	default:
		if ar.Response != "" {
			return ar.Response
		}
		return "No action matched: " + resp
	}
}

func agentHandler(w http.ResponseWriter, r *http.Request) {
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "body inválido: "+err.Error())
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt requerido")
		return
	}
	result := runAgent(req.Prompt)
	writeJSON(w, http.StatusOK, Response{Result: result})
}
