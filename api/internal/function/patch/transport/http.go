package transport

import (
	patchsvc "ollama-gateway/internal/function/patch"
)

type Handler = PatchHandler

func NewHandler(repoRoot string, patchService *patchsvc.Service) *Handler {
	return NewPatchHandler(repoRoot, patchService)
}
