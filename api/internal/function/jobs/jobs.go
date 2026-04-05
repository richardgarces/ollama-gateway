package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	archdomain "ollama-gateway/internal/function/core/domain"
)

const (
	JobTypeSecurityScanRepo JobType = "security.scan_repo"
	JobTypeDocsReadme       JobType = "docs.readme"
	JobTypeArchitectAnalyze JobType = "architect.analyze"
	JobTypeIndexerReindex   JobType = "indexer.reindex"

	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCanceled  JobStatus = "canceled"
)

var (
	ErrJobNotFound      = errors.New("job no encontrado")
	ErrJobResultPending = errors.New("resultado aún no disponible")
	ErrJobCanceled      = errors.New("job cancelado")
)

type JobType string

type JobStatus string

type SecurityScanner interface {
	ScanRepo() (archdomain.SecurityReport, error)
}

type ReadmeGenerator interface {
	GenerateREADME(repoRoot string) (string, error)
	WriteWithBackup(path string, content string) (string, error)
}

type ArchitectureAnalyzer interface {
	AnalyzeProject() (archdomain.ArchReport, error)
}

type Indexer interface {
	IndexRepo() error
}

type Dependencies struct {
	Security  SecurityScanner
	DocGen    ReadmeGenerator
	Architect ArchitectureAnalyzer
	Indexer   Indexer
	Logger    *slog.Logger
	Workers   int
	QueueSize int
}

type CreateInput struct {
	Type        JobType                `json:"type"`
	Params      map[string]interface{} `json:"params,omitempty"`
	RequestedBy string                 `json:"requested_by,omitempty"`
}

type Job struct {
	ID           string                 `json:"id"`
	Type         JobType                `json:"type"`
	Status       JobStatus              `json:"status"`
	Params       map[string]interface{} `json:"params,omitempty"`
	RequestedBy  string                 `json:"requested_by,omitempty"`
	Error        string                 `json:"error,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	FinishedAt   *time.Time             `json:"finished_at,omitempty"`
	CancelReason string                 `json:"cancel_reason,omitempty"`
}

type jobState struct {
	job    Job
	result interface{}
	cancel context.CancelFunc
}

type JobsService struct {
	mu      sync.RWMutex
	deps    Dependencies
	jobs    map[string]*jobState
	queue   chan string
	wg      sync.WaitGroup
	closed  bool
	closeMu sync.Mutex
	one     sync.Once
	seq     uint64
}

func NewJobsService(deps Dependencies) *JobsService {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Workers <= 0 {
		deps.Workers = 2
	}
	if deps.QueueSize <= 0 {
		deps.QueueSize = 256
	}

	s := &JobsService{
		deps:  deps,
		jobs:  make(map[string]*jobState, deps.QueueSize),
		queue: make(chan string, deps.QueueSize),
	}
	for i := 0; i < deps.Workers; i++ {
		s.wg.Add(1)
		go s.worker(i + 1)
	}
	return s
}

func (s *JobsService) Create(in CreateInput) (Job, error) {
	if s == nil {
		return Job{}, fmt.Errorf("jobs service no disponible")
	}

	jobType := normalizeType(in.Type)
	if !isSupportedType(jobType) {
		return Job{}, fmt.Errorf("tipo de job no soportado: %s", in.Type)
	}

	now := time.Now().UTC()
	job := Job{
		ID:          s.nextID(),
		Type:        jobType,
		Status:      JobStatusQueued,
		Params:      copyParams(in.Params),
		RequestedBy: strings.TrimSpace(in.RequestedBy),
		CreatedAt:   now,
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Job{}, fmt.Errorf("jobs service cerrada")
	}
	s.jobs[job.ID] = &jobState{job: job}
	s.mu.Unlock()

	select {
	case s.queue <- job.ID:
		return job, nil
	default:
		s.mu.Lock()
		delete(s.jobs, job.ID)
		s.mu.Unlock()
		return Job{}, fmt.Errorf("cola de jobs llena")
	}
}

func (s *JobsService) Get(jobID string) (Job, error) {
	if s == nil {
		return Job{}, ErrJobNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	return cloneJob(state.job), nil
}

func (s *JobsService) GetResult(jobID string) (interface{}, JobStatus, error) {
	if s == nil {
		return nil, "", ErrJobNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok {
		return nil, "", ErrJobNotFound
	}
	switch state.job.Status {
	case JobStatusSucceeded:
		return state.result, state.job.Status, nil
	case JobStatusFailed:
		if state.job.Error == "" {
			return nil, state.job.Status, fmt.Errorf("job fallido")
		}
		return nil, state.job.Status, errors.New(state.job.Error)
	case JobStatusCanceled:
		return nil, state.job.Status, ErrJobCanceled
	default:
		return nil, state.job.Status, ErrJobResultPending
	}
}

func (s *JobsService) Cancel(jobID string) (Job, error) {
	if s == nil {
		return Job{}, ErrJobNotFound
	}
	trimmedID := strings.TrimSpace(jobID)
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.jobs[trimmedID]
	if !ok {
		return Job{}, ErrJobNotFound
	}
	if state.job.Status == JobStatusSucceeded || state.job.Status == JobStatusFailed || state.job.Status == JobStatusCanceled {
		return cloneJob(state.job), nil
	}
	now := time.Now().UTC()
	state.job.Status = JobStatusCanceled
	state.job.FinishedAt = &now
	if state.job.StartedAt != nil {
		state.job.CancelReason = "cancellation_requested"
		if state.cancel != nil {
			state.cancel()
		}
	} else {
		state.job.CancelReason = "canceled_before_start"
	}
	return cloneJob(state.job), nil
}

func (s *JobsService) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.one.Do(func() {
		s.closeMu.Lock()
		s.closed = true
		close(s.queue)
		s.closeMu.Unlock()
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *JobsService) worker(workerID int) {
	defer s.wg.Done()
	for jobID := range s.queue {
		s.execute(workerID, jobID)
	}
}

func (s *JobsService) execute(workerID int, jobID string) {
	s.mu.Lock()
	state, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return
	}
	if state.job.Status != JobStatusQueued {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()
	state.job.Status = JobStatusRunning
	state.job.StartedAt = &now
	state.cancel = cancel
	job := cloneJob(state.job)
	s.mu.Unlock()

	s.deps.Logger.Info("job started",
		slog.String("job_id", job.ID),
		slog.String("job_type", string(job.Type)),
		slog.Int("worker", workerID),
	)

	result, err := s.runJob(ctx, job)
	finishedAt := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok = s.jobs[jobID]
	if !ok {
		cancel()
		return
	}
	state.cancel = nil
	if state.job.Status == JobStatusCanceled {
		if state.job.FinishedAt == nil {
			state.job.FinishedAt = &finishedAt
		}
		cancel()
		return
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, ErrJobCanceled) {
			state.job.Status = JobStatusCanceled
			if state.job.CancelReason == "" {
				state.job.CancelReason = "canceled_during_execution"
			}
		} else {
			state.job.Status = JobStatusFailed
			state.job.Error = err.Error()
		}
	} else {
		state.job.Status = JobStatusSucceeded
		state.result = result
	}
	state.job.FinishedAt = &finishedAt
	cancel()
}

func (s *JobsService) runJob(ctx context.Context, job Job) (interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	switch job.Type {
	case JobTypeSecurityScanRepo:
		if s.deps.Security == nil {
			return nil, fmt.Errorf("security service no disponible")
		}
		return s.deps.Security.ScanRepo()
	case JobTypeDocsReadme:
		if s.deps.DocGen == nil {
			return nil, fmt.Errorf("docgen service no disponible")
		}
		repoRoot := asString(job.Params["repo_root"])
		apply := asBool(job.Params["apply"])
		path := strings.TrimSpace(asString(job.Params["path"]))
		if path == "" {
			path = "README.md"
		}

		content, err := s.deps.DocGen.GenerateREADME(repoRoot)
		if err != nil {
			return nil, err
		}
		result := map[string]interface{}{"content": content, "applied": false}
		if apply {
			backupPath, err := s.deps.DocGen.WriteWithBackup(path, content)
			if err != nil {
				return nil, err
			}
			result["applied"] = true
			result["path"] = path
			result["backup_path"] = backupPath
		}
		return result, nil
	case JobTypeArchitectAnalyze:
		if s.deps.Architect == nil {
			return nil, fmt.Errorf("architect service no disponible")
		}
		return s.deps.Architect.AnalyzeProject()
	case JobTypeIndexerReindex:
		if s.deps.Indexer == nil {
			return nil, fmt.Errorf("indexer service no disponible")
		}
		if err := s.deps.Indexer.IndexRepo(); err != nil {
			return nil, err
		}
		return map[string]string{"status": "reindex_completed"}, nil
	default:
		return nil, fmt.Errorf("tipo de job no soportado: %s", job.Type)
	}
}

func (s *JobsService) nextID() string {
	n := atomic.AddUint64(&s.seq, 1)
	return fmt.Sprintf("job-%d-%d", time.Now().UTC().UnixMilli(), n)
}

func normalizeType(v JobType) JobType {
	return JobType(strings.TrimSpace(strings.ToLower(string(v))))
}

func isSupportedType(t JobType) bool {
	switch t {
	case JobTypeSecurityScanRepo, JobTypeDocsReadme, JobTypeArchitectAnalyze, JobTypeIndexerReindex:
		return true
	default:
		return false
	}
}

func copyParams(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneJob(job Job) Job {
	copy := job
	copy.Params = copyParams(job.Params)
	return copy
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func asBool(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(strings.ToLower(t)))
		if err == nil {
			return parsed
		}
		lower := strings.TrimSpace(strings.ToLower(t))
		return lower == "1" || lower == "yes" || lower == "on"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}
