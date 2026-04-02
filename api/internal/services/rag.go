package services

import (
	"log/slog"

	"ollama-gateway/internal/domain"
	"ollama-gateway/pkg/cache"
)

type RAGService struct {
	ollamaService *OllamaService
	routerService *RouterService
	qdrantService *QdrantService
	logger        *slog.Logger
	cache         cache.Cache
}

var _ domain.RAGEngine = (*RAGService)(nil)

func NewRAGService(ollamaService *OllamaService, routerService *RouterService, qdrantService *QdrantService, logger *slog.Logger, cacheBackend cache.Cache) *RAGService {
	if logger == nil {
		logger = slog.Default()
	}
	return &RAGService{
		ollamaService: ollamaService,
		routerService: routerService,
		qdrantService: qdrantService,
		logger:        logger,
		cache:         cacheBackend,
	}
}

func (s *RAGService) search(query string) string {
	embedding, err := s.ollamaService.GetEmbedding("nomic-embed-text", query)
	if err != nil || len(embedding) == 0 {
		return ""
	}
	if s.qdrantService == nil {
		return ""
	}
	result, err := s.qdrantService.Search("repo_docs", embedding, 2)
	if err != nil {
		return ""
	}

	return s.extractCode(result)
}

func (s *RAGService) extractCode(result map[string]interface{}) string {
	res, ok := result["result"].([]interface{})
	if !ok {
		return ""
	}

	context := ""
	for _, h := range res {
		item, _ := h.(map[string]interface{})
		payload, _ := item["payload"].(map[string]interface{})
		code, _ := payload["code"].(string)
		context += "\n---\n" + code
	}
	return context
}

func (s *RAGService) GenerateWithContext(prompt string) (string, error) {
	model := s.routerService.SelectModel(prompt)
	fullPrompt := s.buildPrompt(prompt)

	resp, err := s.ollamaService.Generate(model, fullPrompt)
	if err != nil {
		return s.ollamaService.Generate("gemma:2b", prompt)
	}

	return resp, nil
}

func (s *RAGService) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	model := s.routerService.SelectModel(prompt)
	fullPrompt := s.buildPrompt(prompt)
	if err := s.ollamaService.StreamGenerate(model, fullPrompt, onChunk); err != nil {
		return s.ollamaService.StreamGenerate("gemma:2b", prompt, onChunk)
	}
	return nil
}

func (s *RAGService) buildPrompt(prompt string) string {
	context := s.search(prompt)
	if context != "" {
		return "Eres un experto en Go. Usa este contexto: " + context + "\n\nPregunta: " + prompt
	}
	return "Eres un experto en Go.\n\nPregunta: " + prompt
}
