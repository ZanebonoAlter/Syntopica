# Fix Tag Hierarchy Closure Rebuild Events Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `fix-tag-hierarchy-closure` tasks 6.1-6.5 by making backend rebuild events and frontend rebuild state use one `hierarchy_rebuild` contract.

**Architecture:** Keep `RebuildService` as the backend event source. Replace legacy `rebuild_progress` / `rebuild_complete` payloads with one event type and status field, add a failed broadcast path, and make `useWebSocketRebuild` consume all three statuses with typed optional fields.

**Tech Stack:** Go, Gin/GORM domain service, Vue 3 composable with Vitest.

---

### Task 1: Backend Event Contract

**Files:**
- Modify: `backend-go/internal/domain/tagging/rebuild_service.go`
- Test: `backend-go/internal/domain/tagging/rebuild_service_test.go`

**Step 1: Change progress event**

Update `broadcastProgress` to send:

```go
map[string]interface{}{
	"type": "hierarchy_rebuild",
	"status": "processing",
	"job_id": jobID,
	"category": category,
	"processed": processed,
	"total": total,
	"failed_count": failedCount,
	"estimated_remaining_seconds": remainingSeconds,
	"current_tag": currentTag,
}
```

Accept `failedCount int` and `currentTag string` as parameters.

**Step 2: Change complete event**

Update `broadcastComplete` to send `type: "hierarchy_rebuild"`, `status: "completed"`, `processed`, `total`, and `failed_count`.

**Step 3: Add failed event**

Add `broadcastFailed(jobID, category string/uint, processed, total, failedCount int, err string)` with `status: "failed"` and `error`.

**Step 4: Update call sites**

Pass current tag label and failed count to progress broadcasts. On bootstrap/query/status failures, persist failed status with `UpdateRebuildJobStatus(..., errorDetail)` and broadcast failed before returning.

### Task 2: Backend Tests

**Files:**
- Modify: `backend-go/internal/domain/tagging/rebuild_service_test.go`

**Step 1: Add source-level event contract test**

Add or update a minimal test that reads `rebuild_service.go` and asserts legacy event names are absent and `type: "hierarchy_rebuild"`, `status: "processing"`, `status: "completed"`, `status: "failed"`, `failed_count`, `estimated_remaining_seconds`, `current_tag`, and `error` are present.

**Step 2: Add failure persistence test if practical**

Exercise an execution failure path and assert `error_detail` persists. If direct WebSocket capture is impractical, the source-level contract test is acceptable for broadcast shape.

**Step 3: Run backend targeted tests**

Run from `backend-go`: `go test ./internal/domain/tagging -run "Rebuild|PlaceTag" -v`.

### Task 3: Frontend Composable Contract

**Files:**
- Modify: `front/app/composables/useWebSocketRebuild.ts`
- Modify: `front/app/composables/useWebSocketRebuild.test.ts`

**Step 1: Extend message type**

Add `job_id`, `failed_count`, `estimated_remaining_seconds` to `RebuildProgressMessage`.

**Step 2: Track failed count and estimated remaining**

Add refs for `jobId`, `failedCount`, and `estimatedRemainingSeconds`, update them when messages arrive, and reset them in `reset()`.

**Step 3: Clear stale error on non-failed progress**

When status is not `failed`, clear `errorMessage` unless a new `error` is present.

**Step 4: Update tests**

Replace the old legacy-ignore expectation with tests for `processing`, `completed`, and `failed` `hierarchy_rebuild` events.

**Step 5: Run frontend targeted test**

Run from `front`: `pnpm test:unit -- useWebSocketRebuild`.

### Task 4: Mark OpenSpec Tasks

**Files:**
- Modify: `openspec/changes/fix-tag-hierarchy-closure/tasks.md:44-48`

**Step 1: Verify targeted tests pass**

Backend and frontend targeted tests must pass.

**Step 2: Update checkboxes**

Mark 6.1-6.5 complete.

**Step 3: Report**

Summarize changed files and verification output.
