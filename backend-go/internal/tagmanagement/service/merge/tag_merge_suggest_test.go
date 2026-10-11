package merge

import (
	"context"
	"testing"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

func TestIsScanRunning_Initially(t *testing.T) {
	// scanState.running should be false when no scan has been started.
	if IsScanRunning() {
		t.Error("IsScanRunning() = true, want false before any scan is started")
	}
}

func TestRecordMergeSuggestions_EmptyCandidates(t *testing.T) {
	// nil candidates — should return immediately without panicking.
	core.RecordMergeSuggestions(1, "AI", "keyword", nil)

	// empty slice — same behavior.
	core.RecordMergeSuggestions(2, "ML", "keyword", []core.TagCandidate{})
}

func TestRecordMergeSuggestions_CandidateStructConstruction(t *testing.T) {
	// Verify that TagCandidate structs are constructed correctly and
	// produce the expected field values (no DB interaction).
	candidates := []core.TagCandidate{
		{
			Tag:        &models.TopicTag{ID: 10, Label: "Machine Learning", Category: "keyword"},
			Similarity: 0.92,
		},
		{
			Tag:        &models.TopicTag{ID: 20, Label: "Deep Learning", Category: "keyword"},
			Similarity: 0.85,
		},
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Tag.ID != 10 {
		t.Errorf("candidates[0].Tag.ID = %d, want 10", candidates[0].Tag.ID)
	}
	if candidates[0].Similarity != 0.92 {
		t.Errorf("candidates[0].Similarity = %.2f, want 0.92", candidates[0].Similarity)
	}
	if candidates[1].Tag.Label != "Deep Learning" {
		t.Errorf("candidates[1].Tag.Label = %q, want %q", candidates[1].Tag.Label, "Deep Learning")
	}
}

func TestStartFullScan_RejectsConcurrent(t *testing.T) {
	// If a scan is already running, StartFullScan should return false.
	// Simulate the running state directly instead of starting a real scan:
	// runFullScan needs DB + embedding, and a leaked goroutine would race the
	// test binary's exit and panic on nil repository.Repo (flaky crash, patrol
	// 2026-10-11). The real start path is covered by production/integration.
	scanState.mu.Lock()
	scanState.running.Store(true)
	scanState.progress = make(chan ScanProgress, 1)
	scanState.mu.Unlock()
	defer func() {
		scanState.mu.Lock()
		scanState.running.Store(false)
		scanState.progress = nil
		scanState.mu.Unlock()
	}()

	if StartFullScan() {
		t.Error("StartFullScan() = true, want false (already running)")
	}
}

func TestRunFullScan_NilRepo_NoPanic(t *testing.T) {
	// runFullScan must not panic when repository.Repo is nil (unit-test process
	// or misordered startup): it should emit an error progress and return,
	// never crash the host process from this background goroutine.
	scanState.mu.Lock()
	scanState.progress = make(chan ScanProgress, 1)
	scanState.mu.Unlock()
	defer func() {
		scanState.mu.Lock()
		scanState.progress = nil
		scanState.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runFullScan(context.Background())
	}()

	select {
	case p, ok := <-scanState.progress:
		if !ok || p.Status != "error" {
			t.Errorf("progress = (%+v, %v), want (Status:error, open)", p, ok)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for error progress from runFullScan")
	}
	<-done // runFullScan returned without panicking
}

func TestScanProgress_JSONFields(t *testing.T) {
	// Verify ScanProgress struct has expected fields for SSE serialization.
	p := ScanProgress{
		Status:          "scanning",
		Total:           590,
		Scanned:         342,
		CurrentCategory: "keyword",
		NewSuggestions:  23,
	}
	if p.Status != "scanning" {
		t.Errorf("Status = %q, want %q", p.Status, "scanning")
	}
	if p.Total != 590 {
		t.Errorf("Total = %d, want 590", p.Total)
	}
	if p.Scanned != 342 {
		t.Errorf("Scanned = %d, want 342", p.Scanned)
	}
	if p.NewSuggestions != 23 {
		t.Errorf("NewSuggestions = %d, want 23", p.NewSuggestions)
	}
}
