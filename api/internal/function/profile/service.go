package service

import (
	"log/slog"
)

type Service = ProfileService

func NewMongoService(mongoURI string, logger *slog.Logger) (*Service, error) {
	return NewProfileService(mongoURI, logger)
}
