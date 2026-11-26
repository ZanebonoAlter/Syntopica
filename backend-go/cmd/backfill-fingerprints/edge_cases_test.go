package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"syntopica-backend/internal/models"
)

func TestBoardFilterAndLimitIgnoreExistingFingerprints(t *testing.T) {
	db := testDB(t)
	existing := seed(t, db, "existing", []string{"Apple", "手机"})
	existing.Metadata["event_fingerprint"] = map[string]any{"subject": "already present"}
	if err := db.Model(&existing).Update("metadata", existing.Metadata).Error; err != nil {
		t.Fatal(err)
	}
	seed(t, db, "other board", nil)
	target := seed(t, db, "selected", []string{"Apple", "手机"})
	if err := db.Exec("CREATE TABLE topic_tag_board_labels (topic_tag_id integer, semantic_board_id integer)").Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{existing.ID, target.ID} {
		// Duplicate memberships must not multiply AI calls.
		for range 2 {
			if err := db.Exec("INSERT INTO topic_tag_board_labels VALUES (?, 7)", id).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	calls := 0
	llm := func(_ context.Context, tag models.TopicTag) (string, error) {
		calls++
		if tag.ID != target.ID {
			t.Fatal(tag.ID)
		}
		return validResponse, nil
	}
	stats, err := backfill(context.Background(), db, options{Board: 7, Limit: 1, Workers: 1, Attempts: 1}, llm, io.Discard)
	if err != nil || stats.OK != 1 || stats.Skipped != 1 || calls != 1 {
		t.Fatalf("%+v calls=%d err=%v", stats, calls, err)
	}
}

func TestMalformedOutputRetriesAndUntrustedDoesNotBlock(t *testing.T) {
	for _, test := range []struct {
		name, response string
		failed         bool
	}{
		{"invalid JSON", "not json", true},
		{"incomplete", `{"subject":"Apple"}`, true},
		{"invalid subject key", strings.Replace(validResponse, "COMPANY:Apple", "invalid:apple", 1), false},
		{"object mismatch", strings.Replace(validResponse, "手机", "汽车", 1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := testDB(t)
			tag := seed(t, db, "test", []string{"Apple", "手机"})
			calls := 0
			llm := func(context.Context, models.TopicTag) (string, error) { calls++; return test.response, nil }
			stats, err := backfill(context.Background(), db, options{Workers: 1, Attempts: 1}, llm, io.Discard)
			if (err != nil) != test.failed || calls != 1 || stats.Trusted != 0 {
				t.Fatalf("%+v %v", stats, err)
			}
			if err := db.First(&tag, tag.ID).Error; err != nil {
				t.Fatal(err)
			}
			if test.failed {
				if stats.Failed != 1 || hasFingerprint(tag) {
					t.Fatal("invalid response persisted")
				}
			} else if stats.OK != 1 || tag.Metadata["fingerprint_trusted"] != false {
				t.Fatal("untrusted fingerprint blocked")
			}
		})
	}
	t.Run("transient recovery", func(t *testing.T) {
		db := testDB(t)
		seed(t, db, "retry", []string{"Apple", "手机"})
		calls := 0
		llm := func(context.Context, models.TopicTag) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("transient")
			}
			return validResponse, nil
		}
		stats, err := backfill(context.Background(), db, options{Workers: 1, Attempts: 2}, llm, io.Discard)
		if err != nil || calls != 2 || stats.OK != 1 || stats.Failed != 0 {
			t.Fatalf("%+v calls=%d %v", stats, calls, err)
		}
	})
}

func TestWriteFailureRollsBackOnlyOneTag(t *testing.T) {
	db := testDB(t)
	bad := seed(t, db, "bad", []string{"Apple", "手机"})
	good := seed(t, db, "good", []string{"Apple", "手机"})
	if err := db.Exec("CREATE TRIGGER reject_one BEFORE UPDATE OF metadata ON topic_tags WHEN OLD.label = 'bad' BEGIN SELECT RAISE(ABORT, 'test write failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	llm := func(context.Context, models.TopicTag) (string, error) { return validResponse, nil }
	stats, err := backfill(context.Background(), db, options{Workers: 1, Attempts: 1}, llm, io.Discard)
	if err == nil || stats.OK != 1 || stats.Failed != 1 {
		t.Fatalf("%+v %v", stats, err)
	}
	if err := db.First(&bad, bad.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&good, good.ID).Error; err != nil {
		t.Fatal(err)
	}
	if hasFingerprint(bad) || !hasFingerprint(good) {
		t.Fatal("per-tag rollback failed")
	}
}

func TestEmptySelectionAndCancellation(t *testing.T) {
	db := testDB(t)
	llm := func(context.Context, models.TopicTag) (string, error) { t.Fatal("unexpected call"); return "", nil }
	stats, err := backfill(context.Background(), db, options{Workers: 1, Attempts: 1}, llm, io.Discard)
	if err != nil || stats.Planned != 0 {
		t.Fatalf("%+v %v", stats, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backfill(ctx, db, options{Workers: 1, Attempts: 1}, llm, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBackfillParseFailureLogsBoundedRawResponse(t *testing.T) {
	for _, test := range []struct {
		name, response, prefix string
	}{
		{"empty", "", ""},
		{"prose", "not JSON\nsecond line", "not JSON\nsecond line"},
		{"truncated JSON", `{"subject":`, `{"subject":`},
		{"incomplete fingerprint", `{"subject":"Apple"}`, `{"subject":"Apple"}`},
		{"long unicode", strings.Repeat("文", 200) + "PRIVATE_SUFFIX", strings.Repeat("文", 200)},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := testDB(t)
			tag := seed(t, db, "test", nil)
			var out bytes.Buffer
			llm := func(context.Context, models.TopicTag) (string, error) { return test.response, nil }
			stats, err := backfill(context.Background(), db, options{Workers: 1, Attempts: 1}, llm, &out)
			if err == nil || stats.Failed != 1 {
				t.Fatalf("stats=%+v err=%v", stats, err)
			}
			if !strings.Contains(out.String(), fmt.Sprintf("raw_response=%q", test.prefix)) || strings.Contains(out.String(), "PRIVATE_SUFFIX") {
				t.Fatalf("unexpected log: %s", &out)
			}
			if err := db.First(&tag, tag.ID).Error; err != nil {
				t.Fatal(err)
			}
			if hasFingerprint(tag) {
				t.Fatal("failed parse persisted a fingerprint")
			}
		})
	}
}
