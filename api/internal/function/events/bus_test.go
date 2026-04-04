package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

type orderedEvent struct{ N int }

func (orderedEvent) EventName() string { return EventRequestCompleted }

func TestBusDeliveryAndOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := NewBus(ctx, Options{BufferSize: 16, Workers: 1}, nil)
	defer bus.Shutdown()

	got := make([]int, 0, 3)
	var mu sync.Mutex
	done := make(chan struct{}, 1)

	bus.Subscribe(EventRequestCompleted, func(_ context.Context, e Event) {
		ev, ok := e.(orderedEvent)
		if !ok {
			return
		}
		mu.Lock()
		got = append(got, ev.N)
		if len(got) == 3 {
			done <- struct{}{}
		}
		mu.Unlock()
	})

	_ = bus.Publish(context.Background(), orderedEvent{N: 1})
	_ = bus.Publish(context.Background(), orderedEvent{N: 2})
	_ = bus.Publish(context.Background(), orderedEvent{N: 3})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout esperando entrega")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	for i, n := range []int{1, 2, 3} {
		if got[i] != n {
			t.Fatalf("expected order [1 2 3], got %v", got)
		}
	}
}

func TestBusMultiSubscriberDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := NewBus(ctx, Options{BufferSize: 8, Workers: 1}, nil)
	defer bus.Shutdown()

	c1 := make(chan struct{}, 1)
	c2 := make(chan struct{}, 1)

	bus.Subscribe(EventSessionCreated, func(_ context.Context, _ Event) { c1 <- struct{}{} })
	bus.Subscribe(EventSessionCreated, func(_ context.Context, _ Event) { c2 <- struct{}{} })

	if err := bus.Publish(context.Background(), SessionCreated{SessionID: "s1", OwnerID: "u1", At: time.Now().UTC()}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case <-c1:
	case <-time.After(2 * time.Second):
		t.Fatalf("subscriber 1 no recibio evento")
	}
	select {
	case <-c2:
	case <-time.After(2 * time.Second):
		t.Fatalf("subscriber 2 no recibio evento")
	}
}

func TestBusShutdownStopsPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	bus := NewBus(ctx, Options{BufferSize: 4, Workers: 1}, nil)
	cancel()
	bus.Shutdown()

	if err := bus.Publish(context.Background(), FileIndexed{Path: "a.go", At: time.Now().UTC()}); err == nil {
		t.Fatalf("expected publish error after shutdown")
	}
}
