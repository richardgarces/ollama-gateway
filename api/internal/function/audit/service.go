package audit

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	Action    string    `json:"action"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Result    string    `json:"result"`
	Reason    string    `json:"reason,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
}

type RecordInput struct {
	UserID    string
	Role      string
	Action    string
	Method    string
	Path      string
	Result    string
	Reason    string
	RequestID string
}

type Service struct {
	mu       sync.RWMutex
	events   []Event
	maxItems int
}

func NewService(maxItems int) *Service {
	if maxItems <= 0 {
		maxItems = 2000
	}
	return &Service{maxItems: maxItems, events: make([]Event, 0, maxItems)}
}

func (s *Service) Record(in RecordInput) {
	if s == nil {
		return
	}
	e := Event{
		Timestamp: time.Now().UTC(),
		UserID:    strings.TrimSpace(in.UserID),
		Role:      strings.TrimSpace(strings.ToLower(in.Role)),
		Action:    strings.TrimSpace(strings.ToLower(in.Action)),
		Method:    strings.TrimSpace(strings.ToUpper(in.Method)),
		Path:      strings.TrimSpace(in.Path),
		Result:    strings.TrimSpace(strings.ToLower(in.Result)),
		Reason:    strings.TrimSpace(in.Reason),
		RequestID: strings.TrimSpace(in.RequestID),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	if len(s.events) > s.maxItems {
		over := len(s.events) - s.maxItems
		s.events = append([]Event(nil), s.events[over:]...)
	}
}

func (s *Service) List(limit int, action string, result string) []Event {
	if s == nil {
		return nil
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	action = strings.TrimSpace(strings.ToLower(action))
	result = strings.TrimSpace(strings.ToLower(result))

	s.mu.RLock()
	defer s.mu.RUnlock()
	filtered := make([]Event, 0, len(s.events))
	for i := len(s.events) - 1; i >= 0; i-- {
		e := s.events[i]
		if action != "" && e.Action != action {
			continue
		}
		if result != "" && e.Result != result {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func WriteCSV(w io.Writer, events []Event) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{"timestamp", "user_id", "role", "action", "method", "path", "result", "reason", "request_id"}); err != nil {
		return err
	}

	sorted := make([]Event, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	for _, e := range sorted {
		if err := cw.Write([]string{
			e.Timestamp.Format(time.RFC3339),
			e.UserID,
			e.Role,
			e.Action,
			e.Method,
			e.Path,
			e.Result,
			e.Reason,
			e.RequestID,
		}); err != nil {
			return fmt.Errorf("csv write: %w", err)
		}
	}
	return cw.Error()
}
