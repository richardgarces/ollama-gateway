package transport

import sqlreviewservice "ollama-gateway/internal/function/sqlreview"

type Handler = SQLReviewHandler

func NewHandler(svc *sqlreviewservice.Service) *Handler {
	return NewSQLReviewHandler(svc)
}
