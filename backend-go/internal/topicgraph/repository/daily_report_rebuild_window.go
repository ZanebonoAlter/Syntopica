package repository

import (
	"fmt"
	"time"
)

// ReportExistsForBoardDate reports whether a daily report already exists for
// the given (board, calendar date) pair — the upsert key used by
// SaveReport (semantic_board_id + period_date).
//
// The query mirrors SaveReport's existence lookup exactly: the same
// two-column predicate with period_date rendered as a bare YYYY-MM-DD string,
// so the two can never disagree about what "already exists" means.
//
// The pull-and-normalize shape is deliberate (review M3 suggested a raw
// `period_date = 'YYYY-MM-DD'` equality; that works on Postgres where the
// column is a DATE but NOT on SQLite where glebarez stores full timestamps
// as TEXT — the backfill tests caught it). A ±24h raw prefilter bounds the
// pull to at most three rows per board while the in-memory NormalizeReportDate
// comparison stays the single dialect-proof definition of the key.
//
// Used by the daily report backfill scan (offline-catchup design D5) to skip
// reports that already exist — regenerating one is a whole LLM pipeline run
// that would overwrite a possibly better existing report.
func (r *TopicGraphRepository) ReportExistsForBoardDate(boardID uint, date time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("report exists check: repository not initialized")
	}

	target := NormalizeReportDate(date)

	var periodDates []time.Time
	if err := r.db.Model(&BoardDailyReport{}).
		Where("semantic_board_id = ?", boardID).
		Where("period_date > ? AND period_date < ?", target.Add(-24*time.Hour), target.Add(24*time.Hour)).
		Pluck("period_date", &periodDates).Error; err != nil {
		return false, fmt.Errorf("load report dates for board %d: %w", boardID, err)
	}

	for _, periodDate := range periodDates {
		if NormalizeReportDate(periodDate).Equal(target) {
			return true, nil
		}
	}
	return false, nil
}
