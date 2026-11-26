// Command calibrate-heat prints radar threshold samples; it never migrates,
// generates reports, updates settings, or writes frontend constants.
// Run from backend-go with the already-migrated database:
//
//	go run ./cmd/calibrate-heat --days 30 --board 0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/topicgraph/repository"
)

type options struct {
	Days  int
	Board uint
}

func parseOptions(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("calibrate-heat", flag.ContinueOnError)
	fs.IntVar(&opts.Days, "days", 30, "calendar days ending at each board's latest report date")
	fs.UintVar(&opts.Board, "board", 0, "board ID (0 = all boards with reports)")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.Days <= 0 || fs.NArg() != 0 {
		return opts, fmt.Errorf("--days must be positive; no positional arguments accepted")
	}
	return opts, nil
}

// percentile uses the nearest-rank method: rank = ceil(p*N), one-based.
// The input must be ascending. Empty input returns 0; p is clamped to [0,1].
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	return sorted[int(math.Ceil(p*float64(len(sorted))))-1]
}

func collectSamples(ctx context.Context, db *gorm.DB, opts options) ([]float64, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, err
	}
	q := db.WithContext(ctx).Model(&repository.BoardDailyReport{})
	if opts.Board != 0 {
		q = q.Where("semantic_board_id = ?", opts.Board)
	}
	// A formatted MAX(date) is portable across PG DATE and SQLite timestamp
	// storage (aggregate expressions lose SQLite's time scanning metadata).
	var boards []struct {
		BoardID  uint
		LastDate string
	}
	if err := q.Select("semantic_board_id AS board_id, MAX(period_date) AS last_date").
		Group("semantic_board_id").Order("semantic_board_id").Scan(&boards).Error; err != nil {
		return nil, err
	}
	repo := repository.NewTopicGraphRepository(db.WithContext(ctx))
	var samples []float64
	for _, board := range boards {
		if len(board.LastDate) < len(time.DateOnly) {
			return nil, fmt.Errorf("board %d: invalid last report date %q", board.BoardID, board.LastDate)
		}
		latest, err := time.ParseInLocation(time.DateOnly, board.LastDate[:len(time.DateOnly)], loc)
		if err != nil {
			return nil, err
		}
		for offset := 0; offset < opts.Days; offset++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			dayEnd := latest.AddDate(0, 0, -offset+1).Add(-time.Nanosecond)
			_, lanes, err := repo.ComputeBoardHeat(board.BoardID, dayEnd)
			if err != nil {
				return nil, fmt.Errorf("board %d at %s: %w", board.BoardID, dayEnd.Format(time.DateOnly), err)
			}
			for _, heat := range lanes {
				if heat > 0 {
					samples = append(samples, heat)
				}
			}
		}
	}
	sort.Float64s(samples)
	return samples, nil
}

func run(ctx context.Context, args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	// The shared config loader prints fallback/env diagnostics with fmt.Print.
	// Reserve stdout for JSON, including when no config file is present.
	output := os.Stdout
	os.Stdout = os.Stderr
	defer func() { os.Stdout = output }()
	if err := config.LoadConfig("./configs"); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	// Existing dump-sanitizer connection convention: intentionally NOT
	// database.InitDB, which performs schema migrations and seed writes.
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: config.AppConfig.Database.DSN, PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	samples, err := collectSamples(ctx, db, opts)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		Samples     int     `json:"samples"`
		P20         float64 `json:"p20"`
		P80         float64 `json:"p80"`
		RuleVersion string  `json:"rule_version"`
	}{len(samples), percentile(samples, 0.2), percentile(samples, 0.8), repository.HeatRuleVersion})
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
