package jobs

import (
	"testing"
	"time"
)

func TestCleanupBudget_Consume(t *testing.T) {
	b := NewCleanupBudget(5, 30*time.Minute)
	for i := 0; i < 5; i++ {
		if !b.Consume() {
			t.Fatalf("consume %d should succeed", i+1)
		}
	}
	if b.Consume() {
		t.Fatal("6th consume should fail")
	}
}

func TestCleanupBudget_ConsumeForPhase(t *testing.T) {
	b := NewCleanupBudget(100, 30*time.Minute)
	b.SetPhaseQuota("phase4", 3)
	b.SetPhaseQuota("phase5", 3)
	b.SetPhaseQuota("phase6", 10)

	for i := 0; i < 3; i++ {
		if !b.ConsumeForPhase("phase4") {
			t.Fatalf("phase4 consume %d should succeed", i+1)
		}
	}
	if b.ConsumeForPhase("phase4") {
		t.Fatal("phase4 4th consume should fail (quota=3)")
	}

	if !b.ConsumeForPhase("phase5") {
		t.Fatal("phase5 should still succeed (different quota)")
	}
}

func TestCleanupBudget_Timeout(t *testing.T) {
	b := NewCleanupBudget(100, 50*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	if !b.IsTimedOut() {
		t.Fatal("should be timed out")
	}
	if b.Consume() {
		t.Fatal("consume after timeout should fail")
	}
}

func TestCleanupBudget_Stats(t *testing.T) {
	b := NewCleanupBudget(100, 30*time.Minute)
	b.SetPhaseQuota("phase6", 2)
	b.ConsumeForPhase("phase4")
	b.ConsumeForPhase("phase4")
	b.ConsumeForPhase("phase6")

	stats := b.Stats()
	if stats.TotalConsumed != 3 {
		t.Fatalf("expected 3 consumed, got %d", stats.TotalConsumed)
	}
	if stats.PhaseConsumed["phase6"] != 1 {
		t.Fatalf("expected phase6=1, got %d", stats.PhaseConsumed["phase6"])
	}
}
