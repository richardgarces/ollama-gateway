package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	ollamaWebSearchURL    = "https://ollama.com/api/web_search"
	webSearchTimeout      = 15 * time.Second
	maxWebSearchResultLen = 4000
)

// WebSearchResult represents a single search result from the Ollama web search API.
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// WebSearchResponse represents the response from the Ollama web search API.
type WebSearchResponse struct {
	Results []WebSearchResult `json:"results"`
}

// WebSearchService calls the Ollama web search API.
type WebSearchService struct {
	apiKey string
	client *http.Client
	logger *slog.Logger
}

func NewWebSearchService(apiKey string, logger *slog.Logger) *WebSearchService {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebSearchService{
		apiKey: apiKey,
		client: &http.Client{Timeout: webSearchTimeout},
		logger: logger,
	}
}

// Enabled returns true if the API key is configured.
func (s *WebSearchService) Enabled() bool {
	return strings.TrimSpace(s.apiKey) != ""
}

// Search performs a web search query and returns the results.
func (s *WebSearchService) Search(ctx context.Context, query string) (*WebSearchResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("OLLAMA_API_KEY no configurada")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query vacío")
	}

	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, fmt.Errorf("error serializando request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaWebSearchURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error creando request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error en web search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		s.logger.Warn("web search HTTP error", slog.Int("status", resp.StatusCode), slog.String("body", string(respBody)))
		return nil, fmt.Errorf("web search HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var result WebSearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("error parseando respuesta: %w", err)
	}

	s.logger.Info("web search completado", slog.String("query", query), slog.Int("results", len(result.Results)))
	return &result, nil
}

// FormatResults converts search results into a text string suitable for LLM context.
func (s *WebSearchService) FormatResults(results *WebSearchResponse) string {
	if results == nil || len(results.Results) == 0 {
		return "No se encontraron resultados web."
	}

	var sb strings.Builder
	sb.WriteString("Resultados de búsqueda web:\n\n")
	for i, r := range results.Results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, truncate(r.Content, 500)))
		if sb.Len() > maxWebSearchResultLen {
			break
		}
	}
	return sb.String()
}

// SearchFormatted performs a search and returns formatted results in one call.
func (s *WebSearchService) SearchFormatted(ctx context.Context, query string) (string, error) {
	results, err := s.Search(ctx, query)
	if err != nil {
		return "", err
	}
	return s.FormatResults(results), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
