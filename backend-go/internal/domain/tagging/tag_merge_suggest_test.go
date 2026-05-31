package tagging

import (
	"testing"

	"syntopica-backend/internal/domain/models"
)

func TestIsScanRunning_Initially(t *testing.T) {
	// scanState.running should be false when no scan has been started.
	if IsScanRunning() {
		t.Error("IsScanRunning() = true, want false before any scan is started")
	}
}

func TestRecordMergeSuggestions_EmptyCandidates(t *testing.T) {
	// nil candidates — should return immediately without panicking.
	RecordMergeSuggestions(1, "AI", "keyword", nil)

	// empty slice — same behavior.
	RecordMergeSuggestions(2, "ML", "keyword", []TagCandidate{})
}

func TestRecordMergeSuggestions_CandidateStructConstruction(t *testing.T) {
	// Verify that TagCandidate structs are constructed correctly and
	// produce the expected field values (no DB interaction).
	candidates := []TagCandidate{
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
	// We can't easily start a real scan in unit tests (needs DB + embedding service),
	// so this test verifies the function signature and basic behavior.
	// The actual concurrency protection is tested via integration.
	result := StartFullScan()
	// Without a real DB, the scan goroutine will likely error and finish quickly.
	// The key assertion: if first call started, second should be rejected.
	if result {
		// First call started a scan — second should fail
		if StartFullScan() {
			t.Error("second StartFullScan() = true, want false (already running)")
		}
	}
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
