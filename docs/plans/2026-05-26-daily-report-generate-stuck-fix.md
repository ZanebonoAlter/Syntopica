# Daily Report Generate Stuck Fix Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 修复 `/api/daily-reports/generate` 手动触发后前端一直显示“生成中”、日报日期偏移、查询响应结构不一致，以及日志过长/误报 WARN 的问题。

**Architecture:** 后端以 OpenSpec 中 `daily_report_progress` / `daily_report_done` 为权威协议，统一手动触发和调度器的终态广播语义。查询 API 与前端类型保持一致，日期字段按 date-only 语义稳定处理。日志层只做展示截断，不改变 SQL 执行。

**Tech Stack:** Go 1.x, Gin, GORM, PostgreSQL date, WebSocket hub, Nuxt 4/Vue 3/TypeScript, Vitest/Go tests.

---

## Context and References

- Investigation note: `openspec/changes/board-interaction-overhaul/daily-report-generate-stuck-investigation.md`
- OpenSpec tasks: `openspec/changes/board-interaction-overhaul/tasks.md` section 18
- Backend files:
  - `backend-go/internal/domain/daily_report/handler.go`
  - `backend-go/internal/domain/daily_report/repository.go`
  - `backend-go/internal/domain/daily_report/generator.go`
  - `backend-go/internal/platform/database/slow_logger.go`
  - `backend-go/internal/platform/ws/hub.go`
- Frontend files:
  - `front/app/api/dailyReports.ts`
  - `front/app/composables/useDailyReportProgress.ts`
  - `front/app/features/tags/components/NarrativeGenerateDialog.vue`
  - `front/app/features/tags/components/BoardDailyReportTimeline.vue`

## Mandatory Safety

- Before editing any function/method/class, run GitNexus impact analysis if the tool is available, especially for:
  - `triggerGenerateDailyReport`
  - `generateSingleBoard`
  - `generateAllBoards`
  - `broadcastProgress`
  - `ListReports`
  - `GetReportByID`
  - `SlowLogger.Trace`
  - `Client.readPump`
- If GitNexus tools are unavailable in the harness, explicitly note that in the implementation summary.
- Keep changes minimal. Do not refactor unrelated daily report generation prompts or AI logic.

---

## Task 1: Add backend tests for manual generation WebSocket protocol

**Files:**
- Test: `backend-go/internal/domain/daily_report/handler_test.go` or existing daily report test file if present.
- Modify later: `backend-go/internal/domain/daily_report/handler.go`

**Step 1: Write failing tests**

Add focused tests around message construction if direct WS hub inspection is awkward. Prefer extracting a small helper only if needed, e.g. `buildProgressMessage(...)` and `buildDoneMessage(...)`.

Required assertions:

```go
func TestBuildDailyReportProgressMessageMatchesFrontendContract(t *testing.T) {
    msg := buildProgressMessage("job-1", "generating", 2849, "刚果（金）局势", 0, "0/1")

    require.Equal(t, "daily_report_progress", msg["type"])
    require.Equal(t, "job-1", msg["job_id"])
    require.Equal(t, uint(2849), msg["board_id"])
    require.Equal(t, "刚果（金）局势", msg["board_name"])
    require.Equal(t, "generating", msg["status"])
    require.Equal(t, 0, msg["saved"])
    require.Equal(t, "0/1", msg["progress"])
}

func TestBuildDailyReportDoneMessageMatchesFrontendContract(t *testing.T) {
    msg := buildDoneMessage("job-1", 1, 1)

    require.Equal(t, "daily_report_done", msg["type"])
    require.Equal(t, "job-1", msg["job_id"])
    require.Equal(t, 1, msg["total_saved"])
    require.Equal(t, 1, msg["total_boards"])
}
```

Use standard library assertions if this package does not use testify.

**Step 2: Run test to verify it fails**

Run:

```bash
cd backend-go && go test ./internal/domain/daily_report -run 'TestBuildDailyReport.*Message' -v
```

Expected: FAIL because helpers do not exist or message fields are missing.

---

## Task 2: Implement backend WebSocket protocol alignment

**Files:**
- Modify: `backend-go/internal/domain/daily_report/handler.go`

**Step 1: Add minimal helper functions**

Implement pure helpers in `handler.go` near `broadcastProgress`:

```go
func buildProgressMessage(jobID string, status string, boardID uint, boardName string, saved int, progress string) map[string]interface{} {
    return map[string]interface{}{
        "type":       "daily_report_progress",
        "job_id":     jobID,
        "status":     status,
        "board_id":   boardID,
        "board_name": boardName,
        "saved":      saved,
        "progress":   progress,
        "timestamp":  time.Now().Format(time.RFC3339),
    }
}

func buildDoneMessage(jobID string, totalSaved int, totalBoards int) map[string]interface{} {
    return map[string]interface{}{
        "type":         "daily_report_done",
        "job_id":       jobID,
        "total_saved":  totalSaved,
        "total_boards": totalBoards,
        "timestamp":    time.Now().Format(time.RFC3339),
    }
}
```

**Step 2: Fetch board names**

Add a small helper in `handler.go` using `models.SemanticLabel` or the existing board model type used by semantic labels:

```go
func dailyReportBoardName(boardID uint) string {
    var board models.SemanticLabel
    if err := database.DB.Select("label").Where("id = ?", boardID).First(&board).Error; err != nil {
        return fmt.Sprintf("Board #%d", boardID)
    }
    return board.Label
}
```

Use actual model name/fields from `backend-go/internal/domain/models`.

**Step 3: Update single-board flow**

Change `generateSingleBoard` semantics:

- At start: broadcast progress `generating`, saved `0`, progress `0/1`.
- On failure: broadcast progress `failed`, saved `0`, progress `1/1`; then broadcast done `total_saved=0,total_boards=1`.
- On no report: broadcast progress `completed`, saved `0`, progress `1/1`; then done.
- On save success: broadcast progress `completed`, saved `1`, progress `1/1`; then done `total_saved=1,total_boards=1`.

**Step 4: Update all-board flow**

Change `generateAllBoards` semantics:

- `totalBoards := len(boardIDs)`.
- For each board, broadcast `generating` before work with progress like `completed/totalBoards`.
- On each board terminal state, broadcast `completed` or `failed` with progress `(idx+1)/totalBoards`.
- Maintain `savedCount` for successfully saved reports.
- Always broadcast `daily_report_done` at the end, including zero-board and collection-failure cases.

**Step 5: Run tests**

Run:

```bash
cd backend-go && go test ./internal/domain/daily_report -run 'TestBuildDailyReport.*Message' -v
```

Expected: PASS.

---

## Task 3: Fix daily report list/detail response shape

**Files:**
- Modify: `backend-go/internal/domain/daily_report/handler.go`
- Test: daily report handler test file if existing patterns support handler tests.

**Step 1: Write/adjust tests**

Test expected JSON shapes:

- `GET /api/semantic-boards/:id/daily-reports` returns:

```json
{"success":true,"data":{"reports":[...]}}
```

- `GET /api/daily-reports/:id` returns:

```json
{"success":true,"data":{"report":{...}}}
```

**Step 2: Implement minimal change**

Change:

```go
c.JSON(http.StatusOK, gin.H{"success": true, "data": reports})
```

to:

```go
c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"reports": reports}})
```

Change:

```go
c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
```

to:

```go
c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"report": report}})
```

**Step 3: Verify**

Run:

```bash
cd backend-go && go test ./internal/domain/daily_report -v
```

Expected: PASS.

---

## Task 4: Fix `period_date` date-only drift

**Files:**
- Modify: `backend-go/internal/domain/daily_report/generator.go`
- Modify if needed: `backend-go/internal/domain/daily_report/repository.go`
- Test: `backend-go/internal/domain/daily_report/repository_test.go` or existing daily report tests.

**Step 1: Add failing test**

Add a test that creates/saves a report for `2026-05-26` and asserts list/detail return `period_date` for that date, not `2026-05-25`.

If DB integration tests are difficult, add a pure helper and test it:

```go
func reportDateOnly(date time.Time) time.Time {
    return time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
}
```

Expected property: formatting with `Format("2006-01-02")` remains requested date under local/UTC conversion.

**Step 2: Implement minimal date-only normalization**

Use a single helper for daily report period dates, e.g.:

```go
func normalizeReportDate(date time.Time) time.Time {
    return time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
}
```

Use it for:

- `BoardDailyReport.PeriodDate` assembly in `GenerateDailyReport`.
- `SaveReport` range matching should not rely on `PeriodDate.Add(24*time.Hour)` if `PeriodDate` is noon UTC. Instead match by date cast or normalized bounds carefully.
- `ListReports` range should include normalized dates correctly.

Preferred DB-safe upsert lookup for date column:

```go
tx.Where("semantic_board_id = ? AND period_date = ?", report.SemanticBoardID, report.PeriodDate.Format("2006-01-02"))
```

For list ranges, use date strings:

```go
Where("semantic_board_id = ? AND period_date >= ? AND period_date < ?", boardID, rangeStart.Format("2006-01-02"), rangeEnd.Format("2006-01-02"))
```

**Step 3: Verify**

Run:

```bash
cd backend-go && go test ./internal/domain/daily_report -run 'Test.*Date|Test.*Report' -v
```

Expected: PASS and date remains `2026-05-26`.

---

## Task 5: Improve log readability without changing behavior

**Files:**
- Modify: `backend-go/internal/platform/database/slow_logger.go`
- Modify: `backend-go/internal/platform/ws/hub.go`
- Test: `backend-go/internal/platform/database/slow_logger_test.go` if present or add focused test.

**Step 1: Add slow SQL sanitization test**

Add a pure helper test:

```go
func TestSanitizeSlowSQLTruncatesVectorLiterals(t *testing.T) {
    sql := "SELECT '[0.1,0.2,0.3,0.4,0.5]'::vector"
    got := sanitizeSlowSQL(sql)
    if len(got) >= len(sql) && strings.Contains(got, "0.5") {
        t.Fatalf("expected vector literal to be truncated, got %q", got)
    }
}
```

**Step 2: Implement helper**

In `slow_logger.go`, add a helper that caps overall SQL length and truncates vector literals. Keep it simple:

```go
const maxLoggedSQLLength = 2000

func sanitizeSlowSQL(sql string) string {
    if len(sql) > maxLoggedSQLLength {
        return sql[:maxLoggedSQLLength] + "... [truncated]"
    }
    return sql
}
```

If time allows, use regex to replace long `'[...]'::vector` literals, but overall cap is acceptable and low-risk.

Apply `sanitizeSlowSQL(sql)` in both error and slow paths before logging.

**Step 3: Suppress normal WebSocket close WARN**

In `Client.readPump`, treat `websocket.CloseNormalClosure` and `websocket.CloseGoingAway` as normal; do not log WARN for close 1000 manual disconnect.

**Step 4: Verify**

Run:

```bash
cd backend-go && go test ./internal/platform/database ./internal/platform/ws -v
```

Expected: PASS.

---

## Task 6: Frontend robustness for progress messages

**Files:**
- Modify: `front/app/composables/useDailyReportProgress.ts`
- Modify only if needed: `front/app/features/tags/components/NarrativeGenerateDialog.vue`

**Step 1: Make progress composable tolerant**

Even after backend fix, make frontend robust:

- Accept `processing` as alias for `generating`.
- If receiving `daily_report_progress` with `status === 'completed'` and progress indicates final single-board `1/1`, set `done=true` only if no `daily_report_done` follows? Prefer backend done as source of truth; for single board robustness, it is OK to set done when `progress === '1/1'`.
- Default `board_name` to `#${board_id}` if missing.

**Step 2: Verify frontend types**

Run:

```bash
cd front && pnpm lint
cd front && pnpm exec nuxi typecheck
```

Expected: PASS.

---

## Task 7: Update docs/OpenSpec status and run targeted verification

**Files:**
- Modify: `openspec/changes/board-interaction-overhaul/tasks.md`
- Modify/create if useful: `docs/reference/api/...` or relevant docs page only if API response shape is documented there.

**Step 1: Mark OpenSpec section 18 tasks complete**

After implementation and verification, update section 18 checkboxes in `openspec/changes/board-interaction-overhaul/tasks.md`.

**Step 2: Run backend verification**

Run:

```bash
cd backend-go && go test ./internal/domain/daily_report -v
cd backend-go && go test ./internal/platform/database ./internal/platform/ws -v
cd backend-go && go build ./...
```

Expected: PASS.

**Step 3: Run frontend verification**

Run:

```bash
cd front && pnpm lint
cd front && pnpm exec nuxi typecheck
```

Expected: PASS. If native binding issues occur in WSL, capture the exact error and do not claim success.

**Step 4: Manual smoke check**

With backend running and DB available:

```bash
curl -sS -X POST http://localhost:5000/api/daily-reports/generate \
  -H 'Content-Type: application/json' \
  -d '{"date":"2026-05-26","board_id":2849}'
```

Then verify:

```sql
SELECT id, semantic_board_id, period_date, status
FROM board_daily_reports
WHERE semantic_board_id=2849
ORDER BY id DESC
LIMIT 5;
```

Expected:

- Response has `success=true`, `data.job_id`, `data.status="processing"`.
- DB `period_date` is `2026-05-26`.
- Frontend dialog reaches completed state.

---

## Acceptance Checklist

- [ ] `daily_report_done` is emitted for manual single-board generation.
- [ ] `daily_report_done` is emitted for manual all-board generation, including zero-board cases.
- [ ] `daily_report_progress` fields match OpenSpec and frontend expectations.
- [ ] `GET /semantic-boards/:id/daily-reports` returns `data.reports`.
- [ ] `GET /daily-reports/:id` returns `data.report`.
- [ ] Requested date `2026-05-26` persists/lists as `2026-05-26`.
- [ ] Slow SQL logs are capped/truncated.
- [ ] Normal WebSocket close 1000 is not WARN.
- [ ] Targeted backend tests pass.
- [ ] Frontend lint/typecheck pass or exact environment failure is documented.
