package reposcope

import (
	"path/filepath"
	"testing"
)

func TestCanonicalizeAndCollection(t *testing.T) {
	root := t.TempDir()
	roots := CanonicalizeRoots([]string{"", root, root, "   "})
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if CollectionName(root) == "repo_docs" {
		t.Fatalf("expected hashed collection name")
	}
}

func TestStatePathAndRepoMatch(t *testing.T) {
	root := t.TempDir()
	state := StatePathForRepo(filepath.Join(root, ".indexer_state.json"), root)
	if filepath.Dir(state) != root {
		t.Fatalf("state path should be inside repo root")
	}

	matched, ok := MatchRepoFilter(filepath.Base(root), []string{root})
	if !ok || matched != root {
		t.Fatalf("expected base name filter to match")
	}
}

func TestRepoForPath(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	path := filepath.Join(sub, "x.go")
	got := RepoForPath(path, []string{root, sub})
	if got != sub {
		t.Fatalf("expected longest matching root %s, got %s", sub, got)
	}
}
