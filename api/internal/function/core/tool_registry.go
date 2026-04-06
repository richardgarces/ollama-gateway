package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Tool interface {
	Name() string
	Run(args map[string]string) (string, error)
}

type toolSpec struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Type        string            `yaml:"type"`
	Parameters  map[string]string `yaml:"parameters"`
}

type ToolRegistry struct {
	tools      map[string]Tool
	toolSpecs  map[string]toolSpec
	repoRoot   string
	toolsDir   string
	logger     *slog.Logger
	httpClient *http.Client
}

func NewToolRegistry(toolsDir, repoRoot string, logger *slog.Logger) *ToolRegistry {
	if logger == nil {
		logger = slog.Default()
	}

	r := &ToolRegistry{
		tools:      make(map[string]Tool),
		toolSpecs:  make(map[string]toolSpec),
		repoRoot:   repoRoot,
		toolsDir:   toolsDir,
		logger:     logger,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	r.registerBuiltins()
	r.loadFromDir()
	return r
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// RegisterTool allows external packages to register a tool with the registry.
func (r *ToolRegistry) RegisterTool(name, description string, tool Tool) {
	r.register(toolSpec{
		Name:        name,
		Description: description,
		Type:        "function",
		Parameters:  map[string]string{},
	}, tool)
}

func (r *ToolRegistry) ToolDescriptions() []string {
	if len(r.toolSpecs) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.toolSpecs))
	for name := range r.toolSpecs {
		names = append(names, name)
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names))
	for _, name := range names {
		spec := r.toolSpecs[name]
		desc := strings.TrimSpace(spec.Description)
		if desc == "" {
			desc = "sin descripción"
		}
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", spec.Name, spec.Type, desc))
	}
	return lines
}

func (r *ToolRegistry) registerBuiltins() {
	r.register(toolSpec{
		Name:        "get_time",
		Description: "Devuelve la hora actual en formato RFC3339",
		Type:        "function",
		Parameters:  map[string]string{},
	}, &getTimeTool{})

	r.register(toolSpec{
		Name:        "read_file",
		Description: "Lee un archivo dentro del repositorio permitido",
		Type:        "function",
		Parameters:  map[string]string{},
	}, &readFileTool{repoRoot: r.repoRoot})
}

func (r *ToolRegistry) register(spec toolSpec, tool Tool) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return
	}
	r.tools[name] = tool
	r.toolSpecs[name] = spec
}

func (r *ToolRegistry) loadFromDir() {
	if strings.TrimSpace(r.toolsDir) == "" {
		return
	}

	resolvedDir := r.toolsDir
	if !filepath.IsAbs(resolvedDir) {
		resolvedDir = filepath.Join(r.repoRoot, resolvedDir)
	}

	entries, err := os.ReadDir(resolvedDir)
	if err != nil {
		if os.IsNotExist(err) {
			r.logger.Info("directorio de tools no existe; se usarán solo built-in", slog.String("dir", resolvedDir))
			return
		}
		r.logger.Warn("no se pudo leer directorio de tools", slog.String("dir", resolvedDir), slog.String("error", err.Error()))
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		fullPath := filepath.Join(resolvedDir, entry.Name())
		spec, e := readToolSpec(fullPath)
		if e != nil {
			r.logger.Warn("tool YAML inválido", slog.String("file", fullPath), slog.String("error", e.Error()))
			continue
		}

		if spec.Name == "get_time" || spec.Name == "read_file" {
			r.logger.Warn("tool YAML intenta sobrescribir built-in; ignorado", slog.String("name", spec.Name), slog.String("file", fullPath))
			continue
		}

		tool, e := buildConfiguredTool(spec, r.httpClient)
		if e != nil {
			r.logger.Warn("tool YAML no cargado", slog.String("file", fullPath), slog.String("error", e.Error()))
			continue
		}

		r.register(spec, tool)
		r.logger.Info("tool cargado", slog.String("name", spec.Name), slog.String("type", spec.Type))
	}
}

func readToolSpec(path string) (toolSpec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return toolSpec{}, err
	}

	var spec toolSpec
	if err := yaml.Unmarshal(content, &spec); err != nil {
		return toolSpec{}, err
	}

	spec.Name = strings.TrimSpace(spec.Name)
	spec.Description = strings.TrimSpace(spec.Description)
	spec.Type = strings.ToLower(strings.TrimSpace(spec.Type))
	if spec.Parameters == nil {
		spec.Parameters = map[string]string{}
	}

	if spec.Name == "" {
		return toolSpec{}, fmt.Errorf("name es requerido")
	}
	switch spec.Type {
	case "shell", "http", "function":
	default:
		return toolSpec{}, fmt.Errorf("type inválido: %s", spec.Type)
	}

	return spec, nil
}

func buildConfiguredTool(spec toolSpec, httpClient *http.Client) (Tool, error) {
	switch spec.Type {
	case "function":
		return &functionTool{name: spec.Name, parameters: spec.Parameters}, nil
	case "http":
		return &httpTool{name: spec.Name, parameters: spec.Parameters, httpClient: httpClient}, nil
	case "shell":
		return &shellTool{name: spec.Name, parameters: spec.Parameters}, nil
	default:
		return nil, fmt.Errorf("tool type no soportado: %s", spec.Type)
	}
}

type getTimeTool struct{}

func (t *getTimeTool) Name() string { return "get_time" }

func (t *getTimeTool) Run(args map[string]string) (string, error) {
	return time.Now().Format(time.RFC3339), nil
}

type readFileTool struct {
	repoRoot string
}

func (t *readFileTool) Name() string { return "read_file" }

func (t *readFileTool) Run(args map[string]string) (string, error) {
	input := strings.TrimSpace(args["path"])
	if input == "" {
		input = strings.TrimSpace(args["input"])
	}
	if input == "" {
		return "", fmt.Errorf("path requerido")
	}

	absPath, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("ruta inválida")
	}
	allowedRoot, err := filepath.Abs(t.repoRoot)
	if err != nil {
		return "", fmt.Errorf("repo root inválido")
	}

	if absPath != allowedRoot && !strings.HasPrefix(absPath, allowedRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("acceso denegado: fuera del directorio permitido")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type functionTool struct {
	name       string
	parameters map[string]string
}

func (t *functionTool) Name() string { return t.name }

func (t *functionTool) Run(args map[string]string) (string, error) {
	if msg := strings.TrimSpace(t.parameters["response"]); msg != "" {
		return interpolate(msg, args, t.parameters), nil
	}
	payload := map[string]string{}
	for k, v := range t.parameters {
		payload[k] = interpolate(v, args, t.parameters)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type httpTool struct {
	name       string
	parameters map[string]string
	httpClient *http.Client
}

func (t *httpTool) Name() string { return t.name }

func (t *httpTool) Run(args map[string]string) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(t.parameters["method"]))
	if method == "" {
		method = http.MethodGet
	}
	url := interpolate(strings.TrimSpace(t.parameters["url"]), args, t.parameters)
	if url == "" {
		return "", fmt.Errorf("url requerida")
	}

	bodyRaw := interpolate(t.parameters["body"], args, t.parameters)
	var body io.Reader
	if bodyRaw != "" {
		body = strings.NewReader(bodyRaw)
	}

	timeout := 2 * time.Second
	if timeoutText := strings.TrimSpace(t.parameters["timeout_seconds"]); timeoutText != "" {
		if sec, err := strconv.Atoi(timeoutText); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", err
	}
	if contentType := strings.TrimSpace(t.parameters["content_type"]); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	return string(respBody), nil
}

type shellTool struct {
	name       string
	parameters map[string]string
}

func (t *shellTool) Name() string { return t.name }

func (t *shellTool) Run(args map[string]string) (string, error) {
	command := strings.TrimSpace(t.parameters["command"])
	if command == "" {
		return "", fmt.Errorf("command requerido")
	}

	argsText := interpolate(strings.TrimSpace(t.parameters["args"]), args, t.parameters)
	cmdArgs := []string{}
	if argsText != "" {
		cmdArgs = strings.Fields(argsText)
	}

	timeout := 2 * time.Second
	if timeoutText := strings.TrimSpace(t.parameters["timeout_seconds"]); timeoutText != "" {
		if sec, err := strconv.Atoi(timeoutText); err == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%s: %w", trimmed, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func interpolate(input string, args map[string]string, defaults map[string]string) string {
	result := input
	for key, value := range defaults {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	for key, value := range args {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

func joinToolDescriptions(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b bytes.Buffer
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
