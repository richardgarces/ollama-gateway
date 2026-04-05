package resilience

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

type RetryPolicy struct {
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	JitterRatio float64

	RetryIf  func(error) bool
	OnRetry  func(attempt int, err error, nextDelay time.Duration)
	Sleep    func(context.Context, time.Duration) bool
	RandUnit func() float64
}

type nonRetriableError struct {
	err error
}

func (e nonRetriableError) Error() string {
	if e.err == nil {
		return "non-retriable error"
	}
	return e.err.Error()
}

func (e nonRetriableError) Unwrap() error {
	return e.err
}

// MarkNonRetriable wraps an error to indicate retry must stop immediately.
func MarkNonRetriable(err error) error {
	if err == nil {
		return nil
	}
	return nonRetriableError{err: err}
}

// IsRetriableError classifies whether a failure can be retried safely.
func IsRetriableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var nre nonRetriableError
	if errors.As(err, &nre) {
		return false
	}
	return true
}

// Do executes an operation with exponential backoff + jitter retries.
func Do(ctx context.Context, op func(context.Context) error, policy RetryPolicy) error {
	if op == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	policy = normalizePolicy(policy)

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		retriable := policy.RetryIf(err)
		if !retriable || attempt >= policy.MaxAttempts {
			return err
		}

		delay := backoffWithJitter(attempt, policy)
		if policy.OnRetry != nil {
			policy.OnRetry(attempt, err, delay)
		}
		if !policy.Sleep(ctx, delay) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
	}

	return lastErr
}

func normalizePolicy(policy RetryPolicy) RetryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 3
	}
	if policy.BaseBackoff <= 0 {
		policy.BaseBackoff = 200 * time.Millisecond
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = 2 * time.Second
	}
	if policy.MaxBackoff < policy.BaseBackoff {
		policy.MaxBackoff = policy.BaseBackoff
	}
	if policy.JitterRatio < 0 {
		policy.JitterRatio = 0
	}
	if policy.JitterRatio > 1 {
		policy.JitterRatio = 1
	}
	if policy.RetryIf == nil {
		policy.RetryIf = IsRetriableError
	}
	if policy.Sleep == nil {
		policy.Sleep = sleepWithContext
	}
	if policy.RandUnit == nil {
		policy.RandUnit = rand.Float64
	}
	return policy
}

func backoffWithJitter(attempt int, policy RetryPolicy) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	backoff := policy.BaseBackoff << (attempt - 1)
	if backoff > policy.MaxBackoff {
		backoff = policy.MaxBackoff
	}
	if policy.JitterRatio <= 0 || backoff <= 0 {
		return backoff
	}

	base := float64(backoff)
	jitterWindow := base * policy.JitterRatio
	r := policy.RandUnit()
	if r < 0 {
		r = 0
	}
	if r > 1 {
		r = 1
	}

	shift := (2*r - 1) * jitterWindow
	withJitter := base + shift
	if withJitter < 0 {
		withJitter = 0
	}
	return time.Duration(math.Round(withJitter))
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
