package reposcope

import (
	"path/filepath"
	"testing"
)

func TestCanonicalizeRoots(t *testing.T) {
	tmp := t.TempDir()
	roots := CanonicalizeRoots([]string{"", tmp, tmp, "  "})
	if len(roots) != 1 {
		t.Fatalf("expected 1 canonical root, got %d", len(roots))
	}
}

func TestCollectionName(t *testing.T) {
	if got := CollectionName("/tmp/repo"); got == "repo_docs" || got == "" {
		t.Fatalf("expected hashed collection name, got %q", got)
	}
}

func TestStatePathForRepo(t *testing.T) {
	repo := t.TempDir()
	p := StatePathForRepo(filepath.Join(repo, "custom.state"), repo)
	if filepath.Ext(p) != ".state" {
		t.Fatalf("expected .state extension in %q", p)
	}
}

func TestMatchRepoFilter(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()
	if root, ok := MatchRepoFilter(filepath.Base(a), []string{a, b}); !ok || root != a {
		t.Fatalf("expected match by base name")
	}
}

func TestRepoForPath(t *testing.T) {
	a := t.TempDir()
	sub := filepath.Join(a, "x", "y.go")
	root := RepoForPath(sub, []string{a})
	if root != a {
		t.Fatalf("expected repo root %q, got %q", a, root)
	}
}
