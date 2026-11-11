package main

import (
	"testing"
	"time"
)

func TestParseOptionsDates(t *testing.T) {
	now := time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC) // Nov 6 in Shanghai
	opts, err := parseOptions(nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if opts.From.Format(time.DateOnly) != "2026-10-07" || opts.To.Format(time.DateOnly) != "2026-11-06" || opts.BoardID != nil || opts.DryRun {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
	opts, err = parseOptions([]string{"--board", "7", "--from", "2026-11-05", "--to", "2026-11-05", "--dry-run"}, now)
	if err != nil || opts.BoardID == nil || *opts.BoardID != 7 || !opts.DryRun || !opts.From.Equal(opts.To) {
		t.Fatalf("same-day/flags: %+v, %v", opts, err)
	}
	opts, err = parseOptions([]string{"--fill-story-embeddings"}, now)
	if err != nil || !opts.FillStoryEmbeddings {
		t.Fatalf("fill-story-embeddings: %+v, %v", opts, err)
	}
	for _, args := range [][]string{
		{"--from", "2026-11-06", "--to", "2026-11-05"},
		{"--from", "2026-02-30"}, {"--to", "Nov-05"}, {"--board", "-1"},
		{"--unknown"}, {"extra"},
	} {
		if _, err := parseOptions(args, now); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

// TestParseOptionsFillThreadEmbeddings 线程向量回填开关：独立模式；日期过滤只在
// 显式给出 --from/--to 时生效（默认「最近 30 天」窗口不是回填范围）。
func TestParseOptionsFillThreadEmbeddings(t *testing.T) {
	now := time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC)
	opts, err := parseOptions([]string{"--fill-thread-embeddings"}, now)
	if err != nil || !opts.FillThreadEmbeddings {
		t.Fatalf("fill-thread-embeddings: %+v, %v", opts, err)
	}
	if opts.ExplicitFrom || opts.ExplicitTo {
		t.Fatalf("default date window must not count as explicit: %+v", opts)
	}
	opts, err = parseOptions([]string{"--fill-thread-embeddings", "--from", "2026-11-01", "--to", "2026-11-10"}, now)
	if err != nil || !opts.ExplicitFrom || !opts.ExplicitTo {
		t.Fatalf("explicit date range: %+v, %v", opts, err)
	}
	if !opts.From.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, opts.From.Location())) {
		t.Fatalf("unexpected from: %+v", opts)
	}
	// 两个回填开关可同时给（顺序执行：先 story 后 thread）。
	opts, err = parseOptions([]string{"--fill-story-embeddings", "--fill-thread-embeddings"}, now)
	if err != nil || !opts.FillStoryEmbeddings || !opts.FillThreadEmbeddings {
		t.Fatalf("both fill switches: %+v, %v", opts, err)
	}
}

// TestParseOptionsReattribute 重算开关：写模式必须配快照文件；dry-run 不用。
func TestParseOptionsReattribute(t *testing.T) {
	now := time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC)
	opts, err := parseOptions([]string{"--reattribute", "--dry-run"}, now)
	if err != nil {
		t.Fatalf("dry-run reattribute: %v", err)
	}
	if !opts.Reattribute || !opts.DryRun || opts.SnapshotPath != "" {
		t.Fatalf("unexpected options: %+v", opts)
	}
	opts, err = parseOptions([]string{"--reattribute", "--snapshot", "/tmp/story-identity.jsonl"}, now)
	if err != nil {
		t.Fatalf("write reattribute: %v", err)
	}
	if !opts.Reattribute || opts.DryRun || opts.SnapshotPath != "/tmp/story-identity.jsonl" {
		t.Fatalf("unexpected options: %+v", opts)
	}
	// 写模式没有快照 → 直接报错（防不可逆）。
	if _, err := parseOptions([]string{"--reattribute"}, now); err == nil {
		t.Fatal("expected error: --reattribute without --snapshot")
	}
	if _, err := parseOptions([]string{"--reattribute", "--snapshot", "  "}, now); err == nil {
		t.Fatal("expected error: blank --snapshot")
	}
}

// TestParseOptionsRestore 还原是独立模式：忽略 --from/--to。
func TestParseOptionsRestore(t *testing.T) {
	now := time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC)
	opts, err := parseOptions([]string{"--restore", "/tmp/story-identity.jsonl"}, now)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if opts.RestorePath != "/tmp/story-identity.jsonl" {
		t.Fatalf("unexpected restore path: %+v", opts)
	}
	// 还原模式忽略日期：非法日期也不报错（范围不参与还原）。
	opts, err = parseOptions([]string{"--restore", "/tmp/s.jsonl", "--from", "not-a-date", "--to", "also-bad"}, now)
	if err != nil {
		t.Fatalf("restore ignores date range: %v", err)
	}
	if !opts.From.IsZero() || !opts.To.IsZero() {
		t.Fatalf("restore mode must not use the date range: %+v", opts)
	}
}
