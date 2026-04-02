package observability

import (
	"sync"
	"time"
)

type LogEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

type LogStream struct {
	mu          sync.RWMutex
	capacity    int
	buffer      []LogEvent
	subscribers map[chan LogEvent]struct{}
}

func NewLogStream(capacity int) *LogStream {
	if capacity <= 0 {
		capacity = 200
	}
	return &LogStream{
		capacity:    capacity,
		buffer:      make([]LogEvent, 0, capacity),
		subscribers: make(map[chan LogEvent]struct{}),
	}
}

func (s *LogStream) Publish(event LogEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	s.mu.Lock()
	if len(s.buffer) >= s.capacity {
		copy(s.buffer, s.buffer[1:])
		s.buffer[len(s.buffer)-1] = event
	} else {
		s.buffer = append(s.buffer, event)
	}
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *LogStream) Recent(limit int) []LogEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.buffer) {
		limit = len(s.buffer)
	}
	start := len(s.buffer) - limit
	out := make([]LogEvent, limit)
	copy(out, s.buffer[start:])
	return out
}

func (s *LogStream) Subscribe() (<-chan LogEvent, func()) {
	ch := make(chan LogEvent, 32)

	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.mu.Unlock()
	}

	return ch, unsubscribe
}
