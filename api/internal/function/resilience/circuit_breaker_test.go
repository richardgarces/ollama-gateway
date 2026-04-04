package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerOpensOnThreshold(t *testing.T) {
	cb := NewCircuitBreaker(Config{FailureThreshold: 2, OpenTimeout: 200 * time.Millisecond, HalfOpenMaxSuccess: 1})

	fail := func(context.Context) error { return errors.New("down") }
	if err := cb.Execute(context.Background(), fail); err == nil {
		t.Fatalf("expected first failure")
	}
	if err := cb.Execute(context.Background(), fail); err == nil {
		t.Fatalf("expected second failure")
	}

	snap := cb.Snapshot()
	if snap.State != StateOpen {
		t.Fatalf("expected open state, got %s", snap.State)
	}
	if err := cb.Execute(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreakerHalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(Config{FailureThreshold: 1, OpenTimeout: 20 * time.Millisecond, HalfOpenMaxSuccess: 1})

	_ = cb.Execute(context.Background(), func(context.Context) error { return errors.New("down") })
	time.Sleep(40 * time.Millisecond)

	if err := cb.Execute(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("expected success in half-open, got %v", err)
	}
	if state := cb.Snapshot().State; state != StateClosed {
		t.Fatalf("expected closed state, got %s", state)
	}
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(Config{FailureThreshold: 1, OpenTimeout: 20 * time.Millisecond, HalfOpenMaxSuccess: 1})

	_ = cb.Execute(context.Background(), func(context.Context) error { return errors.New("down") })
	time.Sleep(40 * time.Millisecond)

	err := cb.Execute(context.Background(), func(context.Context) error { return errors.New("still down") })
	if err == nil {
		t.Fatalf("expected operation error")
	}
	if state := cb.Snapshot().State; state != StateOpen {
		t.Fatalf("expected open state, got %s", state)
	}
}
