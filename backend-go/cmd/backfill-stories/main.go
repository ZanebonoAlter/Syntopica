// Command backfill-stories fills historical story identity/evidence only.
// Run from backend-go after the server has applied the schema migrations:
//
//	go run ./cmd/backfill-stories --from 2026-10-01 --to 2026-11-01 --dry-run
//
// To repair evidence for already-attributed stories without changing identity:
//
//	go run ./cmd/backfill-stories --board 7 --from 2026-10-30 --to 2026-11-05 --rebuild-signals --dry-run
//
// To backfill the content-identity key of heat evidence (fix-duplicate-article-heat):
//
//	go run ./cmd/backfill-stories --backfill-content-keys --dry-run
//
// To backfill the recall vector of story lines that have none (D13):
//
//	go run ./cmd/backfill-stories --fill-story-embeddings --dry-run
//
// To backfill the recall vector of report threads that have none (D13):
//
//	go run ./cmd/backfill-stories --fill-thread-embeddings --dry-run
//
// To backfill threads of one board inside an explicit date window:
//
//	go run ./cmd/backfill-stories --fill-thread-embeddings --board 1980 --from 2026-10-01 --to 2026-11-01
//
// watch-materialized threads (lane_tier 'watch_*') carry no embedding by design and
// are skipped unless --include-watch-threads is given.
//
// To preview an explicit story-identity recompute (read-only, no writes):
//
//	go run ./cmd/backfill-stories --board 1980 --from 2026-10-12 --to 2026-11-11 --reattribute --dry-run
//
// To recompute the story identity of a board (a snapshot is mandatory — the
// write is irreversible without it):
//
//	go run ./cmd/backfill-stories --board 1980 --from 2026-10-12 --to 2026-11-11 --reattribute --snapshot /tmp/story-identity.jsonl
//
// To restore the story identity from such a snapshot (standalone mode, the
// date range is ignored):
//
//	go run ./cmd/backfill-stories --restore /tmp/story-identity.jsonl
//
// --backfill-content-keys fills story_signals.content_key from articles.link
// (normalized), then recomputes the participant count of every affected story
// line (source counts fall back to reality: one article copied by several feeds
// counts once). It is idempotent (only rows whose key differs are written; a
// story whose count already matches its evidence is not recomputed) and
// orthogonal to --rebuild-signals: rebuild rewrites whole evidence lines (and
// carries content_key along), this backfill covers the historical rows outside
// the rebuilt range.
//
// Remove --dry-run only after reviewing selected story counts. The date range
// selects stories through their reports; each selected story's entire evidence
// union is rebuilt. signals counts rebuilt rows (zero during dry-run).
//
// Dry-run verdicts use persisted candidates only; they do not simulate earlier
// would-be creates/merges. No report generation or display-tier dedup is run.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// printCrossDaySamples prints the cross-day continuation sample pairs of a
// dry run (the section header is greppable: "跨天延续样本对:").
func printCrossDaySamples(stats service.ReattributeStats) {
	fmt.Printf("跨天延续样本对: 共 %d 对（显示 %d 对）\n", stats.SamplePairs, len(stats.Pairs))
	for i, pair := range stats.Pairs {
		distance := "n/a"
		if pair.HasDistance {
			distance = fmt.Sprintf("%.4f", pair.Distance)
		}
		fmt.Printf("  #%d story=%d↔%d | %s/%s/%s ↔ %s/%s/%s | dates=%s↔%s | distance=%s\n",
			i+1, pair.StoryID, pair.PeerStoryID,
			pair.Subject, pair.Action, pair.Object,
			pair.PeerSubject, pair.PeerAction, pair.PeerObject,
			pair.FirstSeenDate, pair.PeerFirstSeenDate, distance)
	}
}

type commandOptions struct {
	service.BackfillOptions
	RebuildSignals       bool
	BackfillContentKeys  bool
	FillStoryEmbeddings  bool
	FillThreadEmbeddings bool
	// IncludeWatchThreads 打开后连 watch 物化板块的线程一起补（默认关闭：
	// 物化线程按设计没有向量，补了会把物化轨的召回从「文章集合」切到「向量」）。
	IncludeWatchThreads bool
	Reattribute         bool
	SnapshotPath        string
	RestorePath         string
	// ExplicitFrom / ExplicitTo 记录 --from/--to 是否被显式给出（默认值是
	// 「最近 30 天」窗口）。线程向量回填只在显式给出时才按日期过滤——默认窗口
	// 当回填范围会把更早的存量漏掉。
	ExplicitFrom bool
	ExplicitTo   bool
}

func parseOptions(args []string, now time.Time) (commandOptions, error) {
	opts := commandOptions{}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return opts, err
	}
	today := now.In(loc)
	fs := flag.NewFlagSet("backfill-stories", flag.ContinueOnError)
	board := fs.Uint("board", 0, "board ID (0 = all boards with reports)")
	from := fs.String("from", today.AddDate(0, 0, -30).Format(time.DateOnly), "first date, inclusive (YYYY-MM-DD, Asia/Shanghai)")
	to := fs.String("to", today.Format(time.DateOnly), "last date, inclusive (YYYY-MM-DD, Asia/Shanghai)")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "preview only; do not write")
	fs.BoolVar(&opts.RebuildSignals, "rebuild-signals", false, "rebuild evidence and counts for existing stories referenced by reports in range; never change attribution")
	fs.BoolVar(&opts.BackfillContentKeys, "backfill-content-keys", false, "backfill story_signals.content_key from articles.link (normalized; whole table, --from/--to not applied), then recompute participant counts of affected story lines; idempotent; orthogonal to --rebuild-signals")
	fs.BoolVar(&opts.FillStoryEmbeddings, "fill-story-embeddings", false, "backfill board_stories.embedding for story lines whose embedding IS NULL (whole table, --from/--to not applied); writes the embedding column only; --dry-run only counts")
	fs.BoolVar(&opts.FillThreadEmbeddings, "fill-thread-embeddings", false, "backfill daily_report_threads.embedding for report threads whose embedding IS NULL and title is non-empty; --board/--from/--to filter by report board / period_date when explicitly given (the default date window is NOT applied); watch-materialized threads (lane_tier 'watch_*') are excluded by default because they carry no embedding by design; writes the embedding column only; --dry-run only counts")
	fs.BoolVar(&opts.IncludeWatchThreads, "include-watch-threads", false, "with --fill-thread-embeddings: also backfill watch-materialized threads (lane_tier 'watch_*'), which carry no embedding BY DESIGN (their story attribution degrades to article-set recall); overriding this switches the watch track to vector recall")
	fs.BoolVar(&opts.Reattribute, "reattribute", false, "explicitly recompute the story identity of the range: reset the identity of non-manual events and re-attribute the reports in ascending date order; a writing run requires --snapshot")
	fs.StringVar(&opts.SnapshotPath, "snapshot", "", "snapshot file (JSONL, 0600) holding the event→story mapping and the full rows of the involved story lines; mandatory for a writing --reattribute run")
	fs.StringVar(&opts.RestorePath, "restore", "", "restore the event→story identity from a snapshot file (standalone mode; --from/--to are ignored)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	// 记录日期是否被显式给出（线程向量回填按此决定要不要按日期过滤）。
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "from":
			opts.ExplicitFrom = true
		case "to":
			opts.ExplicitTo = true
		}
	})
	restoreMode := strings.TrimSpace(opts.RestorePath) != ""
	opts.From, err = time.ParseInLocation(time.DateOnly, *from, loc)
	if err != nil {
		if !restoreMode {
			return opts, fmt.Errorf("invalid --from: %w", err)
		}
	}
	opts.To, err = time.ParseInLocation(time.DateOnly, *to, loc)
	if err != nil {
		if !restoreMode {
			return opts, fmt.Errorf("invalid --to: %w", err)
		}
	}
	if !restoreMode && opts.From.After(opts.To) {
		return opts, fmt.Errorf("--from must be <= --to")
	}
	if *board != 0 {
		opts.BoardID = board
	}
	if opts.Reattribute && !opts.DryRun && strings.TrimSpace(opts.SnapshotPath) == "" {
		return opts, errors.New("--reattribute writes the database: --snapshot <file> is required (the recompute is irreversible without it)")
	}
	return opts, nil
}

func run(ctx context.Context, args []string) error {
	opts, err := parseOptions(args, time.Now())
	if err != nil {
		return err
	}
	if err := config.LoadConfig("./configs"); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logging.Init(config.AppConfig.Log.Level, logging.FileConfig{Enabled: false})
	// Like dump-sanitizer, connect directly instead of InitDB: InitDB runs
	// migrations/seed writes, which must not happen implicitly in a dry run.
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: config.AppConfig.Database.DSN, PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open db (schema must already be migrated): %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	database.DB = db
	repository.InitRepository(db)
	if strings.TrimSpace(opts.RestorePath) != "" {
		// 独立模式：只还原「事件 → 事件线」归属，忽略 --from/--to。
		start := time.Now()
		stats, err := service.RestoreStoryIdentity(ctx, opts.RestorePath)
		fmt.Printf("restore threads=%d stories=%d stories_deleted=%d failed=%d elapsed=%s\n",
			stats.ThreadsRestored, stats.StoriesRestored, stats.StoriesDeleted, stats.Failed, time.Since(start).Round(time.Millisecond))
		if err != nil {
			return err
		}
		if stats.Failed > 0 {
			return fmt.Errorf("restore partially failed (%d errors); see warnings, rerun is safe (the restore is idempotent)", stats.Failed)
		}
		return nil
	}
	if opts.Reattribute {
		// 显式重算：默认不动历史，只在 --reattribute 生效；dry-run 只读。
		start := time.Now()
		stats, err := service.ReattributeStories(ctx, service.ReattributeOptions{
			BackfillOptions: opts.BackfillOptions,
			SnapshotPath:    opts.SnapshotPath,
		})
		fmt.Printf("reattribute dry-run=%t reports=%d threads_reset=%d created=%d attached=%d merged=%d stories_deleted=%d failed=%d resettable=%d old_stories=%d sample_pairs=%d elapsed=%s\n",
			stats.DryRun, stats.Reports, stats.ThreadsReset, stats.Created, stats.Attached, stats.Merged,
			stats.StoriesDeleted, stats.Failed, stats.ResettableThreads, stats.OldStories, stats.SamplePairs,
			time.Since(start).Round(time.Millisecond))
		if stats.DryRun {
			printCrossDaySamples(stats)
		}
		if err != nil {
			return err
		}
		if stats.Failed > 0 {
			return fmt.Errorf("reattribute partially failed (%d errors); see warnings, rerun is safe (the recompute is idempotent)", stats.Failed)
		}
		return nil
	}
	if opts.FillStoryEmbeddings {
		// 独立模式：只补 embedding IS NULL 的存量事件线，不碰归属 / 日报 / 其它列。
		start := time.Now()
		stats, err := fillStoryEmbeddings(ctx, db, opts)
		fmt.Printf("fill-story-embeddings dry-run=%t scanned=%d updated=%d skipped=%d failed=%d elapsed=%s\n",
			opts.DryRun, stats.Scanned, stats.Updated, stats.Skipped, stats.Failed, time.Since(start).Round(time.Millisecond))
		if err != nil {
			return err
		}
		if stats.Failed > 0 {
			return fmt.Errorf("fill story embeddings partially failed (%d errors); rerun is safe (only NULL rows are processed)", stats.Failed)
		}
		if !opts.FillThreadEmbeddings {
			return nil
		}
	}
	if opts.FillThreadEmbeddings {
		// 独立模式：只补 embedding IS NULL 的存量日报线程，不碰归属 / 关系 /
		// fit_distance / 其它列。两个回填开关可同时给：先补事件线、再补线程。
		start := time.Now()
		stats, err := fillThreadEmbeddings(ctx, db, opts)
		fmt.Printf("fill-thread-embeddings dry-run=%t scanned=%d updated=%d skipped=%d failed=%d elapsed=%s\n",
			opts.DryRun, stats.Scanned, stats.Updated, stats.Skipped, stats.Failed, time.Since(start).Round(time.Millisecond))
		if err != nil {
			return err
		}
		if stats.Failed > 0 {
			return fmt.Errorf("fill thread embeddings partially failed (%d errors); rerun is safe (only NULL rows are processed)", stats.Failed)
		}
		return nil
	}
	if opts.RebuildSignals {
		start := time.Now()
		stats, err := rebuildStorySignals(ctx, db, opts)
		fmt.Printf("rebuild-signals dry-run=%t selected=%d rebuilt=%d signals=%d elapsed=%s\n",
			opts.DryRun, stats.Selected, stats.Rebuilt, stats.Signals, time.Since(start).Round(time.Millisecond))
		if err != nil {
			return err
		}
		// 两个开关可同时给：先整线重建（signals 自然带上 content_key），
		// 再补齐重建范围之外的历史行。
		if !opts.BackfillContentKeys {
			return nil
		}
	}
	if opts.BackfillContentKeys {
		start := time.Now()
		stats, err := backfillContentKeys(ctx, db, opts)
		fmt.Printf("backfill-content-keys dry-run=%t scanned=%d updated=%d stories=%d elapsed=%s\n",
			opts.DryRun, stats.Scanned, stats.Updated, stats.Stories, time.Since(start).Round(time.Millisecond))
		return err
	}
	if !opts.DryRun {
		preview, err := service.PreviewStoryBackfill(ctx, opts.BackfillOptions)
		if err != nil {
			return err
		}
		fmt.Printf("将处理 %d 版块 / %d 日报\n", preview.Boards, preview.Reports)
	}
	start := time.Now()
	stats, err := service.BackfillStories(ctx, opts.BackfillOptions)
	fmt.Printf("dry-run=%t boards=%d reports=%d threads=%d created=%d attached=%d merged=%d skipped=%d failed=%d elapsed=%s\n",
		stats.DryRun, stats.Boards, stats.Reports, stats.Threads, stats.Created, stats.Attached, stats.Merged, stats.Skipped, stats.Failed, time.Since(start).Round(time.Millisecond))
	if err != nil {
		return err
	}
	if stats.Failed > 0 {
		return fmt.Errorf("backfill partially failed (%d errors); see warnings, rerun is safe", stats.Failed)
	}
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	cancel()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
