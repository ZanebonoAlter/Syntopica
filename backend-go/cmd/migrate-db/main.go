package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"my-robot-backend/internal/platform/config"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/database/datamigrate"
	"my-robot-backend/internal/platform/logging"
)

type cliOptions struct {
	Mode        datamigrate.Mode
	SQLitePath  string
	PostgresDSN string
	Force       bool
}

func main() {
	options, err := parseFlags()
	if err != nil {
		logging.Fatalf("Invalid flags: %v", err)
	}
	if err := validateExecuteSafety(options); err != nil {
		logging.Fatalf("Invalid flags: %v", err)
	}

	if err := config.LoadConfig("./configs"); err != nil {
		logging.Warnf("Warning: failed to load config: %v", err)
	}

	ctx := context.Background()
	targetDB, reader, writer, err := openDatabases(options)
	if err != nil {
		logging.Fatalf("Failed to open databases: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if modePreparesTargetBeforeResolvingSpecs(options.Mode) {
		if err := prepareTargetSchema(ctx, targetDB); err != nil {
			logging.Fatalf("Failed to prepare target schema: %v", err)
		}
	}

	specs, summaries := resolveSpecsAndSummaries(ctx, options.Mode, reader, writer)

	switch options.Mode {
	case datamigrate.ModeDryRun:
		return
	case datamigrate.ModeExecute:
		if !modeNeedsTargetPreparation(options.Mode) {
			logging.Fatalf("invalid target preparation state for mode %s", options.Mode)
		}
		if err := executeImport(ctx, targetDB, reader, writer, specs, summaries); err != nil {
			logging.Fatalf("Import failed: %v", err)
		}
	}

	report, err := verify(ctx, reader, writer, specs)
	if err != nil {
		logging.Fatalf("Verification failed: %v", err)
	}

	printVerification(report)
}

func resolveSpecsAndSummaries(ctx context.Context, mode datamigrate.Mode, reader *datamigrate.SQLiteReader, writer *datamigrate.PostgresWriter) ([]datamigrate.TableSpec, []datamigrate.TableSummary) {
	specs, err := activeSpecs(ctx, reader, writer, datamigrate.DefaultTableSpecs())
	if err != nil {
		logging.Fatalf("Failed to resolve import order: %v", err)
	}

	summaries, err := dryRun(ctx, reader, specs)
	if err != nil {
		logging.Fatalf("Failed to inspect source tables: %v", err)
	}

	printSummaries(mode, summaries)
	return specs, summaries
}

func parseFlags() (*cliOptions, error) {
	modeFlag := flag.String("mode", string(datamigrate.ModeDryRun), "Mode: dry-run, execute, verify-only")
	sqlitePath := flag.String("sqlite-path", "", "Path to the legacy SQLite database")
	postgresDSN := flag.String("postgres-dsn", "", "Postgres DSN for the target database")
	force := flag.Bool("force", false, "Required for execute mode because it truncates target tables")
	flag.Parse()

	mode, err := parseMode(*modeFlag)
	if err != nil {
		return nil, err
	}

	return &cliOptions{
		Mode:        mode,
		SQLitePath:  strings.TrimSpace(*sqlitePath),
		PostgresDSN: strings.TrimSpace(*postgresDSN),
		Force:       *force,
	}, nil
}

func validateExecuteSafety(options *cliOptions) error {
	if options == nil {
		return fmt.Errorf("options are required")
	}
	if options.Mode == datamigrate.ModeExecute && !options.Force {
		return fmt.Errorf("execute mode requires --force")
	}
	return nil
}

func openDatabases(options *cliOptions) (*gorm.DB, *datamigrate.SQLiteReader, *datamigrate.PostgresWriter, error) {
	sqlitePath := options.SQLitePath
	postgresDSN := options.PostgresDSN
	if config.AppConfig != nil {
		if sqlitePath == "" {
			sqlitePath = config.AppConfig.Database.DSN
		}
		if postgresDSN == "" && strings.EqualFold(config.AppConfig.Database.Driver, "postgres") {
			postgresDSN = config.AppConfig.Database.DSN
		}
	}

	if sqlitePath == "" {
		return nil, nil, nil, errors.New("sqlite-path is required")
	}
	if postgresDSN == "" {
		return nil, nil, nil, errors.New("postgres-dsn is required")
	}
	if _, err := os.Stat(sqlitePath); err != nil {
		return nil, nil, nil, fmt.Errorf("sqlite source %q: %w", sqlitePath, err)
	}

	reader, err := datamigrate.NewSQLiteReader(sqlitePath)
	if err != nil {
		return nil, nil, nil, err
	}

	targetDB, err := openTargetPostgres(postgresDSN)
	if err != nil {
		_ = reader.Close()
		return nil, nil, nil, err
	}

	writer, err := datamigrate.NewPostgresWriter(targetDB)
	if err != nil {
		_ = reader.Close()
		return nil, nil, nil, err
	}

	return targetDB, reader, writer, nil
}

func openTargetPostgres(dsn string) (*gorm.DB, error) {
	logLevel := logger.Silent
	if config.AppConfig != nil && config.AppConfig.Server.Mode == "debug" {
		logLevel = logger.Info
	}

	cstZone := time.FixedZone("CST", 8*3600)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NowFunc: func() time.Time {
			return time.Now().In(cstZone)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres target: %w", err)
	}

	return db, nil
}

func prepareTargetSchema(ctx context.Context, db *gorm.DB) error {
	if err := database.RunMigrations(db); err != nil {
		return fmt.Errorf("run postgres migrations: %w", err)
	}

	statement := `
		CREATE TABLE IF NOT EXISTS topic_analysis_jobs (
			id VARCHAR(64) PRIMARY KEY,
			topic_tag_id BIGINT NOT NULL,
			analysis_type VARCHAR(32),
			window_type VARCHAR(32),
			anchor_date TIMESTAMP,
			priority INTEGER,
			status VARCHAR(32),
			retry_count INTEGER,
			error_message TEXT,
			progress INTEGER,
			created_at TIMESTAMP,
			started_at TIMESTAMP,
			completed_at TIMESTAMP
		)
	`
	if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
		return fmt.Errorf("ensure topic_analysis_jobs: %w", err)
	}
	for _, indexStmt := range []string{
		"CREATE INDEX IF NOT EXISTS idx_topic_analysis_job_lookup ON topic_analysis_jobs(topic_tag_id, analysis_type, window_type)",
		"CREATE INDEX IF NOT EXISTS idx_topic_analysis_job_status ON topic_analysis_jobs(status)",
	} {
		if err := db.WithContext(ctx).Exec(indexStmt).Error; err != nil {
			return fmt.Errorf("ensure topic_analysis_jobs index: %w", err)
		}
	}

	return nil
}

func activeSpecs(ctx context.Context, reader *datamigrate.SQLiteReader, writer *datamigrate.PostgresWriter, specs []datamigrate.TableSpec) ([]datamigrate.TableSpec, error) {
	sourceTables, err := reader.ExistingTables(ctx)
	if err != nil {
		return nil, err
	}
	targetTables, err := writer.ExistingTables(ctx)
	if err != nil {
		return nil, err
	}

	active := make([]datamigrate.TableSpec, 0, len(specs))
	for _, spec := range specs {
		if !sourceTables[spec.Name] {
			if spec.Optional {
				continue
			}
			return nil, fmt.Errorf("source table %s does not exist", spec.Name)
		}
		if !targetTables[spec.Name] {
			if spec.Optional {
				continue
			}
			return nil, fmt.Errorf("target table %s does not exist", spec.Name)
		}
		active = append(active, spec)
	}

	return active, nil
}

func dryRun(ctx context.Context, reader *datamigrate.SQLiteReader, specs []datamigrate.TableSpec) ([]datamigrate.TableSummary, error) {
	summaries := make([]datamigrate.TableSummary, 0, len(specs))
	for _, spec := range specs {
		count, err := reader.CountRows(ctx, spec.Name)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, datamigrate.TableSummary{Table: spec.Name, SourceCount: count})
	}
	return summaries, nil
}

func executeImport(ctx context.Context, db *gorm.DB, reader *datamigrate.SQLiteReader, writer *datamigrate.PostgresWriter, specs []datamigrate.TableSpec, summaries []datamigrate.TableSummary) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL session_replication_role = 'replica'").Error; err != nil {
			return fmt.Errorf("disable triggers: %w", err)
		}

		txWriter, err := writer.WithDB(tx)
		if err != nil {
			return err
		}

		if err := txWriter.TruncateTables(ctx, specs); err != nil {
			return err
		}

		for index, spec := range specs {
			sourceColumns, err := reader.TableColumns(ctx, spec.Name)
			if err != nil {
				return err
			}

			imported, err := txWriter.ImportTable(ctx, spec, sourceColumns, func(handle func(map[string]any) error) error {
				_, _, readErr := reader.ReadRows(ctx, spec, handle)
				return readErr
			})
			if err != nil {
				return err
			}
			summaries[index].ImportedCount = imported
		}

		if _, err := txWriter.ResetSequences(ctx, specs); err != nil {
			return err
		}

		return nil
	})
}

func modeNeedsTargetPreparation(mode datamigrate.Mode) bool {
	return mode == datamigrate.ModeExecute
}

func modePreparesTargetBeforeResolvingSpecs(mode datamigrate.Mode) bool {
	return mode == datamigrate.ModeExecute
}

func parseMode(value string) (datamigrate.Mode, error) {
	switch strings.TrimSpace(value) {
	case string(datamigrate.ModeDryRun):
		return datamigrate.ModeDryRun, nil
	case string(datamigrate.ModeExecute):
		return datamigrate.ModeExecute, nil
	case string(datamigrate.ModeVerifyOnly):
		return datamigrate.ModeVerifyOnly, nil
	default:
		return "", fmt.Errorf("unknown mode %q", value)
	}
}

func verify(ctx context.Context, reader *datamigrate.SQLiteReader, writer *datamigrate.PostgresWriter, specs []datamigrate.TableSpec) (*datamigrate.VerificationReport, error) {
	verifier := &datamigrate.Verifier{Source: reader, Target: writer}
	return verifier.Verify(ctx, specs)
}

func printSummaries(mode datamigrate.Mode, summaries []datamigrate.TableSummary) {
	fmt.Printf("Mode: %s\n", mode)
	fmt.Println("Import order:")
	for _, summary := range summaries {
		fmt.Printf("- %s: %d rows\n", summary.Table, summary.SourceCount)
	}
	fmt.Println()
}

func printVerification(report *datamigrate.VerificationReport) {
	fmt.Printf("Verified at: %s\n", report.CheckedAt.Format(time.RFC3339))
	fmt.Printf("Checked tables: %d\n", len(report.CheckedTables))
	for _, check := range report.CountChecks {
		fmt.Printf("- count %s: %d -> %d\n", check.Table, check.SourceCount, check.TargetCount)
	}
	for _, state := range report.SequenceStates {
		fmt.Printf("- sequence %s: next=%d max=%d\n", state.Table, state.NextValue, state.MaxID)
	}
	if len(report.SampleChecks) > 0 {
		fmt.Printf("- sample checks: %d comparisons\n", len(report.SampleChecks))
	}
	if len(report.CountChecks) > 0 {
		fmt.Println("Verification completed")
	}
}
