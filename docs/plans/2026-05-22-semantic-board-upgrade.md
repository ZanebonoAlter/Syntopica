# Semantic Board Upgrade Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `semantic-label-board-system` phase 5 by adding a service that suggests and confirms SemanticBoard upgrades from accumulated auxiliary labels.

**Architecture:** Add a self-contained `SemanticBoardUpgradeService` in tagging. It collects high-ref active auxiliary labels, clusters them with existing board composition context, enriches clusters with recent co-tag event context, asks an injected LLM client for suggestions, and only writes `semantic_labels(label_type=board)` / `board_composition` when a caller confirms a suggestion. Do not add HTTP APIs, backfill triggers, persistent suggestion tables, or automatic execution in this phase.

**Tech Stack:** Go, GORM, SQLite-backed unit tests, existing semantic label/topic/article models, `models.AISettings`, injected LLM interface for mockable tests.

---

### Task 1: Add SemanticBoard Upgrade Service Skeleton and Candidate Collection

**Files:**
- Create: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`
- Test: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

**Step 1: Run GitNexus impact before editing/reusing existing symbols**

Run impact for existing symbols referenced by this implementation:

```text
gitnexus_impact target=SemanticLabel direction=upstream repo=my-robot
gitnexus_impact target=BoardComposition direction=upstream repo=my-robot
gitnexus_impact target=TopicTagSemanticLabel direction=upstream repo=my-robot
gitnexus_impact target=ArticleTopicTag direction=upstream repo=my-robot
gitnexus_impact target=AISettings direction=upstream repo=my-robot
gitnexus_impact target=parsePgVector direction=upstream repo=my-robot
gitnexus_impact target=floatsToPgVector direction=upstream repo=my-robot
gitnexus_impact target=uniqueSemanticLabelSlug direction=upstream repo=my-robot
```

Stop and report if any result is HIGH or CRITICAL. If GitNexus cannot find a symbol, record that and proceed only if no HIGH/CRITICAL warning is returned.

**Step 2: Write failing candidate collection test**

Create `semantic_board_upgrade_test.go` with `TestSemanticBoardUpgradeCollectsCandidates`:
- Seed active auxiliary labels with ref counts above and below threshold.
- Seed a disabled auxiliary label above threshold.
- Seed an active auxiliary label already present in `board_composition`.
- Assert returned candidates include only active auxiliary labels with `ref_count >= semantic_board_upgrade_ref_count_threshold`, non-nil embedding, and no existing board composition membership.

**Step 3: Implement minimal public types and candidate collection**

Add:

```go
type SemanticBoardUpgradeService struct { db *gorm.DB; llm semanticBoardUpgradeLLM }
type semanticBoardUpgradeLLM interface { SuggestSemanticBoardUpgrades(ctx context.Context, prompt string) ([]SemanticBoardUpgradeSuggestion, error) }
type SemanticBoardUpgradeConfig struct { ... }
type SemanticBoardUpgradeCandidate struct { ... }
type SemanticBoardUpgradeCluster struct { ... }
type SemanticBoardUpgradeEventContext struct { ... }
type SemanticBoardUpgradeSuggestion struct { ... }
type SemanticBoardUpgradeDecision string
type ConfirmSemanticBoardUpgradeRequest struct { ... }
type ConfirmSemanticBoardUpgradeResult struct { ... }
func NewSemanticBoardUpgradeService(db *gorm.DB, llm semanticBoardUpgradeLLM) *SemanticBoardUpgradeService
func (s *SemanticBoardUpgradeService) GenerateSuggestions(ctx context.Context) ([]SemanticBoardUpgradeSuggestion, error)
func (s *SemanticBoardUpgradeService) ConfirmSuggestion(ctx context.Context, req ConfirmSemanticBoardUpgradeRequest) (*ConfirmSemanticBoardUpgradeResult, error)
```

Config defaults:
- `semantic_board_upgrade_ref_count_threshold`: `5`
- `semantic_board_upgrade_cluster_distance_threshold`: `0.7`
- `semantic_board_upgrade_cotag_window_days`: `30`
- `semantic_board_upgrade_cotag_top_n`: `20`
- `semantic_board_upgrade_cotag_dedupe_sim_threshold`: `0.85`
- `semantic_board_upgrade_cotag_hard_limit`: `15`

Candidate rule:
- `semantic_labels.label_type = 'auxiliary'`
- `semantic_labels.status = 'active'`
- `semantic_labels.ref_count >= config.RefCountThreshold`
- `semantic_labels.embedding IS NOT NULL`
- not present in `board_composition.auxiliary_label_id`

**Step 4: Run focused test**

Run from `backend-go`:

```powershell
rtk go test ./internal/domain/tagging -run TestSemanticBoardUpgradeCollectsCandidates -v
```

Expected: pass after implementation.

### Task 2: Add Clustering and Co-Tag Event Context

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

**Step 1: Add failing tests**

Add tests:
- `TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards`: candidates and existing active boards are clustered by cosine distance where `distance = 1 - cosine_similarity <= threshold`.
- `TestSemanticBoardUpgradeLoadsCoTagEventContext`: recent co-occurring active event tags are ranked by frequency, deduped by embedding similarity `> 0.85`, and hard-limited.

**Step 2: Implement clustering**

Implementation notes:
- Load existing active boards and their active composition auxiliary labels.
- Treat each candidate auxiliary label as a clusterable item.
- Treat existing boards as cluster context with their composition auxiliary embeddings and board ID/label.
- Use simple greedy clustering; no new dependency.
- Preserve deterministic ordering by ID for tests.

**Step 3: Implement co-tag event context**

Implementation notes:
- For cluster auxiliary IDs, find associated topic tags through `topic_tag_semantic_labels`.
- Find articles in the configured time window through `article_topic_tags` and `articles`.
- Find co-occurring `topic_tags.category = 'event'` and `status = 'active'` in the same articles.
- Rank by frequency descending, then topic tag ID ascending.
- Deduplicate by available topic tag embedding when similarity is `> CoTagDedupeSimThreshold`; labels without embeddings are retained by label uniqueness.
- Apply `CoTagTopN` before embedding dedupe and `CoTagHardLimit` after dedupe.

**Step 4: Run focused tests**

Run from `backend-go`:

```powershell
rtk go test ./internal/domain/tagging -run "TestSemanticBoardUpgrade(Clusters|LoadsCoTag)" -v
```

Expected: pass.

### Task 3: Add LLM Suggestion Flow

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

**Step 1: Add failing LLM mock test**

Add `TestSemanticBoardUpgradeGenerateSuggestionsUsesLLMMock`:
- Seed enough candidates.
- Use a fake `semanticBoardUpgradeLLM` that captures prompt text and returns both `create_new` and `skip` suggestions.
- Assert `GenerateSuggestions` returns suggestions and does not create `semantic_labels(label_type='board')` or `board_composition` rows.

**Step 2: Implement prompt and suggestion generation**

Implementation notes:
- Build prompt from cluster labels, existing board context, and co-tag event labels.
- The injected LLM interface returns parsed suggestions directly; do not implement real router wiring in this phase.
- Validate suggestions enough to drop invalid decisions and unknown auxiliary IDs.
- User confirmation is required before any write.

**Step 3: Run focused test**

Run from `backend-go`:

```powershell
rtk go test ./internal/domain/tagging -run TestSemanticBoardUpgradeGenerateSuggestionsUsesLLMMock -v
```

Expected: pass.

### Task 4: Add Confirmation Execution

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

**Step 1: Add failing confirmation tests**

Add tests:
- `TestSemanticBoardUpgradeConfirmCreateNew`: `create_new` creates an active board semantic label with `source='llm_suggest'` and writes requested `board_composition` rows.
- `TestSemanticBoardUpgradeConfirmMergeIntoExisting`: `merge_into_existing` validates target board and appends composition rows with duplicate-safe behavior.

**Step 2: Implement confirmation**

Implementation notes:
- Use a transaction.
- For `create_new`, require non-empty board label and auxiliary label IDs; generate unique slug with `uniqueSemanticLabelSlug`.
- For `merge_into_existing`, require active target board.
- Validate auxiliary IDs are active auxiliary labels before writing composition.
- Use `OnConflict DoNothing` for `board_composition` rows.
- Do not trigger backfill.

**Step 3: Run focused confirmation tests**

Run from `backend-go`:

```powershell
rtk go test ./internal/domain/tagging -run "TestSemanticBoardUpgradeConfirm" -v
```

Expected: pass.

### Task 5: Mark OpenSpec Phase 5 Complete

**Files:**
- Modify: `openspec/changes/semantic-label-board-system/tasks.md`

**Step 1: Run full phase 5 focused tests**

Run from `backend-go`:

```powershell
rtk go test ./internal/domain/tagging -run TestSemanticBoardUpgrade -v
```

Expected: pass.

Do not run old hierarchy tests or full package tests for this phase.

**Step 2: Update completed checkboxes**

Mark tasks `5.1` through `5.7` complete. Do not mark `1.7`.

**Step 3: Refresh OpenSpec progress**

Run from repo root:

```powershell
openspec instructions apply --change "semantic-label-board-system" --json
```

Expected: progress complete count increases by 7.

**Step 4: Report changes and verification output**

Return a concise summary with files changed, completed tasks, focused test command result, and any residual risks.
