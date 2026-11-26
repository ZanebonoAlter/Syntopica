package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseOptionsDates(t *testing.T) {
	now := time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC) // Nov 6 in Shanghai
	// 默认回填是写模式（停写后拒绝），只读等价的默认调用带 --dry-run。
	opts, err := parseOptions([]string{"--dry-run"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if opts.From.Format(time.DateOnly) != "2026-10-07" || opts.To.Format(time.DateOnly) != "2026-11-06" || opts.BoardID != nil || !opts.DryRun {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
	opts, err = parseOptions([]string{"--board", "7", "--from", "2026-11-05", "--to", "2026-11-05", "--dry-run"}, now)
	if err != nil || opts.BoardID == nil || *opts.BoardID != 7 || !opts.DryRun || !opts.From.Equal(opts.To) {
		t.Fatalf("same-day/flags: %+v, %v", opts, err)
	}
	opts, err = parseOptions([]string{"--fill-story-embeddings", "--dry-run"}, now)
	if err != nil || !opts.FillStoryEmbeddings || !opts.DryRun {
		t.Fatalf("fill-story-embeddings --dry-run: %+v, %v", opts, err)
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
	opts, err := parseOptions([]string{"--fill-thread-embeddings", "--dry-run"}, now)
	if err != nil || !opts.FillThreadEmbeddings {
		t.Fatalf("fill-thread-embeddings: %+v, %v", opts, err)
	}
	if opts.ExplicitFrom || opts.ExplicitTo {
		t.Fatalf("default date window must not count as explicit: %+v", opts)
	}
	opts, err = parseOptions([]string{"--fill-thread-embeddings", "--from", "2026-11-01", "--to", "2026-11-10", "--dry-run"}, now)
	if err != nil || !opts.ExplicitFrom || !opts.ExplicitTo {
		t.Fatalf("explicit date range: %+v, %v", opts, err)
	}
	if !opts.From.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, opts.From.Location())) {
		t.Fatalf("unexpected from: %+v", opts)
	}
	// 两个回填开关可同时给（顺序执行：先 story 后 thread）。
	opts, err = parseOptions([]string{"--fill-story-embeddings", "--fill-thread-embeddings", "--dry-run"}, now)
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
	// 停写后写模式直接拒绝（不论是否给快照）：退役说明 + 非零退出。
	if _, err := parseOptions([]string{"--reattribute", "--snapshot", "/tmp/story-identity.jsonl"}, now); err == nil || !strings.Contains(err.Error(), "事件线已退役") {
		t.Fatalf("write reattribute must be rejected: %v", err)
	}
	if _, err := parseOptions([]string{"--reattribute"}, now); err == nil || !strings.Contains(err.Error(), "事件线已退役") {
		t.Fatal("expected retirement rejection: --reattribute without --snapshot")
	}
}

// TestParseOptionsRestore 还原没有只读模式：停写后一律拒绝（--dry-run 也不放行）。
func TestParseOptionsRestore(t *testing.T) {
	now := time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC)
	for _, args := range [][]string{
		{"--restore", "/tmp/story-identity.jsonl"},
		{"--restore", "/tmp/story-identity.jsonl", "--dry-run"},
	} {
		if _, err := parseOptions(args, now); err == nil || !strings.Contains(err.Error(), "事件线已退役") {
			t.Fatalf("restore must be rejected (%v): %v", args, err)
		}
	}
}

// TestParseOptionsWriteModesRejected covers retire-story-adopt-event-metrics
// task 1.2: 六个写模式（以及默认回填写模式）在停写后一律拒绝、非零退出、
// 打印「事件线已退役」；只读 / --dry-run 预览仍可用且零写入。
func TestParseOptionsWriteModesRejected(t *testing.T) {
	now := time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC)
	writeModes := [][]string{
		{"--reattribute", "--snapshot", "/tmp/s.jsonl"},
		{"--reattribute"},
		{"--restore", "/tmp/s.jsonl"},
		{"--rebuild-signals"},
		{"--backfill-content-keys"},
		{"--fill-story-embeddings"},
		{"--fill-thread-embeddings"},
		{"--fill-thread-embeddings", "--include-watch-threads"},
		{}, // 默认回填写模式
	}
	for _, args := range writeModes {
		if _, err := parseOptions(args, now); err == nil {
			t.Fatalf("写模式必须拒绝：%v", args)
		} else if !strings.Contains(err.Error(), "事件线已退役") {
			t.Fatalf("写模式必须打印退役说明（%v）：%v", args, err)
		}
	}
	readOnly := [][]string{
		{"--dry-run"},
		{"--reattribute", "--dry-run"},
		{"--rebuild-signals", "--dry-run"},
		{"--backfill-content-keys", "--dry-run"},
		{"--fill-story-embeddings", "--dry-run"},
		{"--fill-thread-embeddings", "--dry-run"},
		{"--fill-thread-embeddings", "--include-watch-threads", "--dry-run"},
	}
	for _, args := range readOnly {
		opts, err := parseOptions(args, now)
		if err != nil {
			t.Fatalf("只读模式必须保留（%v）：%v", args, err)
		}
		if !opts.DryRun {
			t.Fatalf("只读模式必须带 --dry-run：%v", args)
		}
	}
}
