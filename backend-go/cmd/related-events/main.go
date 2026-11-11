// Command related-events runs the two explicitly separated related-event
// operations of enable-story-cross-day-continuation:
//
//	# 1) Explicit index refresh (the ONLY writer of related_event_embeddings):
//	go run ./cmd/related-events --refresh --board 1980
//	go run ./cmd/related-events --refresh --board 1980 --generation g2
//
//	# 2) Read-only historical check (S3): no cache/business writes, only the
//	#    normal audited airouter embedding calls:
//	go run ./cmd/related-events --experiment --board 1980 --out /tmp/related-events-1980.json
//
// The modes are mutually exclusive by design (design D4/D8): refresh indexes the
// current event tags; experiment re-embeds a historical window in memory and
// prints auditable pairs. Neither mode touches tags, auxiliary links, articles,
// threads, story identity or the legacy vectors.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/service/related"
)

const (
	// ModeRefresh builds/repairs the derived vector index for one board.
	ModeRefresh = "refresh"
	// ModeExperiment runs the read-only historical similarity check.
	ModeExperiment = "experiment"
)

// Options are the parsed CLI options of one run.
type Options struct {
	Mode        string
	BoardID     uint
	Generation  string
	Threshold   float64
	WindowDays  int
	SampleLimit int
	NearLimit   int
	OutPath     string
}

// parseOptions validates the mode separation and the required arguments.
func parseOptions(args []string) (*Options, error) {
	fs := flag.NewFlagSet("related-events", flag.ContinueOnError)
	refresh := fs.Bool("refresh", false, "explicitly (re)build the related-event vector index of --board")
	experiment := fs.Bool("experiment", false, "run the read-only historical similarity check (no DB writes)")
	board := fs.Uint("board", 0, "semantic board id (required)")
	generation := fs.String("generation", "", "explicit index generation (refresh only; persisted as related_event_index_generation)")
	threshold := fs.Float64("threshold", -1, fmt.Sprintf(
		"similarity threshold (experiment; default: current ai_settings %s, else %.2f)",
		related.ThresholdKey, related.DefaultThreshold))
	windowDays := fs.Int("window-days", related.WindowDays, "candidate window in days (experiment)")
	sampleLimit := fs.Int("samples", 50, "max qualifying pairs printed by the experiment")
	nearLimit := fs.Int("near", 10, "max below-threshold pairs printed by the experiment")
	out := fs.String("out", "", "optional JSON output path")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	opts := &Options{
		BoardID:     *board,
		Generation:  strings.TrimSpace(*generation),
		Threshold:   *threshold,
		WindowDays:  *windowDays,
		SampleLimit: *sampleLimit,
		NearLimit:   *nearLimit,
		OutPath:     strings.TrimSpace(*out),
	}
	switch {
	case *refresh && *experiment:
		return nil, errors.New("--refresh and --experiment are mutually exclusive (index refresh writes; the experiment is read-only)")
	case *refresh:
		opts.Mode = ModeRefresh
	case *experiment:
		opts.Mode = ModeExperiment
	default:
		return nil, errors.New("one mode is required: --refresh or --experiment")
	}
	if opts.BoardID == 0 {
		return nil, errors.New("--board is required")
	}
	if opts.Mode == ModeExperiment {
		if opts.WindowDays <= 0 {
			return nil, errors.New("--window-days must be positive")
		}
		if opts.SampleLimit <= 0 || opts.NearLimit < 0 {
			return nil, errors.New("--samples must be positive and --near must not be negative")
		}
		// -1 是「未指定」哨兵：显式负数（含 -0.5）一律拒绝，不静默当缺省。
		if opts.Threshold > 1 || (opts.Threshold < 0 && opts.Threshold != -1) {
			return nil, errors.New("--threshold must be within [0, 1]")
		}
	}
	return opts, nil
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

func run(ctx context.Context, args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if err := config.LoadConfig("./configs"); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logging.Init(config.AppConfig.Log.Level, logging.FileConfig{Enabled: false})
	// Connect directly instead of InitDB: running migrations/seed implicitly is
	// not this tool's job (the server has already applied them).
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

	switch opts.Mode {
	case ModeRefresh:
		return runRefresh(ctx, db, opts)
	case ModeExperiment:
		return runExperiment(ctx, db, opts)
	default:
		return fmt.Errorf("unknown mode %q", opts.Mode)
	}
}

func runRefresh(ctx context.Context, db *gorm.DB, opts *Options) error {
	if opts.Generation != "" {
		if err := related.SetGeneration(db, opts.Generation); err != nil {
			return err
		}
	}
	generation := related.LoadGeneration(db)
	started := time.Now()
	result, err := related.NewRefresher(db).RefreshBoard(ctx, opts.BoardID, generation)
	if err != nil {
		return err
	}
	fmt.Printf("refresh board=%d generation=%s space=%s tags=%d indexed=%d reused=%d failed=%d elapsed=%s\n",
		result.BoardID, generation, result.SpaceKey, result.TagCount, result.Indexed, result.Reused, result.Failed,
		time.Since(started).Round(time.Millisecond))
	for _, failure := range result.Failures {
		fmt.Printf("  FAILED tag=%d label=%q reason=%s error=%s\n", failure.TagID, failure.Label, failure.Reason, failure.Error)
	}
	if result.Failed > 0 {
		fmt.Println("failed items keep their previous cache row (if any) and can be retried by re-running this command")
	}
	return nil
}

// resolveThreshold fills the experiment threshold. An explicit --threshold wins;
// otherwise the CURRENT ai_settings value is used (the same reader the HTTP
// endpoint and the UI use), so an operator who tuned the threshold is not
// silently shown results for the compiled-in default. LoadThreshold itself
// falls back to DefaultThreshold when the setting is missing/invalid.
func resolveThreshold(opts *Options, load func() float64) {
	if opts.Threshold < 0 {
		opts.Threshold = load()
	}
}

func runExperiment(ctx context.Context, db *gorm.DB, opts *Options) error {
	resolveThreshold(opts, func() float64 { return related.LoadThreshold(db) })
	result, err := related.NewExperiment(db).Run(ctx, related.ExperimentOptions{
		BoardID:     opts.BoardID,
		WindowDays:  opts.WindowDays,
		Threshold:   opts.Threshold,
		SampleLimit: opts.SampleLimit,
		NearLimit:   opts.NearLimit,
	})
	if err != nil {
		return err
	}
	printExperiment(result)
	if opts.OutPath != "" {
		raw, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(opts.OutPath, append(raw, '\n'), 0o600); err != nil { // #nosec G703 -- --out is explicitly chosen by the local CLI operator; no remote input reaches this exporter.
			return fmt.Errorf("write %s: %w", opts.OutPath, err)
		}
		fmt.Printf("JSON written to %s\n", opts.OutPath)
	}
	return nil
}

func printExperiment(result *related.ExperimentResult) {
	fmt.Printf("experiment board=%d representation=%s provider=%s model=%s threshold=%.2f window=%dd as_of=%s\n",
		result.BoardID, result.RepresentationVersion, result.Provider, result.Model,
		result.Threshold, result.WindowDays, result.AsOf)
	fmt.Printf("tags=%d embedded=%d failed=%d pairs=%d above_threshold=%d cache_writes=%d\n",
		result.TagCount, result.Embedded, result.Failed, result.PairCount, result.AboveCount, result.CacheWrites)
	fmt.Println("sample rule:", result.SampleRule)
	if result.Failed > 0 {
		for _, failure := range result.Failures {
			fmt.Printf("  FAILED tag=%d label=%q reason=%s error=%s\n", failure.TagID, failure.Label, failure.Reason, failure.Error)
		}
	}
	fmt.Println("--- direct pairs (score >= threshold) ---")
	printPairs(result.Direct)
	fmt.Println("--- near-threshold pairs (score < threshold) ---")
	printPairs(result.Near)
	fmt.Println("note: judge each pair by \"is it worth reading these two together\"; this is relevance, not the same real-world event")
}

func printPairs(pairs []related.ExperimentPair) {
	sorted := make([]related.ExperimentPair, len(pairs))
	copy(sorted, pairs)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Score != sorted[j].Score {
			return sorted[i].Score > sorted[j].Score
		}
		if sorted[i].AID != sorted[j].AID {
			return sorted[i].AID < sorted[j].AID
		}
		return sorted[i].BID < sorted[j].BID
	})
	if len(sorted) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, pair := range sorted {
		fmt.Printf("  %.4f  [%d] %s  ⟷  [%d] %s\n", pair.Score, pair.AID, pair.ALabel, pair.BID, pair.BLabel)
		if len(pair.AAux) > 0 || len(pair.BAux) > 0 {
			fmt.Printf("          aux: %s | %s\n", strings.Join(pair.AAux, "、"), strings.Join(pair.BAux, "、"))
		}
	}
}
