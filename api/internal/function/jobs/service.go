package service

type Service = JobsService

func NewService(deps Dependencies) *Service {
	return NewJobsService(deps)
}
