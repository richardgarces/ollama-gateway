package prsummary

import (
	"strings"
	"testing"
)

func TestSummarizeDiff(t *testing.T) {
	svc := NewService(nil)

	t.Run("summarizes functional diff", func(t *testing.T) {
		diff := strings.Join([]string{
			"diff --git a/api/internal/server/server.go b/api/internal/server/server.go",
			"index 111..222 100644",
			"--- a/api/internal/server/server.go",
			"+++ b/api/internal/server/server.go",
			"@@ -10,6 +10,7 @@",
			"+mux.Handle(\"POST /api/pr/summary\", authMiddleware.JWT(http.HandlerFunc(prSummaryHandler.Summarize)))",
			"diff --git a/api/internal/function/prsummary/service.go b/api/internal/function/prsummary/service.go",
			"index 333..444 100644",
		}, "\n")

		result, err := svc.SummarizeDiff(diff)
		if err != nil {
			t.Fatalf("SummarizeDiff() error = %v", err)
		}
		if result.Risk == "" {
			t.Fatalf("expected risk")
		}
		if len(result.AffectedComponents) == 0 {
			t.Fatalf("expected affected components")
		}
		if len(result.SuggestedTests) == 0 || len(result.ReviewChecklist) == 0 {
			t.Fatalf("expected tests/checklist")
		}
	})

	t.Run("rejects empty diff", func(t *testing.T) {
		_, err := svc.SummarizeDiff("  ")
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})
}
