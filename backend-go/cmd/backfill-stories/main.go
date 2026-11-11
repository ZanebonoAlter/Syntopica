// Command backfill-stories was the story (事件线) backfill/repair CLI. Story
// online production is retired (retire-story-adopt-event-metrics D3): every
// WRITE mode is refused here with a non-zero exit, so "停写" cannot be bypassed
// by one command. The implementation files are kept on purpose — a code-level
// rollback re-enables them together with ai_settings.story_attribution_enabled.
//
// What still works is the READ-ONLY surface, which operators use to audit the
// retained historical data:
//
//	go run ./cmd/backfill-stories --dry-run                  # preview the retired backfill plan
//	go run ./cmd/backfill-stories --reattribute --dry-run     # preview a story-identity recompute
//	go run ./cmd/backfill-stories --rebuild-signals --dry-run # count evidence rows to rebuild
//	go run ./cmd/backfill-stories --fill-story-embeddings --dry-run
//	go run ./cmd/backfill-stories --fill-thread-embeddings --dry-run
//
// Every write mode (--reattribute / --restore / --rebuild-signals /
// --backfill-content-keys / --fill-story-embeddings / --fill-thread-embeddings
// and the default no-flag backfill) is rejected with「事件线已退役」.
//
// Historical note on the retired write modes: the date range selected stories
// through their reports; each selected story's entire evidence union was
// rebuilt. Dry-run verdicts used persisted candidates only and never simulated
// earlier would-be creates/merges. --backfill-content-keys filled
// story_signals.content_key from the normalized article link; --reattribute
// required a snapshot because the write was irreversible without it.
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

// storyRetiredNotice is the operator-facing explanation printed when a write
// mode is refused. Story online production is retired; the code stays for
// rollback, but no command may write story identity/evidence any more.
const storyRetiredNotice = "事件线已退役（retire-story-adopt-event-metrics）：停写后本命令的写模式一律拒绝。"

// retiredWriteModes lists the write modes requested by the operator. --restore
// has no read-only variant (it always writes), so it is refused even with
// --dry-run; the other modes keep their documented read-only preview when
// --dry-run is present (zero writes). The default no-flag mode is the original
// backfill writer and is therefore refused too — otherwise one bare command
// would bypass the ai_settings stop-write switch.
func retiredWriteModes(opts commandOptions) []string {
	modes := make([]string, 0, 7)
	if strings.TrimSpace(opts.RestorePath) != "" {
		modes = append(modes, "--restore")
	}
	if opts.DryRun {
		return modes // --reattribute/--rebuild-signals/... --dry-run 是只读预览
	}
	if opts.Reattribute {
		modes = append(modes, "--reattribute")
	}
	if opts.RebuildSignals {
		modes = append(modes, "--rebuild-signals")
	}
	if opts.BackfillContentKeys {
		modes = append(modes, "--backfill-content-keys")
	}
	if opts.FillStoryEmbeddings {
		modes = append(modes, "--fill-story-embeddings")
	}
	if opts.FillThreadEmbeddings {
		modes = append(modes, "--fill-thread-embeddings")
	}
	if len(modes) == 0 {
		modes = append(modes, "默认回填")
	}
	return modes
}

// rejectRetiredWriteModes turns every write request into a non-zero exit with
// the retirement notice. Read-only / dry-run runs pass through untouched.
func rejectRetiredWriteModes(opts commandOptions) error {
	if modes := retiredWriteModes(opts); len(modes) > 0 {
		return fmt.Errorf("%s 拒绝的写模式：%s。只读预览仍可用：为上述开关加 --dry-run（--restore 无只读模式）。",
			storyRetiredNotice, strings.Join(modes, "、"))
	}
	return nil
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
	// 停写后写模式一律拒绝（非零退出 + 退役说明）；只读 / --dry-run 预览保留。
	if err := rejectRetiredWriteModes(opts); err != nil {
		return opts, err
	}
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
