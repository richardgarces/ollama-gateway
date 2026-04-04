package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half-open"
)

var ErrCircuitOpen = errors.New("circuit breaker open")

type Config struct {
	Name               string
	FailureThreshold   int
	OpenTimeout        time.Duration
	HalfOpenMaxSuccess int
}

type Snapshot struct {
	Name             string    `json:"name"`
	State            State     `json:"state"`
	Failures         int       `json:"failures"`
	ConsecutiveOK    int       `json:"consecutive_ok"`
	FailureThreshold int       `json:"failure_threshold"`
	OpenTimeoutMs    int64     `json:"open_timeout_ms"`
	OpenedAt         time.Time `json:"opened_at,omitempty"`
}

type CircuitBreaker struct {
	cfg Config

	mu            sync.Mutex
	state         State
	failures      int
	halfOpenOK    int
	openedAt      time.Time
	halfOpenInUse bool
}

func NewCircuitBreaker(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = 20 * time.Second
	}
	if cfg.HalfOpenMaxSuccess <= 0 {
		cfg.HalfOpenMaxSuccess = 1
	}
	if cfg.Name == "" {
		cfg.Name = "external-provider"
	}
	return &CircuitBreaker{cfg: cfg, state: StateClosed}
}

func (b *CircuitBreaker) Execute(ctx context.Context, op func(context.Context) error) error {
	if b == nil {
		return op(ctx)
	}
	if op == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := b.before(); err != nil {
		return err
	}

	err := op(ctx)
	if err != nil {
		b.onFailure()
		return err
	}
	b.onSuccess()
	return nil
}

func (b *CircuitBreaker) Snapshot() Snapshot {
	if b == nil {
		return Snapshot{Name: "disabled", State: StateClosed}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return Snapshot{
		Name:             b.cfg.Name,
		State:            b.state,
		Failures:         b.failures,
		ConsecutiveOK:    b.halfOpenOK,
		FailureThreshold: b.cfg.FailureThreshold,
		OpenTimeoutMs:    b.cfg.OpenTimeout.Milliseconds(),
		OpenedAt:         b.openedAt,
	}
}

func (b *CircuitBreaker) before() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().UTC()
	switch b.state {
	case StateClosed:
		return nil
	case StateOpen:
		if b.openedAt.IsZero() || now.Sub(b.openedAt) < b.cfg.OpenTimeout {
			return ErrCircuitOpen
		}
		b.state = StateHalfOpen
		b.halfOpenOK = 0
		b.halfOpenInUse = true
		return nil
	case StateHalfOpen:
		if b.halfOpenInUse {
			return ErrCircuitOpen
		}
		b.halfOpenInUse = true
		return nil
	default:
		b.state = StateClosed
		return nil
	}
}

func (b *CircuitBreaker) onSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		b.failures = 0
	case StateHalfOpen:
		b.halfOpenInUse = false
		b.halfOpenOK++
		if b.halfOpenOK >= b.cfg.HalfOpenMaxSuccess {
			b.state = StateClosed
			b.failures = 0
			b.halfOpenOK = 0
			b.openedAt = time.Time{}
		}
	}
}

func (b *CircuitBreaker) onFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		b.failures++
		if b.failures >= b.cfg.FailureThreshold {
			b.state = StateOpen
			b.openedAt = time.Now().UTC()
		}
	case StateHalfOpen:
		b.state = StateOpen
		b.halfOpenInUse = false
		b.halfOpenOK = 0
		b.openedAt = time.Now().UTC()
		b.failures = b.cfg.FailureThreshold
	case StateOpen:
		b.openedAt = time.Now().UTC()
	}
}
