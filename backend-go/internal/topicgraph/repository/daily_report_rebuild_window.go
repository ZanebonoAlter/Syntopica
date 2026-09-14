package repository

import (
	"fmt"
	"time"
)

// ReportExistsForBoardDate reports whether a daily report already exists for
// the given (board, calendar date) pair — the upsert key used by
// SaveReport (semantic_board_id + period_date).
//
// The comparison normalizes BOTH sides with NormalizeReportDate instead of
// comparing period_date in SQL: NormalizeReportDate pins the value to noon UTC,
// so a raw SQL equality against a driver-bindable timestamp would miss the
// stored row depending on dialect/time zone. Reading the board's dates back and
// comparing normalized values is dialect-proof (SQLite in tests, Postgres in
// production) and matches SaveReport's "same board, same calendar day" key.
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
