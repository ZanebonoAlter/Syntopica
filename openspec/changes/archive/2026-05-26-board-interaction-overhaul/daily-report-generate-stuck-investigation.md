# Daily Report Generate Stuck Investigation

Date: 2026-05-26
Endpoint: `POST /api/daily-reports/generate`
Example payload: `{"date":"2026-05-26","board_id":2849}`
Board: `semantic_labels.id=2849` (`刚果（金）局势`)

## Observed Symptoms

- UI dialog keeps showing `生成中...` even after backend generation appears to finish.
- Backend logs are hard to read because slow SQL entries print full vector literals.
- Generated report for `date=2026-05-26` was observed in logs as saved, but DB query later showed `period_date=2026-05-25` for the same report id.

## Execution Chain

1. `backend-go/internal/app/router.go`
   - Registers daily report routes via `dailyreportdomain.RegisterDailyReportRoutes(api)`.
2. `backend-go/internal/domain/daily_report/handler.go`
   - `POST /api/daily-reports/generate` -> `triggerGenerateDailyReport`.
   - Parses `{date, board_id}`.
   - Creates `jobID`.
   - Starts background goroutine:
     - `generateSingleBoard(boardID, date, jobID)` for one board.
     - `generateAllBoards(date, jobID)` for all boards.
3. `generateSingleBoard`
   - Broadcasts `daily_report_progress` with status `processing`.
   - Calls `GenerateDailyReport(ctx, boardID, date)`.
   - Calls `SaveReport(report, sections)`.
   - Broadcasts `daily_report_progress` with status `completed` or `failed`.
4. `backend-go/internal/domain/daily_report/generator.go`
   - `GenerateDailyReport` pipeline:
     - `collectBoardTags(boardID, date)`.
     - `DeduplicateTags`.
     - `ClusterTags` (LLM call).
     - `findPreviousReport`.
     - parallel LLM calls: `GenerateHighlights`, `GenerateDynamics`, `GenerateClusterThreads`.
     - assemble `BoardDailyReport` and `DailyReportSection` records.
5. `backend-go/internal/domain/daily_report/repository.go`
   - `SaveReport` upserts report and replaces sections in one DB transaction.

## Evidence Collected

Database checks against local Docker Postgres (`syntopica` database):

- `semantic_labels.id=2849` exists and is active: `刚果（金）局势`.
- For `2026-05-26`, board 2849 has event data:
  - `event_tags = 5`
  - `articles = 3`
- `ai_call_logs` contains successful daily report LLM calls for this board flow:
  - `daily_report_clustering`
  - `daily_report_highlights`
  - `daily_report_dynamics`
  - `daily_report_threads`
- `backend-go/logs/app.log` contains successful saves, e.g.:
  - `daily-report: saved report 1 for board 2849 on 2026-05-26 (2 sections)`
- Later DB state showed one report:
  - `id=1`, `semantic_board_id=2849`, `period_date=2026-05-25`, `status=completed`

## Root Causes / Mismatches

### 1. WebSocket completion protocol mismatch

Spec says backend should broadcast:

```json
{"type":"daily_report_done","job_id":"...","total_saved":1,"total_boards":1}
```

Current manual endpoint implementation only broadcasts `daily_report_progress` and never broadcasts `daily_report_done`.

Frontend `useDailyReportProgress.ts` only sets `done=true` when receiving `daily_report_done`, so the dialog title remains `生成中...` forever.

### 2. WebSocket field/status mismatch

Frontend expects:

```ts
status: 'waiting' | 'generating' | 'completed' | 'failed'
board_name: string
saved: number
progress: string
```

Backend currently sends fields like:

```json
{"type":"daily_report_progress","status":"processing","board_id":2849,"report_id":1,"completed":0}
```

Mismatches:

- backend `processing` vs frontend `generating`
- backend omits `board_name`
- backend omits `saved`
- backend omits `progress`
- backend omits final `daily_report_done`

### 3. API response shape mismatch

Frontend API types/components expect:

- `getBoardDailyReports`: `data.reports`
- `getDailyReportDetail`: `data.report`

Backend currently returns raw data directly:

- list: `data: reports`
- detail: `data: report`

This can make the daily report timeline show empty or fail to cache details even when reports exist.

### 4. `period_date` date drift risk

The log says report saved for `2026-05-26`, but DB showed `period_date=2026-05-25`. This points to a date-only value being represented as `time.Time` at local midnight and converted across time zones when written/read through PostgreSQL `date`/GORM.

Daily report `period_date` should be treated as a stable date-only value. The implementation should prevent local-midnight timezone drift.

### 5. Log readability problem

`backend-go/internal/platform/database/slow_logger.go` logs full SQL for slow queries. Vector similarity SQL includes huge vector literals, producing extremely long log lines.

`backend-go/internal/platform/ws/hub.go` also logs normal client close (`close 1000 Manual disconnect`) as WARN, making normal dialog open/close cycles look like errors.

## Acceptance Targets for Fix

- `POST /api/daily-reports/generate {"date":"2026-05-26","board_id":2849}` immediately returns `{job_id,status:"processing"}`.
- WebSocket broadcasts progress using the documented shape, including `board_name`, `saved`, and `progress`.
- WebSocket always broadcasts `daily_report_done` at terminal completion for both single-board and all-board manual generation.
- Frontend dialog changes to completed state when generation finishes.
- Board daily report list/detail APIs match frontend expectations or frontend is adjusted consistently.
- `period_date` for requested `2026-05-26` persists and lists as `2026-05-26`, not previous day.
- Slow SQL logs truncate/sanitize long vector literals.
- Normal WebSocket close 1000 no longer logs as WARN.
