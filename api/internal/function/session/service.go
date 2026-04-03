package service

type Service = SessionService

func NewService() *Service {
	return NewSessionService()
}
