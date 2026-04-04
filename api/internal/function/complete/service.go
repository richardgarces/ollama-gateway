package service

import (
	"context"
	"errors"
	"strings"

	coreservice "ollama-gateway/internal/function/core"
)

type Service struct {
	ollama       *coreservice.OllamaService
	defaultModel string
}

type CompleteInput struct {
	Model     string  `json:"model,omitempty"`
	Prefix    string  `json:"prefix"`
	Suffix    string  `json:"suffix"`
	Language  string  `json:"language,omitempty"`
	TopP      float64 `json:"top_p,omitempty"`
	Temp      float64 `json:"temperature,omitempty"`
	MaxTokens int     `json:"num_predict,omitempty"`
}

type CompleteResult struct {
	Completion string `json:"completion"`
	Prompt     string `json:"prompt"`
	Model      string `json:"model"`
}

func NewService(ollama *coreservice.OllamaService, defaultModel string) *Service {
	if strings.TrimSpace(defaultModel) == "" {
		defaultModel = "local-rag"
	}
	return &Service{ollama: ollama, defaultModel: defaultModel}
}

func (s *Service) Complete(ctx context.Context, in CompleteInput) (CompleteResult, error) {
	if s == nil || s.ollama == nil {
		return CompleteResult{}, errors.New("complete service no disponible")
	}
	prefix := strings.TrimSpace(in.Prefix)
	if prefix == "" && strings.TrimSpace(in.Suffix) == "" {
		return CompleteResult{}, errors.New("prefix o suffix requerido")
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = s.defaultModel
	}
	lang := strings.TrimSpace(in.Language)
	fimPrompt := "<PRE>" + in.Prefix + "<SUF>" + in.Suffix + "<MID>"
	if lang != "" {
		fimPrompt = "Language=" + lang + "\n" + fimPrompt
	}
	out, err := s.ollama.GenerateWithContext(ctx, model, fimPrompt)
	if err != nil {
		return CompleteResult{}, err
	}
	return CompleteResult{Completion: out, Prompt: fimPrompt, Model: model}, nil
}
