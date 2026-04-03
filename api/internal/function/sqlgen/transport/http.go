package transport

import (
	sqlgensvc "ollama-gateway/internal/function/sqlgen"
)

type Handler = SQLGenHandler

func NewHandler(svc *sqlgensvc.Service) *Handler {
	return NewSQLGenHandler(svc)
}
