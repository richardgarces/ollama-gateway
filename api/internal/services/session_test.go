package services

import (
	"testing"
	"time"

	"ollama-gateway/internal/domain"
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
	if err := svc.AddMessage(sess.ID, domain.Message{Role: "user", Content: "hi"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	if err := svc.AddMessage(sess.ID, domain.Message{Role: "assistant", Content: "hello"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	msgs, err := svc.GetMessages(sess.ID, time.Time{})
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

	if err := svc.AddMessage(sess.ID, domain.Message{Role: "user", Content: "m1"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}
	since := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	if err := svc.AddMessage(sess.ID, domain.Message{Role: "user", Content: "m2"}); err != nil {
		t.Fatalf("AddMessage() error = %v", err)
	}

	msgs, err := svc.GetMessages(sess.ID, since)
	if err != nil {
		t.Fatalf("GetMessages() error = %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "m2" {
		t.Fatalf("expected only m2 after since filter")
	}
}
