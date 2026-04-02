package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ollama-gateway/internal/domain"
)

type SessionService struct {
	sessions sync.Map
	counter  uint64
}

type sessionMessage struct {
	Message   domain.Message
	CreatedAt time.Time
}

type sessionRecord struct {
	mu      sync.RWMutex
	session domain.ChatSession
	feed    []sessionMessage
}

func NewSessionService() *SessionService {
	return &SessionService{}
}

func (s *SessionService) Create(ownerID string) (*domain.ChatSession, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return nil, fmt.Errorf("owner_id requerido")
	}
	now := time.Now().UTC()
	id := s.nextID()
	rec := &sessionRecord{
		session: domain.ChatSession{
			ID:           id,
			OwnerID:      ownerID,
			Participants: []string{ownerID},
			Messages:     []domain.Message{},
			CreatedAt:    now,
			IsActive:     true,
		},
		feed: make([]sessionMessage, 0, 8),
	}
	s.sessions.Store(id, rec)

	clone := rec.cloneSession()
	return &clone, nil
}

func (s *SessionService) Join(sessionID, userID string) error {
	sessionID = strings.TrimSpace(sessionID)
	userID = strings.TrimSpace(userID)
	if sessionID == "" || userID == "" {
		return fmt.Errorf("session_id y user_id requeridos")
	}
	rec, err := s.get(sessionID)
	if err != nil {
		return err
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !rec.session.IsActive {
		return fmt.Errorf("session inactiva")
	}
	if !contains(rec.session.Participants, userID) {
		rec.session.Participants = append(rec.session.Participants, userID)
		sort.Strings(rec.session.Participants)
	}
	return nil
}

func (s *SessionService) AddMessage(sessionID string, msg domain.Message) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session_id requerido")
	}
	if strings.TrimSpace(msg.Role) == "" {
		return fmt.Errorf("message role requerido")
	}
	rec, err := s.get(sessionID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if !rec.session.IsActive {
		return fmt.Errorf("session inactiva")
	}
	rec.feed = append(rec.feed, sessionMessage{Message: msg, CreatedAt: now})
	rec.session.Messages = append(rec.session.Messages, msg)
	return nil
}

func (s *SessionService) GetMessages(sessionID string, since time.Time) ([]domain.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id requerido")
	}
	rec, err := s.get(sessionID)
	if err != nil {
		return nil, err
	}

	rec.mu.RLock()
	defer rec.mu.RUnlock()
	if !rec.session.IsActive {
		return nil, fmt.Errorf("session inactiva")
	}
	if since.IsZero() {
		out := append([]domain.Message(nil), rec.session.Messages...)
		return out, nil
	}
	out := make([]domain.Message, 0)
	for _, entry := range rec.feed {
		if entry.CreatedAt.After(since) {
			out = append(out, entry.Message)
		}
	}
	return out, nil
}

func (s *SessionService) nextID() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("sess-%d-%d", time.Now().UTC().UnixNano(), n)
}

func (s *SessionService) get(sessionID string) (*sessionRecord, error) {
	v, ok := s.sessions.Load(sessionID)
	if !ok {
		return nil, fmt.Errorf("session no encontrada")
	}
	rec, ok := v.(*sessionRecord)
	if !ok || rec == nil {
		return nil, fmt.Errorf("session corrupta")
	}
	return rec, nil
}

func (r *sessionRecord) cloneSession() domain.ChatSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return domain.ChatSession{
		ID:           r.session.ID,
		OwnerID:      r.session.OwnerID,
		Participants: append([]string(nil), r.session.Participants...),
		Messages:     append([]domain.Message(nil), r.session.Messages...),
		CreatedAt:    r.session.CreatedAt,
		IsActive:     r.session.IsActive,
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(item, want) {
			return true
		}
	}
	return false
}
