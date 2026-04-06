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
		"- edit_file (workspace): edita un archivo existente reemplazando un fragmento específico. Args: {\"path\": \"relative/path\", \"search\": \"texto exacto a buscar\", \"replace\": \"texto de reemplazo\"}. Incluye unas pocas líneas de contexto en search para ser preciso.\n" +
		"- modify_file (workspace): reemplaza TODO el contenido de un archivo. Solo úsalo si necesitas reescribir el archivo completo. Args: {\"path\": \"relative/path\", \"content\": \"contenido nuevo completo del archivo\"}\n" +
		"- delete_file (workspace): elimina un archivo del workspace. Args: {\"path\": \"relative/path\"}\n" +
		"- run_command (workspace): ejecuta un comando en la terminal del workspace (git, npm, go, etc). Args: {\"command\": \"comando a ejecutar\"}\n" +
		"- multi_edit (workspace): aplica ediciones parciales a MÚLTIPLES archivos en un solo paso. Args: {\"edits\": \"JSON array\"} donde cada elemento es {\"path\":\"...\",\"search\":\"...\",\"replace\":\"...\"}. Ejemplo: {\"edits\":\"[{\\\"path\\\":\\\"a.go\\\",\\\"search\\\":\\\"old\\\",\\\"replace\\\":\\\"new\\\"},{\\\"path\\\":\\\"b.go\\\",\\\"search\\\":\\\"old2\\\",\\\"replace\\\":\\\"new2\\\"}]\"}"

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
		"5. Para editar archivos, PREFIERE edit_file (edición parcial search/replace) sobre modify_file. Si necesitas cambiar varios archivos a la vez, usa multi_edit.\n" +
		"6. Para eliminar archivos, usa delete_file.\n" +
		"7. Para ejecutar comandos del sistema (git, npm, go build, etc.), usa run_command.\n" +
		"8. Si la tarea requiere información actualizada de Internet (noticias, documentación, precios, fechas, etc.), usa web_search.\n" +
		"9. Máximo " + fmt.Sprintf("%d", maxAutopilotIterations) + " iteraciones." +
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
			history = append(history, fmt.Sprintf("PASO %d: list_workspace → %d archivos:\n%s", i, len(wsCtx.Tree), truncateForHistory(listing, 3000)))
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
				history = append(history, fmt.Sprintf("PASO %d: read_workspace_file(%s) →\n%s", i, path, truncateForHistory(content, 4000)))
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
						history = append(history, fmt.Sprintf("PASO %d: read_workspace_file(%s) →\n%s", i, path, truncateForHistory(out, 4000)))
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

		case "edit_file":
			path := args["path"]
			search := args["search"]
			replace := args["replace"]
			if path == "" || search == "" {
				f := false
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "edit_file", Success: &f, Output: "path y search son requeridos"})
				history = append(history, fmt.Sprintf("PASO %d: edit_file → falta path o search", i))
				continue
			}
			t := true
			onEvent(AutopilotEvent{
				Event:       "file_edit",
				Iteration:   i,
				Tool:        "edit_file",
				FilePath:    path,
				FileContent: search + "\n---REPLACE---\n" + replace,
			})
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "edit_file", Success: &t, Output: "Edición propuesta para: " + path + " (pendiente de aceptación del usuario)"})
			history = append(history, fmt.Sprintf("PASO %d: edit_file(%s) → search/replace propuesto al usuario", i, path))
			continue

		case "multi_edit":
			editsRaw := args["edits"]
			if editsRaw == "" {
				f := false
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "multi_edit", Success: &f, Output: "edits (JSON array) es requerido"})
				history = append(history, fmt.Sprintf("PASO %d: multi_edit → falta edits", i))
				continue
			}
			var edits []struct {
				Path    string `json:"path"`
				Search  string `json:"search"`
				Replace string `json:"replace"`
			}
			if err := json.Unmarshal([]byte(editsRaw), &edits); err != nil {
				f := false
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "multi_edit", Success: &f, Output: "edits no es JSON válido: " + err.Error()})
				history = append(history, fmt.Sprintf("PASO %d: multi_edit → JSON inválido", i))
				continue
			}
			historyParts := []string{}
			for idx, ed := range edits {
				if ed.Path == "" || ed.Search == "" {
					continue
				}
				onEvent(AutopilotEvent{
					Event:       "file_edit",
					Iteration:   i,
					Tool:        "multi_edit",
					FilePath:    ed.Path,
					FileContent: ed.Search + "\n---REPLACE---\n" + ed.Replace,
				})
				historyParts = append(historyParts, fmt.Sprintf("  %d) %s", idx+1, ed.Path))
			}
			t := true
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "multi_edit", Success: &t, Output: fmt.Sprintf("%d ediciones propuestas (pendiente de aceptación)", len(edits))})
			history = append(history, fmt.Sprintf("PASO %d: multi_edit → %d archivos:\n%s", i, len(edits), strings.Join(historyParts, "\n")))
			continue

		case "delete_file":
			path := args["path"]
			if path == "" {
				f := false
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "delete_file", Success: &f, Output: "path es requerido"})
				history = append(history, fmt.Sprintf("PASO %d: delete_file → falta path", i))
				continue
			}
			t := true
			onEvent(AutopilotEvent{
				Event:     "file_delete",
				Iteration: i,
				Tool:      "delete_file",
				FilePath:  path,
			})
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "delete_file", Success: &t, Output: "Eliminación propuesta: " + path + " (pendiente de aceptación del usuario)"})
			history = append(history, fmt.Sprintf("PASO %d: delete_file(%s) → propuesto al usuario", i, path))
			continue

		case "run_command":
			cmd := args["command"]
			if cmd == "" {
				f := false
				onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "run_command", Success: &f, Output: "command es requerido"})
				history = append(history, fmt.Sprintf("PASO %d: run_command → falta command", i))
				continue
			}
			t := true
			onEvent(AutopilotEvent{
				Event:       "run_command",
				Iteration:   i,
				Tool:        "run_command",
				FileContent: cmd,
			})
			onEvent(AutopilotEvent{Event: "tool_result", Iteration: i, Tool: "run_command", Success: &t, Output: "Comando propuesto: " + cmd + " (pendiente de aceptación del usuario)"})
			history = append(history, fmt.Sprintf("PASO %d: run_command(%s) → propuesto al usuario", i, cmd))
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
			history = append(history, fmt.Sprintf("PASO %d: %s(%v) →\n%s", i, parsed.Action, args, truncateForHistory(output, 4000)))
		}
	}

	onEvent(AutopilotEvent{Event: "answer", Iteration: maxAutopilotIterations, Content: "Se alcanzó el límite de iteraciones. Revisa los pasos anteriores para resultados parciales."})
	return nil
}

func truncateForHistory(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncado)"
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
