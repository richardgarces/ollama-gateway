package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ollama-gateway/internal/domain"
)

func TestAPIExplorerHandler(t *testing.T) {
	routes := []domain.RouteDefinition{{Method: "GET", Path: "/x", Description: "demo"}}
	h := NewAPIExplorerHandler(routes)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api-docs", nil)
	h.Handle(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/x") {
		t.Fatalf("expected route path in explorer html")
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/internal/api-docs/routes", nil)
	h.Routes(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "demo") {
		t.Fatalf("expected json routes response")
	}
}
