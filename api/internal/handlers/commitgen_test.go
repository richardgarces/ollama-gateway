package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeCommitGenService struct {
	msg       string
	errMsg    error
	errStaged error
}

func (f *fakeCommitGenService) GenerateMessage(diff string) (string, error) {
	if f.errMsg != nil {
		return "", f.errMsg
	}
	return f.msg, nil
}

func (f *fakeCommitGenService) GenerateFromStaged(repoRoot string) (string, error) {
	if f.errStaged != nil {
		return "", f.errStaged
	}
	return f.msg, nil
}

func TestCommitGenHandlerMessage(t *testing.T) {
	h := NewCommitGenHandler(&fakeCommitGenService{msg: "feat(api): add x"})
	body := []byte(`{"diff":"diff --git a b"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/commit/message", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Message(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["message"] == "" {
		t.Fatalf("expected message in response")
	}
}

func TestCommitGenHandlerMessageBadRequest(t *testing.T) {
	h := NewCommitGenHandler(&fakeCommitGenService{msg: "ok"})
	r := httptest.NewRequest(http.MethodPost, "/api/commit/message", bytes.NewReader([]byte(`{`)))
	w := httptest.NewRecorder()
	h.Message(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid body")
	}
}

func TestCommitGenHandlerStagedErrorMapping(t *testing.T) {
	h := NewCommitGenHandler(&fakeCommitGenService{errStaged: errors.New("no hay cambios staged")})
	r := httptest.NewRequest(http.MethodPost, "/api/commit/staged", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.Staged(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for staged service error")
	}
}
