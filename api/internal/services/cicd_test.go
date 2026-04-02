package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizePipelinePlatform(t *testing.T) {
	cases := map[string]string{
		"github-actions": "github-actions",
		"github":         "github-actions",
		"gitlab-ci":      "gitlab-ci",
		"gitlab":         "gitlab-ci",
		"jenkins":        "jenkins",
		"jenkinsfile":    "jenkins",
		"azure":          "",
	}
	for in, want := range cases {
		if got := normalizePipelinePlatform(in); got != want {
			t.Fatalf("normalizePipelinePlatform(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGeneratePipelineRejectsInvalidPlatform(t *testing.T) {
	svc := NewCICDService(fakeRAG{response: "name: ci"}, t.TempDir(), nil)
	if _, err := svc.GeneratePipeline("azure", ""); err == nil {
		t.Fatalf("expected error for invalid platform")
	}
}

func TestGeneratePipelineIncludesBuildContext(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module test\ngo 1.24\n"), 0o644); err != nil {
		t.Fatalf("write go.mod error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "Makefile"), []byte("test:\n\tgo test ./...\n"), 0o644); err != nil {
		t.Fatalf("write Makefile error: %v", err)
	}

	svc := NewCICDService(fakeRAG{response: "```yaml\nname: CI\n```"}, repoRoot, nil)
	out, err := svc.GeneratePipeline("github-actions", "")
	if err != nil {
		t.Fatalf("GeneratePipeline() error = %v", err)
	}
	if !strings.Contains(out, "name: CI") {
		t.Fatalf("unexpected pipeline output: %q", out)
	}
}

func TestOptimizePipeline(t *testing.T) {
	svc := NewCICDService(fakeRAG{response: "stages:\n  - lint\n  - test"}, t.TempDir(), nil)
	out, err := svc.OptimizePipeline("stages: [test]", "gitlab-ci")
	if err != nil {
		t.Fatalf("OptimizePipeline() error = %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected optimized content")
	}
}

func TestApplyPipelineWritesTargetFile(t *testing.T) {
	repoRoot := t.TempDir()
	svc := NewCICDService(fakeRAG{response: "name: CI"}, repoRoot, nil)

	appliedPath, backupPath, err := svc.ApplyPipeline("github-actions", "", "name: CI")
	if err != nil {
		t.Fatalf("ApplyPipeline() error = %v", err)
	}
	if backupPath != "" {
		t.Fatalf("expected empty backup on first write, got %s", backupPath)
	}
	if !strings.HasSuffix(filepath.ToSlash(appliedPath), ".github/workflows/ci.yml") {
		t.Fatalf("unexpected target path: %s", appliedPath)
	}

	data, err := os.ReadFile(appliedPath)
	if err != nil {
		t.Fatalf("read applied file error: %v", err)
	}
	if strings.TrimSpace(string(data)) != "name: CI" {
		t.Fatalf("unexpected applied content: %q", string(data))
	}
}
