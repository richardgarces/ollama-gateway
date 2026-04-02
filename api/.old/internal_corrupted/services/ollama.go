package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ollama-gateway/internal/domain"
)

type OllamaService struct {
	baseURL string
	client  *http.Client
}

func NewOllamaService(baseURL string) *OllamaService {
	return &OllamaService{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (s *OllamaService) Generate(model, prompt string) (string, error) {
	reqBody := domain.OllamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Post(s.baseURL+"/api/generate", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result domain.OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama decode error: %w", err)
	}

	return result.Response, nil
}

func (s *OllamaService) GetEmbedding(model, text string) ([]float64, error) {
	reqBody := map[string]interface{}{
		"model": model,
		"prompt": text,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Post(s.baseURL+"/api/embeddings", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("ollama embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embedding decode error: %w", err)
	}

	return result.Embedding, nil
}


import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OllamaService struct {
	baseURL string
	client  *http.Client
}

func NewOllamaService(baseURL string) *OllamaService {
	return &OllamaService{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}

func (s *OllamaService) Generate(model, prompt string) (string, error) {
	reqBody := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := s.client.Post(s.baseURL+"/api/generate", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama decode error: %w", err)
	}

	return result.Response, nil
}

type embeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

func (s *OllamaService) GetEmbedding(text string) ([]float64, error) {
	body := map[string]string{
		"model":  "nomic-embed-text",
		"prompt": text,
	}
	data, _ := json.Marshal(body)

	resp, err := s.client.Post(s.baseURL+"/api/embeddings", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return []float64{}, err
	}
	defer resp.Body.Close()

	var result embeddingResponse
	json.NewDecoder(resp.Body).Decode(&result)

	return result.Embedding, nil
}
