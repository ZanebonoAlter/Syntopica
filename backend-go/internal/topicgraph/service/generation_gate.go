package service

import "sync/atomic"

// dailyReportGenerationRunning is THE process-wide mutual-exclusion gate for
// the daily-report generation pipeline (guard-daily-report-generate D1).
// Both trigger surfaces — the scheduled DailyReportJob and the manual
// POST /api/daily-reports/generate endpoint — share this single gate, so at
// most one full LLM pipeline can run at any moment. Single-user single-process
// deployment: an in-process atomic flag is sufficient (no advisory lock).
//
// Re-exported by the topicgraph root package (wire.go) so the admin scheduler
// can reach it via its existing `daily_report` import; the HTTP handler calls
// it directly through this service package (the root package imports handler,
// so the handler cannot import the root back — import cycle).
var dailyReportGenerationRunning atomic.Bool

// TryAcquireDailyReportGeneration attempts to claim the generation gate
// (non-blocking CAS). Returns true when the caller is now the exclusive owner
// of the generation pipeline and MUST pair the claim with
// ReleaseDailyReportGeneration (deferred, so panics also release).
// Returns false when a pipeline is already running — the caller must skip
// (scheduler) or answer 409 already_running (manual endpoint) without
// starting a second pipeline or sending any terminal notification.
func TryAcquireDailyReportGeneration() bool {
	return dailyReportGenerationRunning.CompareAndSwap(false, true)
}

// ReleaseDailyReportGeneration releases the generation gate. Safe to call only
// by the owner (the goroutine/path whose TryAcquire returned true). Idempotent
// against an already-free gate, but pairing discipline is: acquire → defer
// release, so every terminal path (success, failure, panic) frees the gate.
func ReleaseDailyReportGeneration() {
	dailyReportGenerationRunning.Store(false)
}
