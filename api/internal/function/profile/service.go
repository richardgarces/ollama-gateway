package service

import (
	"log/slog"
)

type Service = ProfileService

func NewMongoService(mongoURI string, logger *slog.Logger) (*Service, error) {
	return NewProfileService(mongoURI, logger)
}

func NewMongoServiceWithPool(mongoURI string, maxOpen, maxIdle, timeoutSeconds int, logger *slog.Logger) (*Service, error) {
	return NewProfileServiceWithPool(mongoURI, maxOpen, maxIdle, timeoutSeconds, logger)
}
