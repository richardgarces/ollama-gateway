package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

type RAGService struct {
	ollamaService *OllamaService
	routerService *RouterService
	qdrantURL     string
	client        *http.Client
}

func NewRAGService(ollamaService *OllamaService, routerService *RouterService, qdrantURL string) *RAGService {
	return &RAGService{
		ollamaService: ollamaService,
		routerService: routerService,
		qdrantURL:     qdrantURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *RAGService) search(query string) string {
	embedding, err := s.ollamaService.GetEmbedding("nomic-embed-text", query)
	if err != nil || len(embedding) == 0 {
		return ""
	}

	req := map[string]interface{}{
		"vector": embedding,
		"limit":  2,
	}
	data, _ := json.Marshal(req)

	resp, err := s.client.Post(s.qdrantURL+"/collections/code/points/search", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

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
	context := s.search(prompt)

	var fullPrompt string
	if context != "" {
		fullPrompt = "Eres un experto en Go. Usa este contexto: " + context + "\n\nPregunta: " + prompt
	} else {
		fullPrompt = "Eres un experto en Go.\n\nPregunta: " + prompt
	}

	resp, err := s.ollamaService.Generate(model, fullPrompt)
	if err != nil {
		return s.ollamaService.Generate("gemma:2b", prompt)
	}

	return resp, nil
}
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

type RAGService struct {
	ollamaService *OllamaService
	routerService *RouterService
	qdrantURL     string
	client        *http.Client
}

func NewRAGService(ollamaService *OllamaService, routerService *RouterService, qdrantURL string) *RAGService {
	return &RAGService{
		ollamaService: ollamaService,
		routerService: routerService,
		qdrantURL:     qdrantURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (r *RAGService) Search(query string) string {
	embedding, err := r.ollamaService.GetEmbedding(query)
	if err != nil || len(embedding) == 0 {
		return ""
	}






















































}	return resp, nil	}		return r.ollamaService.Generate("gemma:2b", prompt)	if err != nil {	resp, err := r.ollamaService.Generate(model, fullPrompt)	}		fullPrompt = "Eres un experto en Go.\n\nPregunta: " + prompt	} else {		fullPrompt = "Eres un experto en Go. Usa este contexto: " + context + "\n\nPregunta: " + prompt	if context != "" {	var fullPrompt string	context := r.Search(prompt)	model := r.routerService.SelectModel(prompt)func (r *RAGService) GenerateWithContext(prompt string) (string, error) {}	return context	}		context += "\n---\n" + code		code, _ := payload["code"].(string)		payload, _ := item["payload"].(map[string]interface{})		item, _ := h.(map[string]interface{})	for _, h := range res {	context := ""	}		return ""	if !ok {	res, ok := result["result"].([]interface{})func (r *RAGService) extractCode(result map[string]interface{}) string {}	return r.extractCode(result)	json.NewDecoder(resp.Body).Decode(&result)	var result map[string]interface{}	defer resp.Body.Close()	}		return ""	if err != nil {	resp, err := r.client.Post(r.qdrantURL+"/collections/code/points/search", "application/json", bytes.NewBuffer(data))	data, _ := json.Marshal(req)	}		"limit":  2,		"vector": embedding,	req := map[string]interface{}{