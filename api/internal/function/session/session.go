package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ollama-gateway/internal/function/core/domain"
	eventservice "ollama-gateway/internal/function/events"
)

type SessionService struct {
	sessions sync.Map
	counter  uint64
	events   eventservice.Publisher
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

func (s *SessionService) SetEventPublisher(p eventservice.Publisher) {
	if s == nil {
		return
	}
	s.events = p
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
			ID:               id,
			OwnerID:          ownerID,
			Participants:     []string{ownerID},
			ParticipantRoles: map[string]string{ownerID: domain.SessionRoleOwner},
			Messages:         []domain.Message{},
			CreatedAt:        now,
			IsActive:         true,
		},
		feed: make([]sessionMessage, 0, 8),
	}
	s.sessions.Store(id, rec)
	if s.events != nil {
		_ = s.events.Publish(context.Background(), eventservice.SessionCreated{
			SessionID: id,
			OwnerID:   ownerID,
			At:        now,
		})
	}

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
	if rec.session.ParticipantRoles == nil {
		rec.session.ParticipantRoles = make(map[string]string)
	}
	if _, ok := rec.session.ParticipantRoles[userID]; !ok {
		rec.session.ParticipantRoles[userID] = domain.SessionRoleViewer
	}
	return nil
}

func (s *SessionService) AddMessage(sessionID, userID string, msg domain.Message) error {
	sessionID = strings.TrimSpace(sessionID)
	userID = strings.TrimSpace(userID)
	if sessionID == "" {
		return fmt.Errorf("session_id requerido")
	}
	if userID == "" {
		return fmt.Errorf("user_id requerido")
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
	if !canWrite(rec.session, userID) {
		return fmt.Errorf("permiso denegado: rol sin permiso de escritura")
	}
	rec.feed = append(rec.feed, sessionMessage{Message: msg, CreatedAt: now})
	rec.session.Messages = append(rec.session.Messages, msg)
	return nil
}

func (s *SessionService) GetMessages(sessionID, userID string, since time.Time) ([]domain.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	userID = strings.TrimSpace(userID)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id requerido")
	}
	if userID == "" {
		return nil, fmt.Errorf("user_id requerido")
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
	if !canRead(rec.session, userID) {
		return nil, fmt.Errorf("permiso denegado: usuario no participante")
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

func (s *SessionService) SetParticipantRole(sessionID, actorID, targetUserID, role string) error {
	sessionID = strings.TrimSpace(sessionID)
	actorID = strings.TrimSpace(actorID)
	targetUserID = strings.TrimSpace(targetUserID)
	role = normalizeRole(role)
	if sessionID == "" || actorID == "" || targetUserID == "" {
		return fmt.Errorf("session_id, actor_id y target_user requeridos")
	}
	if !isValidRole(role) {
		return fmt.Errorf("rol inválido")
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
	if !contains(rec.session.Participants, actorID) {
		return fmt.Errorf("permiso denegado: actor no participante")
	}
	actorRole := roleFor(rec.session, actorID)
	if actorRole != domain.SessionRoleOwner && actorRole != domain.SessionRoleModerator {
		return fmt.Errorf("permiso denegado: solo owner/moderator puede cambiar roles")
	}
	if !contains(rec.session.Participants, targetUserID) {
		return fmt.Errorf("participante no encontrado")
	}
	if strings.EqualFold(targetUserID, rec.session.OwnerID) {
		return fmt.Errorf("no se puede cambiar el rol del owner")
	}

	if rec.session.ParticipantRoles == nil {
		rec.session.ParticipantRoles = make(map[string]string)
	}
	oldRole := roleFor(rec.session, targetUserID)
	rec.session.ParticipantRoles[targetUserID] = role
	rec.session.RoleAudit = append(rec.session.RoleAudit, domain.RoleAuditEntry{
		ActorID:    actorID,
		TargetUser: targetUserID,
		OldRole:    oldRole,
		NewRole:    role,
		ChangedAt:  time.Now().UTC(),
	})
	return nil
}

func (s *SessionService) GetSession(sessionID string) (*domain.ChatSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id requerido")
	}
	rec, err := s.get(sessionID)
	if err != nil {
		return nil, err
	}
	clone := rec.cloneSession()
	return &clone, nil
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
		ID:               r.session.ID,
		OwnerID:          r.session.OwnerID,
		Participants:     append([]string(nil), r.session.Participants...),
		ParticipantRoles: cloneRoles(r.session.ParticipantRoles),
		RoleAudit:        append([]domain.RoleAuditEntry(nil), r.session.RoleAudit...),
		Messages:         append([]domain.Message(nil), r.session.Messages...),
		CreatedAt:        r.session.CreatedAt,
		IsActive:         r.session.IsActive,
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

func cloneRoles(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func isValidRole(role string) bool {
	switch normalizeRole(role) {
	case domain.SessionRoleOwner, domain.SessionRoleEditor, domain.SessionRoleViewer, domain.SessionRoleModerator:
		return true
	default:
		return false
	}
}

func roleFor(session domain.ChatSession, userID string) string {
	if strings.EqualFold(userID, session.OwnerID) {
		return domain.SessionRoleOwner
	}
	if session.ParticipantRoles == nil {
		return domain.SessionRoleViewer
	}
	if role, ok := session.ParticipantRoles[userID]; ok {
		return normalizeRole(role)
	}
	return domain.SessionRoleViewer
}

func canRead(session domain.ChatSession, userID string) bool {
	return contains(session.Participants, userID)
}

func canWrite(session domain.ChatSession, userID string) bool {
	if !contains(session.Participants, userID) {
		return false
	}
	role := roleFor(session, userID)
	return role == domain.SessionRoleOwner || role == domain.SessionRoleEditor || role == domain.SessionRoleModerator
}
