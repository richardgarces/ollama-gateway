package service

import "testing"

func TestAnalyzeDiff(t *testing.T) {
	svc := NewService()

	t.Run("prioritizes middleware and server", func(t *testing.T) {
		diff := `diff --git a/api/internal/middleware/auth.go b/api/internal/middleware/auth.go
index 111..222 100644
--- a/api/internal/middleware/auth.go
+++ b/api/internal/middleware/auth.go
@@ -1,1 +1,1 @@
-old
+new
diff --git a/api/internal/server/server.go b/api/internal/server/server.go
index 333..444 100644
--- a/api/internal/server/server.go
+++ b/api/internal/server/server.go`

		report := svc.AnalyzeDiff(AnalyzeInput{Diff: diff})
		if len(report.ChangedFiles) != 2 {
			t.Fatalf("expected 2 changed files, got %d", len(report.ChangedFiles))
		}
		if len(report.Targets) == 0 {
			t.Fatalf("expected prioritized targets")
		}
		if report.RiskLevel == "low" {
			t.Fatalf("expected risk level not low for middleware+server changes")
		}
	})

	t.Run("falls back when no matched files", func(t *testing.T) {
		report := svc.AnalyzeDiff(AnalyzeInput{Diff: "+++ b/README.md"})
		if len(report.Targets) == 0 {
			t.Fatalf("expected fallback target")
		}
		if report.Targets[0].Package == "" {
			t.Fatalf("expected non-empty package target")
		}
	})
}
