package events

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	EventRequestCompleted = "RequestCompleted"
	EventSessionCreated   = "SessionCreated"
	EventFileIndexed      = "FileIndexed"
)

var ErrBusClosed = errors.New("event bus closed")

type Event interface {
	EventName() string
}

type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

type Handler func(context.Context, Event)

type RequestCompleted struct {
	RequestID  string    `json:"request_id"`
	Path       string    `json:"path"`
	StatusCode int       `json:"status_code"`
	DurationMS int64     `json:"duration_ms"`
	At         time.Time `json:"at"`
}

func (RequestCompleted) EventName() string { return EventRequestCompleted }

type SessionCreated struct {
	SessionID string    `json:"session_id"`
	OwnerID   string    `json:"owner_id"`
	At        time.Time `json:"at"`
}

func (SessionCreated) EventName() string { return EventSessionCreated }

type FileIndexed struct {
	Path     string    `json:"path"`
	RepoRoot string    `json:"repo_root"`
	At       time.Time `json:"at"`
}

func (FileIndexed) EventName() string { return EventFileIndexed }

type Options struct {
	BufferSize int
	Workers    int
}

type Bus struct {
	logger      *slog.Logger
	queue       chan Event
	mu          sync.RWMutex
	subscribers map[string][]Handler
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	closed      atomic.Bool
}

func NewBus(parent context.Context, opts Options, logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	if opts.BufferSize <= 0 {
		opts.BufferSize = 256
	}
	if opts.Workers <= 0 {
		opts.Workers = 2
	}
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)
	b := &Bus{
		logger:      logger,
		queue:       make(chan Event, opts.BufferSize),
		subscribers: make(map[string][]Handler),
		ctx:         ctx,
		cancel:      cancel,
	}

	for i := 0; i < opts.Workers; i++ {
		b.wg.Add(1)
		go b.worker(i)
	}

	return b
}

func (b *Bus) Publish(ctx context.Context, event Event) error {
	if b == nil {
		return ErrBusClosed
	}
	if b.closed.Load() {
		return ErrBusClosed
	}
	if event == nil {
		return errors.New("event nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-b.ctx.Done():
		return ErrBusClosed
	case <-ctx.Done():
		return ctx.Err()
	case b.queue <- event:
		return nil
	}
}

func (b *Bus) Subscribe(eventName string, handler Handler) func() {
	if b == nil || eventName == "" || handler == nil {
		return func() {}
	}

	b.mu.Lock()
	idx := len(b.subscribers[eventName])
	b.subscribers[eventName] = append(b.subscribers[eventName], handler)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		handlers := b.subscribers[eventName]
		if idx < 0 || idx >= len(handlers) || handlers[idx] == nil {
			return
		}
		handlers[idx] = nil
		b.subscribers[eventName] = handlers
	}
}

func (b *Bus) Shutdown() {
	if b == nil {
		return
	}
	b.closed.Store(true)
	b.cancel()
	b.wg.Wait()
}

func (b *Bus) worker(_ int) {
	defer b.wg.Done()
	for {
		select {
		case <-b.ctx.Done():
			return
		case ev := <-b.queue:
			if ev == nil {
				continue
			}
			b.dispatch(ev)
		}
	}
}

func (b *Bus) dispatch(event Event) {
	eventName := event.EventName()
	b.mu.RLock()
	handlers := append([]Handler(nil), b.subscribers[eventName]...)
	b.mu.RUnlock()

	for _, h := range handlers {
		if h == nil {
			continue
		}
		func(handler Handler) {
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("panic en event handler", slog.String("event", eventName))
				}
			}()
			handler(b.ctx, event)
		}(h)
	}
}
