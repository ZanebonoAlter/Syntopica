// Command admin hosts one-shot operational subcommands against the connected
// Syntopica database. Each subcommand is a self-contained flag set so future
// ops tools can join without a new binary.
//
// Currently available:
//
//	fix-event-tag-anchor   scan event tags for stale time anchors and
//	                       regenerate descriptions (change
//	                       fix-event-tag-time-anchor)
//
// Run from backend-go/ so configs/config.yaml resolves:
//
//	go run ./cmd/admin fix-event-tag-anchor                 # dry-run
//	go run ./cmd/admin fix-event-tag-anchor --apply --ids 183730,197750
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/tagmanagement/repository"
	tagcore "syntopica-backend/internal/tagmanagement/service/core"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "fix-event-tag-anchor":
		if err := runFixEventTagAnchor(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "fix-event-tag-anchor: %v\n", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "admin: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: admin <subcommand> [flags]

subcommands:
  fix-event-tag-anchor
      Scan active event tags whose description time anchor precedes the tag
      creation by >=1 year. Dry-run by default: prints the candidate list and
      exports a backup CSV of the original descriptions. To regenerate:
      --apply --ids <id1,id2,...> (IDs must come from the candidate list;
      non-candidate IDs abort the whole run without touching anything).
      flags:
        --apply        regenerate descriptions (requires --ids)
        --ids  list    comma-separated topic tag IDs to regenerate
        --out  path    backup CSV path (default event-tag-anchor-backup-<ts>.csv)
`)
}

// runFixEventTagAnchor implements the fix-event-tag-anchor subcommand.
func runFixEventTagAnchor(args []string) error {
	fs := flag.NewFlagSet("fix-event-tag-anchor", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "regenerate descriptions for the given tag IDs (requires --ids)")
	ids := fs.String("ids", "", "comma-separated topic tag IDs to regenerate (required with --apply)")
	out := fs.String("out", "", "backup CSV path (default event-tag-anchor-backup-<ts>.csv)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *apply && strings.TrimSpace(*ids) == "" {
		return fmt.Errorf("--apply requires --ids (run a dry-run first to get the candidate list)")
	}

	if err := config.LoadConfig("./configs"); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.AppConfig != nil {
		logging.Init(config.AppConfig.Log.Level, logging.FileConfig{Enabled: false})
	}
	if err := database.InitDB(config.AppConfig); err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	repository.InitRepository(database.DB)

	candidates, err := tagcore.ScanEventTagAnchorCandidates()
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	fmt.Printf("anchor-scan: %d candidate event tag(s) (time anchor >=1 year before creation)\n\n", len(candidates))
	for _, c := range candidates {
		fmt.Printf("  [id:%d] %s\n    created=%s earliestYear=%d desc=%s\n",
			c.ID, c.Label, c.CreatedAt.Format("2006-01-02"), c.EarliestYear, c.DescriptionPreview)
	}

	// 备份 CSV 在任何重生成之前导出（含原始 description 全文）
	csvPath := *out
	if csvPath == "" {
		csvPath = fmt.Sprintf("event-tag-anchor-backup-%s.csv", time.Now().Format("20060102-150405"))
	}
	if err := writeBackupCSV(csvPath, candidates); err != nil {
		return fmt.Errorf("write backup CSV: %w", err)
	}
	fmt.Printf("\nbackup CSV written: %s\n", csvPath)

	if !*apply {
		if len(candidates) == 0 {
			fmt.Println("dry-run complete: nothing to fix")
			return nil
		}
		fmt.Println("dry-run complete: no rows modified. To regenerate, rerun with --apply --ids <id1,id2,...>")
		return nil
	}

	parsedIDs, err := parseIDList(*ids)
	if err != nil {
		return err
	}

	results, err := tagcore.ApplyEventTagAnchorFix(candidates, parsedIDs)
	if err != nil {
		return err
	}

	var okCount, failCount int
	for _, r := range results {
		if r.Err != nil {
			failCount++
			fmt.Printf("  [id:%d] %s FAILED: %v (description unchanged)\n", r.TagID, r.Label, r.Err)
			continue
		}
		okCount++
		fmt.Printf("  [id:%d] %s regenerated:\n    old: %s\n    new: %s\n", r.TagID, r.Label, r.OldDescription, r.NewDescription)
	}
	fmt.Printf("\napply complete: %d regenerated, %d failed (failures logged above, nothing partially written)\n", okCount, failCount)
	if okCount > 0 {
		fmt.Println("note: re-embedding tasks were queued; they are processed by the running server's embedding worker")
	}
	return nil
}

// parseIDList parses "1,2,3" into uint IDs.
func parseIDList(s string) ([]uint, error) {
	fields := strings.Split(s, ",")
	ids := make([]uint, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		id, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid tag ID %q in --ids: %w", f, err)
		}
		ids = append(ids, uint(id))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("--ids is empty after parsing")
	}
	return ids, nil
}

// writeBackupCSV dumps the candidate list with full original descriptions so a
// bad regeneration can be reverted by hand.
func writeBackupCSV(path string, candidates []tagcore.EventAnchorCandidate) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create dir: %w", err)
		}
	}
	f, err := os.Create(path) //nolint:gosec // G304：一次性运维工具，路径来自本机 CLI flag/默认时间戳名，非外部输入
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"id", "label", "created_at", "earliest_year", "description"}); err != nil {
		return err
	}
	for _, c := range candidates {
		if err := w.Write([]string{
			strconv.FormatUint(uint64(c.ID), 10),
			c.Label,
			c.CreatedAt.Format(time.RFC3339),
			strconv.Itoa(c.EarliestYear),
			c.Description,
		}); err != nil {
			return err
		}
	}
	return w.Error()
}
