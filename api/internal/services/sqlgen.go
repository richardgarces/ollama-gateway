package services

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ollama-gateway/internal/domain"
)

type SQLGenService struct {
	rag      domain.RAGEngine
	repoRoot string
	logger   *slog.Logger
}

func NewSQLGenService(rag domain.RAGEngine, repoRoot string, logger *slog.Logger) *SQLGenService {
	if logger == nil {
		logger = slog.Default()
	}
	return &SQLGenService{rag: rag, repoRoot: repoRoot, logger: logger}
}

func (s *SQLGenService) GenerateQuery(description, dialect string) (string, error) {
	description = strings.TrimSpace(description)
	dialect = normalizeSQLDialect(dialect)
	if description == "" {
		return "", fmt.Errorf("description requerido")
	}
	if dialect == "" {
		return "", fmt.Errorf("dialect inválido (permitidos: postgres, mysql, sqlite)")
	}

	schemaContext := s.collectSchemaContext(8, 22000)
	prompt := strings.Join([]string{
		fmt.Sprintf("Generate a %s SQL query for: %s. Return ONLY the SQL query, no explanations.", dialect, description),
		"Respect existing schema and naming from project context when available.",
		"Do not execute anything. Return plain SQL text only.",
		"Project schema context:",
		schemaContext,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return cleanupGeneratedSQL(out), nil
}

func (s *SQLGenService) GenerateMigration(description, dialect string) (string, error) {
	description = strings.TrimSpace(description)
	dialect = normalizeSQLDialect(dialect)
	if description == "" {
		return "", fmt.Errorf("description requerido")
	}
	if dialect == "" {
		return "", fmt.Errorf("dialect inválido (permitidos: postgres, mysql, sqlite)")
	}

	schemaContext := s.collectSchemaContext(10, 26000)
	prompt := strings.Join([]string{
		fmt.Sprintf("Generate a %s SQL migration script (CREATE/ALTER TABLE) for: %s.", dialect, description),
		"Return ONLY SQL migration script, no explanations.",
		"Include safe patterns when possible (idempotency/checks) according to dialect conventions.",
		"Do not execute anything. Return plain SQL text only.",
		"Project schema context:",
		schemaContext,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return cleanupGeneratedSQL(out), nil
}

func (s *SQLGenService) ExplainQuery(sql string) (string, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "", fmt.Errorf("sql requerido")
	}

	schemaContext := s.collectSchemaContext(8, 18000)
	prompt := strings.Join([]string{
		"Analyze this SQL query and explain what it does, its likely complexity, and possible optimizations.",
		"Mention indexes or query-shape improvements when relevant.",
		"Return plain text in Spanish.",
		"Query:",
		sql,
		"Project schema context:",
		schemaContext,
	}, "\n")

	out, err := s.rag.GenerateWithContext(prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stripMarkdownFence(out)), nil
}

func normalizeSQLDialect(d string) string {
	v := strings.ToLower(strings.TrimSpace(d))
	switch v {
	case "postgres", "postgresql":
		return "postgres"
	case "mysql":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return ""
	}
}

func cleanupGeneratedSQL(raw string) string {
	text := strings.TrimSpace(stripMarkdownFence(raw))
	text = strings.TrimSuffix(text, ";;;")
	return strings.TrimSpace(text)
}

func (s *SQLGenService) collectSchemaContext(limit int, maxBytes int) string {
	if limit <= 0 {
		limit = 8
	}
	if maxBytes <= 0 {
		maxBytes = 22000
	}
	rootAbs, err := filepath.Abs(s.repoRoot)
	if err != nil {
		return ""
	}

	type candidate struct {
		path  string
		score int
	}
	items := make([]candidate, 0, limit*4)

	_ = filepath.Walk(rootAbs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		name := strings.ToLower(filepath.Base(path))
		score := 0
		switch ext {
		case ".sql":
			score = 100
		case ".go":
			score = 40
		case ".yaml", ".yml", ".json":
			score = 10
		default:
			if name != "docker-compose.yml" {
				return nil
			}
			score = 8
		}
		if score == 0 {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		text := string(data)
		if ext == ".go" {
			if !strings.Contains(text, "db:\"") && !strings.Contains(text, "bson:\"") {
				return nil
			}
			score += strings.Count(text, "db:\"")*4 + strings.Count(text, "bson:\"")*2
		}
		if ext == ".sql" {
			l := strings.ToLower(text)
			score += strings.Count(l, "create table")*3 + strings.Count(l, "alter table")*2
		}

		items = append(items, candidate{path: path, score: score})
		return nil
	})

	if len(items) == 0 {
		return "(no schema context found)"
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].path < items[j].path
		}
		return items[i].score > items[j].score
	})
	if len(items) > limit {
		items = items[:limit]
	}

	remaining := maxBytes
	var b strings.Builder
	for _, item := range items {
		if remaining <= 0 {
			break
		}
		data, err := os.ReadFile(item.path)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(rootAbs, item.path)
		if relErr != nil {
			rel = item.path
		}
		header := fmt.Sprintf("FILE: %s\n", filepath.ToSlash(rel))
		b.WriteString(header)
		snippet := string(data)
		if len(snippet) > 2200 {
			snippet = snippet[:2200]
		}
		if len(snippet) > remaining {
			snippet = snippet[:remaining]
		}
		b.WriteString(snippet)
		b.WriteString("\n\n")
		remaining = maxBytes - b.Len()
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return "(no schema context found)"
	}
	return out
}
