# Semantic Label Board System Tasks 11-12 Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Complete OpenSpec change `semantic-label-board-system` tasks 11 and 12, then mark their task checkboxes complete only after verification.

**Architecture:** Task 11 is mostly present in the current tree and should be verified/fixed surgically. Task 12 introduces auxiliary-label descriptions and split embeddings: `semantic_labels.embedding` remains the storage embedding generated from `label + description`, while new `semantic_labels.merge_embedding` is generated from label-only and used only for L2 auxiliary-label merge.

**Tech Stack:** Go + Gin + GORM + PostgreSQL/pgvector; Nuxt 4 + Vue 3 `<script setup lang="ts">` + TypeScript.

---

## Context Files

Read these before editing:
- `openspec/changes/semantic-label-board-system/tasks.md`
- `openspec/changes/semantic-label-board-system/design.md`
- `openspec/changes/semantic-label-board-system/specs/auxiliary-label/spec.md`
- `openspec/changes/semantic-label-board-system/specs/semantic-label-model/spec.md`
- `openspec/changes/semantic-label-board-system/specs/board-management-api/spec.md`
- `backend-go/AGENTS.md`
- `front/AGENTS.md`

## Guardrails

- Before editing any function/method/class, attempt GitNexus impact analysis per `AGENTS.md` (for example via available GitNexus tool or CLI). If GitNexus is unavailable, record that fact in your report and continue only with minimal scoped edits.
- Ignore unrelated dirty worktree changes.
- Do not run formatters/linter config changes.
- Mark `openspec/changes/semantic-label-board-system/tasks.md` checkbox items complete only after implementing and verifying them.

---

### Task 1: Verify/fix task 11 manual auxiliary recommendation and composition management

**TDD scenario:** Modifying tested code — run existing focused tests first; add only missing coverage if behavior is absent.

**Files:**
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_handler.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_handler_test.go`
- Modify if needed: `front/app/api/semanticBoards.ts`
- Modify if needed: `front/app/features/tags/components/AuxiliaryLabelPicker.vue`
- Modify if needed: `front/app/features/tags/components/AddSemanticBoardDialog.vue`
- Modify if needed: `front/app/features/tags/components/BoardCompositionPanel.vue`
- Modify if needed: `front/app/features/tags/components/TagsPage.vue`

**Step 1: Run focused backend tests for task 11**

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'TestSemanticBoardHandlerSuggestAuxiliaries|TestSemanticBoardHandlerSuggestAuxiliariesForBoard|TestSemanticBoardHandlerAddComposition|TestSemanticBoardHandlerCRUDAndComposition' -v
```
Expected: PASS.

**Step 2: Inspect task 11 requirements against implementation**

Check that all are true:
- `GET /api/semantic-boards/suggest-auxiliaries` accepts `label`, optional `description`, `search`, `page`, `page_size`, optional `exclude_board_id`.
- `GET /api/semantic-boards/:id/suggest-auxiliaries` uses existing board label+description and excludes already composed labels before pagination.
- Suggestions use active auxiliary labels and `semantic_labels.embedding` storage embedding, sorted by cosine similarity descending.
- `POST /api/semantic-boards/:id/composition` validates board exists, auxiliary exists and active, writes idempotently, and does not trigger backfill.
- Frontend API has `suggestAuxiliaries`, `suggestAuxiliariesForBoard`, `addComposition`.
- `AuxiliaryLabelPicker.vue` supports search, pagination, checkbox selection, create/edit modes.
- `AddSemanticBoardDialog.vue` passes selected auxiliary IDs with create request.
- `BoardCompositionPanel.vue` lets user add labels to an existing board and warns that historical tag-board relations need manual backfill.

**Step 3: If a gap exists, write the smallest test or UI change**

Likely small fixes to consider:
- In `BoardCompositionPanel.vue`, after successful add, surface a user-facing note such as `已添加构成标签。历史标签归属不会自动回填，可手动触发 board 回填。`
- Ensure edit-board picker excludes already composed labels server-side via `suggestAuxiliariesForBoard`.
- Do not refactor surrounding UI.

**Step 4: Re-run focused task 11 checks**

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'TestSemanticBoardHandlerSuggestAuxiliaries|TestSemanticBoardHandlerSuggestAuxiliariesForBoard|TestSemanticBoardHandlerAddComposition|TestSemanticBoardHandlerCRUDAndComposition' -v
cd front && pnpm exec nuxi typecheck
```
Expected: PASS / typecheck success.

**Step 5: Mark task 11 checkboxes**

If all task 11 items are implemented and verified, update `openspec/changes/semantic-label-board-system/tasks.md` items `11.1` through `11.11` from `- [ ]` to `- [x]`.

---

### Task 2: Add semantic_labels.merge_embedding model, migration, and migration tests

**TDD scenario:** New schema capability — test first.

**Files:**
- Modify: `backend-go/internal/domain/models/semantic_label.go`
- Modify: `backend-go/internal/domain/models/semantic_label_test.go`
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`
- Modify: `backend-go/internal/platform/database/db_test.go`

**Step 1: Add failing model/migration tests**

Update tests to require:
- `models.SemanticLabel` has `MergeEmbedding *string` with `gorm:"type:vector(2048);column:merge_embedding"`.
- Migration source contains `merge_embedding vector(2048)` in `CREATE TABLE IF NOT EXISTS semantic_labels`.
- A later migration (for already-applied databases) contains `ALTER TABLE semantic_labels ADD COLUMN IF NOT EXISTS merge_embedding vector(2048)`.
- Migration source contains an index name such as `idx_semantic_labels_merge_embedding`.

Run:
```bash
cd backend-go && go test ./internal/domain/models ./internal/platform/database -run 'TestSemanticLabel|TestSemanticLabelBoardSystemMigrationDocumentsSchemaCutover' -v
```
Expected: FAIL before implementation.

**Step 2: Implement model and migrations**

- In `SemanticLabel`, add `MergeEmbedding *string` after `Embedding`.
- In migration `20260521_0001`, include `merge_embedding vector(2048)` and create `idx_semantic_labels_merge_embedding` HNSW index if consistent with current embedding indexing style.
- Add new migration version after current latest, e.g. `20260522_0002`, to add `merge_embedding` to existing databases and create/drop/recreate the index as needed.
- If `EnsureSemanticLabelVectorDimension` changes embedding dimensions at runtime, update it or add a sibling helper so both `embedding` and `merge_embedding` have the same vector dimension and appropriate index behavior.

**Step 3: Re-run schema tests**

Run:
```bash
cd backend-go && go test ./internal/domain/models ./internal/platform/database -run 'TestSemanticLabel|TestSemanticLabelBoardSystemMigrationDocumentsSchemaCutover' -v
```
Expected: PASS.

---

### Task 3: Change extraction types, parser, prompt, and schema to auxiliary label objects

**TDD scenario:** New parser behavior — red/green.

**Files:**
- Modify: `backend-go/internal/domain/tagging/types.go`
- Modify: `backend-go/internal/domain/tagging/extractor_enhanced.go`
- Modify: `backend-go/internal/domain/tagging/extractor_test.go`

**Step 1: Write failing parser/schema tests**

Add/modify tests to require:
- `event` accepts `auxiliary_labels` as objects: `[{"label":"伊朗","description":"中东地区国家"}, ...]` and preserves both fields.
- `person` has same 3-5 object requirement.
- `keyword` accepts empty/missing `auxiliary_labels` and does not fail.
- `event/person` reject missing description, empty description, description equal to label, too few/too many objects.
- Schema defines `auxiliary_labels` items as objects with required `label` and `description`, and `auxiliary_labels` is not universally required for keyword.
- Prompt text states keyword does not need auxiliary labels and event/person auxiliary labels must be objects with description.

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'TestParseExtractedTags|TestBuildExtractionSystemPrompt|TestTagExtractionSchema' -v
```
Expected: FAIL before implementation.

**Step 2: Implement types and parsing**

- Add a small struct, for example:
```go
type AuxiliaryLabel struct {
    Label string `json:"label"`
    Description string `json:"description"`
}
```
- Change `TopicTag.AuxiliaryLabels` and `ExtractedTag.AuxiliaryLabels` from `[]string` to `[]AuxiliaryLabel`.
- Update raw JSON parsing to accept object arrays. Backward compatibility for old `[]string` is optional only if easy; if implemented, reject old string arrays for event/person without descriptions unless tests explicitly keep compatibility.
- Split validation:
  - event/person: require 3-5 auxiliary label objects with valid label and description.
  - keyword: allow zero auxiliary labels; if objects are present, validate them.
- Add `validateAuxiliaryLabelDescription(label, description string) error` with non-empty, max 500 chars, not just repeating label.

**Step 3: Update prompt and schema**

- Update prompt auxiliary-label section and example JSON.
- JSON schema should model `auxiliary_labels` as an array of objects `{label, description}`.
- Required fields at tag level should remain `label`, `category`; do not force keyword to emit auxiliaries.

**Step 4: Re-run parser/schema tests**

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'TestParseExtractedTags|TestBuildExtractionSystemPrompt|TestTagExtractionSchema' -v
```
Expected: PASS.

---

### Task 4: Implement auxiliary-label description and split embedding service

**TDD scenario:** New service behavior — red/green.

**Files:**
- Modify: `backend-go/internal/domain/tagging/auxiliary_label_service.go`
- Modify: `backend-go/internal/domain/tagging/auxiliary_label_service_test.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_handler.go`

**Step 1: Add failing service tests**

Update tests to require:
- `ResolveAuxiliaryLabel(ctx, label, description)` stores `Description` on L3 create.
- L3 create generates two embeddings: label-only `merge_embedding` and label+description storage `embedding`.
- L2 merge compares existing/new `merge_embedding`, not storage `embedding`.
- L1 exact/alias match does not call embedder.
- Missing/invalid description for L3 event/person style call is rejected.
- Existing task 11 suggestion APIs still sort by storage `embedding`.

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'TestAuxiliaryLabelService' -v
```
Expected: FAIL before implementation.

**Step 2: Change embedder API minimally**

Replace single-mode embedder with an embedder that can distinguish operations. One minimal option:
```go
type auxiliaryLabelEmbeddingMode string
const (
  auxiliaryLabelEmbeddingModeMerge auxiliaryLabelEmbeddingMode = "merge"
  auxiliaryLabelEmbeddingModeStorage auxiliaryLabelEmbeddingMode = "storage"
)
type auxiliaryLabelEmbedder func(ctx context.Context, input string, mode auxiliaryLabelEmbeddingMode) (string, []float64, error)
```

Update `defaultAuxiliaryLabelEmbedder` metadata operation names:
- merge mode: `auxiliary_label_merge_embedding`
- storage mode: `auxiliary_label_storage_embedding`

**Step 3: Implement split behavior**

- `ResolveAuxiliaryLabel(ctx, label, description)`:
  - L1: slug/alias exact using label only.
  - L2: generate merge embedding from label only; compare with existing `MergeEmbedding`; choose highest `RefCount`/lowest ID; add alias.
  - L3: generate storage embedding from `label + ": " + description`; create semantic label with both `MergeEmbedding` and `Embedding`, `Description` set.
- Update all callers.
- Ensure board creation/recommendation still uses board label+description storage embedding. If reusing `semanticBoardLabelEmbedder`, pass storage mode or use a wrapper.

**Step 4: Re-run service tests**

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'TestAuxiliaryLabelService|TestSemanticBoardHandlerSuggestAuxiliaries' -v
```
Expected: PASS.

---

### Task 5: Update article tagging for keyword direct-to-pool and event/person attachment

**TDD scenario:** New article tagging flow — red/green.

**Files:**
- Modify: `backend-go/internal/domain/tagging/article_tagger.go`
- Modify tests in existing suitable files or create focused tests in `backend-go/internal/domain/tagging/article_tagger_test.go` if absent.

**Step 1: Write failing tests**

Add focused tests for helper logic if full `TagArticle` integration is too heavy:
- keyword tag with description calls auxiliary service with `{label: tag.Label, description: tag.Description}` even when `AuxiliaryLabels` is empty.
- event/person tags call auxiliary service with parsed auxiliary label objects.
- keyword without description is skipped or returns validation error according to existing failure style; do not call async description generation to satisfy this requirement.

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'Test.*Auxiliary|Test.*KeywordDirect' -v
```
Expected: FAIL before implementation.

**Step 2: Implement minimal flow**

In `tagArticle` after `findOrCreateTag` and before article link creation:
- If normalized category is keyword: call a new `AttachKeywordDirectToPool(ctx, dbTag.ID, tag.Label, tag.Description)` or reuse `AttachAuxiliaryLabels(ctx, dbTag.ID, []AuxiliaryLabel{{Label: tag.Label, Description: tag.Description}})` with keyword-aware single-label path.
- If category is event/person and `len(tag.AuxiliaryLabels) > 0`: attach parsed auxiliary label objects.
- Do not auto-generate auxiliary labels for keyword.

**Step 3: Re-run tests**

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'Test.*Auxiliary|Test.*KeywordDirect|TestParseExtractedTags' -v
```
Expected: PASS.

---

### Task 6: Ensure board matching, recommendation, upgrade, and backfill keep using storage embedding

**TDD scenario:** Regression protection.

**Files:**
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_matching.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_matching_test.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_backfill.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_backfill_test.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_handler.go`
- Modify if needed: `backend-go/internal/domain/tagging/semantic_board_handler_test.go`

**Step 1: Add assertions if missing**

Tests should make it clear these flows read `semantic_labels.embedding`, not `merge_embedding`:
- matching evaluates storage vectors.
- upgrade candidate clustering reads storage vectors.
- backfill reads storage vectors.
- suggest-auxiliaries reads storage vectors.

Run:
```bash
cd backend-go && go test ./internal/domain/tagging -run 'TestSemanticBoardMatching|TestSemanticBoardUpgrade|TestSemanticBoardBackfill|TestSemanticBoardHandlerSuggestAuxiliaries' -v
```
Expected: PASS after implementation.

---

### Task 7: Full backend verification and task 12 checkbox update

**TDD scenario:** Verification.

**Files:**
- Modify: `openspec/changes/semantic-label-board-system/tasks.md`

**Step 1: Run backend verification**

Run:
```bash
cd backend-go && go test ./internal/domain/models ./internal/platform/database ./internal/domain/tagging -v
cd backend-go && go test ./...
cd backend-go && go build ./...
```
Expected: PASS.

If `golangci-lint` is installed, also run:
```bash
cd backend-go && golangci-lint run ./...
```
Expected: PASS. If not installed, report that it could not be run.

**Step 2: Mark task 12 checkboxes**

If all task 12 items are implemented and verified, update `openspec/changes/semantic-label-board-system/tasks.md` items `12.1` through `12.12` from `- [ ]` to `- [x]`.

---

### Task 8: Final task 11/12 verification report

**TDD scenario:** Final verification.

**Files:**
- No code edits unless fixing verification failures.

**Step 1: Show changed files**

Run:
```bash
git diff -- openspec/changes/semantic-label-board-system/tasks.md backend-go/internal/domain/models/semantic_label.go backend-go/internal/domain/tagging backend-go/internal/platform/database front/app/api/semanticBoards.ts front/app/features/tags/components
```

**Step 2: Report exact verification evidence**

Report:
- Which OpenSpec task IDs were completed.
- Exact test/build commands run and their exit status.
- Any skipped command and reason.
- Any known limitation or follow-up.
