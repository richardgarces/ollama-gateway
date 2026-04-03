package transport

import (
	profilesvc "ollama-gateway/internal/function/profile"
)

type Handler = ProfileHandler

func NewHandler(profiles *profilesvc.Service) *Handler {
	return NewProfileHandler(profiles)
}
