package service

import "log/slog"

type Service = TechDebtService

func NewService(repoRoot string, logger *slog.Logger) *Service {
	return NewTechDebtService(repoRoot, logger)
}
