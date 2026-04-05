package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoStopsOnSuccess(t *testing.T) {
	attempts := 0
	err := Do(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("temp")
		}
		return nil
	}, RetryPolicy{
		MaxAttempts: 5,
		Sleep:       func(context.Context, time.Duration) bool { return true },
		RandUnit:    func() float64 { return 0.5 },
	})

	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoHonorsMaxAttempts(t *testing.T) {
	attempts := 0
	err := Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("always fail")
	}, RetryPolicy{
		MaxAttempts: 3,
		Sleep:       func(context.Context, time.Duration) bool { return true },
		RandUnit:    func() float64 { return 0.5 },
	})

	if err == nil {
		t.Fatalf("expected error after max attempts")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoStopsOnNonRetriableError(t *testing.T) {
	attempts := 0
	err := Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return MarkNonRetriable(errors.New("bad request"))
	}, RetryPolicy{
		MaxAttempts: 5,
		Sleep:       func(context.Context, time.Duration) bool { return true },
		RandUnit:    func() float64 { return 0.5 },
	})

	if err == nil {
		t.Fatalf("expected non-retriable error")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for non-retriable error, got %d", attempts)
	}
}

func TestDoBackoffAndJitterWindow(t *testing.T) {
	delays := make([]time.Duration, 0, 3)
	attempts := 0
	err := Do(context.Background(), func(ctx context.Context) error {
		attempts++
		return errors.New("temp")
	}, RetryPolicy{
		MaxAttempts: 4,
		BaseBackoff: 100 * time.Millisecond,
		MaxBackoff:  300 * time.Millisecond,
		JitterRatio: 0.2,
		RandUnit:    func() float64 { return 1.0 },
		Sleep: func(ctx context.Context, d time.Duration) bool {
			delays = append(delays, d)
			return true
		},
	})

	if err == nil {
		t.Fatalf("expected error after retries")
	}
	if attempts != 4 {
		t.Fatalf("expected 4 attempts, got %d", attempts)
	}
	if len(delays) != 3 {
		t.Fatalf("expected 3 delays, got %d", len(delays))
	}

	// attempt backoff base values: 100ms, 200ms, 300ms(cap). With jitter +20% => 120, 240, 360.
	want := []time.Duration{120 * time.Millisecond, 240 * time.Millisecond, 360 * time.Millisecond}
	for i := range want {
		if delays[i] != want[i] {
			t.Fatalf("delay[%d]=%s, want %s", i, delays[i], want[i])
		}
	}
}
