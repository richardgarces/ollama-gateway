package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ollama-gateway/internal/config"
)

type RouterService struct {
	logger            *slog.Logger
	ollamaService     *OllamaService
	feedbackProvider  FeedbackScoreProvider
	modelHintProvider ModelHintProvider
	remoteAPIURL      string
	remoteAPIKey      string
	defaultChatModel  string
	httpClient        *http.Client
	categorySamples   map[string]string
	categoryModels    map[string]string
	categoryVectors   map[string][]float64
	loadOnce          sync.Once
}

type FeedbackScoreProvider interface {
	GetModelFeedbackScore(ctx context.Context, model string) (float64, error)
}

type ModelHintProvider interface {
	RecommendHintForPrompt(ctx context.Context, prompt, category string) (string, error)
}

func NewRouterService(cfg *config.Config, ollamaService *OllamaService, logger *slog.Logger) *RouterService {
	if logger == nil {
		logger = slog.Default()
	}

	remoteURL := ""
	remoteKey := ""
	chatModel := "phi3:latest"
	if cfg != nil {
		remoteURL = strings.TrimSpace(cfg.RemoteAPIURL)
		remoteKey = strings.TrimSpace(cfg.RemoteAPIKey)
		if m := strings.TrimSpace(cfg.ChatModel); m != "" {
			chatModel = m
		}
	}

	return &RouterService{
		logger:           logger,
		ollamaService:    ollamaService,
		remoteAPIURL:     remoteURL,
		remoteAPIKey:     remoteKey,
		defaultChatModel: chatModel,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		categorySamples: map[string]string{
			"code":     "Go code implementation, refactor, debugging, APIs, structs, interfaces, tests",
			"creative": "creative writing, storytelling, ideas, copywriting, tone, style",
			"analysis": "deep analysis, architecture decisions, tradeoffs, pros and cons",
			"chat":     "casual conversation, Q&A, simple assistant response",
		},
		categoryModels: map[string]string{
			"code":     chatModel,
			"creative": chatModel,
			"analysis": chatModel,
			"chat":     chatModel,
		},
		categoryVectors: make(map[string][]float64),
	}
}

func (s *RouterService) SelectModel(prompt string) string {
	model, _, _, _ := s.selectModelWithCategory(prompt)
	return model
}

func (s *RouterService) SetFeedbackProvider(provider FeedbackScoreProvider) {
	if s == nil {
		return
	}
	s.feedbackProvider = provider
}

func (s *RouterService) SetModelHintProvider(provider ModelHintProvider) {
	if s == nil {
		return
	}
	s.modelHintProvider = provider
}

func (s *RouterService) GenerateWithFallback(prompt, fullPrompt string) (string, error) {
	model, category, score, requestID := s.selectModelWithCategory(prompt)
	out, err := s.ollamaService.Generate(model, fullPrompt)
	if err == nil {
		return out, nil
	}

	s.logger.Warn("modelo local fallo; intentando fallback remoto",
		slog.String("request_id", requestID),
		slog.String("model", model),
		slog.String("category", category),
		slog.Float64("similarity", score),
		slog.String("error", err.Error()),
	)

	remoteOut, remoteErr := s.generateRemote(fullPrompt)
	if remoteErr != nil {
		return "", fmt.Errorf("local y remoto fallaron: local=%w remoto=%v", err, remoteErr)
	}
	return remoteOut, nil
}

func (s *RouterService) StreamGenerateWithFallback(prompt, fullPrompt string, onChunk func(string) error) error {
	model, category, score, requestID := s.selectModelWithCategory(prompt)
	err := s.ollamaService.StreamGenerate(model, fullPrompt, onChunk)
	if err == nil {
		return nil
	}

	s.logger.Warn("stream local fallo; intentando fallback remoto",
		slog.String("request_id", requestID),
		slog.String("model", model),
		slog.String("category", category),
		slog.Float64("similarity", score),
		slog.String("error", err.Error()),
	)

	remoteOut, remoteErr := s.generateRemote(fullPrompt)
	if remoteErr != nil {
		return fmt.Errorf("stream local y remoto fallaron: local=%w remoto=%v", err, remoteErr)
	}
	if remoteOut != "" {
		return onChunk(remoteOut)
	}
	return nil
}

func (s *RouterService) selectModelWithCategory(prompt string) (string, string, float64, string) {
	requestID := extractRequestID(prompt)

	if strings.TrimSpace(prompt) == "" {
		model := s.categoryModels["chat"]
		s.logger.Info("routing decision",
			slog.String("request_id", requestID),
			slog.String("category", "chat"),
			slog.String("model", model),
			slog.Float64("similarity", 0),
		)
		return model, "chat", 0, requestID
	}

	if s.ollamaService == nil {
		model, category := s.fallbackByLength(prompt)
		s.logger.Info("routing decision",
			slog.String("request_id", requestID),
			slog.String("category", category),
			slog.String("model", model),
			slog.String("strategy", "length_fallback"),
		)
		return model, category, 0, requestID
	}

	if err := s.ensureCategoryEmbeddings(); err != nil {
		s.logger.Warn("no se pudieron inicializar embeddings de categorias",
			slog.String("request_id", requestID),
			slog.String("error", err.Error()),
		)
		model, category := s.fallbackByLength(prompt)
		s.logger.Info("routing decision",
			slog.String("request_id", requestID),
			slog.String("category", category),
			slog.String("model", model),
			slog.String("strategy", "length_fallback"),
		)
		return model, category, 0, requestID
	}

	promptVec, err := s.ollamaService.GetEmbedding("nomic-embed-text", prompt)
	if err != nil || len(promptVec) == 0 {
		s.logger.Warn("embedding del prompt fallo; aplicando fallback",
			slog.String("request_id", requestID),
			slog.String("error", errorString(err)),
		)
		model, category := s.fallbackByLength(prompt)
		s.logger.Info("routing decision",
			slog.String("request_id", requestID),
			slog.String("category", category),
			slog.String("model", model),
			slog.String("strategy", "length_fallback"),
		)
		return model, category, 0, requestID
	}

	bestCategory := "chat"
	bestScore := -1.0
	for category, vec := range s.categoryVectors {
		score := cosineSimilarityScore(promptVec, vec)
		model := s.categoryModels[category]
		feedbackBoost := 0.0
		if s.feedbackProvider != nil && model != "" {
			if fb, err := s.feedbackProvider.GetModelFeedbackScore(context.Background(), model); err == nil {
				feedbackBoost = 0.2 * clampFeedbackScore(fb)
			}
		}
		score += feedbackBoost
		if score > bestScore {
			bestScore = score
			bestCategory = category
		}
	}

	model := s.categoryModels[bestCategory]
	if model == "" {
		model = s.defaultChatModel
	}
	if s.modelHintProvider != nil {
		if hinted, err := s.modelHintProvider.RecommendHintForPrompt(context.Background(), prompt, bestCategory); err == nil {
			hinted = strings.TrimSpace(hinted)
			if hinted != "" {
				model = hinted
			}
		}
	}

	s.logger.Info("routing decision",
		slog.String("request_id", requestID),
		slog.String("category", bestCategory),
		slog.String("model", model),
		slog.Float64("similarity", bestScore),
		slog.String("strategy", "embeddings"),
	)
	return model, bestCategory, bestScore, requestID
}

func (s *RouterService) ensureCategoryEmbeddings() error {
	var loadErr error
	s.loadOnce.Do(func() {
		for category, sample := range s.categorySamples {
			vec, err := s.ollamaService.GetEmbedding("nomic-embed-text", sample)
			if err != nil {
				loadErr = fmt.Errorf("categoria %s: %w", category, err)
				return
			}
			if len(vec) == 0 {
				loadErr = fmt.Errorf("categoria %s sin embedding", category)
				return
			}
			s.categoryVectors[category] = vec
		}
	})
	return loadErr
}

func (s *RouterService) fallbackByLength(prompt string) (model string, category string) {
	if len(prompt) > 300 {
		return s.defaultChatModel, "analysis"
	}

	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "func") || strings.Contains(lower, "refactor") || strings.Contains(lower, "golang") {
		return s.defaultChatModel, "code"
	}
	return s.defaultChatModel, "chat"
}

func (s *RouterService) generateRemote(prompt string) (string, error) {
	if s.remoteAPIURL == "" || s.remoteAPIKey == "" {
		return "", errors.New("REMOTE_API_URL/REMOTE_API_KEY no configurados")
	}

	endpoint, err := url.JoinPath(strings.TrimRight(s.remoteAPIURL, "/"), "/v1/chat/completions")
	if err != nil {
		return "", err
	}

	body := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.remoteAPIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("remote status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("respuesta remota sin choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func cosineSimilarityScore(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return -1
	}
	var dot float64
	var normA float64
	var normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return -1
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func extractRequestID(prompt string) string {
	if strings.Contains(prompt, "request_id=") {
		parts := strings.Split(prompt, "request_id=")
		if len(parts) > 1 {
			candidate := strings.Fields(parts[1])
			if len(candidate) > 0 {
				return strings.TrimSpace(strings.Trim(candidate[0], "]}"))
			}
		}
	}
	return "unknown"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func clampFeedbackScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
