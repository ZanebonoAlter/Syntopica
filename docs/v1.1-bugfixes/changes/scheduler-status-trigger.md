# Scheduler Status And Trigger Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `auto_refresh` and `auto_summary` report truthful runtime state and make scheduler trigger results visible in the existing settings tab.

**Architecture:** Keep the current scheduler entrypoints and settings dialog, but tighten the contract between backend and frontend. Backend becomes the source of truth for whether a trigger was accepted, actually started, skipped, or blocked, and frontend renders that state instead of guessing from HTTP 200 alone.

**Tech Stack:** Go, Gin, GORM, SQLite, Vue 3, Nuxt 4, TypeScript

---

### Task 1: Add failing backend tests for scheduler runtime truthfulness

**Files:**
- Create: `backend-go/internal/schedulers/auto_refresh_test.go`
- Create: `backend-go/internal/schedulers/auto_summary_test.go`
- Modify: `backend-go/internal/handlers/scheduler_test.go`

**Step 1: Write the failing test**

Add tests that prove:

- `auto_refresh` updates `scheduler_tasks` after a run
- `auto_summary` manual trigger actually starts work or returns a blocked result
- scheduler trigger handler returns structured result instead of fake success

**Step 2: Run test to verify it fails**

Run: `go test ./internal/schedulers ./internal/handlers -run "AutoRefresh|AutoSummary|TriggerScheduler"`

Expected: failures showing missing status updates or missing real trigger behavior.

**Step 3: Write minimal implementation**

Implement only enough scheduler state and trigger response changes to satisfy the tests.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/schedulers ./internal/handlers -run "AutoRefresh|AutoSummary|TriggerScheduler"`

Expected: PASS.

### Task 2: Implement backend scheduler state and trigger fixes

**Files:**
- Modify: `backend-go/internal/schedulers/auto_refresh.go`
- Modify: `backend-go/internal/schedulers/auto_summary.go`
- Modify: `backend-go/internal/handlers/scheduler.go`
- Modify: `backend-go/internal/models/ai_models.go`

**Step 1: Implement `auto_refresh` run summary**

Record scan counts, triggered counts, skips, errors, execution time, and next run.

**Step 2: Implement real `auto_summary` trigger**

Add `Trigger()` support that launches the same summary cycle path used by cron, while preserving mutex protection.

**Step 3: Implement truthful trigger responses**

Return structured data including whether trigger was accepted, started, skipped, or blocked.

**Step 4: Verify backend behavior**

Run: `go test ./internal/schedulers ./internal/handlers`

Expected: PASS.

### Task 3: Update frontend scheduler API and types

**Files:**
- Modify: `front/app/api/scheduler.ts`
- Modify: `front/app/types/scheduler.ts`

**Step 1: Write the failing type expectation**

Make the trigger API expect structured trigger response data.

**Step 2: Run typecheck to verify it fails**

Run: `pnpm exec nuxi typecheck`

Expected: frontend uses outdated scheduler trigger types.

**Step 3: Write minimal implementation**

Update scheduler types for trigger results, run summaries, and explanation fields used by the UI.

**Step 4: Run typecheck to verify it passes**

Run: `pnpm exec nuxi typecheck`

Expected: typecheck passes or remaining unrelated errors are isolated.

### Task 4: Extend scheduler tab UI feedback

**Files:**
- Modify: `front/app/components/dialog/GlobalSettingsDialog.vue`

**Step 1: Write the failing UI wiring**

Bind UI to the new backend response fields for `auto_refresh` and `auto_summary`.

**Step 2: Run typecheck to verify it fails**

Run: `pnpm exec nuxi typecheck`

Expected: missing properties and UI references before implementation.

**Step 3: Write minimal implementation**

Add explanation panels and trigger feedback inside the existing schedulers tab. Differentiate real execution, already-running, misconfiguration, and no-op cases.

**Step 4: Run typecheck to verify it passes**

Run: `pnpm exec nuxi typecheck`

Expected: PASS or only unrelated pre-existing issues remain.

### Task 5: Full verification

**Files:**
- Check: `backend-go/internal/schedulers/*.go`
- Check: `backend-go/internal/handlers/*.go`
- Check: `front/app/api/scheduler.ts`
- Check: `front/app/types/scheduler.ts`
- Check: `front/app/components/dialog/GlobalSettingsDialog.vue`

**Step 1: Run backend verification**

Run: `go test ./internal/schedulers ./internal/handlers`

**Step 2: Run frontend verification**

Run: `pnpm exec nuxi typecheck`

**Step 3: Run final status check**

Run: `git status --short`

Expected: only intended scheduler backend/frontend/doc changes remain.
