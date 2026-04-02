package services

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"ollama-gateway/internal/domain"
)

type AgentService struct {
	ollamaService *OllamaService
	logger        *slog.Logger
	toolRegistry  *ToolRegistry
}

var _ domain.AgentRunner = (*AgentService)(nil)

func NewAgentService(ollamaService *OllamaService, logger *slog.Logger, toolRegistry *ToolRegistry) *AgentService {
	if logger == nil {
		logger = slog.Default()
	}
	if toolRegistry == nil {
		toolRegistry = NewToolRegistry("", ".", logger)
	}
	return &AgentService{
		ollamaService: ollamaService,
		logger:        logger,
		toolRegistry:  toolRegistry,
	}
}

func (s *AgentService) Run(prompt string) string {
	toolsDef := joinToolDescriptions(s.toolRegistry.ToolDescriptions())
	if toolsDef == "" {
		toolsDef = "- get_time (function): devuelve hora actual\n- read_file (function): lee archivo del proyecto"
	}

	instructions := `
Herramientas disponibles:
` + toolsDef + `

Formato JSON obligado:
{"action":"nombre_tool","args":{"key":"value"},"input":"opcional","response":"..."}
`

	fullPrompt := prompt + "\n" + instructions

	model := "qwen2.5:7b"
	resp, err := s.ollamaService.Generate(model, fullPrompt)
	if err != nil {
		return err.Error()
	}

	var ar struct {
		Action   string            `json:"action"`
		Args     map[string]string `json:"args"`
		Input    string            `json:"input"`
		Response string            `json:"response"`
	}
	if err := json.Unmarshal([]byte(resp), &ar); err != nil {
		return resp
	}

	if ar.Action != "" {
		args := ar.Args
		if args == nil {
			args = map[string]string{}
		}
		if ar.Input != "" && args["path"] == "" && ar.Action == "read_file" {
			args["path"] = ar.Input
		}

		tool, ok := s.toolRegistry.Get(ar.Action)
		if ok {
			out, e := tool.Run(args)
			if e != nil {
				s.logger.Warn("tool execution failed", slog.String("tool", ar.Action), slog.String("error", e.Error()))
				return e.Error()
			}
			return out
		}
		s.logger.Warn("tool not found", slog.String("tool", ar.Action))
		return fmt.Sprintf("tool no encontrada: %s", ar.Action)
	}

	if ar.Response != "" {
		return ar.Response
	}
	return resp
}
