package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeCommitGenService struct {
	messageFn func(diff string) (string, error)
	stagedFn  func(repoRoot string) (string, error)
}

func (f *fakeCommitGenService) GenerateMessage(diff string) (string, error) {
	if f.messageFn != nil {
		return f.messageFn(diff)
	}
	return "", nil
}

func (f *fakeCommitGenService) GenerateFromStaged(repoRoot string) (string, error) {
	if f.stagedFn != nil {
		return f.stagedFn(repoRoot)
	}
	return "", nil
}

func TestCommitGenHandlerMessageOK(t *testing.T) {
	h := NewCommitGenHandler(&fakeCommitGenService{messageFn: func(diff string) (string, error) {
		if !strings.Contains(diff, "diff --git") {
			t.Fatalf("expected diff payload")
		}
		return "feat(api): add endpoint", nil
	}})

	req := httptest.NewRequest(http.MethodPost, "/api/commit/message", bytes.NewBufferString(`{"diff":"diff --git a b"}`))
	rr := httptest.NewRecorder()
	h.Message(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "feat(api): add endpoint") {
		t.Fatalf("expected message in response")
	}
}

func TestCommitGenHandlerMessageBadBody(t *testing.T) {
	h := NewCommitGenHandler(&fakeCommitGenService{})
	req := httptest.NewRequest(http.MethodPost, "/api/commit/message", bytes.NewBufferString(`{"diff":`))
	rr := httptest.NewRecorder()
	h.Message(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCommitGenHandlerMessageServiceError(t *testing.T) {
	h := NewCommitGenHandler(&fakeCommitGenService{messageFn: func(diff string) (string, error) {
		return "", fmt.Errorf("diff requerido")
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/commit/message", bytes.NewBufferString(`{"diff":"x"}`))
	rr := httptest.NewRecorder()
	h.Message(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestCommitGenHandlerStagedOK(t *testing.T) {
	h := NewCommitGenHandler(&fakeCommitGenService{stagedFn: func(repoRoot string) (string, error) {
		if repoRoot != "." {
			t.Fatalf("expected repo_root propagated")
		}
		return "chore(repo): update docs", nil
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/commit/staged", bytes.NewBufferString(`{"repo_root":"."}`))
	rr := httptest.NewRecorder()
	h.Staged(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "chore(repo): update docs") {
		t.Fatalf("expected staged message in response")
	}
}
