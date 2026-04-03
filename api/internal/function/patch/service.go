package service

import (
	"log/slog"
)

type Service = PatchService

func NewService(logger *slog.Logger) *Service {
	return NewPatchService(logger)
}
