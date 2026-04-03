package transport

import (
	reviewsvc "ollama-gateway/internal/function/review"
)

type Handler = ReviewHandler

func NewHandler(reviewService *reviewsvc.Service) *Handler {
	return NewReviewHandler(reviewService)
}
