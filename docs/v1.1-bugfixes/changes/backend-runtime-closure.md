# Backend Runtime Closure Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the remaining backend runtime gaps so scheduler APIs reflect live state, placeholder endpoints become real, and the backend runtime doc matches implementation.

**Architecture:** Unify runtime scheduler exposure behind `runtimeinfo`, add concrete capability methods on schedulers and handlers, and aggregate task status from existing runtime components instead of a fake queue endpoint. Keep backward compatibility for existing scheduler route names while adding clearer canonical names.

**Tech Stack:** Go, Gin, GORM, SQLite, existing scheduler/runtime packages, Go test

---

### Task 1: Capture scheduler behavior in tests

**Files:**
- Modify: `backend-go/internal/jobs/handler_test.go`
- Create: `backend-go/internal/app/runtimeinfo/reset_test.go`

**Step 1: Write failing tests**
- Add tests for unified scheduler status including `preference_update` and `digest`
- Add tests for interval update and reset behavior using scheduler stubs
- Add test coverage for alias handling between `content_completion` and `ai_summary`

**Step 2: Run tests to verify failure**

Run: `go test ./internal/jobs -run Test -v`

**Step 3: Implement minimal production code**
- Add runtime registry support and handler dispatch helpers needed for the new tests

**Step 4: Run tests to verify pass**

Run: `go test ./internal/jobs -run Test -v`

### Task 2: Close scheduler runtime gaps

**Files:**
- Modify: `backend-go/internal/app/runtime.go`
- Modify: `backend-go/internal/app/runtimeinfo/schedulers.go`
- Modify: `backend-go/internal/jobs/handler.go`
- Modify: `backend-go/internal/jobs/preference_update.go`
- Modify: `backend-go/internal/jobs/auto_refresh.go`
- Modify: `backend-go/internal/jobs/auto_summary.go`
- Modify: `backend-go/internal/jobs/content_completion.go`

**Step 1: Write failing tests**
- Cover missing preference scheduler exposure
- Cover real reset and interval update behavior

**Step 2: Run tests to verify failure**

Run: `go test ./internal/jobs -run Test -v`

**Step 3: Implement minimal production code**
- Register preference scheduler in runtime info
- Add unified scheduler lookup and capability dispatch
- Implement `GetStatus`, `TriggerNow`, `ResetStats`, and `UpdateInterval` where supported
- Preserve backward compatibility for `ai_summary`

**Step 4: Run tests to verify pass**

Run: `go test ./internal/jobs -run Test -v`

### Task 3: Replace fake task status endpoint

**Files:**
- Modify: `backend-go/internal/app/router.go`
- Modify: `backend-go/internal/jobs/handler.go`
- Modify: `backend-go/internal/domain/preferences/handler.go`
- Modify: `backend-go/internal/domain/summaries/summary_queue.go` (if snapshot helpers are needed)

**Step 1: Write failing tests**
- Add tests for `/api/tasks/status` aggregation shape
- Add tests for preference update delegating to runtime scheduler before fallback

**Step 2: Run tests to verify failure**

Run: `go test ./internal/jobs ./internal/domain/preferences -run Test -v`

**Step 3: Implement minimal production code**
- Add a real tasks status handler aggregating summary queue, firecrawl, and content completion state
- Route `/api/tasks/status` to the new handler
- Make manual preference update use runtime scheduler when available

**Step 4: Run tests to verify pass**

Run: `go test ./internal/jobs ./internal/domain/preferences -run Test -v`

### Task 4: Update docs and verify

**Files:**
- Modify: `docs/architecture/backend-runtime.md`
- Modify: `docs/architecture/backend-go.md` (only if runtime API descriptions changed there too)

**Step 1: Update docs**
- Remove outdated placeholder statements
- Document unified scheduler coverage, alias behavior, and real task status semantics

**Step 2: Run focused verification**

Run: `go test ./internal/jobs ./internal/domain/preferences ./internal/domain/digest -v`

**Step 3: Run broader backend verification**

Run: `go build ./...`
