# Semantic Label Governance Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `semantic-label-board-system` phase 3: auxiliary-label disable, manual alias merge, board composition removal, and governance tests.

**Architecture:** Keep governance operations in `AuxiliaryLabelService` because the service already owns auxiliary-label lookup, alias handling, and attachment. Governance actions must only update semantic labels, tag-auxiliary associations, and board composition; they must not rewrite `topic_tag_board_labels` or enqueue backfill.

**Tech Stack:** Go, GORM, PostgreSQL models, existing sqlmock-style/unit tests under `backend-go/internal/domain/tagging`.

---

### Task 1: Add Governance Service Methods

**Files:**
- Modify: `backend-go/internal/domain/tagging/auxiliary_label_service.go`
- Test: `backend-go/internal/domain/tagging/auxiliary_label_service_test.go`

**Step 1: Run GitNexus impact before editing**

Run impact for existing symbols before modifying the service:

```text
gitnexus_impact target=AuxiliaryLabelService direction=upstream repo=my-robot
gitnexus_impact target=AttachAuxiliaryLabels direction=upstream repo=my-robot
gitnexus_impact target=ResolveAuxiliaryLabel direction=upstream repo=my-robot
gitnexus_impact target=loadActiveAuxiliaryLabels direction=upstream repo=my-robot
gitnexus_impact target=addAlias direction=upstream repo=my-robot
```

Stop and report if any result is HIGH or CRITICAL.

**Step 2: Add failing tests**

Add tests covering:
- `DisableAuxiliaryLabel` marks only auxiliary labels as `disabled`.
- `MergeAuxiliaryLabelAlias` moves source tag associations to target, deduplicates target aliases, disables source, and recalculates ref counts.
- `RemoveBoardComposition` deletes only the requested `(board_id, auxiliary_label_id)` row.
- Governance actions do not delete or rewrite `topic_tag_board_labels`.

Use existing test helpers and fixtures in `auxiliary_label_service_test.go`.

**Step 3: Run tests and confirm failure**

Run from `backend-go`:

```powershell
go test ./internal/domain/tagging -run "TestAuxiliaryLabelService" -v
```

Expected: fail because governance methods do not exist.

**Step 4: Implement minimal service methods**

Add methods on `AuxiliaryLabelService`:

```go
func (s *AuxiliaryLabelService) DisableAuxiliaryLabel(ctx context.Context, labelID uint) error
func (s *AuxiliaryLabelService) MergeAuxiliaryLabelAlias(ctx context.Context, sourceID uint, targetID uint) error
func (s *AuxiliaryLabelService) RemoveBoardComposition(ctx context.Context, boardID uint, auxiliaryLabelID uint) error
```

Implementation requirements:
- Validate non-zero IDs.
- Require source/target auxiliary labels for disable and merge.
- Require board label type for board ID and auxiliary label type for auxiliary ID when removing composition.
- Merge alias list by adding source label and source aliases to target aliases with existing case/slug-insensitive semantics.
- Migrate `topic_tag_semantic_labels` source associations to target with `OnConflict DoNothing`, then delete source associations.
- Recount source and target `ref_count` from `topic_tag_semantic_labels` after migration.
- Set merged source status to `disabled`.
- Do not modify `topic_tag_board_labels` and do not enqueue backfill.

**Step 5: Run focused tests**

Run from `backend-go`:

```powershell
go test ./internal/domain/tagging -run "TestAuxiliaryLabelService" -v
```

Expected: pass.

### Task 2: Verify Existing Exclusion Behavior

**Files:**
- Modify: `backend-go/internal/domain/tagging/auxiliary_label_service_test.go`

**Step 1: Add or extend test for disabled exclusion**

Ensure an existing disabled auxiliary label is ignored by L1 alias/slug reuse and by L2 embedding merge candidate loading. The existing `loadActiveAuxiliaryLabels` filter should already satisfy this.

**Step 2: Run focused tests**

Run from `backend-go`:

```powershell
go test ./internal/domain/tagging -run "TestAuxiliaryLabelService" -v
```

Expected: pass.

### Task 3: Mark OpenSpec Tasks Complete

**Files:**
- Modify: `openspec/changes/semantic-label-board-system/tasks.md`

**Step 1: Update completed checkboxes**

Mark these tasks complete:
- `3.1 实现禁用辅助标签：status=disabled，后续匹配和升级候选排除`
- `3.2 实现手动合并 alias：迁移 topic_tag_semantic_labels，积累 aliases`
- `3.3 实现从 board_composition 移除辅助标签`
- `3.4 编写治理能力测试：禁用、alias 合并、composition 移除不自动回填`

Do not mark `1.7`; it remains blocked until phase 9 deprecated-code deletion.

**Step 2: Run final phase 3 verification**

Run from `backend-go`:

```powershell
go test ./internal/domain/tagging ./internal/domain/models ./internal/platform/database -v
```

Expected: pass.

**Step 3: Report changes and verification output**

Return a concise summary with files changed, completed tasks, and exact test command results.
