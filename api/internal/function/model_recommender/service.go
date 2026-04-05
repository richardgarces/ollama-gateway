package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
)

type FeedbackProvider interface {
	GetModelFeedbackScore(ctx context.Context, model string) (float64, error)
}

type ModelProfile struct {
	Name        string
	LatencyMS   int
	MaxTokens   int
	BaseQuality float64
	CostPer1K   float64
}

type Recommendation struct {
	Model       string  `json:"model"`
	Score       float64 `json:"score"`
	Explanation string  `json:"explanation"`
}

type Service struct {
	logger           *slog.Logger
	feedbackProvider FeedbackProvider
	profiles         []ModelProfile
}

func NewService(logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		logger: logger,
		profiles: []ModelProfile{
			{Name: "phi3:latest", LatencyMS: 900, MaxTokens: 4096, BaseQuality: 0.72, CostPer1K: 0.3},
			{Name: "qwen2.5-coder:7b", LatencyMS: 1800, MaxTokens: 4096, BaseQuality: 0.82, CostPer1K: 0.6},
			{Name: "qwen2.5:7b", LatencyMS: 2100, MaxTokens: 8192, BaseQuality: 0.85, CostPer1K: 0.7},
		},
	}
}

func (s *Service) SetFeedbackProvider(provider FeedbackProvider) {
	if s == nil {
		return
	}
	s.feedbackProvider = provider
}

func (s *Service) Recommend(taskType string, slaLatencyMS, tokenBudget int) Recommendation {
	task := normalizeTaskType(taskType)
	if slaLatencyMS <= 0 {
		slaLatencyMS = defaultSLAForTask(task)
	}
	if tokenBudget <= 0 {
		tokenBudget = defaultTokenBudgetForTask(task)
	}

	qualityWeight, speedWeight, costWeight := taskWeights(task)
	best := Recommendation{Model: "phi3:latest", Score: -1}

	for _, p := range s.profiles {
		tokenFit := tokenFitScore(p.MaxTokens, tokenBudget)
		if tokenFit <= 0 {
			continue
		}
		speed := speedScore(p.LatencyMS, slaLatencyMS)
		cost := costScore(p.CostPer1K)
		feedbackBoost := s.feedbackBoost(p.Name)
		score := qualityWeight*p.BaseQuality + speedWeight*speed + costWeight*cost + 0.15*tokenFit + feedbackBoost
		if score > best.Score {
			best = Recommendation{Model: p.Name, Score: score}
		}
	}

	best.Explanation = fmt.Sprintf(
		"task=%s, sla=%dms, budget=%dtokens -> model=%s (score=%.3f)",
		task, slaLatencyMS, tokenBudget, best.Model, best.Score,
	)
	return best
}

func (s *Service) RecommendHintForPrompt(ctx context.Context, prompt, category string) (string, error) {
	task := normalizeTaskType(category)
	budget := estimatePromptTokenBudget(prompt)
	rec := s.Recommend(task, defaultSLAForTask(task), budget)
	if strings.TrimSpace(rec.Model) == "" {
		return "", nil
	}
	return rec.Model, nil
}

func normalizeTaskType(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "code", "coding", "bugfix":
		return "code"
	case "analysis", "reasoning", "architecture":
		return "analysis"
	case "creative", "writing":
		return "creative"
	default:
		return "chat"
	}
}

func taskWeights(task string) (quality, speed, cost float64) {
	switch task {
	case "code":
		return 0.48, 0.30, 0.22
	case "analysis":
		return 0.55, 0.25, 0.20
	case "creative":
		return 0.45, 0.20, 0.35
	default:
		return 0.35, 0.40, 0.25
	}
}

func defaultSLAForTask(task string) int {
	switch task {
	case "chat":
		return 1200
	case "code":
		return 2200
	case "analysis":
		return 3200
	case "creative":
		return 2800
	default:
		return 2000
	}
}

func defaultTokenBudgetForTask(task string) int {
	switch task {
	case "analysis":
		return 6000
	case "code":
		return 3500
	case "creative":
		return 4500
	default:
		return 2000
	}
}

func estimatePromptTokenBudget(prompt string) int {
	clean := strings.TrimSpace(prompt)
	if clean == "" {
		return 1200
	}
	rough := len([]rune(clean))/4 + 800
	if rough < 800 {
		return 800
	}
	if rough > 8192 {
		return 8192
	}
	return rough
}

func speedScore(latencyMS, slaMS int) float64 {
	if slaMS <= 0 {
		return 0.5
	}
	ratio := float64(latencyMS) / float64(slaMS)
	if ratio <= 1 {
		return 1 - 0.3*ratio
	}
	penalty := math.Min(0.9, (ratio-1)*0.6)
	return math.Max(0.05, 0.7-penalty)
}

func costScore(costPer1K float64) float64 {
	if costPer1K <= 0 {
		return 1
	}
	v := 1 / (1 + costPer1K)
	if v < 0.1 {
		return 0.1
	}
	return v
}

func tokenFitScore(maxTokens, budget int) float64 {
	if budget <= 0 {
		return 1
	}
	if maxTokens < budget {
		return 0
	}
	ratio := float64(budget) / float64(maxTokens)
	return math.Max(0.35, 1-ratio*0.4)
}

func (s *Service) feedbackBoost(model string) float64 {
	if s == nil || s.feedbackProvider == nil || strings.TrimSpace(model) == "" {
		return 0
	}
	v, err := s.feedbackProvider.GetModelFeedbackScore(context.Background(), model)
	if err != nil {
		return 0
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return 0.1 * v
}
