package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/service"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:fingerprints-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.TopicTag{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func seed(t *testing.T, db *gorm.DB, label string, keywords []string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: label, Category: "event", Aliases: `["Apple"]`, Metadata: models.MetadataMap{"event_keywords": keywords, "keep": "unchanged"}}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatal(err)
	}
	return tag
}

const validResponse = `{"subject":"Apple","subject_key":" COMPANY:Apple ","action":"发布","object":"手机","occurred_at":""}`

func TestBackfillWritesTrustAndSkipsSecondRun(t *testing.T) {
	db := testDB(t)
	a := seed(t, db, "苹果发布手机", []string{"苹果", "手机"})
	b := seed(t, db, "无关键词", nil)
	var calls atomic.Int32
	llm := func(context.Context, models.TopicTag) (string, error) { calls.Add(1); return validResponse, nil }
	opts := options{Workers: 2, Attempts: 1}
	for iteration := 0; iteration < 2; iteration++ {
		stats, err := backfill(context.Background(), db, opts, llm, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if iteration == 0 && (stats.OK != 2 || stats.Trusted != 1) {
			t.Fatalf("stats=%+v", stats)
		}
		if iteration == 1 && (stats.Skipped != 2 || stats.OK != 0) {
			t.Fatalf("stats=%+v", stats)
		}
	}
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	for _, tag := range []models.TopicTag{a, b} {
		if err := db.First(&tag, tag.ID).Error; err != nil {
			t.Fatal(err)
		}
		fp, complete := service.ParseEventFingerprint(tag)
		if !complete || fp.SubjectKey != "company:apple" || tag.Metadata["fingerprint_trusted"] != fp.Trusted || tag.Metadata["keep"] != "unchanged" {
			t.Fatalf("tag=%+v fp=%+v", tag, fp)
		}
	}
}
func TestBackfillFailureLimitAndDryRun(t *testing.T) {
	db := testDB(t)
	seed(t, db, "bad", []string{"Apple", "手机"})
	seed(t, db, "good", []string{"Apple", "手机"})
	seed(t, db, "later", []string{"Apple", "手机"})
	calls := 0
	llm := func(_ context.Context, tag models.TopicTag) (string, error) {
		calls++
		if tag.Label == "bad" {
			return "", errors.New("offline")
		}
		return validResponse, nil
	}
	opts := options{Workers: 1, Attempts: 2, Limit: 2, DryRun: true}
	stats, err := backfill(context.Background(), db, opts, llm, io.Discard)
	if err != nil || stats.Planned != 2 || calls != 0 {
		t.Fatalf("stats=%+v calls=%d err=%v", stats, calls, err)
	}
	var tags []models.TopicTag
	if err := db.Find(&tags).Error; err != nil {
		t.Fatal(err)
	}
	for _, tag := range tags {
		if hasFingerprint(tag) {
			t.Fatal("dry-run wrote fingerprint")
		}
	}
	opts.DryRun = false
	stats, err = backfill(context.Background(), db, opts, llm, io.Discard)
	if err == nil || stats.OK != 1 || stats.Failed != 1 || stats.Planned != 2 || calls != 3 {
		t.Fatalf("stats=%+v calls=%d err=%v", stats, calls, err)
	}
	var last models.TopicTag
	if err := db.First(&last, 3).Error; err != nil {
		t.Fatal(err)
	}
	if hasFingerprint(last) {
		t.Fatal("limit ignored")
	}
}
func TestFirstWriterWinsAndPreservesMetadata(t *testing.T) {
	db := testDB(t)
	tag := seed(t, db, "race", []string{"Apple", "手机"})
	llm := func(_ context.Context, _ models.TopicTag) (string, error) {
		tag.Metadata["event_fingerprint"] = map[string]any{"subject": "winner"}
		tag.Metadata["new_key"] = "preserved"
		data, _ := json.Marshal(tag.Metadata)
		return validResponse, db.Model(&models.TopicTag{}).Where("id = ?", tag.ID).UpdateColumn("metadata", string(data)).Error
	}
	stats, err := backfill(context.Background(), db, options{Workers: 1, Attempts: 1}, llm, io.Discard)
	if err != nil || stats.Skipped != 1 || stats.OK != 0 {
		t.Fatalf("%+v %v", stats, err)
	}
	if err := db.First(&tag, tag.ID).Error; err != nil {
		t.Fatal(err)
	}
	fp, _ := service.ParseEventFingerprint(tag)
	if fp.Subject != "winner" || tag.Metadata["new_key"] != "preserved" {
		t.Fatal(tag.Metadata)
	}
}
func TestOptions(t *testing.T) {
	for _, args := range [][]string{{"--limit", "-1"}, {"--workers", "0"}, {"--attempts", "0"}, {"unexpected"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatal(args)
		}
	}
	opts, err := parseOptions([]string{"--board", "7", "--limit", "2", "--dry-run"})
	if err != nil || opts.Board != 7 || opts.Limit != 2 || !opts.DryRun {
		t.Fatalf("%+v %v", opts, err)
	}
}

// Completeness and keyword trust remain the contract; entity granularity is
// not inferred from the spelling, length, or titles in a subject key.
func TestBackfillDoesNotApplySubjectKeyGranularityHeuristics(t *testing.T) {
	for _, key := range []string{"product:数据中心", "person:美国总统特朗普", "org:ExampleInternationalResearchInstitute"} {
		t.Run(key, func(t *testing.T) {
			db := testDB(t)
			tag := seed(t, db, "event", []string{"Apple", "手机"})
			response := strings.Replace(validResponse, " COMPANY:Apple ", key, 1)
			llm := func(context.Context, models.TopicTag) (string, error) { return response, nil }
			stats, err := backfill(t.Context(), db, options{Workers: 1, Attempts: 1}, llm, io.Discard)
			if err != nil || stats.OK != 1 || stats.Failed != 0 {
				t.Fatalf("stats=%+v err=%v", stats, err)
			}
			if err := db.First(&tag, tag.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !hasFingerprint(tag) {
				t.Fatal("complete response not persisted")
			}
		})
	}
}
