package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

var ragClient = &http.Client{
	Timeout: 30 * time.Second,
}

type EmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

func getEmbedding(text string) []float64 {
	body := map[string]string{
		"model":  "nomic-embed-text",
		"prompt": text,
	}
	data, _ := json.Marshal(body)

	resp, err := ragClient.Post(cfg.OllamaURL+"/api/embeddings", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return []float64{}
	}
	defer resp.Body.Close()

	var result EmbeddingResponse
	json.NewDecoder(resp.Body).Decode(&result)

	return result.Embedding
}

func search(query string) string {
	embedding := getEmbedding(query)
	if len(embedding) == 0 {
		return ""
	}

	req := map[string]interface{}{
		"vector": embedding,
		"limit":  2,
	}
	data, _ := json.Marshal(req)

	resp, err := ragClient.Post(cfg.QdrantURL+"/collections/code/points/search", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	return extractCode(result)
}

func extractCode(result map[string]interface{}) string {
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

func generateWithRAG(prompt string) (string, error) {
	model := selectModel(prompt)

	context := search(prompt)

	var fullPrompt string
	if context != "" {
		fullPrompt = "Eres un experto en Go. Usa este contexto: " + context + "\n\nPregunta: " + prompt
	} else {
		fullPrompt = "Eres un experto en Go.\n\nPregunta: " + prompt
	}

	resp, err := callOllama(model, fullPrompt)
	if err != nil {
		return callOllama("gemma:2b", prompt)
	}

	return resp, nil
}
