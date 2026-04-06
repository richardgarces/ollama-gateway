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
	webSearcher   WebSearcher
}

// WebSearcher is an optional interface for web search enrichment.
type WebSearcher interface {
	Enabled() bool
	SearchFormatted(ctx context.Context, query string) (string, error)
}

func NewOpenAIHandler(
	o domain.OllamaClient,
	r domain.RAGEngine,
	c domain.ConversationStore,
	p domain.ProfileStore,
) *OpenAIHandler {
	return &OpenAIHandler{ollama: o, rag: r, conversations: c, profiles: p}
}

// SetWebSearcher injects an optional web search service for auto-enrichment.
func (h *OpenAIHandler) SetWebSearcher(ws WebSearcher) {
	h.webSearcher = ws
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
	Model  string `json:"model,omitempty"`
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
		streamFn := func(chunk string) error {
			return httputil.WriteSSEData(w, map[string]interface{}{
				"id":     "cmpl-local-1",
				"object": "text_completion",
				"choices": []map[string]interface{}{{
					"text": chunk,
				}},
			})
		}
		if strings.TrimSpace(req.Model) != "" {
			if err := h.ollama.StreamGenerate(req.Model, prompt, streamFn); err != nil {
				return
			}
		} else if err := h.rag.StreamGenerateWithContext(prompt, streamFn); err != nil {
			return
		}
		_ = httputil.WriteSSEDone(w)
		return
	}
	prompt := withRequestIDPrompt(r, req.Prompt)
	var out string
	var err error
	if strings.TrimSpace(req.Model) != "" {
		out, err = h.ollama.Generate(req.Model, prompt)
	} else {
		out, err = h.rag.GenerateWithContext(prompt)
	}
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
	if req.Lang != "" && !hasSystemMessage(runtimeMessages) {
		runtimeMessages = append([]domain.Message{{Role: "system", Content: "Respond in language: " + req.Lang}}, runtimeMessages...)
	}

	// Auto-enrich with web search when the user query looks like it needs current info.
	runtimeMessages = h.maybeEnrichWithWebSearch(r.Context(), runtimeMessages)

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
		// OMITIR el chunk inicial vacío (role assistant, content "") para evitar cortar el stream en el cliente
		streamChunk := func(chunk string) error {
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
		}
		// Use Ollama /api/chat with structured messages for better quality.
		if err := h.ollama.StreamChat(model, runtimeMessages, streamChunk); err != nil {
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

	// Non-streaming: use /api/chat with structured messages.
	var outBuilder strings.Builder
	if err := h.ollama.StreamChat(model, runtimeMessages, func(chunk string) error {
		outBuilder.WriteString(chunk)
		return nil
	}); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := outBuilder.String()

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

// maybeEnrichWithWebSearch detects if the user query is likely about current
// events or factual web info, and if so, injects search results as context.
func (h *OpenAIHandler) maybeEnrichWithWebSearch(ctx context.Context, messages []domain.Message) []domain.Message {
	if h.webSearcher == nil || !h.webSearcher.Enabled() {
		return messages
	}

	userQuery := lastUserContent(messages)
	if userQuery == "" || !looksLikeWebQuery(userQuery) {
		return messages
	}

	formatted, err := h.webSearcher.SearchFormatted(ctx, userQuery)
	if err != nil || formatted == "" {
		return messages
	}

	// Insert web search context as a system message right before the last user message.
	enriched := make([]domain.Message, 0, len(messages)+1)
	for i, m := range messages {
		if i == len(messages)-1 && strings.EqualFold(m.Role, "user") {
			enriched = append(enriched, domain.Message{
				Role:    "system",
				Content: "El usuario hizo una pregunta que puede requerir información actualizada. Aquí hay resultados de búsqueda web relevantes. Úsalos como contexto para responder de forma precisa:\n\n" + formatted,
			})
		}
		enriched = append(enriched, m)
	}
	return enriched
}

func lastUserContent(messages []domain.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "user") {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

// looksLikeWebQuery heuristically determines if a query needs web information.
func looksLikeWebQuery(query string) bool {
	q := strings.ToLower(query)
	// Patterns that suggest the user wants current/web information.
	webPatterns := []string{
		"qué es ", "que es ", "what is ", "what are ",
		"quién es ", "quien es ", "who is ",
		"cuándo ", "cuando ", "when ",
		"dónde ", "donde ", "where ",
		"busca ", "buscar ", "search ",
		"noticias ", "news ",
		"última ", "ultimo ", "últimas ", "latest ",
		"actual ", "current ",
		"precio ", "price ",
		"cómo funciona ", "como funciona ", "how does ",
		"wikipedia", "google",
		"2024", "2025", "2026",
		"hoy ", "today ",
		"reciente", "recent",
	}
	for _, p := range webPatterns {
		if strings.Contains(q, p) {
			return true
		}
	}
	// Questions starting with interrogative markers.
	if strings.HasPrefix(q, "¿") || strings.HasPrefix(q, "?") {
		return true
	}
	return false
}
