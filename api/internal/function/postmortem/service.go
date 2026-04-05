package postmortem

import "log/slog"

type Service = PostmortemService

func NewService(logger *slog.Logger) *Service {
	return NewPostmortemService(logger)
}
