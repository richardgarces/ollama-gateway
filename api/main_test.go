package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/domain"
	"ollama-gateway/internal/handlers"
	"ollama-gateway/internal/observability"
	"ollama-gateway/internal/services"
	"ollama-gateway/pkg/httputil"
)

type fakeRAGService struct {
	result string
	err    error
}

func (f fakeRAGService) GenerateWithContext(prompt string) (string, error) {
	return f.result, f.err
}

func (f fakeRAGService) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	if f.err != nil {
		return f.err
	}
	return onChunk(f.result)
}

type fakeAgentService struct{}

func (fakeAgentService) Run(prompt string) string {
	return prompt
}

type fakeOllamaService struct {
	embedding []float64
	err       error
}

func (f fakeOllamaService) Generate(model, prompt string) (string, error) {
	return "", f.err
}

func (f fakeOllamaService) StreamGenerate(model, prompt string, onChunk func(string) error) error {
	if f.err != nil {
		return f.err
	}
	return onChunk("")
}

func (f fakeOllamaService) GetEmbedding(model, text string) ([]float64, error) {
	return f.embedding, f.err
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler := handlers.NewHealthHandler(&config.Config{})
	handler.Liveness(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGenerateHandlerBadBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/generate", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	handler := handlers.NewGenerateHandler(fakeRAGService{})
	handler.Handle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGenerateHandlerEmptyPrompt(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"prompt": ""})
	req := httptest.NewRequest("POST", "/api/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler := handlers.NewGenerateHandler(fakeRAGService{})
	handler.Handle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAgentHandlerBadBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/agent", bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()
	handler := handlers.NewAgentHandler(fakeAgentService{})
	handler.Handle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAgentHandlerEmptyPrompt(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"input": ""})
	req := httptest.NewRequest("POST", "/api/agent", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler := handlers.NewAgentHandler(fakeAgentService{})
	handler.Handle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatHandlerBadBody(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/chat/completions", bytes.NewBufferString("nope"))
	w := httptest.NewRecorder()
	handler := handlers.NewChatHandler(fakeRAGService{})
	handler.Handle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatHandlerEmptyMessages(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"messages": []map[string]string{}})
	req := httptest.NewRequest("POST", "/api/v1/chat/completions", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	handler := handlers.NewChatHandler(fakeRAGService{})
	handler.Handle(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSelectModelCode(t *testing.T) {
	m := services.NewRouterService(nil, nil, slog.Default()).SelectModel("implement a func to parse JSON")
	if m != "deepseek-coder:6.7b" {
		t.Errorf("expected deepseek-coder, got %s", m)
	}
}

func TestSelectModelLong(t *testing.T) {
	longPrompt := string(make([]byte, 400))
	m := services.NewRouterService(nil, nil, slog.Default()).SelectModel(longPrompt)
	if m != "qwen2.5:7b" {
		t.Errorf("expected qwen2.5:7b, got %s", m)
	}
}

func TestSelectModelDefault(t *testing.T) {
	m := services.NewRouterService(nil, nil, slog.Default()).SelectModel("hola")
	if m != "gemma:2b" {
		t.Errorf("expected gemma:2b, got %s", m)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	httputil.WriteError(w, http.StatusNotFound, "not found")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	var e httputil.APIError
	json.NewDecoder(w.Body).Decode(&e)
	if e.Code != 404 || e.Error != "not found" {
		t.Errorf("unexpected error body: %+v", e)
	}
}

func TestMetricsCollector(t *testing.T) {
	collector := observability.NewMetricsCollector()
	collector.Observe(http.MethodGet, "/health", http.StatusOK, 10)
	collector.Observe(http.MethodGet, "/health", http.StatusInternalServerError, 20)

	snapshot := collector.Snapshot()
	if snapshot.TotalRequests != 2 {
		t.Fatalf("expected 2 requests, got %d", snapshot.TotalRequests)
	}
	if len(snapshot.Routes) != 1 {
		t.Fatalf("expected 1 route metric, got %d", len(snapshot.Routes))
	}
	if snapshot.Routes[0].Errors != 1 {
		t.Fatalf("expected 1 error, got %d", snapshot.Routes[0].Errors)
	}
}

func TestGenerateHandlerServiceError(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"prompt": "hola"})
	req := httptest.NewRequest(http.MethodPost, "/api/generate", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler := handlers.NewGenerateHandler(fakeRAGService{err: errors.New("fallo")})
	handler.Handle(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestOpenAIChatCompletions(t *testing.T) {
	handler := handlers.NewOpenAIHandler(fakeOllamaService{}, fakeRAGService{result: "respuesta"}, nil, nil)
	body, _ := json.Marshal(map[string]any{
		"model":    "local-rag",
		"messages": []domain.Message{{Role: "user", Content: "hola"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var response map[string]any
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if response["object"] != "chat.completion" {
		t.Fatalf("expected chat.completion object, got %v", response["object"])
	}
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("unexpected choices payload: %#v", response["choices"])
	}
}

func TestOpenAIChatCompletionsStream(t *testing.T) {
	handler := handlers.NewOpenAIHandler(fakeOllamaService{}, fakeRAGService{result: "respuesta"}, nil, nil)
	body, _ := json.Marshal(map[string]any{
		"model":    "local-rag",
		"stream":   true,
		"messages": []domain.Message{{Role: "user", Content: "hola"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.ChatCompletions(w, req)

	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}
	payload := w.Body.String()
	if !strings.Contains(payload, "chat.completion.chunk") {
		t.Fatalf("expected streaming chunk payload, got %q", payload)
	}
	if !strings.Contains(payload, "[DONE]") {
		t.Fatalf("expected [DONE] marker, got %q", payload)
	}
}
