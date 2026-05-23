# Semantic Board Narrative Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor daily `NarrativeBoard` generation so all boards derive from active global `SemanticBoard` records instead of abstract-tree or old board-concept paths.

**Architecture:** `SemanticBoard` remains the long-lived global semantic asset in `semantic_labels(label_type=board)`. Daily `narrative_boards` are scope-specific instances created from persisted `topic_tag_board_labels` matches for active event tags on a date. Previous-board continuation is matched by `semantic_board_id + scope + previous day`, and board narrative prompts use the SemanticBoard label/description.

**Tech Stack:** Go, GORM, SQLite focused tests, existing `narrative` domain package, OpenSpec change `semantic-label-board-system`.

**Current State:** This plan starts from a partially edited working tree. Files already touched for phase 7 are `backend-go/internal/domain/narrative/collector.go`, `board_creation.go`, `board_narrative_generator.go`, `service.go`, and `service_test.go`. Do not reset unrelated phase 3-6 changes. Do not commit.

---

### Task 1: Verify Current Partial Implementation Compiles

**Files:**
- Inspect: `backend-go/internal/domain/narrative/collector.go`
- Inspect: `backend-go/internal/domain/narrative/board_creation.go`
- Inspect: `backend-go/internal/domain/narrative/board_narrative_generator.go`
- Inspect: `backend-go/internal/domain/narrative/service.go`
- Inspect: `backend-go/internal/domain/narrative/service_test.go`

**Step 1: Run focused compile/test command**

Run: `rtk go test ./internal/domain/narrative -run "TestCollectSemanticBoardNarrativeInputs|TestCreateBoardFromSemanticBoard|TestBuildBoardNarrativePrompt" -count=1 -v`

Expected: It may fail initially because the current state was interrupted mid-edit. Capture exact compiler/test errors.

**Step 2: Fix only compile errors caused by current phase 7 edits**

Allowed fixes:
- Remove unused imports.
- Fix helper signatures.
- Fix GORM query syntax mistakes.
- Fix test helper migration omissions.

Do not change behavior beyond making the phase 7 implementation compile.

**Step 3: Re-run focused compile/test command**

Run: `rtk go test ./internal/domain/narrative -run "TestCollectSemanticBoardNarrativeInputs|TestCreateBoardFromSemanticBoard|TestBuildBoardNarrativePrompt" -count=1 -v`

Expected: Either tests pass or fail for a clear behavior mismatch addressed in later tasks.

---

### Task 2: Complete SemanticBoard Input Collection

**Files:**
- Modify: `backend-go/internal/domain/narrative/collector.go`
- Test: `backend-go/internal/domain/narrative/service_test.go`

**Step 1: Ensure tests cover input collection**

Required tests:
- Cold start with no active SemanticBoard returns zero inputs and no error.
- Category scope returns only event tags from feeds in that category.
- Global scope returns event tags across categories.
- A single event tag assigned to two SemanticBoards appears in both inputs.

**Step 2: Implement minimal collection query**

Implementation requirements:
- Read active SemanticBoards from `semantic_labels` where `label_type='board'` and `status='active'`.
- Join `topic_tag_board_labels`, `topic_tags`, `article_topic_tags`, and `articles`.
- Filter `topic_tags.status='active'` and `topic_tags.category='event'`.
- Filter articles by `[startOfDay, endOfDay)`.
- For category scope, join `feeds` and filter `feeds.category_id`.
- Group by `semantic_board_id` and tag fields.
- Do not read `topic_tag_relations`.

**Step 3: Run targeted tests**

Run: `rtk go test ./internal/domain/narrative -run "TestCollectSemanticBoardNarrativeInputs" -count=1 -v`

Expected: All collection tests pass.

---

### Task 3: Complete NarrativeBoard Creation And Previous Continuation

**Files:**
- Modify: `backend-go/internal/domain/narrative/board_creation.go`
- Test: `backend-go/internal/domain/narrative/service_test.go`

**Step 1: Ensure board creation writes new semantic fields**

Required behavior:
- `narrative_boards.semantic_board_id` is set to the SemanticBoard id.
- `event_tag_ids` is JSON containing all matched event tag ids.
- `scope_type`, `scope_category_id`, and `scope_label` come from `ScopeSaveOpts`.
- Old `abstract_tag_id`, `abstract_tag_ids`, and `board_concept_id` are not populated by this new path.

**Step 2: Ensure previous board matching uses semantic identity**

Required behavior:
- Previous board ids are matched from the previous day only.
- Match by `semantic_board_id`, `scope_type`, and `scope_category_id` or `IS NULL` for global scope.
- Store matched ids in `prev_board_ids` JSON.

**Step 3: Run targeted tests**

Run: `rtk go test ./internal/domain/narrative -run "TestCreateBoardFromSemanticBoard" -count=1 -v`

Expected: Board creation and previous continuation tests pass.

---

### Task 4: Complete Service Refactor Away From Old Paths

**Files:**
- Modify: `backend-go/internal/domain/narrative/service.go`

**Step 1: Verify old hotspot path is not called by daily generation**

Search: `GenerateAndSaveGlobal`, `GenerateAndSaveForCategory`

Required behavior:
- `GenerateAndSaveGlobal` calls semantic-board scoped generation.
- `GenerateAndSaveForCategory` calls semantic-board scoped generation.
- Neither method calls `CollectTagInputs`, `CollectAbstractTreeInputsByCategory`, `CollectUnclassifiedEventTagsByCategory`, `MatchTagToConcept`, `BuildBoardFromMatchedTags`, or `createBoardFromAbstractTree`.

**Step 2: Keep old helper functions if still needed by non-phase-7 code**

Do not delete old abstract-tree/concept helper files in phase 7. Deletion is deferred to phase 9.

**Step 3: Run grep verification**

Run: `rg "CollectAbstractTreeInputsByCategory|CollectUnclassifiedEventTagsByCategory|MatchTagToConcept|BuildBoardFromMatchedTags|createBoardFromAbstractTree" backend-go/internal/domain/narrative/service.go`

Expected: No matches inside `GenerateAndSaveGlobal` or `GenerateAndSaveForCategory`; remaining matches are only unrelated legacy helpers if any.

---

### Task 5: Complete Board Narrative Prompt Context

**Files:**
- Modify: `backend-go/internal/domain/narrative/board_narrative_generator.go`
- Test: `backend-go/internal/domain/narrative/service_test.go` or `generator_test.go`

**Step 1: Ensure prompt uses SemanticBoard context**

Required behavior:
- `BoardNarrativeContext` can carry SemanticBoard label and description.
- Prompt includes SemanticBoard label and description.
- Prompt does not require old abstract tag or board concept context.

**Step 2: Preserve backward compatibility for existing non-semantic callers**

If no SemanticBoard fields are provided, fallback to `NarrativeBoard.Name` and `NarrativeBoard.Description` to avoid breaking old tests before phase 9 deletion.

**Step 3: Run targeted prompt test**

Run: `rtk go test ./internal/domain/narrative -run "TestBuildBoardNarrativePrompt_UsesSemanticBoardContext" -count=1 -v`

Expected: Prompt test passes.

---

### Task 6: Mark OpenSpec Tasks And Update Knowledge Docs

**Files:**
- Modify: `openspec/changes/semantic-label-board-system/tasks.md`
- Modify or create docs update under `docs/` as appropriate

**Step 1: Mark phase 7 tasks complete**

Only mark these checkboxes if implementation and focused tests pass:
- `7.1`
- `7.2`
- `7.3`
- `7.4`
- `7.5`
- `7.6`
- `7.7`

Do not mark `1.7`; it remains deferred until phase 9.

**Step 2: Update docs knowledge base**

Add a concise note to the relevant docs/reference page explaining:
- SemanticBoard is long-lived/global.
- NarrativeBoard is daily/scope-specific.
- Daily boards derive from `topic_tag_board_labels` and `semantic_board_id`.
- Duplicate event tags across boards are intentional.

**Step 3: Run docs/diff check**

Run: `rtk git diff --check -- backend-go/internal/domain/narrative openspec/changes/semantic-label-board-system/tasks.md docs`

Expected: No whitespace errors.

---

### Task 7: Final Focused Verification And Review

**Files:**
- Verify: `backend-go/internal/domain/narrative/*`
- Verify: `openspec/changes/semantic-label-board-system/tasks.md`
- Verify: `docs/*`

**Step 1: Run focused narrative tests**

Run: `rtk go test ./internal/domain/narrative -run "TestCollectSemanticBoardNarrativeInputs|TestCreateBoardFromSemanticBoard|TestBuildBoardNarrativePrompt" -count=1 -v`

Expected: All phase 7 focused tests pass.

**Step 2: Run broader narrative package tests if focused tests pass**

Run: `rtk go test ./internal/domain/narrative -count=1 -v`

Expected: Prefer pass. If old unrelated failures appear, document them separately and do not block phase 7 unless caused by this change.

**Step 3: Run OpenSpec status**

Run: `openspec instructions apply --change "semantic-label-board-system" --json`

Expected: Progress increased by 7 tasks for phase 7.

**Step 4: Run review subthread**

Dispatch a read-only review subthread focused on phase 7 diffs. The reviewer must not edit files. Ask for bugs, regressions, missing tests, or spec mismatches.

**Step 5: Report checkpoint**

Summarize:
- Changed files.
- Tests run and results.
- OpenSpec progress.
- Review findings and fixes, if any.
