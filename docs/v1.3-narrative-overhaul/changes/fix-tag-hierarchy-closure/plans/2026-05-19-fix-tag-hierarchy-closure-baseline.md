# Fix Tag Hierarchy Closure Baseline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add the first baseline tests for `fix-tag-hierarchy-closure` tasks 1.1-1.5 so the current hierarchy closure breakpoints are executable and visible.

**Architecture:** Keep this batch test-only. Prefer narrow tests that document the current broken contract or the expected contract before production changes. Do not refactor production code in this batch unless a test cannot be expressed without a tiny seam.

**Tech Stack:** Go test with Gin/GORM/sqlite for backend domain tests; Vitest + happy-dom for Vue composable tests.

---

### Task 1: Template Preview Side-Effect Baseline

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_config_test.go`
- Read-only context: `backend-go/internal/domain/tagging/hierarchy_handler.go`

**Step 1: Add a focused test**

Add a test near existing config impact tests that seeds a config, an abstract node, an abstract relation, and an active leaf tag. The test should exercise the preview path that currently exists (`previewConfigImpact`) and assert it has no side effects on `HierarchyConfig`, `HierarchyConfigVersion`, `RebuildJob`, `TopicTag`, and `TopicTagRelation`.

Use this shape:

```go
func TestPreviewConfigImpactHasNoPersistenceSideEffects(t *testing.T) {
    db := setupConfigTestDB(t)
    mgr := GetHierarchyManager()
    mgr.LoadSystemDefaults()

    parent := models.TopicTag{Label: "preview-parent", Slug: "preview-parent", Category: "event", Source: "abstract", Status: "active"}
    child := models.TopicTag{Label: "preview-child", Slug: "preview-child", Category: "event", Source: "llm", Status: "active"}
    if err := db.Create(&parent).Error; err != nil { t.Fatalf("create parent: %v", err) }
    if err := db.Create(&child).Error; err != nil { t.Fatalf("create child: %v", err) }
    if err := db.Create(&models.TopicTagRelation{ParentID: parent.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil { t.Fatalf("create relation: %v", err) }

    before := countHierarchySideEffects(t, db)
    _, err := previewConfigImpact(&[]CategoryHierarchyTemplate{{Category: "event", MaxLevel: 2, Levels: []AbstractionLevel{{Level: 1, Name: "事件类型"}, {Level: 2, Name: "具体事件", IsLeaf: true}}}})
    if err != nil { t.Fatalf("previewConfigImpact: %v", err) }
    after := countHierarchySideEffects(t, db)
    if after != before { t.Fatalf("preview side effects: before=%+v after=%+v", before, after) }
}
```

If `RebuildJob` is not migrated in `setupConfigTestDB`, add it to that helper. Keep helper changes scoped to test migration only.

**Step 2: Run the focused test**

Run from `backend-go`:

```bash
go test ./internal/domain/tagging -run TestPreviewConfigImpactHasNoPersistenceSideEffects -v
```

Expected: PASS because this test targets pure preview helper side effects. Production API preview/apply split remains for later tasks.

### Task 2: WebSocket Protocol Mismatch Baseline

**Files:**
- Create: `front/app/composables/useWebSocketRebuild.test.ts`
- Read-only context: `front/app/composables/useWebSocketRebuild.ts`

**Step 1: Mock WebSocket and mount a probe component**

Create a local mock class that records the last socket instance and exposes `emitMessage(data: unknown)`.

**Step 2: Assert current frontend consumes only `hierarchy_rebuild`**

Add tests that emit current backend messages and verify they are ignored:

```ts
socket.emitMessage({ type: 'rebuild_progress', processed: 1, total: 3, category: 'event' })
expect(exposed.status.value).toBe('idle')
```

Also emit the target contract and verify it updates:

```ts
socket.emitMessage({ type: 'hierarchy_rebuild', status: 'processing', processed: 1, total: 3, category: 'event', current_tag: 'Tag A' })
expect(exposed.status.value).toBe('processing')
expect(exposed.currentTag.value).toBe('Tag A')
```

**Step 3: Run the focused test**

Run from `front`:

```bash
pnpm exec vitest run app/composables/useWebSocketRebuild.test.ts
```

Expected: PASS and documents the mismatch.

### Task 3: Placement No-Parent Baseline

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_placement_test.go`
- Read-only context: `backend-go/internal/domain/tagging/hierarchy_placement.go`

**Step 1: Add a direct `placeTagAtLevel` test**

Use `setupPlacementTestDB`, create a leaf `TopicTag`, and call `placeTagAtLevel` with a concept match and no abstract parent candidates. Assert action is `unplaced`, `ParentID` is nil, and no `TopicTagRelation` rows exist.

**Step 2: Run the focused test**

Run from `backend-go`:

```bash
go test ./internal/domain/tagging -run TestPlaceTagAtLevelNoParentLeavesTagUnplaced -v
```

Expected: PASS and documents the current broken behavior that later tasks will replace.

### Task 4: Cold-Start Bootstrap Gap Baseline

**Files:**
- Modify: `backend-go/internal/domain/tagging/sector_generation_test.go`
- Read-only context: `backend-go/internal/domain/tagging/sector_generation.go`, `backend-go/internal/app/runtime.go`

**Step 1: Add a source-level guard test**

Add a narrow test that reads `internal/app/runtime.go` and asserts it does not contain `AutoGenerateSectors`. This is a minimal baseline proving runtime/placement startup currently does not trigger Sector bootstrap.

**Step 2: Run the focused test**

Run from `backend-go`:

```bash
go test ./internal/domain/tagging -run TestRuntimeColdStartDoesNotTriggerAutoGenerateSectors -v
```

Expected: PASS and documents the gap.

### Task 5: LLM Sector Confirm Response Baseline

**Files:**
- Create: `backend-go/internal/domain/narrative/sector_handler_test.go`
- Read-only context: `backend-go/internal/domain/narrative/sector_handler.go`

**Step 1: Add a handler response test for empty diff**

Set up sqlite `database.DB` with `BoardConcept` and `TopicTag`, create a Gin router that calls `confirmRegenerateSectorsHandler`, post an empty diff, and assert the response contains top-level `message` but no per-item execution result array.

**Step 2: Run the focused test**

Run from `backend-go`:

```bash
go test ./internal/domain/narrative -run TestConfirmRegenerateSectorsReturnsOnlyMessage -v
```

Expected: PASS and documents the missing itemized result contract.

### Task 6: Update OpenSpec Task Checkboxes

**Files:**
- Modify: `openspec/changes/fix-tag-hierarchy-closure/tasks.md`

**Step 1: Mark completed baseline tasks**

After the focused tests pass, update tasks `1.1` through `1.5` from `- [ ]` to `- [x]`.

**Step 2: Run OpenSpec status**

Run from repo root:

```bash
openspec status --change "fix-tag-hierarchy-closure" --json
```

Expected: progress reports `5/56` complete.
