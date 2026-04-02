package services

import (
	"os"
	"path/filepath"
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
