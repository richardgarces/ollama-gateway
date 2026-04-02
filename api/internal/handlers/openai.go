package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/httputil"
)

type OpenAIHandler struct {
	ollama interface {
		GetEmbedding(model, text string) ([]float64, error)
	}
	rag interface {
		GenerateWithContext(prompt string) (string, error)
		StreamGenerateWithContext(prompt string, onChunk func(string) error) error
	}
}

func NewOpenAIHandler(
	o interface {
		GetEmbedding(model, text string) ([]float64, error)
	},
	r interface {
		GenerateWithContext(prompt string) (string, error)
		StreamGenerateWithContext(prompt string, onChunk func(string) error) error
	},
) *OpenAIHandler {
	return &OpenAIHandler{ollama: o, rag: r}
}

type embeddingReq struct {
	Input string `json:"input"`
}

func (h *OpenAIHandler) Embeddings(w http.ResponseWriter, r *http.Request) {
	var req embeddingReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	emb, err := h.ollama.GetEmbedding("default", req.Input)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]interface{}{
		"data": []interface{}{map[string]interface{}{"embedding": emb}},
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

type completionReq struct {
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type openAIChatCompletion struct {
	Model    string           `json:"model"`
	Messages []domain.Message `json:"messages"`
	Stream   bool             `json:"stream"`
}

func (h *OpenAIHandler) Completions(w http.ResponseWriter, r *http.Request) {
	var req completionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Stream {
		if err := httputil.WriteSSEHeaders(w); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.rag.StreamGenerateWithContext(req.Prompt, func(chunk string) error {
			return httputil.WriteSSEData(w, map[string]interface{}{
				"id":     "cmpl-local-1",
				"object": "text_completion",
				"choices": []map[string]interface{}{{
					"text": chunk,
				}},
			})
		}); err != nil {
			return
		}
		_ = httputil.WriteSSEDone(w)
		return
	}
	out, err := h.rag.GenerateWithContext(req.Prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]interface{}{"id": "cmpl-local-1", "object": "text_completion", "choices": []interface{}{map[string]interface{}{"text": out}}}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func (h *OpenAIHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req openAIChatCompletion
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Messages) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "messages requerido")
		return
	}
	prompt := joinMessages(req.Messages)
	model := req.Model
	if model == "" {
		model = "local-rag"
	}
	id := "chatcmpl-local-1"
	created := time.Now().Unix()

	if req.Stream {
		if err := httputil.WriteSSEHeaders(w); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := httputil.WriteSSEData(w, map[string]interface{}{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]interface{}{{
				"index": 0,
				"delta": map[string]string{"role": "assistant", "content": ""},
			}},
		}); err != nil {
			return
		}
		if err := h.rag.StreamGenerateWithContext(prompt, func(chunk string) error {
			return httputil.WriteSSEData(w, map[string]interface{}{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]interface{}{{
					"index": 0,
					"delta": map[string]string{"content": chunk},
				}},
			})
		}); err != nil {
			return
		}
		_ = httputil.WriteSSEData(w, map[string]interface{}{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]interface{}{{
				"index":         0,
				"delta":         map[string]string{},
				"finish_reason": "stop",
			}},
		})
		_ = httputil.WriteSSEDone(w)
		return
	}

	out, err := h.rag.GenerateWithContext(prompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []map[string]interface{}{{
			"index": 0,
			"message": map[string]string{
				"role":    "assistant",
				"content": out,
			},
			"finish_reason": "stop",
		}},
	})
}

func joinMessages(messages []domain.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Role+": "+message.Content)
	}
	return strings.Join(parts, "\n")
}
