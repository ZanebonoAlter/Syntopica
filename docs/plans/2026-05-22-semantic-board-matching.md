# Semantic Board Matching Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete OpenSpec `semantic-label-board-system` phase 4 by adding tag-to-SemanticBoard matching through auxiliary-label composition.

**Architecture:** Add a focused matching service under `backend-go/internal/domain/tagging` that reads a tag's active auxiliary labels and active board compositions, evaluates direct and indirect match rules, then replaces that tag's persisted `topic_tag_board_labels`. Keep this phase self-contained; do not wire it into article tagging, backfill, API, or old hierarchy flows yet.

**Tech Stack:** Go, GORM, SQLite-backed unit tests, existing semantic label models, `models.AISettings` for configurable thresholds.

---

### Task 1: Add SemanticBoard Matching Service

**Files:**
- Create: `backend-go/internal/domain/tagging/semantic_board_matching.go`
- Test: `backend-go/internal/domain/tagging/semantic_board_matching_test.go`

**Step 1: Run GitNexus impact before editing related existing symbols**

Run impact for existing symbols referenced or reused by this implementation:

```text
gitnexus_impact target=SemanticLabel direction=upstream repo=my-robot
gitnexus_impact target=TopicTagBoardLabel direction=upstream repo=my-robot
gitnexus_impact target=BoardComposition direction=upstream repo=my-robot
gitnexus_impact target=parsePgVector direction=upstream repo=my-robot
```

Stop and report if any result is HIGH or CRITICAL. If GitNexus cannot find a symbol, record that and proceed only if no HIGH/CRITICAL warning is returned.

**Step 2: Write failing tests**

Create `semantic_board_matching_test.go` with tests:
- `TestSemanticBoardMatchingDirectHit`: tag auxiliary label is part of a board composition, match reason is `direct_hit`, score is highest, and one `topic_tag_board_labels` row is written.
- `TestSemanticBoardMatchingThreeRules`: cover hit-rate, max-sim, and weighted-rule matches using deterministic embeddings and config rows.
- `TestSemanticBoardMatchingMaxBoardsTruncation`: when more boards match than configured max, persist only top-scoring boards ordered by score descending.
- `TestSemanticBoardMatchingNoMatchReplacesExistingLabels`: no current match returns empty and removes old labels for that tag only.
- `TestSemanticBoardMatchingColdStartNoBoard`: no active boards returns empty without error and clears old labels.
- `TestSemanticBoardMatchingIgnoresDisabledLabels`: disabled auxiliary labels and disabled boards are excluded.

Use the existing in-memory SQLite pattern from `auxiliary_label_service_test.go`. Auto-migrate at least `TopicTag`, `SemanticLabel`, `TopicTagSemanticLabel`, `TopicTagBoardLabel`, `BoardComposition`, and `AISettings`.

**Step 3: Run tests and confirm failure**

Run from `backend-go`:

```powershell
rtk go test ./internal/domain/tagging -run TestSemanticBoardMatching -v
```

Expected: fail because the matching service does not exist.

**Step 4: Implement minimal service**

Add these exported types/functions:

```go
type SemanticBoardMatchingService struct { db *gorm.DB }

func NewSemanticBoardMatchingService(db *gorm.DB) *SemanticBoardMatchingService

type SemanticBoardMatchConfig struct {
    SimThreshold      float64
    DirectHitRate     float64
    DirectMaxSim      float64
    WeightSim         float64
    WeightDensity     float64
    WeightedThreshold float64
    MaxBoards         int
}

type SemanticBoardMatchResult struct {
    SemanticBoardID uint
    Score           float64
    MatchReason     string
}

func (s *SemanticBoardMatchingService) MatchTopicTag(ctx context.Context, topicTagID uint) ([]SemanticBoardMatchResult, error)
```

Internal behavior:
- Validate `topicTagID != 0`.
- Load active auxiliary labels joined through `topic_tag_semantic_labels`.
- Load active boards and active board composition auxiliary labels, grouped by board ID.
- Direct hit if a tag auxiliary ID exists in board composition auxiliary IDs: `MatchReason = "direct_hit"`, `Score = 1.0`.
- Otherwise calculate pairwise cosine similarities between tag auxiliary embeddings and board composition auxiliary embeddings.
- For each tag auxiliary label, use its best similarity against that board composition.
- `hit_rate = count(best_sim >= SimThreshold) / len(tagAuxiliaries)`.
- `max_sim = max(all pairwise similarities)`.
- `weighted = WeightSim * max_sim + WeightDensity * hit_rate`.
- Rule order: `hit_rate > DirectHitRate` gives `hit_rate`; `max_sim >= DirectMaxSim` gives `max_sim`; `weighted >= WeightedThreshold` gives `weighted`.
- Sort matches by score descending, then board ID ascending, truncate to `MaxBoards`.
- Replace only this tag's `topic_tag_board_labels` in one transaction: delete existing rows for the tag, then insert new rows.
- Return empty matches without error for no auxiliary labels, no board compositions, or no matches.

Reuse or duplicate only small private helpers as needed. Keep the service independent from article tagging and backfill.

**Step 5: Add matching config loading**

Read these `ai_settings` keys, with defaults if absent or invalid:
- `semantic_board_match_sim_threshold`: `0.6`
- `semantic_board_match_direct_hit_rate`: `0.5`
- `semantic_board_match_direct_max_sim`: `0.8`
- `semantic_board_match_weight_sim`: `0.6`
- `semantic_board_match_weight_density`: `0.4`
- `semantic_board_match_weighted_threshold`: `0.6`
- `semantic_board_match_max_boards`: `3`

Parsing rules:
- Float thresholds must be between `0` and `1`; invalid values fall back to defaults.
- `MaxBoards` must be greater than `0`; invalid values fall back to default.

**Step 6: Run focused tests**

Run from `backend-go`:

```powershell
rtk go test ./internal/domain/tagging -run TestSemanticBoardMatching -v
```

Expected: pass.

Do not run old hierarchy tests or full package tests for this phase.

### Task 2: Mark OpenSpec Phase 4 Complete

**Files:**
- Modify: `openspec/changes/semantic-label-board-system/tasks.md`

**Step 1: Update completed checkboxes**

Mark these tasks complete:
- `4.1 创建 semantic_board_matching.go：读取 tag 辅助标签和 active SemanticBoard composition`
- `4.2 实现直接命中匹配（tag 辅助标签 ∈ board 构成标签）`
- `4.3 实现间接匹配：计算命中率（hit_count / tag 辅助标签总数）和 max_sim`
- `4.4 实现三规则挂载判断：命中率、max_sim、加权综合分`
- `4.5 实现多 board 挂载：按分数排序，默认最多 3 个，写入 topic_tag_board_labels`
- `4.6 实现匹配参数读取：从 ai_settings 读取 semantic_board_match_* 配置`
- `4.7 编写匹配逻辑单元测试：覆盖直接命中、三规则、多 board 截断、无匹配、冷启动无 board`

Do not mark `1.7`; it remains blocked until phase 9 deprecated-code deletion.

**Step 2: Refresh OpenSpec progress**

Run from repo root:

```powershell
openspec instructions apply --change "semantic-label-board-system" --json
```

Expected: progress complete count increases by 7.

**Step 3: Report changes and verification output**

Return a concise summary with files changed, completed tasks, focused test command result, and any residual risks.
