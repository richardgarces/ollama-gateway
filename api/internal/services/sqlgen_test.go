package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeSQLDialect(t *testing.T) {
	cases := map[string]string{
		"postgres":   "postgres",
		"PostgreSQL": "postgres",
		"mysql":      "mysql",
		"sqlite3":    "sqlite",
		"oracle":     "",
	}
	for in, want := range cases {
		if got := normalizeSQLDialect(in); got != want {
			t.Fatalf("normalizeSQLDialect(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateQueryRejectsInvalidDialect(t *testing.T) {
	svc := NewSQLGenService(fakeRAG{response: "SELECT 1;"}, t.TempDir(), nil)
	if _, err := svc.GenerateQuery("listar usuarios", "oracle"); err == nil {
		t.Fatalf("expected error for invalid dialect")
	}
}

func TestGenerateQueryUsesSchemaContext(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "schema.sql"), []byte("CREATE TABLE users (id bigint primary key, email text);"), 0o644); err != nil {
		t.Fatalf("write sql error: %v", err)
	}
	modelPath := filepath.Join(repoRoot, "internal", "domain", "user.go")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("package domain\ntype User struct { Email string `db:\"email\" bson:\"email\"` }"), 0o644); err != nil {
		t.Fatalf("write go error: %v", err)
	}

	svc := NewSQLGenService(fakeRAG{response: "```sql\nSELECT email FROM users;\n```"}, repoRoot, nil)
	out, err := svc.GenerateQuery("listar emails", "postgres")
	if err != nil {
		t.Fatalf("GenerateQuery() error = %v", err)
	}
	if !strings.Contains(out, "SELECT email FROM users") {
		t.Fatalf("unexpected sql output: %q", out)
	}
}

func TestExplainQuery(t *testing.T) {
	svc := NewSQLGenService(fakeRAG{response: "Consulta la tabla users y sugiere índice por email."}, t.TempDir(), nil)
	out, err := svc.ExplainQuery("SELECT * FROM users WHERE email = 'a@b.com';")
	if err != nil {
		t.Fatalf("ExplainQuery() error = %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("expected non-empty explanation")
	}
}
