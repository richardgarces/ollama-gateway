package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestASTAnalyzerAnalyzeFileExtractsSummary(t *testing.T) {
	repoRoot := t.TempDir()
	filePath := filepath.Join(repoRoot, "sample.go")
	content := `package sample

import "fmt"

// Person representa un usuario.
type Person struct {
	Name string ` + "`json:\"name\"`" + `
}

type Speaker interface {
	Speak(msg string) error
}

// Greet imprime un saludo.
func Greet(name string) string {
	fmt.Println(name)
	return "ok"
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	analyzer := NewASTAnalyzer([]string{repoRoot})
	analysis, err := analyzer.AnalyzeFile(filePath)
	if err != nil {
		t.Fatalf("AnalyzeFile() error = %v", err)
	}

	if analysis.Package != "sample" {
		t.Fatalf("expected package sample, got %s", analysis.Package)
	}
	if len(analysis.Imports) != 1 || analysis.Imports[0] != "fmt" {
		t.Fatalf("expected fmt import, got %#v", analysis.Imports)
	}
	if len(analysis.Functions) != 1 || analysis.Functions[0].Name != "Greet" {
		t.Fatalf("expected function Greet, got %#v", analysis.Functions)
	}
	if len(analysis.Structs) != 1 || analysis.Structs[0].Name != "Person" {
		t.Fatalf("expected struct Person, got %#v", analysis.Structs)
	}
	if len(analysis.Interfaces) != 1 || analysis.Interfaces[0].Name != "Speaker" {
		t.Fatalf("expected interface Speaker, got %#v", analysis.Interfaces)
	}
}

func TestASTAnalyzerRejectsPathOutsideRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "other.go")
	if err := os.WriteFile(outsideFile, []byte("package outside"), 0644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	analyzer := NewASTAnalyzer([]string{repoRoot})
	if _, err := analyzer.AnalyzeFile(outsideFile); err == nil {
		t.Fatalf("expected AnalyzeFile() to reject path outside repo root")
	}
}

func TestASTAnalyzerAnalyzePackageBuildsDependencies(t *testing.T) {
	repoRoot := t.TempDir()
	pkgA := filepath.Join(repoRoot, "pkg", "a")
	pkgB := filepath.Join(repoRoot, "pkg", "b")
	if err := os.MkdirAll(pkgA, 0755); err != nil {
		t.Fatalf("mkdir pkg a: %v", err)
	}
	if err := os.MkdirAll(pkgB, 0755); err != nil {
		t.Fatalf("mkdir pkg b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgB, "b.go"), []byte("package b\n"), 0644); err != nil {
		t.Fatalf("write pkg b file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgA, "a.go"), []byte("package a\nimport \"example/pkg/b\"\n"), 0644); err != nil {
		t.Fatalf("write pkg a file: %v", err)
	}

	analyzer := NewASTAnalyzer([]string{repoRoot})
	pkgAnalysis, err := analyzer.AnalyzePackage(pkgA)
	if err != nil {
		t.Fatalf("AnalyzePackage() error = %v", err)
	}

	deps := pkgAnalysis.PackageDependencies
	if len(deps) != 1 || deps[0] != "pkg/b" {
		t.Fatalf("expected dependency pkg/b, got %#v", deps)
	}
}
