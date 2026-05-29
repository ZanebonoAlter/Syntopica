package datamigrate

import (
	"context"
	"testing"
	"time"
)

func TestVerifyCountsMatch(t *testing.T) {
	counts := []TableCountCheck{
		{Table: "feeds", SourceCount: 3, TargetCount: 3},
		{Table: "articles", SourceCount: 12, TargetCount: 12},
	}

	if err := VerifyCountsMatch(counts); err != nil {
		t.Fatalf("expected counts to match: %v", err)
	}

	mismatch := []TableCountCheck{
		{Table: "feeds", SourceCount: 3, TargetCount: 2},
	}

	if err := VerifyCountsMatch(mismatch); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestVerifySequenceIsResetAfterImport(t *testing.T) {
	state := SequenceState{Table: "feeds", Sequence: "feeds_id_seq", MaxID: 42, NextValue: 43}
	if err := VerifySequenceIsResetAfterImport(state); err != nil {
		t.Fatalf("expected sequence to be valid: %v", err)
	}

	broken := SequenceState{Table: "feeds", Sequence: "feeds_id_seq", MaxID: 42, NextValue: 40}
	if err := VerifySequenceIsResetAfterImport(broken); err == nil {
		t.Fatal("expected sequence reset error")
	}
}

func TestVerifyEmbeddingRowCountMatches(t *testing.T) {
	if err := VerifyEmbeddingRowCountMatches(7, 7); err != nil {
		t.Fatalf("expected embedding counts to match: %v", err)
	}

	if err := VerifyEmbeddingRowCountMatches(7, 6); err == nil {
		t.Fatal("expected embedding mismatch error")
	}
}

func TestVerifyEmbeddingVectorValuesPresent(t *testing.T) {
	valid := EmbeddingVectorCheck{Table: "topic_tag_embeddings", PrimaryKey: 1, SourceVector: "[0.1,0.2]", TargetVector: "[0.1,0.2]"}
	if err := VerifyEmbeddingVectorValuesPresent([]EmbeddingVectorCheck{valid}); err != nil {
		t.Fatalf("expected vector check to pass: %v", err)
	}

	missing := EmbeddingVectorCheck{Table: "topic_tag_embeddings", PrimaryKey: 2, SourceVector: "[0.1,0.2]", TargetVector: ""}
	if err := VerifyEmbeddingVectorValuesPresent([]EmbeddingVectorCheck{missing}); err == nil {
		t.Fatal("expected missing vector error")
	}

	mismatch := EmbeddingVectorCheck{Table: "topic_tag_embeddings", PrimaryKey: 3, SourceVector: "[0.1, 0.2]", TargetVector: "[0.1,0.3]"}
	if err := VerifyEmbeddingVectorValuesPresent([]EmbeddingVectorCheck{mismatch}); err == nil {
		t.Fatal("expected mismatched vector error")
	}
}

func TestVerifierVerifyUsesExtendedSampleCoverageAndEmbeddingChecks(t *testing.T) {
	now := time.Date(2026, 4, 3, 10, 0, 0, 0, time.UTC)
	source := &fakeReader{
		tables: map[string]bool{"topic_tag_embeddings": true},
		counts: map[string]int64{"topic_tag_embeddings": 4},
		samples: map[string][]map[string]any{
			"topic_tag_embeddings": {
				{"id": 1, "topic_tag_id": 10, "dimension": 2, "model": "m1", "text_hash": "a", "vector": "[0.1,0.2]"},
				{"id": 2, "topic_tag_id": 11, "dimension": 2, "model": "m1", "text_hash": "b", "vector": "[0.2,0.3]"},
				{"id": 3, "topic_tag_id": 12, "dimension": 2, "model": "m1", "text_hash": "c", "vector": "[0.3,0.4]"},
				{"id": 4, "topic_tag_id": 13, "dimension": 2, "model": "m1", "text_hash": "d", "vector": "[0.4,0.5]"},
			},
		},
	}
	target := &fakeWriter{
		tables:    map[string]bool{"topic_tag_embeddings": true},
		counts:    map[string]int64{"topic_tag_embeddings": 4},
		sequences: map[string]SequenceState{"topic_tag_embeddings": {Table: "topic_tag_embeddings", Sequence: "topic_tag_embeddings_id_seq", MaxID: 4, NextValue: 5}},
		samples: map[string][]map[string]any{
			"topic_tag_embeddings": {
				{"id": 1, "topic_tag_id": 10, "dimension": 2, "model": "m1", "text_hash": "a", "vector": "[0.1,0.2]"},
				{"id": 2, "topic_tag_id": 11, "dimension": 2, "model": "m1", "text_hash": "b", "vector": "[0.2,0.3]"},
				{"id": 3, "topic_tag_id": 12, "dimension": 2, "model": "m1", "text_hash": "c", "vector": "[0.3,0.4]"},
				{"id": 4, "topic_tag_id": 13, "dimension": 2, "model": "m1", "text_hash": "d", "vector": "[0.4,0.5]"},
			},
		},
		embeddings: map[string][]EmbeddingVectorCheck{
			"topic_tag_embeddings": {
				{Table: "topic_tag_embeddings", PrimaryKey: 1, TargetVector: "[0.1,0.2]"},
				{Table: "topic_tag_embeddings", PrimaryKey: 2, TargetVector: "[0.2,0.3]"},
				{Table: "topic_tag_embeddings", PrimaryKey: 3, TargetVector: "[0.3,0.4]"},
				{Table: "topic_tag_embeddings", PrimaryKey: 4, TargetVector: "[0.4,0.5]"},
			},
		},
	}

	verifier := &Verifier{Source: source, Target: target, Now: func() time.Time { return now }}
	report, err := verifier.Verify(context.Background(), []TableSpec{{
		Name:          "topic_tag_embeddings",
		PrimaryKey:    "id",
		SampleColumns: []string{"topic_tag_id", "dimension", "model", "text_hash", "vector"},
	}})
	if err != nil {
		t.Fatalf("expected verifier to pass: %v", err)
	}

	if report.CheckedAt != now {
		t.Fatalf("expected checked at %v, got %v", now, report.CheckedAt)
	}
	if source.lastSampleLimit != verificationSampleLimit {
		t.Fatalf("expected source sample limit %d, got %d", verificationSampleLimit, source.lastSampleLimit)
	}
	if target.lastSampleLimit != verificationSampleLimit {
		t.Fatalf("expected target sample limit %d, got %d", verificationSampleLimit, target.lastSampleLimit)
	}
	if len(report.EmbeddingChecks) != 4 {
		t.Fatalf("expected 4 embedding checks, got %d", len(report.EmbeddingChecks))
	}
}

func TestVerifierVerifyFailsWhenEmbeddingVectorMissing(t *testing.T) {
	verifier := &Verifier{
		Source: &fakeReader{
			tables: map[string]bool{"topic_tag_embeddings": true},
			counts: map[string]int64{"topic_tag_embeddings": 1},
			samples: map[string][]map[string]any{
				"topic_tag_embeddings": {{"id": 1, "vector": "[0.1,0.2]"}},
			},
		},
		Target: &fakeWriter{
			tables:     map[string]bool{"topic_tag_embeddings": true},
			counts:     map[string]int64{"topic_tag_embeddings": 1},
			sequences:  map[string]SequenceState{"topic_tag_embeddings": {Table: "topic_tag_embeddings", Sequence: "topic_tag_embeddings_id_seq", MaxID: 1, NextValue: 2}},
			embeddings: map[string][]EmbeddingVectorCheck{"topic_tag_embeddings": {{Table: "topic_tag_embeddings", PrimaryKey: 1, TargetVector: ""}}},
		},
		Now: time.Now,
	}

	_, err := verifier.Verify(context.Background(), []TableSpec{{Name: "topic_tag_embeddings", PrimaryKey: "id", SampleColumns: []string{"vector"}}})
	if err == nil {
		t.Fatal("expected verifier to fail on missing embedding vector")
	}
}

func TestVerifierVerifyHandlesAllowedMissingTargetColumnsInSamples(t *testing.T) {
	verifier := &Verifier{
		Source: &fakeReader{
			tables: map[string]bool{"topic_tag_embeddings": true},
			counts: map[string]int64{"topic_tag_embeddings": 1},
			samples: map[string][]map[string]any{
				"topic_tag_embeddings": {{"id": 1, "topic_tag_id": 10, "vector": "[0.1,0.2]"}},
			},
		},
		Target: &fakeWriter{
			tables:    map[string]bool{"topic_tag_embeddings": true},
			counts:    map[string]int64{"topic_tag_embeddings": 1},
			sequences: map[string]SequenceState{"topic_tag_embeddings": {Table: "topic_tag_embeddings", Sequence: "topic_tag_embeddings_id_seq", MaxID: 1, NextValue: 2}},
			samples: map[string][]map[string]any{
				"topic_tag_embeddings": {{"id": 1, "topic_tag_id": 10}},
			},
			embeddings: map[string][]EmbeddingVectorCheck{"topic_tag_embeddings": {{Table: "topic_tag_embeddings", PrimaryKey: 1, TargetVector: "[0.1,0.2]"}}},
		},
		Now: time.Now,
	}

	_, err := verifier.Verify(context.Background(), []TableSpec{{
		Name:                        "topic_tag_embeddings",
		PrimaryKey:                  "id",
		SampleColumns:               []string{"topic_tag_id", "vector"},
		AllowedMissingTargetColumns: []string{"vector"},
	}})
	if err != nil {
		t.Fatalf("expected verifier to tolerate allowed missing target vector column: %v", err)
	}
}

type fakeReader struct {
	tables          map[string]bool
	counts          map[string]int64
	samples         map[string][]map[string]any
	lastSampleLimit int
}

func (f *fakeReader) ExistingTables(context.Context) (map[string]bool, error) { return f.tables, nil }
func (f *fakeReader) CountRows(_ context.Context, table string) (int64, error) {
	return f.counts[table], nil
}
func (f *fakeReader) SampleRows(_ context.Context, spec TableSpec, limit int) ([]map[string]any, error) {
	f.lastSampleLimit = limit
	return f.samples[spec.Name], nil
}

type fakeWriter struct {
	tables          map[string]bool
	counts          map[string]int64
	sequences       map[string]SequenceState
	samples         map[string][]map[string]any
	embeddings      map[string][]EmbeddingVectorCheck
	lastSampleLimit int
}

func (f *fakeWriter) ExistingTables(context.Context) (map[string]bool, error) { return f.tables, nil }
func (f *fakeWriter) CountRows(_ context.Context, table string) (int64, error) {
	return f.counts[table], nil
}
func (f *fakeWriter) LoadSequenceState(_ context.Context, spec TableSpec) (SequenceState, error) {
	return f.sequences[spec.Name], nil
}
func (f *fakeWriter) SampleRows(_ context.Context, spec TableSpec, limit int) ([]map[string]any, error) {
	f.lastSampleLimit = limit
	return f.samples[spec.Name], nil
}
func (f *fakeWriter) LoadEmbeddingChecks(_ context.Context, spec TableSpec, limit int) ([]EmbeddingVectorCheck, error) {
	return f.embeddings[spec.Name], nil
}
