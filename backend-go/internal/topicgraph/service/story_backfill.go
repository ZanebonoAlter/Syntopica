package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// BackfillOptions selects inclusive Asia/Shanghai calendar dates.
type BackfillOptions struct {
	BoardID  *uint
	From, To time.Time
	DryRun   bool
}

// BackfillStats counts successful decisions (Merged also increments Attached).
// Threads includes protected/composite rows; Skipped includes those rows and
// lock-contended boards. Failed counts individual thread/date/board failures.
// DryRun counts independent verdicts against current persisted candidates: it
// does not simulate stories that an earlier dry-run verdict would have created.
type BackfillStats struct {
	Boards, Reports, Threads, Created, Attached, Merged, Skipped, Failed int
	DryRun                                                               bool
}

// Distinct from report-save transaction locks; held on a dedicated connection
// across the whole board, never in a long-running transaction.
const storyBackfillLockNamespace int64 = 0x5354424600000000

// errStoryBoardLocked signals that another run holds the board lock — the
// caller skips the board instead of failing the whole run.
var errStoryBoardLocked = errors.New("story-backfill: board locked by another run")

// withStoryBoardLock runs fn while holding the per-board advisory lock
// (Postgres only; other dialects run fn unlocked). The lock is taken on the
// dedicated connection handed out by database.DB.Connection and released
// before the connection returns to the pool.
func withStoryBoardLock(db *gorm.DB, boardID uint, fn func(*gorm.DB) error) error {
	// Connection hands out a non-cloning statement; use fresh query sessions
	// so report filters cannot leak into thread/lock queries.
	db = db.Session(&gorm.Session{})
	if db.Name() != "postgres" {
		return fn(db)
	}
	if boardID > math.MaxUint32 {
		return fmt.Errorf("board %d exceeds the backfill lock namespace capacity", boardID)
	}
	key := storyBackfillLockNamespace + int64(boardID)
	var locked bool
	if err := db.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&locked).Error; err != nil {
		return err
	}
	if !locked {
		return errStoryBoardLocked
	}
	defer func() {
		// Cancellation must not leave a session lock in the pool.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.WithContext(unlockCtx).Exec("SELECT pg_advisory_unlock(?)", key).Error; err != nil {
			logging.Warnf("story-backfill: unlock board %d: %v", boardID, err)
			// Never return a possibly locked session to the pool.
			if conn, ok := db.Statement.ConnPool.(*sql.Conn); ok {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
	}()
	return fn(db)
}

func backfillDateBounds(opts BackfillOptions) (string, string, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return "", "", err
	}
	from, to := opts.From.In(loc).Format(time.DateOnly), opts.To.In(loc).Format(time.DateOnly)
	if opts.From.IsZero() || opts.To.IsZero() || from > to {
		return "", "", fmt.Errorf("backfill: require nonzero dates and from <= to")
	}
	return from, to, nil
}

func backfillReportQuery(db *gorm.DB, opts BackfillOptions) (*gorm.DB, error) {
	from, to, err := backfillDateBounds(opts)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse(time.DateOnly, to)
	if err != nil {
		return nil, err
	}
	// Half-open SQL range implements inclusive calendar dates on both PG
	// DATE and SQLite's timestamp-backed date columns.
	q := db.Model(&repository.BoardDailyReport{}).Where("period_date >= ? AND period_date < ?", from, end.AddDate(0, 0, 1).Format(time.DateOnly))
	if opts.BoardID != nil {
		q = q.Where("semantic_board_id = ?", *opts.BoardID)
	}
	return q, nil
}

// PreviewStoryBackfill reports the selected workload without AI or writes.
func PreviewStoryBackfill(ctx context.Context, opts BackfillOptions) (BackfillStats, error) {
	stats := BackfillStats{DryRun: opts.DryRun}
	q, err := backfillReportQuery(database.DB.WithContext(ctx), opts)
	if err != nil {
		return stats, err
	}
	var boards []uint
	if err := q.Distinct("semantic_board_id").Order("semantic_board_id").Pluck("semantic_board_id", &boards).Error; err != nil {
		return stats, err
	}
	// Rebuild the query: GORM's statement chain above changes its SELECT.
	q, err = backfillReportQuery(database.DB.WithContext(ctx), opts)
	if err != nil {
		return stats, err
	}
	var reports int64
	if err := q.Count(&reports).Error; err != nil {
		return stats, err
	}
	stats.Boards, stats.Reports = len(boards), int(reports)
	if opts.BoardID != nil {
		stats.Boards = 1
	}
	return stats, nil
}

// BackfillStories fills only identity/evidence; it never runs display dedup.
// Row/date/board errors are counted and logged, then other work continues.
// Callers should treat Failed > 0 as a partial failure; cancellation is returned.
func BackfillStories(ctx context.Context, opts BackfillOptions) (BackfillStats, error) {
	stats := BackfillStats{DryRun: opts.DryRun}
	q, err := backfillReportQuery(database.DB.WithContext(ctx), opts)
	if err != nil {
		return stats, err
	}
	var boards []uint
	if opts.BoardID != nil {
		boards = []uint{*opts.BoardID}
	} else if err := q.Distinct("semantic_board_id").Order("semantic_board_id").Pluck("semantic_board_id", &boards).Error; err != nil {
		return stats, err
	}
	for _, boardID := range boards {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		err := database.DB.WithContext(ctx).Connection(func(db *gorm.DB) error {
			err := withStoryBoardLock(db, boardID, func(db *gorm.DB) error {
				stats.Boards++
				return backfillStoryBoard(ctx, db, opts, boardID, &stats)
			})
			if errors.Is(err, errStoryBoardLocked) {
				stats.Skipped++
				logging.Warnf("story-backfill: board %d locked by another backfill; skipped", boardID)
				return nil
			}
			return err
		})
		if err != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			stats.Failed++
			logging.Warnf("story-backfill: board %d: %v", boardID, err)
		}
	}
	return stats, ctx.Err()
}

func backfillStoryBoard(ctx context.Context, db *gorm.DB, opts BackfillOptions, boardID uint, stats *BackfillStats) error {
	boardOpts := opts
	boardOpts.BoardID = &boardID
	q, err := backfillReportQuery(db, boardOpts)
	if err != nil {
		return err
	}
	// Loading report IDs together with their unique board/date key avoids
	// driver-specific scanning of DISTINCT date expressions (SQLite/PG).
	var reports []repository.BoardDailyReport
	if err := q.Select("id", "period_date").Order("period_date ASC").Find(&reports).Error; err != nil {
		return err
	}
	store := storyStore(repoStoryStore{repo: repository.NewTopicGraphRepository(db)})
	if storyStoreForTest != nil {
		store = storyStoreForTest
	}
	for _, report := range reports {
		if err := ctx.Err(); err != nil {
			return err
		}
		stats.Reports++
		var protected int64
		err := db.Model(&repository.DailyReportThread{}).
			Where("report_id = ? AND (story_id IS NOT NULL OR story_manual = ?)", report.ID, true).
			Count(&protected).Error
		if err != nil {
			stats.Failed++
			logging.Warnf("story-backfill: report %d protected rows: %v", report.ID, err)
			continue
		}
		stats.Threads += int(protected)
		stats.Skipped += int(protected)
		part, err := attributeStoriesForReportWithStats(ctx, store, boardID, report.ID, report.PeriodDate, opts.DryRun)
		stats.Threads += part.Threads
		stats.Created += part.Created
		stats.Attached += part.Attached
		stats.Merged += part.Merged
		stats.Skipped += part.Skipped
		stats.Failed += part.Failed
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			stats.Failed++
			logging.Warnf("story-backfill: report %d: %v", report.ID, err)
		}
	}
	return nil
}
