package observability

import "testing"

func TestMetricsCollectorPoolSaturation(t *testing.T) {
	collector := NewMetricsCollector()
	collector.RegisterPool("embedding", 4)

	collector.ObservePoolAcquire("embedding", false)
	collector.ObservePoolAcquire("embedding", true)
	collector.ObservePoolAcquire("embedding", false)
	collector.ObservePoolRelease("embedding")

	snap := collector.Snapshot()
	if len(snap.Pools) != 1 {
		t.Fatalf("expected 1 pool metric, got %d", len(snap.Pools))
	}

	p := snap.Pools[0]
	if p.Name != "embedding" {
		t.Fatalf("unexpected pool name: %s", p.Name)
	}
	if p.Capacity != 4 {
		t.Fatalf("unexpected capacity: %d", p.Capacity)
	}
	if p.InUse != 2 {
		t.Fatalf("unexpected in_use: %d", p.InUse)
	}
	if p.MaxInUse != 3 {
		t.Fatalf("unexpected max_in_use: %d", p.MaxInUse)
	}
	if p.WaitCount != 1 {
		t.Fatalf("unexpected wait_count: %d", p.WaitCount)
	}
	if p.Saturation <= 0 {
		t.Fatalf("expected positive saturation, got %f", p.Saturation)
	}
}
