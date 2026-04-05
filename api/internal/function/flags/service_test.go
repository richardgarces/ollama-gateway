package flags

import (
	"testing"
	"time"
)

func TestEvaluateEnabled(t *testing.T) {
	now := time.Now().UTC()

	t.Run("enabled with full rollout", func(t *testing.T) {
		flag := Flag{Tenant: "default", Feature: "postmortem", Enabled: true, RolloutPercentage: 100}
		if !evaluateEnabled(flag, "acme", now) {
			t.Fatalf("expected enabled")
		}
	})

	t.Run("disabled by date window", func(t *testing.T) {
		start := now.Add(1 * time.Hour)
		flag := Flag{Tenant: "default", Feature: "postmortem", Enabled: true, RolloutPercentage: 100, StartAt: &start}
		if evaluateEnabled(flag, "acme", now) {
			t.Fatalf("expected disabled before start")
		}
	})

	t.Run("rollout is deterministic", func(t *testing.T) {
		flag := Flag{Tenant: "default", Feature: "gate", Enabled: true, RolloutPercentage: 20}
		a := evaluateEnabled(flag, "tenant-a", now)
		b := evaluateEnabled(flag, "tenant-a", now)
		if a != b {
			t.Fatalf("expected deterministic rollout result")
		}
	})
}
