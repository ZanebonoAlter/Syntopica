// Command aux-label-dedup-parity is the one-off parity check for
// fix-aux-label-dedup-ann design D8: it replays stored auxiliary-label merge
// vectors through BOTH the legacy in-process full cosine scan and the new
// database ANN recall (+ exact rerank) path, then reports every disagreement.
//
// It is strictly read-only: the connection carries
// options='-c default_transaction_read_only=on', so even an accidental write
// would be rejected by PostgreSQL. Safe to run against the live database.
//
// It lives in cmd/ (not in the service package) on purpose: the frozen legacy
// algorithm must not be compiled into the server binary (review R-6).
//
// Usage:
//
//	go run ./cmd/aux-label-dedup-parity --sample 200
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"syntopica-backend/internal/platform/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "aux-label-dedup-parity: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("aux-label-dedup-parity", flag.ExitOnError)
	sample := fs.Int("sample", 200, "样本数：从库中现有 active 辅助标签回放的同数量向量")
	batch := fs.Int("batch", 512, "旧实现流式扫描的批大小（只影响内存占用）")
	configDir := fs.String("config", "./configs", "配置目录（读 database.dsn）")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if err := config.LoadConfig(*configDir); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	dsn := config.AppConfig.Database.DSN
	if dsn == "" {
		return fmt.Errorf("database.dsn is empty in config")
	}
	// Read-only session: the red line says no DDL/DML against the real database.
	dsn += " options='-c default_transaction_read_only=on'"

	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		defer func() { _ = sqlDB.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	report, err := RunDedupParity(ctx, db, DedupParityOptions{
		SampleSize: *sample,
		BatchSize:  *batch,
	})
	if err != nil {
		return err
	}
	printReport(report)
	return nil
}

func printReport(report *DedupParityReport) {
	fmt.Printf("== aux-label L2 dedup parity (fix-aux-label-dedup-ann D8) ==\n")
	fmt.Printf("samples                : %d\n", report.SampleSize)
	fmt.Printf("active auxiliary labels: %d\n", report.ActiveLabels)
	fmt.Printf("dimension              : %d\n", report.Dimension)
	fmt.Printf("threshold              : %.4f\n", report.Threshold)
	fmt.Printf("recall plan uses index : %t\n", report.RecallPlanUsesIndex)
	fmt.Printf("recall plan            :\n")
	for _, line := range report.RecallPlan {
		fmt.Printf("    %s\n", line)
	}
	fmt.Printf("samples with ≥2 peer neighbors within threshold: %d\n", report.SamplesWithMultiNeighbor)
	fmt.Printf("mismatches             : %d\n", len(report.Mismatches))
	for _, m := range report.Mismatches {
		fmt.Printf("  - sample %d %q\n", m.SampleID, m.SampleLabel)
		fmt.Printf("      legacy : %s\n", describeTarget(m.Legacy))
		fmt.Printf("      current: %s\n", describeTarget(m.Current))
		fmt.Printf("      neighbors(legacy) = %d\n", m.NeighborCount)
		fmt.Printf("      attribution: %s\n", m.Attribution)
	}
	if len(report.Mismatches) == 0 {
		fmt.Printf("mismatch=0\n")
	}
	fmt.Printf("elapsed                : %s\n", report.Elapsed.Round(time.Millisecond))
}

func describeTarget(t DedupParityTarget) string {
	if !t.Found {
		return "<none>"
	}
	return fmt.Sprintf("id=%d label=%q ref_count=%d sim=%.9f", t.ID, t.Label, t.RefCount, t.Sim)
}
