# Fix Tag Hierarchy Closure Sector Results Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `fix-tag-hierarchy-closure` tasks 7.1-7.5 so LLM Sector confirmation returns backend execution facts and the UI displays partial failures.

**Architecture:** Change `LLMExecuteSectorDiff` from fire-and-forget error logging to a per-operation result collector. Keep executing independent items after failures, return result rows with operation type/status/counts/IDs/error, and let `SectorApprovalPanel` render backend results rather than estimating success counts from the submitted diff.

**Tech Stack:** Go, Gin/GORM, Vue 3 `<script setup>` and TypeScript.

---

### Task 1: Define Execution Result DTO

**Files:**
- Modify: `backend-go/internal/domain/tagging/sector_generation.go`
- Modify: `front/app/api/boardConcepts.ts`

**Step 1: Add backend DTOs**

Add exported structs:

```go
type SectorDiffExecutionResult struct {
	Items []SectorDiffExecutionItem `json:"items"`
	Succeeded int `json:"succeeded"`
	Failed int `json:"failed"`
	AffectedTagCount int `json:"affected_tag_count"`
	CreatedIDs []uint `json:"created_ids,omitempty"`
	MovedTagCount int `json:"moved_tag_count"`
}

type SectorDiffExecutionItem struct {
	Operation string `json:"operation"`
	Name string `json:"name,omitempty"`
	Status string `json:"status"`
	AffectedTagCount int `json:"affected_tag_count"`
	CreatedIDs []uint `json:"created_ids,omitempty"`
	MovedTagCount int `json:"moved_tag_count"`
	Error string `json:"error,omitempty"`
}
```

**Step 2: Add frontend types**

Mirror the JSON shape in `boardConcepts.ts` and change `confirmRegenerateSectors` return type to `ApiResponse<SectorDiffExecutionResult>`.

### Task 2: Return Per-Operation Results

**Files:**
- Modify: `backend-go/internal/domain/tagging/sector_generation.go`

**Step 1: Change signature**

Change `LLMExecuteSectorDiff` to return `(*SectorDiffExecutionResult, error)`.

**Step 2: Add result helpers**

Add private helper methods/functions to append success/failed items and aggregate totals.

**Step 3: Add operation behavior**

For add: create concept, set source, generate embedding; success includes created ID.

For merge: move tags from each source to target, count moved rows using `RowsAffected`, deactivate source concepts, regenerate target embedding; success includes moved count.

For split: create new concepts, reassign source tags to new concepts, count moved rows, deactivate source only when empty; success includes created IDs and moved count.

**Step 4: Continue after item failures**

Failed item should not abort other items. Only return a non-nil error for invalid call-level inputs such as nil diff.

### Task 3: Handler Response

**Files:**
- Modify: `backend-go/internal/domain/narrative/sector_handler.go`
- Modify: `backend-go/internal/domain/narrative/sector_handler_test.go`

**Step 1: Return result data**

`confirmRegenerateSectorsHandler` should call `LLMExecuteSectorDiff` and return `gin.H{"success": true, "data": result}`.

**Step 2: Update tests**

Replace the legacy "only message" test with a test that asserts `data.items`, `data.succeeded`, and `data.failed` exist for an empty diff.

### Task 4: Frontend Display Backend Results

**Files:**
- Modify: `front/app/features/tags/components/SectorApprovalPanel.vue`

**Step 1: Use API result**

Build `execResult` from `res.data`, not local diff lengths.

**Step 2: Show partial failures**

Render each failed item error. Keep existing summary styling; just map backend fields to existing summary data.

**Step 3: Preserve partial success UX**

If `res.success` is true but `data.failed > 0`, show execution completed with failed reasons rather than treating the whole request as failed.

### Task 5: Refresh Closure Status Hook Point

**Files:**
- Modify minimally where practical in `front/app/features/tags/components/TagsPage.vue` if existing `done` handler already refreshes sector/hierarchy/closure state.

**Step 1: Inspect existing handler**

If `done` already refreshes the dependent panels, leave it unchanged.

**Step 2: If missing, add refresh calls**

On Sector approval `done`, refresh Sector list, hierarchy tree, timeline/pending count, and closure status using existing page functions only.

### Task 6: Verify And Mark Tasks

**Files:**
- Modify: `openspec/changes/fix-tag-hierarchy-closure/tasks.md:52-56`

**Step 1: Run tests**

Run from `backend-go`: `go test ./internal/domain/tagging ./internal/domain/narrative -run "Sector|Regenerate" -v`.

Run from `front`: `pnpm test:unit -- SectorApprovalPanel` if a focused test exists; otherwise run `pnpm test:unit -- useWebSocketRebuild` to ensure changed API types did not break current unit suite and report no focused panel test exists.

**Step 2: Update checkboxes**

Mark 7.1-7.5 complete only if implemented and verified.

**Step 3: Report**

Summarize changed files and verification output.
