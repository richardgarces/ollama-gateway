package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ollama-gateway/internal/function/core/domain"
	"ollama-gateway/internal/middleware"
	"ollama-gateway/pkg/httputil"
)

type OpenAIHandler struct {
	ollama        domain.OllamaClient
	rag           domain.RAGEngine
	conversations domain.ConversationStore
	profiles      domain.ProfileStore
}

func NewOpenAIHandler(
	o domain.OllamaClient,
	r domain.RAGEngine,
	c domain.ConversationStore,
	p domain.ProfileStore,
) *OpenAIHandler {
	return &OpenAIHandler{ollama: o, rag: r, conversations: c, profiles: p}
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
	Model          string           `json:"model"`
	Messages       []domain.Message `json:"messages"`
	Stream         bool             `json:"stream"`
	ConversationID string           `json:"conversation_id,omitempty"`
	Lang           string           `json:"lang,omitempty"`
	SystemPrompt   string           `json:"system_prompt,omitempty"`
	Temperature    *float64         `json:"temperature,omitempty"`
	MaxTokens      *int             `json:"max_tokens,omitempty"`
}

func (h *OpenAIHandler) Completions(w http.ResponseWriter, r *http.Request) {
	var req completionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Stream {
		prompt := withRequestIDPrompt(r, req.Prompt)
		if err := httputil.WriteSSEHeaders(w); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.rag.StreamGenerateWithContext(prompt, func(chunk string) error {
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
	prompt := withRequestIDPrompt(r, req.Prompt)
	out, err := h.rag.GenerateWithContext(prompt)
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
	userID := userIDFromRequest(r)
	runtimeMessages := append([]domain.Message{}, req.Messages...)
	selectedSystemPrompt := req.SystemPrompt
	if h.profiles != nil && userID != "anonymous" {
		if profile, err := h.profiles.GetByUserID(r.Context(), userID); err == nil && profile != nil {
			if req.Model == "" && profile.PreferredModel != "" {
				req.Model = profile.PreferredModel
			}
			if selectedSystemPrompt == "" && strings.TrimSpace(profile.SystemPrompt) != "" {
				selectedSystemPrompt = profile.SystemPrompt
			}
			if req.Temperature == nil && profile.Temperature > 0 {
				t := profile.Temperature
				req.Temperature = &t
			}
			if req.MaxTokens == nil && profile.MaxTokens > 0 {
				mt := profile.MaxTokens
				req.MaxTokens = &mt
			}
		}
	}
	if req.ConversationID != "" {
		if h.conversations == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "conversation store no disponible")
			return
		}
		conversation, err := h.conversations.GetByID(r.Context(), req.ConversationID, userID)
		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "conversation no encontrada")
			return
		}
		runtimeMessages = append(append([]domain.Message{}, conversation.Messages...), req.Messages...)
	}
	if selectedSystemPrompt != "" && !hasSystemMessage(runtimeMessages) {
		runtimeMessages = append([]domain.Message{{Role: "system", Content: selectedSystemPrompt}}, runtimeMessages...)
	}

	prompt := joinMessages(runtimeMessages)
	prompt = withLangDirective(prompt, req.Lang)
	routedPrompt := withRequestIDPrompt(r, prompt)
	model := req.Model
	if model == "" {
		model = "local-rag"
	}
	id := "chatcmpl-local-1"
	created := time.Now().Unix()

	if req.Stream {
		var assistantContent strings.Builder
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
		if err := h.rag.StreamGenerateWithContext(routedPrompt, func(chunk string) error {
			assistantContent.WriteString(chunk)
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

		conversationID, err := h.persistConversationTurn(r.Context(), userID, req.ConversationID, req.Messages, assistantContent.String())
		if err != nil {
			_ = httputil.WriteSSEData(w, map[string]interface{}{
				"error": "conversation persistence failed",
			})
		} else {
			req.ConversationID = conversationID
		}

		_ = httputil.WriteSSEData(w, map[string]interface{}{
			"id":              id,
			"object":          "chat.completion.chunk",
			"created":         created,
			"model":           model,
			"conversation_id": req.ConversationID,
			"choices": []map[string]interface{}{{
				"index":         0,
				"delta":         map[string]string{},
				"finish_reason": "stop",
			}},
		})
		_ = httputil.WriteSSEDone(w)
		return
	}

	out, err := h.rag.GenerateWithContext(routedPrompt)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	conversationID, err := h.persistConversationTurn(r.Context(), userID, req.ConversationID, req.Messages, out)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "no se pudo persistir conversación")
		return
	}
	req.ConversationID = conversationID

	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":              id,
		"object":          "chat.completion",
		"created":         created,
		"model":           model,
		"conversation_id": req.ConversationID,
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

func (h *OpenAIHandler) persistConversationTurn(ctx context.Context, userID, conversationID string, requestMessages []domain.Message, assistantOutput string) (string, error) {
	if h.conversations == nil {
		return conversationID, nil
	}

	turnMessages := []domain.Message{latestUserMessage(requestMessages), {Role: "assistant", Content: assistantOutput}}
	if conversationID == "" {
		created, err := h.conversations.Create(ctx, userID, turnMessages)
		if err != nil {
			return "", err
		}
		return created.ID, nil
	}

	updated, err := h.conversations.Append(ctx, conversationID, userID, turnMessages)
	if err != nil {
		return "", err
	}
	return updated.ID, nil
}

func latestUserMessage(messages []domain.Message) domain.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			return messages[i]
		}
	}
	if len(messages) == 0 {
		return domain.Message{Role: "user", Content: ""}
	}
	return messages[len(messages)-1]
}

func userIDFromRequest(r *http.Request) string {
	if r == nil {
		return "anonymous"
	}
	if userID := strings.TrimSpace(middleware.UserIDFromContext(r.Context())); userID != "" {
		return userID
	}
	return "anonymous"
}

func joinMessages(messages []domain.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Role+": "+message.Content)
	}
	return strings.Join(parts, "\n")
}

func hasSystemMessage(messages []domain.Message) bool {
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			return true
		}
	}
	return false
}

func withLangDirective(prompt, lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return prompt
	}
	return "[prompt_lang=" + lang + "]\n" + prompt
}
