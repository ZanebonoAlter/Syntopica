// Command admin hosts one-shot operational subcommands against the connected
// Syntopica database. Each subcommand is a self-contained flag set so future
// ops tools can join without a new binary.
//
// Currently available:
//
//	fix-event-tag-anchor   scan event tags for stale time anchors and
//	                       regenerate descriptions (change
//	                       fix-event-tag-time-anchor)
//	board-checkup          run one full board-health checkup round over
//	                       every active board (same entry as the daily-
//	                       report tail step; change fix-board-direction-
//	                       gap-exempt task 4.2 manual trigger)
//
// Run from backend-go/ so configs/config.yaml resolves:
//
//	go run ./cmd/admin fix-event-tag-anchor                 # dry-run
//	go run ./cmd/admin fix-event-tag-anchor --apply --ids 183730,197750
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	tagmgmt "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/tagmanagement/repository"
	tagcore "syntopica-backend/internal/tagmanagement/service/core"

	"gorm.io/gorm"
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
	case "board-checkup":
		if err := runBoardCheckup(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "board-checkup: %v\n", err)
			os.Exit(1)
		}
	case "tagguard-cleanup":
		if err := runTagguardCleanup(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "tagguard-cleanup: %v\n", err)
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

// runBoardCheckup executes one full board-health checkup round with the
// production airouter-backed LLM — the same entry the daily-report tail
// step calls (tagmgmt.RunBoardHealthCheckupWrapped). Writes
// board_health_snapshots + cooldown-gated anomaly notifications only
// (red line 20: no suggestion rows, no matching-table writes).
func runBoardCheckup(_ []string) error {
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
	tagmgmt.RunBoardHealthCheckupWrapped(context.Background(), database.DB)
	fmt.Println("board-checkup: one full round executed over every active board")
	return nil
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
  tagguard-cleanup
      Remove article_topic_tags links whose tag label copies a tagging-prompt
      few-shot exemplar (PostgreSQL, Kubernetes, ...) while the article text
      (title+content+firecrawl_content) has zero mention of it (change
      guard-tag-exemplar-leakage). Dry-run by default: prints the leaked-link
      list and total exemplar-link counts for the record. To delete:
      --apply (re-running dry-run afterwards must be empty).
      flags:
        --apply        delete leaked links, recalc tag_count, cleanup orphans
        --out  path    backup CSV path (default tagguard-backup-<ts>.csv)
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

// exemplarLeakCandidate is one article_topic_tags row whose tag label copies a
// prompt exemplar, plus the article text fields needed for the evidence check.
type exemplarLeakCandidate struct {
	LinkID    uint   `gorm:"column:link_id"`
	ArticleID uint   `gorm:"column:article_id"`
	TagID     uint   `gorm:"column:tag_id"`
	Label     string `gorm:"column:label"`
	Title     string `gorm:"column:title"`
	Content   string `gorm:"column:content"`
	Firecrawl string `gorm:"column:firecrawl_content"`
}

// visibleText is the evidence corpus for the leak check: same fields as the
// design doc (title + content + firecrawl_content), case-insensitive Contains.
func (c exemplarLeakCandidate) visibleText() string {
	return c.Title + "\n" + c.Content + "\n" + c.Firecrawl
}

// runTagguardCleanup implements the tagguard-cleanup subcommand (change
// guard-tag-exemplar-leakage): removes stored article_topic_tags links whose
// tag label copies a tagging-prompt few-shot exemplar while the article text
// has zero mention of it. Dry-run by default; --apply deletes inside one
// transaction, recalcs articles.tag_count, then cleans orphaned tags.
func runTagguardCleanup(args []string) error {
	fs := flag.NewFlagSet("tagguard-cleanup", flag.ContinueOnError)
	apply := fs.Bool("apply", false, "delete leaked exemplar tag links (default: dry-run)")
	out := fs.String("out", "", "backup CSV path (default tagguard-backup-<ts>.csv)")
	if err := fs.Parse(args); err != nil {
		return err
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

	// Candidate superset via SQL (label lower-bound IN exemplar labels), then
	// the same-source Go judgment as the extraction guard.
	labels := tagcore.AllPromptExemplarLabels()
	lowerLabels := make([]string, 0, len(labels))
	seenLower := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		low := strings.ToLower(l)
		if _, dup := seenLower[low]; dup {
			continue
		}
		seenLower[low] = struct{}{}
		lowerLabels = append(lowerLabels, low)
	}

	var candidates []exemplarLeakCandidate
	// content / firecrawl_content are nullable — COALESCE keeps the scan from
	// failing with "converting NULL to string is unsupported" on articles that
	// were never enriched (guard-tag-exemplar-leakage R-1).
	if err := repository.Repo.DB().Raw(`
		SELECT att.id AS link_id, att.article_id, att.topic_tag_id AS tag_id,
		       tt.label, a.title, COALESCE(a.content, '') AS content,
		       COALESCE(a.firecrawl_content, '') AS firecrawl_content
		FROM article_topic_tags att
		JOIN topic_tags tt ON tt.id = att.topic_tag_id
		JOIN articles a ON a.id = att.article_id
		WHERE lower(tt.label) IN ?`, lowerLabels).Scan(&candidates).Error; err != nil {
		return fmt.Errorf("scan exemplar links: %w", err)
	}

	var leaked []exemplarLeakCandidate
	for _, c := range candidates {
		if tagcore.IsPromptExemplarWithoutEvidence(c.Label, c.visibleText()) {
			leaked = append(leaked, c)
		}
	}

	// count(*) 留底：示例词标签关联总数 / 待删 / 保留。
	fmt.Printf("tagguard-cleanup: exemplar-label links total=%d leaked(no mention)=%d kept(has mention)=%d\n\n",
		len(candidates), len(leaked), len(candidates)-len(leaked))
	for _, c := range leaked {
		fmt.Printf("  [link:%d article:%d tag:%d] %s | %s\n",
			c.LinkID, c.ArticleID, c.TagID, c.Label, truncateRunes(c.Title, 40))
	}

	csvPath := *out
	if csvPath == "" {
		csvPath = fmt.Sprintf("tagguard-backup-%s.csv", time.Now().Format("20060102-150405"))
	}
	if err := writeTagguardBackupCSV(csvPath, leaked); err != nil {
		return fmt.Errorf("write backup CSV: %w", err)
	}
	fmt.Printf("\nbackup CSV written: %s\n", csvPath)

	if !*apply {
		if len(leaked) == 0 {
			fmt.Println("dry-run: nothing to delete (already clean)")
		} else {
			fmt.Printf("dry-run: %d link(s) would be deleted; re-run with --apply to proceed\n", len(leaked))
		}
		return nil
	}
	if len(leaked) == 0 {
		fmt.Println("apply: nothing to delete (already clean)")
		return nil
	}

	linkIDs := make([]uint, 0, len(leaked))
	articleIDs := make([]uint, 0, len(leaked))
	tagIDSet := make(map[uint]struct{})
	for _, c := range leaked {
		linkIDs = append(linkIDs, c.LinkID)
		articleIDs = append(articleIDs, c.ArticleID)
		tagIDSet[c.TagID] = struct{}{}
	}

	err := repository.Repo.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id IN ?", linkIDs).Delete(&models.ArticleTopicTag{}).Error; err != nil {
			return fmt.Errorf("delete links: %w", err)
		}
		if err := tx.Exec(`
			UPDATE articles SET tag_count = (SELECT COUNT(*) FROM article_topic_tags WHERE article_id = articles.id)
			WHERE id IN ?`, articleIDs).Error; err != nil {
			return fmt.Errorf("recalc tag_count: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("apply transaction: %w", err)
	}

	tagIDs := make([]uint, 0, len(tagIDSet))
	for id := range tagIDSet {
		tagIDs = append(tagIDs, id)
	}
	tagcore.CleanupOrphanedTags(tagIDs)

	fmt.Printf("apply: deleted %d link(s) across %d article(s); orphan cleanup ran for %d tag(s)\n",
		len(linkIDs), len(articleIDs), len(tagIDs))
	fmt.Println("verify: re-run dry-run (must report leaked=0)")
	return nil
}

func writeTagguardBackupCSV(path string, leaked []exemplarLeakCandidate) error {
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
	if err := w.Write([]string{"link_id", "article_id", "tag_id", "label", "title"}); err != nil {
		return err
	}
	for _, c := range leaked {
		if err := w.Write([]string{
			strconv.FormatUint(uint64(c.LinkID), 10),
			strconv.FormatUint(uint64(c.ArticleID), 10),
			strconv.FormatUint(uint64(c.TagID), 10),
			c.Label, c.Title,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
