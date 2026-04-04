package transport

import (
	guardrailsservice "ollama-gateway/internal/function/guardrails"
	patchsvc "ollama-gateway/internal/function/patch"
)

type Handler = PatchHandler

func NewHandler(repoRoot string, patchService *patchsvc.Service, guardrails *guardrailsservice.Service) *Handler {
	return NewPatchHandler(repoRoot, patchService, guardrails)
}
