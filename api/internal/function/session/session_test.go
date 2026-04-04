package service

import (
	"testing"
	"time"

	"ollama-gateway/internal/function/core/domain"
)

func TestSessionCreateJoinAndMessages(t *testing.T) {
	svc := NewSessionService()
	sess, err := svc.Create("owner")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sess.ID == "" {
		t.Fatalf("expected session id")
	}

	if err := svc.Join(sess.ID, "alice"); err != nil {
		t.Fatalf("Join() error = %v", err)
	}
	if err := svc.SetParticipantRole(sess.ID, "owner", "alice", domain.SessionRoleEditor); err != nil {
		t.Fatalf("SetParticipantRole() error = %v", err)
	}
	if err := svc.AddMessage(sess.ID, "alice", domain.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	if err := svc.AddMessage(sess.ID, "owner", domain.Message{Role: "assistant", Content: "hello"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	msgs, err := svc.GetMessages(sess.ID, "alice", time.Time{})
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestSessionGetMessagesSince(t *testing.T) {
	svc := NewSessionService()
	sess, err := svc.Create("owner")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.AddMessage(sess.ID, "owner", domain.Message{Role: "user", Content: "m1"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	since := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	if err := svc.AddMessage(sess.ID, "owner", domain.Message{Role: "user", Content: "m2"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	msgs, err := svc.GetMessages(sess.ID, "owner", since)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "m2" {
		t.Fatalf("expected only m2 after since filter")
	}
}

func TestSessionViewerCannotWriteButCanRead(t *testing.T) {
	svc := NewSessionService()
	sess, err := svc.Create("owner")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Join(sess.ID, "viewer1"); err != nil {
		t.Fatalf("Join() error = %v", err)
	}

	if err := svc.AddMessage(sess.ID, "viewer1", domain.Message{Role: "user", Content: "blocked"}); err == nil {
		t.Fatalf("expected viewer write to fail")
	}

	if err := svc.AddMessage(sess.ID, "owner", domain.Message{Role: "user", Content: "ok"}); err != nil {
		t.Fatalf("owner AddMessage() error = %v", err)
	}

	msgs, err := svc.GetMessages(sess.ID, "viewer1", time.Time{})
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected viewer to read 1 message, got %d", len(msgs))
	}
}

func TestSessionRoleAudit(t *testing.T) {
	svc := NewSessionService()
	sess, err := svc.Create("owner")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Join(sess.ID, "bob"); err != nil {
		t.Fatalf("Join() error = %v", err)
	}
	if err := svc.SetParticipantRole(sess.ID, "owner", "bob", domain.SessionRoleModerator); err != nil {
		t.Fatalf("SetParticipantRole() error = %v", err)
	}

	updated, err := svc.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got := updated.ParticipantRoles["bob"]; got != domain.SessionRoleModerator {
		t.Fatalf("expected bob role moderator, got %q", got)
	}
	if len(updated.RoleAudit) != 1 {
		t.Fatalf("expected 1 role audit entry, got %d", len(updated.RoleAudit))
	}
	if updated.RoleAudit[0].ActorID != "owner" {
		t.Fatalf("expected actor owner, got %q", updated.RoleAudit[0].ActorID)
	}
}
