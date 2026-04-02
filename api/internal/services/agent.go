package services

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ollama-gateway/internal/domain"
)

type AgentService struct {
	ollamaService *OllamaService
	repoRoot      string
	logger        *slog.Logger
}

var _ domain.AgentRunner = (*AgentService)(nil)

func NewAgentService(ollamaService *OllamaService, repoRoot string, logger *slog.Logger) *AgentService {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentService{
		ollamaService: ollamaService,
		repoRoot:      repoRoot,
		logger:        logger,
	}
}

func (s *AgentService) safeReadFile(input string) string {
	absPath, err := filepath.Abs(input)
	if err != nil {
		return "ruta inválida"
	}
	allowedRoot, _ := filepath.Abs(s.repoRoot)
	if !strings.HasPrefix(absPath, allowedRoot+string(os.PathSeparator)) && absPath != allowedRoot {
		return "acceso denegado: fuera del directorio permitido"
	}
	data, e := os.ReadFile(absPath)
	if e != nil {
		return e.Error()
	}
	return string(data)
}

func (s *AgentService) Run(prompt string) string {
	toolsDef := `
Herramientas:
1. get_time -> devuelve hora
2. read_file -> lee archivo del proyecto

Formato JSON obligado:
{"action": "...", "input": "...", "response": "..."}
`
	fullPrompt := prompt + "\n" + toolsDef

	model := "qwen2.5:7b"
	resp, err := s.ollamaService.Generate(model, fullPrompt)
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
		return s.safeReadFile(ar.Input)
	default:
		if ar.Response != "" {
			return ar.Response
		}
		return "No action matched: " + resp
	}
}
