package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type nonFlusherRW struct {
	http.ResponseWriter
}

func TestWriteJSONAndError(t *testing.T) {
	rr := httptest.NewRecorder()
	WriteJSON(rr, http.StatusCreated, map[string]string{"ok": "yes"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %s", got)
	}

	rr = httptest.NewRecorder()
	WriteError(rr, http.StatusBadRequest, "bad")
	var out APIError
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json error body: %v", err)
	}
	if out.Error != "bad" || out.Code != http.StatusBadRequest {
		t.Fatalf("unexpected error body: %+v", out)
	}
}

func TestSSEHelpers(t *testing.T) {
	rr := httptest.NewRecorder()
	if err := WriteSSEHeaders(rr); err != nil {
		t.Fatalf("WriteSSEHeaders() error = %v", err)
	}
	if rr.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream")
	}
	if err := WriteSSEData(rr, map[string]string{"a": "b"}); err != nil {
		t.Fatalf("WriteSSEData() error = %v", err)
	}
	if err := WriteSSEDone(rr); err != nil {
		t.Fatalf("WriteSSEDone() error = %v", err)
	}
	if rr.Body.Len() == 0 {
		t.Fatalf("expected SSE body")
	}
}

func TestWriteSSEHeadersRequiresFlusher(t *testing.T) {
	rr := httptest.NewRecorder()
	nf := nonFlusherRW{ResponseWriter: rr}
	if err := WriteSSEHeaders(nf); err == nil {
		t.Fatalf("expected error when ResponseWriter does not implement Flusher")
	}
}

func TestPaginationHelpers(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?page=2&page_size=999", nil)
	page, pageSize := ParsePagination(r)
	if page != 2 || pageSize != 200 {
		t.Fatalf("unexpected pagination values page=%d pageSize=%d", page, pageSize)
	}

	rr := httptest.NewRecorder()
	WritePaginatedJSON(rr, []string{"a", "b"}, 10, page, pageSize)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200")
	}
}
