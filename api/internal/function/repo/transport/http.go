package transport

import (
	reposvc "ollama-gateway/internal/function/repo"
)

type Handler = RepoHandler

func NewHandler(repoService *reposvc.Service) *Handler {
	return NewRepoHandler(repoService)
}
