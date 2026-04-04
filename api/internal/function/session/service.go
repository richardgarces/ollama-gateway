package service

import eventservice "ollama-gateway/internal/function/events"

type Service = SessionService

func NewService(events eventservice.Publisher) *Service {
	svc := NewSessionService()
	svc.SetEventPublisher(events)
	return svc
}
