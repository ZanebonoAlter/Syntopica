# Fix Tag Hierarchy Closure Anchor Signals Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `fix-tag-hierarchy-closure` tasks 5.1-5.4 by persisting anchor signals and making placement consume them.

**Architecture:** Add a lightweight `hierarchy_anchor_signals` table with TTL. `GenerateAnchorSignals` writes current category signals to the table, cleanup removes expired rows, and `searchAnchors` reads active signals to add parent context before existing cotag/embedding anchors.

**Tech Stack:** Go, GORM, PostgreSQL migrations, existing tagging placement flow.

---

### Task 1: Add Anchor Signal Model And Migration

**Files:**
- Create: `backend-go/internal/domain/models/hierarchy_anchor_signal.go`
- Modify: `backend-go/internal/platform/database/migrator.go`
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`
- Test: `backend-go/internal/domain/tagging/hierarchy_placement_test.go`

**Step 1: Create model**

Add `HierarchyAnchorSignal` with `ID`, `Category`, `CenterTagID`, `MemberTagIDs []uint` serialized as JSON, `ExpiresAt`, `CreatedAt`, `UpdatedAt`, and `TableName() string` returning `hierarchy_anchor_signals`.

**Step 2: Add first-time migration model**

Add `&models.HierarchyAnchorSignal{}` to `autoMigrateModels`.

**Step 3: Add versioned PostgreSQL migration**

Append a migration after the existing latest version that creates `hierarchy_anchor_signals` with JSONB `member_tag_ids`, indexes on `category`, `center_tag_id`, and `expires_at`.

**Step 4: Update test migrator**

Add the model to placement test `AutoMigrate`.

### Task 2: Persist Generated Signals

**Files:**
- Modify: `backend-go/internal/domain/tagging/cleanup_v2.go:223-290`
- Test: add or update tagging tests.

**Step 1: Add TTL constant**

Use a short explicit package constant such as `anchorSignalTTL = 24 * time.Hour`.

**Step 2: Add cleanup helper**

Create `cleanupExpiredAnchorSignals(db *gorm.DB) error` that deletes rows with `expires_at <= now`.

**Step 3: Write generated signals**

At the end of `GenerateAnchorSignals`, delete existing non-expired rows for the category and insert rows for each generated signal with `ExpiresAt = now + anchorSignalTTL`.

**Step 4: Preserve return type**

Keep returning `[]AnchorSignal` for existing scheduler summary behavior.

### Task 3: Consume Signals During Placement

**Files:**
- Modify: `backend-go/internal/domain/tagging/hierarchy_placement.go:319-398`
- Test: `backend-go/internal/domain/tagging/hierarchy_placement_test.go`

**Step 1: Load active signals**

In `searchAnchors`, load active `HierarchyAnchorSignal` rows for the tag category where `expires_at > now`, then filter in Go for signals whose `MemberTagIDs` contain the current tag ID.

**Step 2: Convert signal members into parent anchors**

For other member tag IDs, find their existing abstract parent relation and parent tag. If parent concept matches the requested concept, append an `Anchor` with `Source: "anchor_signal"` and similarity around `0.82`.

**Step 3: Keep existing cotag and embedding fallback**

Do not remove cotag/embedding logic. Ensure duplicate parents are skipped with the existing `seen` map.

**Step 4: Test consumption**

Add a SQLite unit test that creates a signal containing the trigger tag and a sibling tag with a parent, calls a helper or `searchAnchors`, and asserts an anchor with `Source == "anchor_signal"` is returned.

### Task 4: Mark OpenSpec Tasks

**Files:**
- Modify: `openspec/changes/fix-tag-hierarchy-closure/tasks.md:36-40`

**Step 1: Run targeted tests**

Run from `backend-go`: `go test ./internal/domain/tagging -run "Anchor|PlaceTag" -v`.

**Step 2: Update task checkboxes**

Mark 5.1-5.4 complete. Leave 5.5 unchecked because the implementation kept anchor signal persistence instead of deleting the capability.

**Step 3: Report**

Summarize changed files and verification output.
