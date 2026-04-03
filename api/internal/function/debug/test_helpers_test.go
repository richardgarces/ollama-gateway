package service

import "ollama-gateway/internal/function/core/domain"

type fakeRAG struct {
	response string
}

func (f fakeRAG) GenerateWithContext(prompt string) (string, error) {
	return f.response, nil
}

func (f fakeRAG) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	return nil
}

var _ domain.RAGEngine = fakeRAG{}
