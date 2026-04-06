package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	coreservice "ollama-gateway/internal/function/core"
)

const maxAutopilotIterations = 10

// WorkspaceFile represents a file from the client workspace.
type WorkspaceFile struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
}

// WorkspaceContext holds the client workspace state sent by the extension.
type WorkspaceContext struct {
	Tree  []string        `json:"tree"`  // list of relative file paths
	Files []WorkspaceFile `json:"files"` // files with content (open/selected)
}

// AutopilotEvent represents a single event emitted during an autopilot run.
type AutopilotEvent struct {
	Event       string            `json:"event"`
	Iteration   int               `json:"iteration,omitempty"`
	Content     string            `json:"content,omitempty"`
	Tool        string            `json:"tool,omitempty"`
	Args        map[string]string `json:"args,omitempty"`
	Success     *bool             `json:"success,omitempty"`
	Output      string            `json:"output,omitempty"`
	FilePath    string            `json:"file_path,omitempty"`
	FileContent string            `json:"file_content,omitempty"`
}

type AutopilotService struct {
	ollamaService *coreservice.OllamaService
	logger        *slog.Logger
	toolRegistry  *coreservice.ToolRegistry
}

func NewAutopilotService(ollamaService *coreservice.OllamaService, logger *slog.Logger, toolRegistry *coreservice.ToolRegistry) *AutopilotService {
	if logger == nil {
		logger = slog.Default()
	}
	if toolRegistry == nil {
		toolRegistry = coreservice.NewToolRegistry("", ".", logger)
	}
	return &AutopilotService{
		ollamaService: ollamaService,
		logger:        logger,
		toolRegistry:  toolRegistry,
	}
}

func (s *AutopilotService) RunStream(ctx context.Context, task, model string, wsCtx WorkspaceContext, onEvent func(AutopilotEvent)) error {
	toolsDef := joinToolDescriptions(s.toolRegistry.ToolDescriptions())
	if toolsDef == "" {
		toolsDef = "- get_time (function): devuelve hora actual"
	}

	// Build workspace file index for read_workspace_file
	wsIndex := make(map[string]string, len(wsCtx.Files))
	for _, f := range wsCtx.Files {
		wsIndex[f.Path] = f.Content
	}

	workspaceTools := "\n" +
		"- list_workspace (workspace): lista los archivos del workspace del usuario. No requiere args.\n" +
		"- read_workspace_file (workspace): lee un archivo del workspace. Args: {\"path\": \"relative/path\"}\n" +
		"- create_file (workspace): crea un archivo nuevo en el workspace. Args: {\"path\": \"relative/path\", \"content\": \"contenido completo del archivo\"}\n" +
		"- modify_file (workspace): modifica un archivo existente en el workspace. Args: {\"path\": \"relative/path\", \"content\": \"contenido nuevo completo del archivo\"}"

	// If workspace context has files, mention them
	wsInfo := ""
	if len(wsCtx.Tree) > 0 {
		treeSample := wsCtx.Tree
		if len(treeSample) > 100 {
			treeSample = treeSample[:100]
		}
		wsInfo = "\n\nARCHIVOS EN EL WORKSPACE (" + fmt.Sprintf("%d", len(wsCtx.Tree)) + " total):\n" + strings.Join(treeSample, "\n")
		if len(wsCtx.Tree) > 100 {
			wsInfo += "\n... y " + fmt.Sprintf("%d", len(wsCtx.Tree)-100) + " más"
		}
	}
	if len(wsCtx.Files) > 0 {
		names := make([]string, 0, len(wsCtx.Files))
		for _, f := range wsCtx.Files {
			names = append(names, f.Path)
		}
		wsInfo += "\n\nARCHIVOS ABIERTOS/CONTEXTO (contenido disponible vía read_workspace_file):\n" + strings.Join(names, "\n")
	}

	systemPrompt := "Eres un agente autónomo (autopilot) con acceso al workspace del usuario. Tu objetivo es completar la tarea paso a paso.\n\n" +
		"Herramientas disponibles:\n" + toolsDef + workspaceTools + "\n\n" +
		"REGLAS:\n" +
		"1. En cada paso, responde con EXACTAMENTE un objeto JSON (sin markdown, sin texto adicional).\n" +
		"2. Para usar una herramienta: {\"action\":\"nombre_tool\",\"args\":{\"key\":\"value\"},\"thought\":\"tu razonamiento\"}\n" +
		"3. Para dar la respuesta final: {\"action\":\"answer\",\"response\":\"tu respuesta completa\",\"thought\":\"resumen\"}\n" +
		"4. Analiza los resultados de herramientas anteriores antes de decidir el siguiente paso.\n" +
		"5. Para crear o modificar archivos, usa create_file o modify_file con el contenido COMPLETO del archivo.\n" +
		"6. Máximo " + fmt.Sprintf("%d", maxAutopilotIterations) + " iteraciones." +
		wsInfo

	if model == "" {
		model = "qwen2.5:7b"
	}

	history := []string{"TAREA: " + task}

	for i := 1; i <= maxAutopilotIterations; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fullPrompt := systemPrompt + "\n\n" + strings.Join(history, "\n\n")

		resp, err := s.ollamaService.GenerateWithContext(ctx, model, fullPrompt)
		if err != nil {
			return fmt.Errorf("iteración %d: %w", i, err)
		}

		resp = strings.TrimSpace(resp)
		resp = stripCodeFence(resp)

		var parsed struct {
			Action   string            `json:"action"`
			Args     map[string]string `json:"args"`
			Thought  string            `json:"thought"`
			Response string            `json:"response"`
		}

		if err := json.Unmarshal([]byte(resp), &parsed); err != nil {
			onEvent(AutopilotEvent{Event: "answer", Iteration: i, Content: resp})
			return nil
		}

		if parsed.Thought != "" {
			onEvent(AutopilotEvent{Event: "thinking", Iteration: i, Content: parsed.Thought})
		}

		if parsed.Action == "answer" || parsed.Action == "" {
			answer := parsed.Response
			if answer == "" {
				answer = resp
			}
			onEvent(AutopilotEvent{Event: "answer", Iteration: i, Content: answer})
			return nil
		}

		args := parsed.Args
		if args == nil {
			args = map[string]string{}
		}

		onEvent(AutopilotEvent{
			Event:     "tool_call",
			Iteration: i,
			Tool:      parsed.Action,
			Args:      args,
		})

		// Handle workspace tools (client-side)
		switch parsed.Action {
		case "list_workspace":
			t := true
			listing := strings.Join(wsCtx.Tree, "\n")
			if listing == "" {
				listing = "(workspace vacío o sin archivos)"
			}
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "list_workspace", Success: &t, Output: listing})
			history = append(history, fmt.Sprintf("PASO %d: list_workspace → %d archivos", i, len(wsCtx.Tree)))
			continue

		case "read_workspace_file":
			path := args["path"]
			if content, ok := wsIndex[path]; ok {
				t := true
				truncated := content
				if len(truncated) > 6000 {
					truncated = truncated[:6000] + "\n...(truncado)"
				}
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "read_workspace_file", Success: &t, Output: truncated})
				history = append(history, fmt.Sprintf("PASO %d: read_workspace_file(%s) → %d chars", i, path, len(content)))
			} else {
				// Try server-side read_file as fallback
				if tool, ok := s.toolRegistry.Get("read_file"); ok {
					out, toolErr := tool.Run(map[string]string{"path": path})
					if toolErr != nil {
						f := false
						onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "read_workspace_file", Success: &f, Output: "Archivo no disponible: " + path + ". " + toolErr.Error()})
						history = append(history, fmt.Sprintf("PASO %d: read_workspace_file(%s) → error: %s", i, path, toolErr.Error()))
					} else {
						t := true
						truncated := out
						if len(truncated) > 6000 {
							truncated = truncated[:6000] + "\n...(truncado)"
						}
						onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "read_workspace_file", Success: &t, Output: truncated})
						history = append(history, fmt.Sprintf("PASO %d: read_workspace_file(%s) → %d chars (server)", i, path, len(out)))
					}
				} else {
					f := false
					msg := "Archivo no disponible en el contexto: " + path
					onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "read_workspace_file", Success: &f, Output: msg})
					history = append(history, fmt.Sprintf("PASO %d: read_workspace_file(%s) → no disponible", i, path))
				}
			}
			continue

		case "create_file":
			path := args["path"]
			content := args["content"]
			if path == "" || content == "" {
				f := false
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "create_file", Success: &f, Output: "path y content son requeridos"})
				history = append(history, fmt.Sprintf("PASO %d: create_file → falta path o content", i))
				continue
			}
			t := true
			onEvent(AutopilotEvent{
				Event:       "file_create",
				Iteration:   i,
				Tool:        "create_file",
				FilePath:    path,
				FileContent: content,
			})
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "create_file", Success: &t, Output: "Archivo propuesto: " + path + " (pendiente de aceptación del usuario)"})
			history = append(history, fmt.Sprintf("PASO %d: create_file(%s) → propuesto al usuario", i, path))
			continue

		case "modify_file":
			path := args["path"]
			content := args["content"]
			if path == "" || content == "" {
				f := false
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "modify_file", Success: &f, Output: "path y content son requeridos"})
				history = append(history, fmt.Sprintf("PASO %d: modify_file → falta path o content", i))
				continue
			}
			t := true
			onEvent(AutopilotEvent{
				Event:       "file_modify",
				Iteration:   i,
				Tool:        "modify_file",
				FilePath:    path,
				FileContent: content,
			})
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "modify_file", Success: &t, Output: "Cambios propuestos para: " + path + " (pendiente de aceptación del usuario)"})
			history = append(history, fmt.Sprintf("PASO %d: modify_file(%s) → propuesto al usuario", i, path))
			continue
		}

		// Fallback to server-side tools (ToolRegistry)
		tool, ok := s.toolRegistry.Get(parsed.Action)
		if !ok {
			errMsg := fmt.Sprintf("herramienta no encontrada: %s", parsed.Action)
			f := false
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: parsed.Action, Success: &f, Output: errMsg})
			history = append(history, fmt.Sprintf("PASO %d: %s → no existe", i, parsed.Action))
			continue
		}

		output, toolErr := tool.Run(args)
		if toolErr != nil {
			s.logger.Warn("autopilot tool error", slog.String("tool", parsed.Action), slog.Int("iteration", i), slog.String("error", toolErr.Error()))
			f := false
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: parsed.Action, Success: &f, Output: toolErr.Error()})
			history = append(history, fmt.Sprintf("PASO %d: %s(%v) → error: %s", i, parsed.Action, args, toolErr.Error()))
		} else {
			truncated := output
			if len(truncated) > 4000 {
				truncated = truncated[:4000] + "\n...(truncado)"
			}
			t := true
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: parsed.Action, Success: &t, Output: truncated})
			history = append(history, fmt.Sprintf("PASO %d: %s(%v) → %d chars", i, parsed.Action, args, len(output)))
		}
	}

	onEvent(AutopilotEvent{Event: "answer", Iteration: maxAutopilotIterations, Content: "Se alcanzó el límite de iteraciones. Revisa los pasos anteriores para resultados parciales."})
	return nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
