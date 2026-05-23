# Hierarchy Config Preview Apply Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `fix-tag-hierarchy-closure` tasks 3.1-3.5 by separating hierarchy config preview from confirmed apply.

**Architecture:** Backend hierarchy config API must default to side-effect-free preview. Confirmed apply must save only changed templates, reject active rebuild conflicts, trigger rebuild only for changed categories, and return impact metadata. Existing partial edits in this worktree are in-progress and must be reviewed before continuing.

**Tech Stack:** Go, Gin handlers, GORM repositories, source-level tests where DB/sqlite setup is obsolete per user instruction.

---

### Task 1: Review Current Partial Edits

**Files:**
- Review: `backend-go/internal/domain/tagging/hierarchy_handler.go`
- Review: `backend-go/internal/domain/tagging/hierarchy_config.go`
- Review: `backend-go/internal/domain/tagging/hierarchy_config_test.go`

**Step 1: Inspect current diff**

Run:

```bash
git diff -- backend-go/internal/domain/tagging/hierarchy_handler.go backend-go/internal/domain/tagging/hierarchy_config.go backend-go/internal/domain/tagging/hierarchy_config_test.go
```

Expected: See in-progress preview/apply split and source-level tests.

**Step 2: Identify compile errors before editing**

Run:

```bash
go test ./internal/domain/tagging -run "TestHierarchyConfigHandlerSeparatesPreviewAndApply|TestConfigImpactIncludesRebuildEstimateAndViolationSummary" -v
```

Expected: May fail or fail to compile. Fix only within this plan scope.

### Task 2: Make Source-Level Tests Express Target Behavior

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_config_test.go`

**Step 1: Ensure tests are non-sqlite**

Tests for 3.1-3.5 must use source inspection or pure helpers only. Do not add sqlite-backed tests.

**Step 2: Cover required target markers**

Ensure tests verify:

- `UpdateHierarchyConfig` no longer directly calls `SaveConfig`, `TriggerTemplateRebuild`, or `ExecuteJob`.
- `PreviewHierarchyConfig` exists and `RegisterHierarchyRoutes` includes `POST("/config/preview"`.
- `applyHierarchyConfig` contains changed-category filtering, active rebuild conflict detection, `SaveConfig`, `TriggerTemplateRebuild`, and `ExecuteJob`.
- `ConfigImpact` includes `AffectedTagCount`, `EstimatedRebuildDurationSeconds`, and `ViolationSummary`.

**Step 3: Run tests and verify RED/GREEN as applicable**

Run:

```bash
go test ./internal/domain/tagging -run "TestHierarchyConfigHandlerSeparatesPreviewAndApply|TestConfigImpactIncludesRebuildEstimateAndViolationSummary" -v
```

Expected: PASS after implementation.

### Task 3: Complete Preview/Apply Handler Split

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_handler.go`

**Step 1: Keep preview side-effect free**

Implement or correct:

- `PreviewHierarchyConfig(c *gin.Context)` parses templates, validates no add/delete, calls `previewConfigImpact`, returns `preview_only: true`.
- `UpdateHierarchyConfig(c *gin.Context)` supports default preview and confirmed apply via `apply: true` or `mode: "apply"`.
- Default `PUT /config` without explicit apply must be preview-only.

**Step 2: Implement confirmed apply**

Implement or correct `applyHierarchyConfig`:

- Determine changed template categories using serialized template comparison.
- If no changed categories, return no-op result without save/rebuild.
- Reject changed categories with active rebuild job using `409 Conflict` behavior through a typed conflict error.
- Save config once.
- Trigger template rebuild and start execution only for changed categories.

**Step 3: Keep changes minimal**

Do not introduce compatibility layers beyond `mode/apply` because current change explicitly defines the new contract.

### Task 4: Complete Impact DTO Metadata

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_config.go`

**Step 1: Add fields**

Ensure `ConfigImpact` includes:

```go
AffectedTagCount                int            `json:"affected_tag_count"`
EstimatedRebuildDurationSeconds int            `json:"estimated_rebuild_duration_seconds"`
ViolationSummary                map[string]int `json:"violation_summary,omitempty"`
```

**Step 2: Populate fields**

After preview details are computed, set:

- `AffectedTagCount`: distinct impacted `TagID` count from details.
- `ViolationSummary`: counts grouped by detail issue.
- `EstimatedRebuildDurationSeconds`: minimal deterministic estimate based on affected count and `defaultAvgPlacementTime`.

### Task 5: Verify and Mark Tasks

**Files:**
- Modify: `openspec/changes/fix-tag-hierarchy-closure/tasks.md`

**Step 1: Run focused verification**

Run:

```bash
go test ./internal/domain/tagging -run "TestHierarchyConfigHandlerSeparatesPreviewAndApply|TestConfigImpactIncludesRebuildEstimateAndViolationSummary" -v
```

Expected: PASS.

**Step 2: Compile related package if feasible**

Run:

```bash
go test ./internal/domain/tagging -run "TestPreviewConfigImpact|TestGeneratePendingChanges" -v
```

Expected: PASS or report unrelated existing failures clearly.

**Step 3: Mark OpenSpec tasks**

If verification passes, mark tasks 3.1-3.5 as complete in `openspec/changes/fix-tag-hierarchy-closure/tasks.md`.

**Step 4: Report back**

Return concise Chinese report:

- files changed
- tests run
- tasks marked
- blockers, if any
