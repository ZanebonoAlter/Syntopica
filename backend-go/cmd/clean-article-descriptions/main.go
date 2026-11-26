// Command clean-article-descriptions rewrites articles.description that is out
// of the summary contract into the plain-text summary produced by
// internal/platform/htmltext.
//
// Two shapes are out of contract: a summary that still carries source-site
// markup (the whole article HTML was stored there), and a plain-text summary
// longer than the 300-rune cap. Anything else is left alone — a plain-text
// summary merely mentioning "<" is not markup, and rewriting it would change a
// value the spec promises to keep.
//
// Usage:
//
//	go run ./cmd/clean-article-descriptions           # dry-run: report only
//	go run ./cmd/clean-article-descriptions --apply   # write the cleaned rows
//
// The default is a dry-run: the command reports how many rows would change, a
// few samples and the storage that would be released, without touching the
// database. Only the description column is ever written — content and every
// other field stay untouched, because content is the body source for the
// content-source switch and the Firecrawl fallback chain (spec
// article-description-sanitization).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/htmltext"
	"syntopica-backend/internal/platform/logging"
)

// descriptionSummaryMaxRunes mirrors reader/service/feed_service.go: the
// stored summary is capped at 300 runes, same cleaning as the list lede.
const descriptionSummaryMaxRunes = 300

// batchPause is the idle time between two batches so the backfill does not
// monopolise the Raspberry Pi's I/O.
const batchPause = 200 * time.Millisecond

type options struct {
	Apply   bool
	Batch   int
	Limit   int
	Samples int
}

// candidate is one scanned row: only the columns the backfill needs.
type candidate struct {
	ID          uint
	Description string
}

// update is one row whose summary changes, with the value before and after.
type update struct {
	ID     uint
	Before string
	After  string
}

// stats is the backfill accounting reported to the operator.
type stats struct {
	Scanned     int
	Affected    int
	Updated     int
	BytesBefore int
	BytesAfter  int
}

func (s stats) releasedBytes() int {
	return s.BytesBefore - s.BytesAfter
}

func parseOptions(args []string) (options, error) {
	opts := options{}
	fs := flag.NewFlagSet("clean-article-descriptions", flag.ContinueOnError)
	fs.BoolVar(&opts.Apply, "apply", false, "write the cleaned summaries (default: dry-run, nothing is written)")
	fs.IntVar(&opts.Batch, "batch", 200, "rows per transaction (1-2000)")
	fs.IntVar(&opts.Limit, "limit", 0, "maximum number of candidate rows to scan (0 = unlimited)")
	fs.IntVar(&opts.Samples, "samples", 5, "number of sample rows to print (0-50)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 || opts.Batch < 1 || opts.Batch > 2000 || opts.Limit < 0 || opts.Samples < 0 || opts.Samples > 50 {
		return opts, fmt.Errorf("invalid options: batch 1-2000, limit >= 0, samples 0-50, no positional arguments")
	}
	return opts, nil
}

// planUpdates returns the rows whose cleaned summary differs from the stored
// one. Rows that are already clean are skipped — that is what makes a second
// run affect zero rows.
func planUpdates(rows []candidate, maxRunes int) ([]update, int, int) {
	updates := make([]update, 0, len(rows))
	bytesBefore, bytesAfter := 0, 0
	for _, row := range rows {
		// The SQL pre-filter is coarse ("contains a `<`" catches summaries that
		// merely mention a tag). The real test is the cleaner's own markup
		// detection, so a plain-text summary like "使用 <br> 标签" is left alone
		// instead of losing characters the spec promises to keep.
		if !htmltext.HasMarkup(row.Description) && len([]rune(row.Description)) <= maxRunes {
			continue
		}
		cleaned := htmltext.PlainSummary(row.Description, maxRunes)
		if cleaned == row.Description {
			continue
		}
		updates = append(updates, update{ID: row.ID, Before: row.Description, After: cleaned})
		bytesBefore += len(row.Description)
		bytesAfter += len(cleaned)
	}
	return updates, bytesBefore, bytesAfter
}

// scanCandidates reads the rows the backfill may rewrite, in batches ordered by
// id so the scan can resume where it stopped.
//
// The SQL narrows cheaply on dialect-neutral conditions — carries a '<', carries
// an escaped '<' (&lt;), or is longer than the summary cap — and the real
// decision is made in Go (planUpdates) with htmltext.HasMarkup. The filter MUST
// stay a superset of that Go judgment, otherwise a row the cleaner would rewrite
// is never scanned: HasMarkup decodes entities first, so "&lt;b&gt;x" is markup
// although it carries no literal '<'. Postgres regex (`~`) is not used on
// purpose: the tests run on sqlite and the two dialects do not agree on it.
func scanCandidates(ctx context.Context, db *gorm.DB, opts options, lastID uint, batch int, maxRunes int) ([]candidate, error) {
	var rows []candidate
	err := db.WithContext(ctx).
		Table("articles").
		Select("id, COALESCE(description, '') AS description").
		Where("id > ?", lastID).
		Where("description LIKE ? OR description LIKE ? OR length(description) > ?", "%<%", "%&lt;%", maxRunes).
		Order("id ASC").
		Limit(batch).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("scan candidates: %w", err)
	}
	return rows, nil
}

func writeUpdates(ctx context.Context, db *gorm.DB, updates []update) error {
	// One transaction per batch: a failure rolls back the batch only, and the
	// already committed batches stay.
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, u := range updates {
			if err := tx.Table("articles").
				Where("id = ?", u.ID).
				UpdateColumn("description", u.After).Error; err != nil {
				return fmt.Errorf("update article %d: %w", u.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("write batch: %w", err)
	}
	return nil
}

func printSample(out io.Writer, u update) {
	_, _ = fmt.Fprintf(out, "sample id=%d before=%q after=%q\n", u.ID, truncate(u.Before, 120), truncate(u.After, 120))
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

func run(ctx context.Context, db *gorm.DB, opts options, out io.Writer) (stats, error) {
	stats := stats{}
	lastID := uint(0)

	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		rows, err := scanCandidates(ctx, db, opts, lastID, opts.Batch, descriptionSummaryMaxRunes)
		if err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			break
		}
		lastID = rows[len(rows)-1].ID
		stats.Scanned += len(rows)

		updates, bytesBefore, bytesAfter := planUpdates(rows, descriptionSummaryMaxRunes)
		stats.Affected += len(updates)
		stats.BytesBefore += bytesBefore
		stats.BytesAfter += bytesAfter

		for i, u := range updates {
			if i < opts.Samples {
				printSample(out, u)
			}
		}

		if opts.Apply && len(updates) > 0 {
			if err := writeUpdates(ctx, db, updates); err != nil {
				return stats, err
			}
			stats.Updated += len(updates)
		}

		if opts.Limit > 0 && stats.Scanned >= opts.Limit {
			break
		}
		if !opts.Apply {
			// Dry-run only reads; no need to slow the database down.
			continue
		}
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		case <-time.After(batchPause):
		}
	}

	return stats, nil
}

func openDB() (*gorm.DB, error) {
	if err := config.LoadConfig("./configs"); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	logging.Init(config.AppConfig.Log.Level, logging.FileConfig{Enabled: false})
	// Do not use InitDB: the backfill must not migrate or seed anything.
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: config.AppConfig.Database.DSN, PreferSimpleProtocol: true}), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db (schema must already exist): %w", err)
	}
	return db, nil
}

// execute runs the backfill and returns the process exit code. It exists so
// that the deferred cleanup (signal context, DB pool) always runs before the
// process exits.
func execute() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	db, err := openDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = sqlDB.Close() }()

	mode := "dry-run"
	if opts.Apply {
		mode = "apply"
	}
	fmt.Printf("mode=%s candidates are rows whose description carries markup or exceeds %d runes\n", mode, descriptionSummaryMaxRunes)

	stats, err := run(ctx, db, opts, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	fmt.Printf("scanned=%d affected=%d updated=%d\n", stats.Scanned, stats.Affected, stats.Updated)
	fmt.Printf("description bytes before=%s after=%s released=%s\n",
		humanBytes(stats.BytesBefore), humanBytes(stats.BytesAfter), humanBytes(stats.releasedBytes()))
	if !opts.Apply {
		fmt.Println("dry-run: nothing was written, rerun with --apply to persist")
	}
	return 0
}

func main() {
	os.Exit(execute())
}

func humanBytes(size int) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
