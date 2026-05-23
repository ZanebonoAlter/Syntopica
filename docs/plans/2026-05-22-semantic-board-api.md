# Semantic Board API Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement phase 8 backend APIs for SemanticBoard, auxiliary labels, upgrade suggestions, backfill, matching config, and tag associations.

**Architecture:** Keep business logic in `internal/domain/tagging` and only register routes from `internal/app/router.go`. Add one HTTP handler file plus focused tests; reuse existing phase 3-6 services instead of duplicating matching, governance, upgrade, and backfill logic.

**Tech Stack:** Go, Gin, GORM, sqlite in-memory handler tests, existing `airouter` for LLM-backed upgrade suggestions.

---

### Task 1: Handler Test Coverage

**Files:**
- Create: `backend-go/internal/domain/tagging/semantic_board_handler_test.go`

**Step 1: Write failing tests**
- Test SemanticBoard CRUD returns active boards with `tag_count`.
- Test board composition list and remove endpoint.
- Test auxiliary label list, disable, and merge alias endpoints.
- Test upgrade candidates and confirm execution endpoints.
- Test backfill enqueue and job lookup endpoints.
- Test matching config GET/PUT endpoint.
- Test tag auxiliary-label and semantic-board association endpoints.

**Step 2: Run tests to verify RED**
- Run: `rtk go test ./internal/domain/tagging -run TestSemanticBoardHandler -count=1 -v`
- Expected: compile or route symbol failures because handler/routes do not exist yet.

### Task 2: API Handler Implementation

**Files:**
- Create: `backend-go/internal/domain/tagging/semantic_board_handler.go`

**Step 1: Implement minimal handler methods**
- Register `/api/semantic-boards` routes.
- Implement CRUD with `semantic_labels.label_type='board'`.
- Implement composition list/removal using `BoardComposition` and `AuxiliaryLabelService.RemoveBoardComposition`.
- Implement auxiliary label list/search/disable/merge using `AuxiliaryLabelService`.
- Implement upgrade candidates from `SemanticBoardUpgradeService.collectCandidates` and `clusterCandidates`.
- Implement upgrade suggest through a default LLM adapter implementing `semanticBoardUpgradeLLM`.
- Implement upgrade execute through `SemanticBoardUpgradeService.ConfirmSuggestion`.
- Implement backfill through a shared `SemanticBoardBackfillService` instance.
- Implement matching config GET/PUT backed by `ai_settings`.
- Implement tag association list endpoints.

**Step 2: Run tests to verify GREEN**
- Run: `rtk go test ./internal/domain/tagging -run TestSemanticBoardHandler -count=1 -v`
- Expected: handler tests pass.

### Task 3: Router Registration And Deprecated Route Gate

**Files:**
- Modify: `backend-go/internal/app/router.go`

**Step 1: Register new routes**
- Call `topicanalysisdomain.RegisterSemanticBoardRoutes(api)`.

**Step 2: Remove old route registration from router**
- Remove hierarchy/concept route registration from `SetupRoutes` only; do not delete old files until phase 9.
- Remove now-unused import if applicable.

**Step 3: Run focused compile/test**
- Run: `rtk go test ./internal/app ./internal/domain/tagging -run TestSemanticBoardHandler -count=1 -v`
- Expected: compile succeeds and handler tests pass.

### Task 4: OpenSpec Checklist And API Docs

**Files:**
- Modify: `openspec/changes/semantic-label-board-system/tasks.md`
- Modify: `docs/reference/api/_index.md`
- Create: `docs/reference/api/semantic-boards.md`

**Step 1: Mark phase 8 tasks complete**
- Check `8.1` through `8.10` only after tests pass.

**Step 2: Write frontend-facing API docs**
- Include response envelope, endpoints, query params, request bodies, response shapes, and notes for async backfill/LLM suggestion.

**Step 3: Run final focused verification**
- Run: `rtk go test ./internal/domain/tagging -run "TestSemanticBoardHandler|TestSemanticBoardMatching|TestSemanticBoardUpgrade|TestSemanticBoardBackfill|TestAuxiliaryLabelService" -count=1 -v`
- Run: `rtk git diff --check -- backend-go/internal/domain/tagging backend-go/internal/app/router.go openspec/changes/semantic-label-board-system/tasks.md docs/reference/api`
- Expected: all pass.
