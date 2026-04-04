package service

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ollama-gateway/internal/function/core/domain"
)

const (
	StepStatusPending = "pending"
	StepStatusRunning = "running"
	StepStatusDone    = "done"
	StepStatusFailed  = "failed"
)

type Step struct {
	ID         string `json:"id"`
	Input      string `json:"input"`
	RetryLimit int    `json:"retry_limit,omitempty"`
	BackoffMS  int    `json:"backoff_ms,omitempty"`
}

type StepStateChange struct {
	Status     string    `json:"status"`
	At         time.Time `json:"at"`
	Attempt    int       `json:"attempt"`
	Message    string    `json:"message,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

type StepTimeline struct {
	StepID      string            `json:"step_id"`
	Input       string            `json:"input"`
	Status      string            `json:"status"`
	Attempts    int               `json:"attempts"`
	Output      string            `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	StateChange []StepStateChange `json:"state_changes"`
}

type PlanResult struct {
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	Timeline   []StepTimeline `json:"timeline"`
}

type Service struct {
	runner            domain.AgentRunner
	logger            *slog.Logger
	defaultRetryLimit int
	defaultBackoff    time.Duration
}

func NewService(runner domain.AgentRunner, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		runner:            runner,
		logger:            logger,
		defaultRetryLimit: 1,
		defaultBackoff:    350 * time.Millisecond,
	}
}

func (s *Service) ExecutePlan(steps []Step) PlanResult {
	start := time.Now().UTC()
	result := PlanResult{
		Status:    StepStatusDone,
		StartedAt: start,
		Timeline:  make([]StepTimeline, 0, len(steps)),
	}
	if len(steps) == 0 {
		result.Status = StepStatusFailed
		result.FinishedAt = time.Now().UTC()
		return result
	}

	for i, step := range steps {
		item := s.executeStep(i, step)
		result.Timeline = append(result.Timeline, item)
		if item.Status == StepStatusFailed {
			result.Status = StepStatusFailed
			break
		}
	}

	result.FinishedAt = time.Now().UTC()
	return result
}

func (s *Service) executeStep(index int, step Step) StepTimeline {
	stepID := strings.TrimSpace(step.ID)
	if stepID == "" {
		stepID = fmt.Sprintf("step-%d", index+1)
	}
	input := strings.TrimSpace(step.Input)
	now := time.Now().UTC()

	timeline := StepTimeline{
		StepID:    stepID,
		Input:     input,
		Status:    StepStatusPending,
		StartedAt: now,
		StateChange: []StepStateChange{{
			Status:  StepStatusPending,
			At:      now,
			Attempt: 0,
		}},
	}

	if input == "" {
		err := errors.New("input requerido")
		timeline.Status = StepStatusFailed
		timeline.Error = err.Error()
		timeline.FinishedAt = time.Now().UTC()
		timeline.StateChange = append(timeline.StateChange, StepStateChange{
			Status:  StepStatusFailed,
			At:      timeline.FinishedAt,
			Attempt: 0,
			Message: err.Error(),
		})
		return timeline
	}

	retryLimit := step.RetryLimit
	if retryLimit <= 0 {
		retryLimit = s.defaultRetryLimit
	}
	backoff := time.Duration(step.BackoffMS) * time.Millisecond
	if backoff <= 0 {
		backoff = s.defaultBackoff
	}

	maxAttempts := retryLimit + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		runStart := time.Now().UTC()
		timeline.Status = StepStatusRunning
		timeline.Attempts = attempt
		timeline.StateChange = append(timeline.StateChange, StepStateChange{
			Status:  StepStatusRunning,
			At:      runStart,
			Attempt: attempt,
			Message: "ejecutando step",
		})

		output := ""
		if s.runner != nil {
			output = s.runner.Run(input)
		}
		output = strings.TrimSpace(output)
		runFinished := time.Now().UTC()
		runDuration := runFinished.Sub(runStart).Milliseconds()

		if output != "" {
			timeline.Status = StepStatusDone
			timeline.Output = output
			timeline.FinishedAt = runFinished
			timeline.StateChange = append(timeline.StateChange, StepStateChange{
				Status:     StepStatusDone,
				At:         runFinished,
				Attempt:    attempt,
				Message:    "step completado",
				DurationMS: runDuration,
			})
			return timeline
		}

		errMsg := "ejecución sin salida"
		timeline.Error = errMsg
		timeline.StateChange = append(timeline.StateChange, StepStateChange{
			Status:     StepStatusFailed,
			At:         runFinished,
			Attempt:    attempt,
			Message:    errMsg,
			DurationMS: runDuration,
		})

		if attempt < maxAttempts {
			s.logger.Warn("planner retry",
				slog.String("step", stepID),
				slog.Int("attempt", attempt),
				slog.String("reason", errMsg),
			)
			time.Sleep(backoff)
			continue
		}

		timeline.Status = StepStatusFailed
		timeline.FinishedAt = runFinished
		return timeline
	}

	timeline.Status = StepStatusFailed
	timeline.FinishedAt = time.Now().UTC()
	if timeline.Error == "" {
		timeline.Error = "step failed"
	}
	return timeline
}
