package transport

import onboardingservice "ollama-gateway/internal/function/onboarding"

type HTTPHandler = Handler

func NewHTTPHandler(svc *onboardingservice.Service) *Handler {
	return NewHandler(svc)
}
