# Fix Tag Hierarchy Closure Placement Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `fix-tag-hierarchy-closure` tasks 4.1-4.6 so placement either links a tag, creates a valid abstract node, or returns a structured blocker.

**Architecture:** Keep `PlaceTagInHierarchy` as the single placement entrypoint. Add blocker metadata to `PlacementResult`, route the no-parent path through a guarded node-creation decision, and remove the secondary node-creation behavior from orphan aggregation.

**Tech Stack:** Go, GORM, existing tagging domain services, SQLite-backed unit tests.

---

### Task 1: Placement Result Contract

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_placement.go:21-28`
- Test: `backend-go/internal/domain/tagging/hierarchy_placement_test.go`

**Step 1: Update result shape**

Add JSON fields to `PlacementResult`:

```go
BlockerReason string `json:"blocker_reason,omitempty"`
DiagnosticAction string `json:"diagnostic_action,omitempty"`
```

**Step 2: Add helper setters**

Add a small helper in `hierarchy_placement.go`:

```go
func markPlacementBlocker(result *PlacementResult, action, reason, diagnostic string) *PlacementResult {
	result.Action = action
	result.BlockerReason = reason
	result.DiagnosticAction = diagnostic
	return result
}
```

**Step 3: Replace generic blocker returns**

Use the helper for `pending_embedding`, `concept_match_failed`, `no_matching_concept`, `already_at_max_depth`, `no_suitable_level`, and no-parent failures.

**Step 4: Run targeted test**

Run: `go test ./internal/domain/tagging -run TestPlaceTag -v`

Expected: pass after updating tests.

### Task 2: Node Creation Decision

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_placement.go:90-160`
- Test: `backend-go/internal/domain/tagging/hierarchy_placement_test.go`

**Step 1: Add candidate context type**

Add a private type with sibling tag IDs and max article overlap:

```go
type nodeCreationContext struct {
	CandidateChildIDs []uint
	MaxArticleJaccard float64
}
```

**Step 2: Add context collector**

Collect the triggering tag plus anchor/candidate children in the same concept/category. Use distinct IDs and keep the query simple.

**Step 3: Add validation**

Reject with:
- `insufficient_siblings` when fewer than 2 distinct children exist.
- `low_information_gain` when max Jaccard is greater than 0.70 or leaf-to-depth ratio would fall below 1.5.
- `no_anchor_context` when no anchors/candidates provide context.

**Step 4: Route no-parent path**

After `resolveParent` returns nil, call the node-creation decision instead of setting `Action = "unplaced"`.

**Step 5: Run targeted test**

Run: `go test ./internal/domain/tagging -run TestPlaceTagAtLevel -v`

Expected: tests cover blocker/action shape.

### Task 3: Created Node Action

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_placement.go:350-391`
- Test: `backend-go/internal/domain/tagging/hierarchy_placement_test.go`

**Step 1: Use existing creator**

Call `createAbstractAtLevel` only from `placeTagAtLevel` node-creation decision.

**Step 2: Populate result**

On success set:

```go
result.Action = "created_node"
result.ParentID = &parentID
result.ParentLabel = parentLabel
result.CreatedParents = append(result.CreatedParents, parentID)
```

**Step 3: Preserve embedding generation**

Keep `createAbstractAtLevel` async embedding generation intact.

**Step 4: Run targeted test**

Run: `go test ./internal/domain/tagging -run TestPlaceTagAtLevel -v`

Expected: created-node path sets action and parent metadata.

### Task 4: Remove Second Node Creation Entry

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_aggregation.go:98-106`
- Test: `backend-go/internal/domain/tagging/hierarchy_placement_test.go`

**Step 1: Remove fallback creation**

In `aggregateToUpperLevel`, delete the fallback that calls `createAbstractAtLevel`.

**Step 2: Log structured skip**

Log that orphan aggregation skipped node creation because `PlaceTagInHierarchy` owns node creation.

**Step 3: Run targeted tests**

Run: `go test ./internal/domain/tagging -run "PlaceTag|Aggregate" -v`

Expected: no direct `createAbstractAtLevel` call remains outside `placeTagAtLevel`.

### Task 5: Mark OpenSpec Tasks

**Files:**
- Modify: `openspec/changes/fix-tag-hierarchy-closure/tasks.md:27-32`

**Step 1: Confirm implementation**

Verify 4.1-4.6 behavior is implemented and targeted tests pass.

**Step 2: Update task checkboxes**

Change tasks 4.1-4.6 from `- [ ]` to `- [x]`.

**Step 3: Report**

Summarize completed tasks and targeted verification output.
