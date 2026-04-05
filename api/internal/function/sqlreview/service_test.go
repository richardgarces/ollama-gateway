package sqlreview

import (
	"strings"
	"testing"
)

func TestReviewMigration(t *testing.T) {
	svc := NewService(nil)

	t.Run("detects dangerous operations and high risk", func(t *testing.T) {
		sql := `
ALTER TABLE users ALTER COLUMN email TYPE TEXT;
DROP TABLE sessions;
UPDATE users SET active = false;
`
		res, err := svc.ReviewMigration(sql, "postgres")
		if err != nil {
			t.Fatalf("ReviewMigration() error = %v", err)
		}
		if res.GlobalRisk != "high" {
			t.Fatalf("expected high global risk, got %s", res.GlobalRisk)
		}
		if len(res.Findings) == 0 {
			t.Fatalf("expected findings")
		}
		joinedRollback := strings.ToLower(strings.Join(res.RollbackChecks, "\n"))
		if !strings.Contains(joinedRollback, "rollback") {
			t.Fatalf("expected rollback checks")
		}
		joinedIdempotency := strings.ToLower(strings.Join(res.IdempotencyChecks, "\n"))
		if !strings.Contains(joinedIdempotency, "idempot") {
			t.Fatalf("expected idempotency checks")
		}
	})

	t.Run("returns low risk for safe idempotent migration", func(t *testing.T) {
		sql := `
-- rollback: drop table if exists feature_flags;
CREATE TABLE IF NOT EXISTS feature_flags (id bigint primary key, name text);
CREATE INDEX IF NOT EXISTS idx_feature_flags_name ON feature_flags(name);
`
		res, err := svc.ReviewMigration(sql, "postgres")
		if err != nil {
			t.Fatalf("ReviewMigration() error = %v", err)
		}
		if res.GlobalRisk == "high" {
			t.Fatalf("expected non-high risk for safe migration")
		}
	})

	t.Run("validates required fields", func(t *testing.T) {
		if _, err := svc.ReviewMigration("", "postgres"); err == nil {
			t.Fatalf("expected error for empty sql")
		}
		if _, err := svc.ReviewMigration("select 1", ""); err == nil {
			t.Fatalf("expected error for empty dialect")
		}
	})
}
