package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSecurityFindingsExtractsJSONArray(t *testing.T) {
	raw := "texto previo\n[{\"severity\":\"HIGH\",\"category\":\"injection\",\"line\":12,\"description\":\"unsafe sql\",\"fix\":\"use prepared statement\"}]\ntexto final"
	findings, err := parseSecurityFindings(raw)
	if err != nil {
		t.Fatalf("parseSecurityFindings() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != "high" {
		t.Fatalf("expected normalized severity high, got %s", findings[0].Severity)
	}
}

func TestSecurityScanFileRejectsOutsideRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	outside := filepath.Join(os.TempDir(), "outside-security-file.go")
	_ = os.WriteFile(outside, []byte("package main"), 0o644)
	defer os.Remove(outside)

	svc := NewSecurityService(fakeRAG{response: "[]"}, repoRoot, nil)
	if _, err := svc.ScanFile(outside); err == nil {
		t.Fatalf("expected error for path outside repo root")
	}
}

func TestSecurityScanRepoConsolidatesFindings(t *testing.T) {
	repoRoot := t.TempDir()
	fileA := filepath.Join(repoRoot, "internal", "handlers", "a.go")
	fileB := filepath.Join(repoRoot, "internal", "services", "b.go")
	if err := os.MkdirAll(filepath.Dir(fileA), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(fileB), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(fileA, []byte("package handlers\nfunc A() {}"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("package services\nfunc B() {}"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	svc := NewSecurityService(fakeRAG{response: `[{"severity":"critical","category":"broken auth","line":8,"description":"hardcoded token","fix":"move to env"}]`}, repoRoot, nil)
	report, err := svc.ScanRepo()
	if err != nil {
		t.Fatalf("ScanRepo() error = %v", err)
	}
	if report.ScannedFiles == 0 {
		t.Fatalf("expected scanned files > 0")
	}
	if report.TotalFindings == 0 {
		t.Fatalf("expected findings in report")
	}
	if report.FindingsByLevel["critical"] == 0 {
		t.Fatalf("expected critical findings count")
	}
	if report.HighOrCritical == 0 {
		t.Fatalf("expected high or critical count")
	}
}

func TestScanSecretsRepo(t *testing.T) {
	repoRoot := t.TempDir()
	file := filepath.Join(repoRoot, "internal", "config", "secrets.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	content := strings.Join([]string{
		"package config",
		"const GithubToken = \"ghp_123456789012345678901234567890\"",
		"const Password = \"super-secret-password\"",
	}, "\n")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	svc := NewSecurityService(fakeRAG{response: "[]"}, repoRoot, nil)
	report, err := svc.ScanSecretsRepo()
	if err != nil {
		t.Fatalf("ScanSecretsRepo() error = %v", err)
	}
	if report.TotalFinding == 0 {
		t.Fatalf("expected secret findings > 0")
	}
	if report.ByLevel["high"] == 0 && report.ByLevel["critical"] == 0 {
		t.Fatalf("expected high or critical secret findings")
	}
}

func TestEvaluatePolicyBlocksApply(t *testing.T) {
	repoRoot := t.TempDir()
	file := filepath.Join(repoRoot, "internal", "config", "secrets.go")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(file, []byte("package config\nconst ApiKey = \"super-secret-token-12345\"\n"), 0o644); err != nil {
		t.Fatalf("write error: %v", err)
	}

	svc := NewSecurityService(fakeRAG{response: "[]"}, repoRoot, nil)
	decision, err := svc.EvaluatePolicy("cicd:apply")
	if err != nil {
		t.Fatalf("EvaluatePolicy() error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("expected policy to block cicd:apply when secrets exist")
	}
}
