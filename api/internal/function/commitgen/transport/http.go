package transport

import (
	commitgensvc "ollama-gateway/internal/function/commitgen"
)

type Handler = CommitGenHandler

func NewHandler(svc *commitgensvc.Service) *Handler {
	return NewCommitGenHandler(svc)
}
