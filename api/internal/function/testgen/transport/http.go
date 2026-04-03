package transport

import (
	testgensvc "ollama-gateway/internal/function/testgen"
)

type Handler = TestGenHandler

func NewHandler(svc *testgensvc.Service) *Handler {
	return NewTestGenHandler(svc)
}
