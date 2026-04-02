package handlers

import (
	"net/http"

	"ollama-gateway/internal/middleware"
)

func withRequestIDPrompt(r *http.Request, prompt string) string {
	if r == nil {
		return prompt
	}
	requestID := middleware.RequestIDFromContext(r.Context())
	if requestID == "" {
		return prompt
	}
	return "[request_id=" + requestID + "]\n" + prompt
}
