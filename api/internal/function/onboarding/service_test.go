package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateGuide(t *testing.T) {
	svc := NewService(".", nil)

	t.Run("supports required roles", func(t *testing.T) {
		roles := []string{"backend", "frontend", "devops", "qa"}
		for _, role := range roles {
			guide, err := svc.GenerateGuide(role)
			if err != nil {
				t.Fatalf("GenerateGuide(%s) error = %v", role, err)
			}
			if guide.Role != role {
				t.Fatalf("expected role %s, got %s", role, guide.Role)
			}
			if len(guide.SetupSteps) == 0 || len(guide.Commands) == 0 || len(guide.KeyPaths) == 0 {
				t.Fatalf("expected non-empty onboarding content for role %s", role)
			}
		}
	})

	t.Run("rejects unsupported role", func(t *testing.T) {
		_, err := svc.GenerateGuide("product")
		if err == nil {
			t.Fatalf("expected error for unsupported role")
		}
	})
}

func TestSaveGuide(t *testing.T) {
	repoRoot := t.TempDir()
	svc := NewService(repoRoot, nil)

	guide, err := svc.GenerateGuide("backend")
	if err != nil {
		t.Fatalf("GenerateGuide() error = %v", err)
	}

	relPath, err := svc.SaveGuide(guide)
	if err != nil {
		t.Fatalf("SaveGuide() error = %v", err)
	}
	if relPath != "docs/onboarding/backend.md" {
		t.Fatalf("unexpected output path: %s", relPath)
	}

	absPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	content, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("expected file written at %s: %v", absPath, err)
	}
	if !strings.Contains(string(content), "## Setup") {
		t.Fatalf("expected markdown setup section")
	}
}
