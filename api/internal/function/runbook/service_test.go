package runbook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRunbook(t *testing.T) {
	svc := NewService(".", nil)

	t.Run("success with sql signals", func(t *testing.T) {
		rb, err := svc.GenerateRunbook("sql-migration-lock", "timeouts and db lock after deploy")
		if err != nil {
			t.Fatalf("GenerateRunbook() error = %v", err)
		}
		if rb.Title == "" || rb.IncidentType == "" {
			t.Fatalf("expected title and incident type")
		}
		if len(rb.DiagnosisSteps) == 0 || len(rb.MitigationSteps) == 0 {
			t.Fatalf("expected diagnosis and mitigation steps")
		}
		joinedRollback := strings.ToLower(strings.Join(rb.RollbackSteps, "\n"))
		if !strings.Contains(joinedRollback, "rollback") || !strings.Contains(joinedRollback, "idempotencia") {
			t.Fatalf("expected rollback and idempotency checks in rollback steps")
		}
	})

	t.Run("requires incident type", func(t *testing.T) {
		_, err := svc.GenerateRunbook("   ", "context")
		if err == nil {
			t.Fatalf("expected validation error for empty incident type")
		}
	})
}

func TestSaveRunbook(t *testing.T) {
	repoRoot := t.TempDir()
	svc := NewService(repoRoot, nil)

	t.Run("writes markdown inside repo root", func(t *testing.T) {
		rb, err := svc.GenerateRunbook("auth-jwt-failure", "token expired")
		if err != nil {
			t.Fatalf("GenerateRunbook() error = %v", err)
		}

		relPath, err := svc.SaveRunbook(rb)
		if err != nil {
			t.Fatalf("SaveRunbook() error = %v", err)
		}
		if relPath != "docs/runbooks/auth-jwt-failure.md" {
			t.Fatalf("unexpected relative path: %s", relPath)
		}

		absPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		content, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("expected file written at %s: %v", absPath, err)
		}
		if !strings.Contains(string(content), "## Rollback") {
			t.Fatalf("expected rollback section in markdown")
		}
	})

	t.Run("rejects invalid incident type", func(t *testing.T) {
		err := func() error {
			_, saveErr := svc.SaveRunbook(Runbook{IncidentType: "!!!"})
			return saveErr
		}()
		if err == nil {
			t.Fatalf("expected error for invalid incident type")
		}
	})
}

func TestListAndGetRunbook(t *testing.T) {
	repoRoot := t.TempDir()
	svc := NewService(repoRoot, nil)

	rbAuth, err := svc.GenerateRunbook("auth-jwt-failure", "token expired")
	if err != nil {
		t.Fatalf("GenerateRunbook() error = %v", err)
	}
	if _, err := svc.SaveRunbook(rbAuth); err != nil {
		t.Fatalf("SaveRunbook(auth) error = %v", err)
	}

	rbDB, err := svc.GenerateRunbook("db-lock-timeout", "db lock")
	if err != nil {
		t.Fatalf("GenerateRunbook() error = %v", err)
	}
	if _, err := svc.SaveRunbook(rbDB); err != nil {
		t.Fatalf("SaveRunbook(db) error = %v", err)
	}

	t.Run("list all", func(t *testing.T) {
		items, err := svc.ListRunbooks("")
		if err != nil {
			t.Fatalf("ListRunbooks() error = %v", err)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 runbooks, got %d", len(items))
		}
	})

	t.Run("filter by incident type", func(t *testing.T) {
		items, err := svc.ListRunbooks("auth-jwt-failure")
		if err != nil {
			t.Fatalf("ListRunbooks(filter) error = %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 runbook, got %d", len(items))
		}
		if items[0].IncidentType != "auth-jwt-failure" {
			t.Fatalf("unexpected incident type: %s", items[0].IncidentType)
		}
	})

	t.Run("get existing runbook", func(t *testing.T) {
		item, err := svc.GetRunbook("auth-jwt-failure")
		if err != nil {
			t.Fatalf("GetRunbook() error = %v", err)
		}
		if item.MarkdownPath != "docs/runbooks/auth-jwt-failure.md" {
			t.Fatalf("unexpected markdown path: %s", item.MarkdownPath)
		}
		if !strings.Contains(item.Markdown, "## Diagnostico") {
			t.Fatalf("expected markdown content in runbook")
		}
	})

	t.Run("get missing runbook", func(t *testing.T) {
		if _, err := svc.GetRunbook("missing-incident"); err == nil {
			t.Fatalf("expected not found error")
		}
	})
}
