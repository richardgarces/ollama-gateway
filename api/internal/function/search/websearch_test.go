package service

import (
	"context"
	"strings"
	"testing"
)

func TestWebSearchService_Enabled(t *testing.T) {
	t.Run("disabled when no key", func(t *testing.T) {
		svc := NewWebSearchService("", nil)
		if svc.Enabled() {
			t.Error("expected disabled")
		}
	})

	t.Run("enabled with key", func(t *testing.T) {
		svc := NewWebSearchService("test-key", nil)
		if !svc.Enabled() {
			t.Error("expected enabled")
		}
	})
}

func TestWebSearchService_SearchErrors(t *testing.T) {
	t.Run("error when disabled", func(t *testing.T) {
		svc := NewWebSearchService("", nil)
		_, err := svc.Search(context.Background(), "test")
		if err == nil {
			t.Error("expected error when disabled")
		}
	})

	t.Run("error on empty query", func(t *testing.T) {
		svc := NewWebSearchService("key", nil)
		_, err := svc.Search(context.Background(), "")
		if err == nil {
			t.Error("expected error on empty query")
		}
	})
}

func TestWebSearchService_FormatResults(t *testing.T) {
	svc := NewWebSearchService("key", nil)

	t.Run("nil results", func(t *testing.T) {
		result := svc.FormatResults(nil)
		if result != "No se encontraron resultados web." {
			t.Errorf("unexpected: %s", result)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		result := svc.FormatResults(&WebSearchResponse{})
		if result != "No se encontraron resultados web." {
			t.Errorf("unexpected: %s", result)
		}
	})

	t.Run("formats results correctly", func(t *testing.T) {
		resp := &WebSearchResponse{
			Results: []WebSearchResult{
				{Title: "Go lang", URL: "https://go.dev", Content: "Go is an open source programming language."},
				{Title: "Rust lang", URL: "https://rust-lang.org", Content: "Rust is a systems programming language."},
			},
		}
		formatted := svc.FormatResults(resp)
		if !strings.Contains(formatted, "1. Go lang") {
			t.Error("expected numbered results")
		}
		if !strings.Contains(formatted, "2. Rust lang") {
			t.Error("expected second result")
		}
		if !strings.Contains(formatted, "https://go.dev") {
			t.Error("expected URL in results")
		}
	})
}

func TestWebSearchTool(t *testing.T) {
	t.Run("error on empty query", func(t *testing.T) {
		svc := NewWebSearchService("key", nil)
		tool := NewWebSearchTool(svc)
		_, err := tool.Run(map[string]string{})
		if err == nil {
			t.Error("expected error on empty query")
		}
	})

	t.Run("error when disabled", func(t *testing.T) {
		svc := NewWebSearchService("", nil)
		tool := NewWebSearchTool(svc)
		_, err := tool.Run(map[string]string{"query": "test"})
		if err == nil {
			t.Error("expected error when disabled")
		}
	})

	t.Run("name is web_search", func(t *testing.T) {
		svc := NewWebSearchService("key", nil)
		tool := NewWebSearchTool(svc)
		if tool.Name() != "web_search" {
			t.Errorf("expected name 'web_search', got '%s'", tool.Name())
		}
	})
}

func TestSearchFormatted_Disabled(t *testing.T) {
	svc := NewWebSearchService("", nil)
	_, err := svc.SearchFormatted(context.Background(), "test")
	if err == nil {
		t.Error("expected error when disabled")
	}
}
