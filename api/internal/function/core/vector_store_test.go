package service

import (
	"path/filepath"
	"testing"
)

func TestDiskVectorStorePersistsAndSearches(t *testing.T) {
	repoRoot := t.TempDir()
	storePath := filepath.Join(repoRoot, ".vector_store.json")

	store, err := newDiskVectorStore(repoRoot, storePath)
	if err != nil {
		t.Fatalf("newDiskVectorStore() error = %v", err)
	}

	t.Run("persist points", func(t *testing.T) {
		if err := store.UpsertPoint("repo_docs", "a", []float64{1, 0}, map[string]interface{}{"code": "alpha"}); err != nil {
			t.Fatalf("UpsertPoint() error = %v", err)
		}
		if err := store.UpsertPoint("repo_docs", "b", []float64{0, 1}, map[string]interface{}{"code": "beta"}); err != nil {
			t.Fatalf("UpsertPoint() error = %v", err)
		}
	})

	t.Run("reload from disk", func(t *testing.T) {
		reloaded, err := newDiskVectorStore(repoRoot, storePath)
		if err != nil {
			t.Fatalf("newDiskVectorStore() error = %v", err)
		}
		res, err := reloaded.Search("repo_docs", []float64{0.9, 0.1}, 1)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		items, ok := res["result"].([]map[string]interface{})
		if ok {
			if len(items) != 1 {
				t.Fatalf("expected 1 result, got %d", len(items))
			}
			return
		}
		rawItems, ok := res["result"].([]interface{})
		if !ok {
			t.Fatalf("unexpected result type: %T", res["result"])
		}
		if len(rawItems) != 1 {
			t.Fatalf("expected 1 result, got %d", len(rawItems))
		}
		first, _ := rawItems[0].(map[string]interface{})
		if first["id"] != "a" {
			t.Fatalf("expected closest point 'a', got %v", first["id"])
		}
	})
}

func TestResolvePathWithinRootRejectsTraversal(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := resolvePathWithinRoot(repoRoot, filepath.Join("..", "outside.json")); err == nil {
		t.Fatal("expected traversal path to be rejected")
	}
}
