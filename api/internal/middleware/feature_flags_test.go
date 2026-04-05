package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeFlagEvaluator struct {
	enabled bool
	err     error
	tenant  string
	feature string
}

func (f *fakeFlagEvaluator) IsEnabledWithContext(ctx context.Context, tenant, feature string) (bool, error) {
	_ = ctx
	f.tenant = tenant
	f.feature = feature
	if f.err != nil {
		return false, f.err
	}
	return f.enabled, nil
}

func TestRequireFeatureFlag(t *testing.T) {
	t.Run("allows request when enabled", func(t *testing.T) {
		eval := &fakeFlagEvaluator{enabled: true}
		h := RequireFeatureFlag(eval, "postmortem")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodPost, "/api/postmortem/analyze", nil)
		req.Header.Set("X-Tenant-ID", "acme")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d", rr.Code)
		}
		if eval.tenant != "acme" || eval.feature != "postmortem" {
			t.Fatalf("unexpected evaluator args: tenant=%s feature=%s", eval.tenant, eval.feature)
		}
	})

	t.Run("blocks request when disabled", func(t *testing.T) {
		eval := &fakeFlagEvaluator{enabled: false}
		h := RequireFeatureFlag(eval, "runbooks")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/runbooks/generate", nil))
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", rr.Code)
		}
	})
}
