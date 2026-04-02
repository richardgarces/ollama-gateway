package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type noFlushRecorder struct {
	h http.Header
	b strings.Builder
	c int
}

func (n *noFlushRecorder) Header() http.Header {
	if n.h == nil {
		n.h = make(http.Header)
	}
	return n.h
}

func (n *noFlushRecorder) Write(p []byte) (int, error) { return n.b.Write(p) }
func (n *noFlushRecorder) WriteHeader(code int)        { n.c = code }

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusCreated, map[string]string{"ok": "yes"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected json content type")
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteError(rr, http.StatusBadRequest, "bad request")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "bad request") {
		t.Fatalf("expected body with error")
	}
}

func TestWriteSSEHeadersNeedsFlusher(t *testing.T) {
	n := &noFlushRecorder{}
	err := WriteSSEHeaders(n)
	if err == nil {
		t.Fatalf("expected error when flusher is missing")
	}
}

func TestWriteSSEDataAndDone(t *testing.T) {
	rr := httptest.NewRecorder()
	if err := WriteSSEData(rr, map[string]string{"x": "y"}); err != nil {
		t.Fatalf("WriteSSEData error = %v", err)
	}
	if err := WriteSSEDone(rr); err != nil {
		t.Fatalf("WriteSSEDone error = %v", err)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "data: {") {
		t.Fatalf("expected SSE data line")
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected SSE done line")
	}
}
