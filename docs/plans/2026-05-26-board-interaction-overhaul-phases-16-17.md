# Board Interaction Overhaul Phases 16-17 Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Implement the on-demand semantic board match-detail API and the frontend match-detail side panel with KaTeX formula rendering.

**Architecture:** Backend adds a read-only match-detail endpoint under existing semantic board routes, reusing the current matching service, config loader, pgvector parsing, and persisted `topic_tag_board_labels` score/reason. Frontend adds typed API support, a reusable KaTeX renderer, and a right-side panel opened by clicking board article tag chips.

**Tech Stack:** Go + Gin + GORM + pure Go unit tests for matching detail logic (do not add/run sqlite handler tests); Nuxt 4 + Vue 3 + TypeScript + pnpm + KaTeX.

---

## Global Instructions

- Start by announcing: `I'm using the executing-plans skill to implement this plan.`
- Use TDD: write/adjust tests before production code, verify the new tests fail for the missing behavior, then implement minimal code.
- Project requirement: before editing any existing function/method/class, run GitNexus impact analysis if available. Target symbols likely include:
  - `SemanticBoardMatchingService.loadBoardAuxiliaries`
  - `evaluateSemanticBoardMatches`
  - `scoreSemanticBoardSimilarity`
  - `RegisterSemanticBoardRoutes`
  - `semanticBoardHandler.getBoardArticles`
  - `useSemanticBoardsApi`
  - `TagsPage`
- If GitNexus MCP tools are unavailable in the subagent environment, record that fact in the batch report and continue with normal targeted verification.
- Current worktree has many unrelated dirty files. Do **not** clean, reset, or reformat unrelated files. Do **not** commit unless explicitly instructed by the user.
- OpenSpec tasks to mark complete after implementation/verification:
  - Backend: `openspec/changes/board-interaction-overhaul/tasks.md` tasks 16.1-16.7
  - Frontend: tasks 17.1-17.8

## Context Files Already Read

- `openspec/changes/board-interaction-overhaul/proposal.md`
- `openspec/changes/board-interaction-overhaul/design.md`
- `openspec/changes/board-interaction-overhaul/tasks.md`
- `openspec/changes/board-interaction-overhaul/specs/tag-to-board-matching/spec.md`
- `openspec/changes/board-interaction-overhaul/specs/match-score-visualization/spec.md`
- `backend-go/AGENTS.md`
- `front/AGENTS.md`

## Task 1: Backend RED tests for match-detail core logic (no sqlite)

**Files:**
- Modify/Create: `backend-go/internal/domain/tagging/semantic_board_match_detail_test.go`
- Read/reference: `backend-go/internal/domain/tagging/semantic_board_matching_test.go`

**Important:** Do **not** add or run sqlite-backed handler tests. The existing sqlite handler test harness is outdated and is outside this task's verification scope.

**Step 1: Add failing pure Go unit tests**

Create pure unit tests that do not open a database. Use in-memory `models.SemanticLabel` and `boardAuxiliaryLabel` values with `floatsToPgVector`/`ptrStr` helpers from existing package tests.

Test cases to add:

1. `TestComputeMatchDetailPairsAndMetrics`
   - Config: `SimThreshold=0.72`, `MinEffectiveSample=3`.
   - Tag auxiliaries: three labels with vectors `[1,0,0]`, `[0,1,0]`, `[0,0,1]` and labels `Tag A/B/C`.
   - Board auxiliaries: vectors `[1,0,0]`, `[0,1,0]`, `[1,0,0]` with labels `Board A/B/Fallback`, so first two tag auxiliaries hit and the third's best similarity is below threshold.
   - Call `computeMatchDetail`.
   - Assert `Hits == 2`, `HitRate ~= 0.6667`, `MaxSimilarity ~= 1.0`, `Pairs` length is 3, each pair contains tag/board IDs, labels, similarity, and `IsHit` correctly.

2. `TestComputeMatchDetailUsesEffectiveSampleDenominator`
   - One tag auxiliary and one matching board auxiliary.
   - Config `MinEffectiveSample=3`.
   - Assert `Hits == 1` and `HitRate ~= 0.3333`.

3. `TestComputeMatchDetailEmptyInputs`
   - Empty tag auxiliaries and/or board auxiliaries.
   - Assert zero metrics and empty `Pairs`.

4. `TestDirectHitAuxiliaryDetection` if a helper is introduced for response assembly/direct-hit DTOs:
   - Given matching tag auxiliary ID and board auxiliary ID, assert direct hit DTO includes exact tag/board labels.

**Step 2: Run test to verify RED**

Run:

```bash
cd backend-go && go test ./internal/domain/tagging/ -run 'TestComputeMatchDetail|TestDirectHitAuxiliary' -v
```

Expected: FAIL because `computeMatchDetail` and/or direct-hit helper do not exist yet. This command must not execute sqlite-backed handler tests.

## Task 2: Backend matching detail data structures and compute function

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching.go`

**Step 1: Update helper structs minimally**

Extend `boardAuxiliaryLabel` with label data needed for detail display:

```go
type boardAuxiliaryLabel struct {
    BoardID          uint
    AuxiliaryLabelID uint
    Label            string
    Embedding        *string
}
```

Update `loadBoardAuxiliaries` select to include `auxiliary.label`:

```go
Select("board_composition.board_id, board_composition.auxiliary_label_id, auxiliary.label, auxiliary.embedding")
```

**Step 2: Add `loadBoardAuxiliariesByBoardID`**

Add method near `loadBoardAuxiliaries`:

```go
func (s *SemanticBoardMatchingService) loadBoardAuxiliariesByBoardID(ctx context.Context, boardID uint) ([]boardAuxiliaryLabel, error) {
    var labels []boardAuxiliaryLabel
    err := s.db.WithContext(ctx).
        Table("board_composition").
        Select("board_composition.board_id, board_composition.auxiliary_label_id, auxiliary.label, auxiliary.embedding").
        Joins("JOIN semantic_labels AS board ON board.id = board_composition.board_id AND board.label_type = ? AND board.status = ?", "board", "active").
        Joins("JOIN semantic_labels AS auxiliary ON auxiliary.id = board_composition.auxiliary_label_id AND auxiliary.label_type = ? AND auxiliary.status = ?", "auxiliary", "active").
        Where("board_composition.board_id = ?", boardID).
        Scan(&labels).Error
    return labels, err
}
```

**Step 3: Add detail structs**

Add unexported structs in `semantic_board_matching.go`:

```go
type matchDetailPair struct {
    TagAuxiliaryID      uint
    TagAuxiliaryLabel   string
    BoardAuxiliaryID    uint
    BoardAuxiliaryLabel string
    Similarity          float64
    IsHit               bool
}

type computedMatchDetail struct {
    Hits          int
    HitRate       float64
    MaxSimilarity float64
    Pairs         []matchDetailPair
}
```

**Step 4: Add `computeMatchDetail`**

Behavior:
- Input: `tagAuxiliaries []models.SemanticLabel`, `boardAuxiliaries []boardAuxiliaryLabel`, `config SemanticBoardMatchConfig`.
- For each tag auxiliary:
  - Skip vector parsing if `Embedding == nil` or invalid. It should not create a pair.
  - Compare with every board auxiliary that has a valid embedding.
  - Track the best board auxiliary for that tag auxiliary.
  - Append exactly one `matchDetailPair` for the best match, with `IsHit = bestSimilarity >= config.SimThreshold`.
- `hits` increments once per tag auxiliary whose best match is a hit.
- `maxSimilarity` is the max across all compared pairs.
- `hitRate = hits / max(len(tagAuxiliaries), config.MinEffectiveSample)` when denominator > 0; return 0 when no tag auxiliaries.
- Use `airouter.CosineSimilarity` and existing `parsePgVector`.

**Step 5: Run backend tests**

Run:

```bash
cd backend-go && go test ./internal/domain/tagging/ -run 'TestEvaluateSemanticBoardMatches|TestMatchDetail' -v
```

Expected at this stage: existing evaluate tests and new pure match-detail tests pass. Do not run sqlite-backed handler tests.

## Task 3: Backend handler, DTO, and route registration

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go`
- Note: task 16.5 says `backend-go/internal/app/router.go`, but this repo registers semantic board subroutes inside `RegisterSemanticBoardRoutes`; add the new route there, next to articles/narratives/composition.

**Step 1: Add response DTOs**

Near other semantic board DTOs add:

```go
type matchDetailConfigDTO struct {
    SimThreshold              float64 `json:"sim_threshold"`
    HitRateSimBlend           float64 `json:"hit_rate_sim_blend"`
    MinEffectiveSample        int     `json:"min_effective_sample"`
    DirectHitRate             float64 `json:"direct_hit_rate"`
    DirectMaxSim              float64 `json:"direct_max_sim"`
    DirectMaxSimMinHits       int     `json:"direct_max_sim_min_hits"`
    DirectMaxSimMinHitRate    float64 `json:"direct_max_sim_min_hit_rate"`
    WeightSim                 float64 `json:"weight_sim"`
    WeightDensity             float64 `json:"weight_density"`
    WeightedThreshold         float64 `json:"weighted_threshold"`
}

type directHitAuxiliaryDTO struct {
    TagAuxiliaryID   uint   `json:"tag_auxiliary_id"`
    TagLabel         string `json:"tag_label"`
    BoardAuxiliaryID uint   `json:"board_auxiliary_id"`
    BoardLabel       string `json:"board_label"`
}

type matchDetailPairDTO struct {
    TagAuxiliaryID      uint    `json:"tag_auxiliary_id"`
    TagAuxiliaryLabel   string  `json:"tag_auxiliary_label"`
    BoardAuxiliaryID    uint    `json:"board_auxiliary_id"`
    BoardAuxiliaryLabel string  `json:"board_auxiliary_label"`
    Similarity          float64 `json:"similarity"`
    IsHit               bool    `json:"is_hit"`
}

type matchDetailResponse struct {
    TopicTagID           uint                    `json:"topic_tag_id"`
    TopicTagLabel        string                  `json:"topic_tag_label"`
    SemanticBoardID      uint                    `json:"semantic_board_id"`
    MatchReason          string                  `json:"match_reason"`
    Score                float64                 `json:"score"`
    Config               matchDetailConfigDTO    `json:"config"`
    DirectHitAuxiliaries []directHitAuxiliaryDTO `json:"direct_hit_auxiliaries"`
    TagAuxiliaryCount    int                     `json:"tag_auxiliary_count"`
    Hits                 int                     `json:"hits"`
    HitRate              float64                 `json:"hit_rate"`
    MaxSimilarity        float64                 `json:"max_similarity"`
    Pairs                []matchDetailPairDTO    `json:"pairs"`
}
```

**Step 2: Register route**

Inside `RegisterSemanticBoardRoutes`, add near existing board detail subroutes:

```go
boards.GET("/:id/match-detail/:tagId", handler.getTagMatchDetail)
```

**Step 3: Implement `getTagMatchDetail`**

Handler flow:
1. Parse `id` and `tagId` with `parseUintParam`.
2. Load persisted board/tag match row:

```go
var stored models.TopicTagBoardLabel
err := h.db.WithContext(ctx).
    Where("semantic_board_id = ? AND topic_tag_id = ?", boardID, tagID).
    First(&stored).Error
```

Return 404 when not found.

3. Load topic tag label:

```go
var tag models.TopicTag
err := h.db.WithContext(ctx).First(&tag, tagID).Error
```

4. Create `matcher := NewSemanticBoardMatchingService(h.db)`.
5. Load tag auxiliaries and board auxiliaries.
6. Load config.
7. Compute direct hits by ID intersection. For every tag auxiliary whose ID equals a board auxiliary ID, append a `directHitAuxiliaryDTO`.
8. If direct hits exist, keep `Pairs` empty and aggregate metrics zero or metrics from `computeMatchDetail` only if desired. To satisfy spec, direct hit should clearly include `direct_hit_auxiliaries`.
9. If no direct hits, call `computeMatchDetail` and convert pairs to DTOs.
10. Respond with `respondOK(c, matchDetailResponse{...})`.

**Important details:**
- Response must include empty arrays, not null, for `direct_hit_auxiliaries` and `pairs`.
- Persisted `MatchReason` and `Score` from `topic_tag_board_labels` are authoritative and must be echoed even if current config changed.
- `tag_auxiliary_count` should be `len(tagAuxiliaries)` even when some embeddings are missing.
- For tag without auxiliaries, return 200 with empty metrics if persisted relation exists.

**Step 4: Run RED/GREEN backend tests**

Run pure targeted tests only:

```bash
cd backend-go && go test ./internal/domain/tagging/ -run 'TestComputeMatchDetail|TestDirectHitAuxiliary' -v
```

Expected: PASS.

Then run broader targeted checks that do not execute sqlite handler tests:

```bash
cd backend-go && go test ./internal/domain/tagging/ -run 'TestComputeMatchDetail|TestDirectHitAuxiliary|TestSemanticBoardMatching|TestEvaluateSemanticBoardMatches' -v
```

Expected: PASS.

## Task 4: Backend verification and OpenSpec task marking

**Files:**
- Modify: `openspec/changes/board-interaction-overhaul/tasks.md`

**Step 1: Full backend phase verification**

Run exactly:

```bash
cd backend-go && go test ./internal/domain/tagging/ -run 'TestComputeMatchDetail|TestDirectHitAuxiliary' -v
cd backend-go && go build ./...
```

Expected: both exit 0. Do not run sqlite-backed handler tests.

**Step 2: Mark backend tasks complete**

Only after verification passes, update tasks 16.1 through 16.7 from `- [ ]` to `- [x]` in:

```text
openspec/changes/board-interaction-overhaul/tasks.md
```

**Step 3: Report checkpoint**

Report changed backend files and exact command outputs. Stop for review if any backend verification fails.

## Task 5: Frontend dependency and API types

**Files:**
- Modify: `front/package.json`
- Modify: `front/pnpm-lock.yaml`
- Modify: `front/app/api/semanticBoards.ts`

**Step 1: Install KaTeX**

Run:

```bash
cd front && pnpm add katex && pnpm add -D @types/katex
```

Expected: `package.json` gains `katex` under dependencies and `@types/katex` under devDependencies; lockfile updates.

**Step 2: Add match-detail interfaces**

In `front/app/api/semanticBoards.ts`, add types near `BoardArticleTag`:

```ts
export interface MatchDetailConfig {
  sim_threshold: number
  hit_rate_sim_blend: number
  min_effective_sample: number
  direct_hit_rate: number
  direct_max_sim: number
  direct_max_sim_min_hits: number
  direct_max_sim_min_hit_rate: number
  weight_sim: number
  weight_density: number
  weighted_threshold: number
}

export interface DirectHitAuxiliary {
  tag_auxiliary_id: number
  tag_label: string
  board_auxiliary_id: number
  board_label: string
}

export interface MatchDetailPair {
  tag_auxiliary_id: number
  tag_auxiliary_label: string
  board_auxiliary_id: number
  board_auxiliary_label: string
  similarity: number
  is_hit: boolean
}

export interface MatchDetailResponse {
  topic_tag_id: number
  topic_tag_label: string
  semantic_board_id: number
  match_reason: string
  score: number
  config: MatchDetailConfig
  direct_hit_auxiliaries: DirectHitAuxiliary[]
  tag_auxiliary_count: number
  hits: number
  hit_rate: number
  max_similarity: number
  pairs: MatchDetailPair[]
}
```

**Step 3: Add API method**

Inside `useSemanticBoardsApi()` add:

```ts
async function getMatchDetail(boardId: number, tagId: number): Promise<ApiResponse<MatchDetailResponse>> {
  return apiClient.get(`/semantic-boards/${boardId}/match-detail/${tagId}`)
}
```

Return it from the composable.

**Step 4: Typecheck just enough if possible**

Run:

```bash
cd front && pnpm exec nuxi typecheck
```

Expected at this stage: may fail because components not added yet; record output and proceed to Task 6.

## Task 6: KaTeXRender component

**Files:**
- Create: `front/app/components/KaTeXRender.vue`

**Step 1: Create component**

Use `<script setup lang="ts">` and import KaTeX CSS. Minimal component:

```vue
<script setup lang="ts">
import { computed } from 'vue'
import katex from 'katex'
import 'katex/dist/katex.min.css'

const props = withDefaults(defineProps<{
  latex: string
  display?: boolean
}>(), {
  display: false,
})

const html = computed(() => katex.renderToString(props.latex, {
  displayMode: props.display,
  throwOnError: false,
  strict: false,
}))
</script>

<template>
  <span class="katex-render" :class="{ 'katex-render--display': display }" v-html="html" />
</template>
```

Add scoped CSS for display mode and dark-theme color if needed.

**Step 2: Run typecheck if possible**

Run:

```bash
cd front && pnpm exec nuxi typecheck
```

Expected: no type error from KaTeX import. If other unrelated type errors appear, record them.

## Task 7: MatchDetailPanel component

**Files:**
- Create: `front/app/features/tags/components/MatchDetailPanel.vue`

**Step 1: Add component skeleton**

Props:

```ts
const props = defineProps<{
  boardId: number
  tag: BoardArticleTag | null
}>()

const emit = defineEmits<{ close: [] }>()
```

Imports:
- `ref`, `computed`, `watch`
- `Icon` from `@iconify/vue`
- `KaTeXRender` from `~/components/KaTeXRender.vue`
- `useSemanticBoardsApi`, type `BoardArticleTag`, type `MatchDetailResponse`, type `MatchDetailPair`

**Step 2: Load detail on tag changes**

- Watch `[() => props.boardId, () => props.tag?.id]`.
- If `tag` is null, clear detail/error/loading.
- Otherwise call `sbApi.getMatchDetail(props.boardId, props.tag.id)`.
- Use a request sequence integer to avoid stale responses if user clicks quickly.
- Display loading skeleton while request is active.

**Step 3: Implement formula helpers**

Computed/helper functions:

```ts
function formatScore(value: number | undefined, digits = 2): string
function reasonLabel(reason: string): string
function reasonColor(reason: string): string
function primaryFormula(detail: MatchDetailResponse): string
function substitutionFormula(detail: MatchDetailResponse): string
function rateFormula(detail: MatchDetailResponse): string
```

Formula rules:
- `direct_hit`: no formula; show direct hit auxiliary list.
- `hit_rate`: `\text{score}=\alpha\cdot S_{\max}+(1-\alpha)\cdot R`
- `max_sim`: `\text{score}=S_{\max}` plus condition text: `Smax ≥ direct_max_sim`, `hits ≥ min(direct_max_sim_min_hits, N)`, `R ≥ direct_max_sim_min_hit_rate`.
- `weighted`: `\text{score}=w_{\text{sim}}\cdot S_{\max}+w_{\text{density}}\cdot R`
- Shared rate formula: `R=\frac{\text{hits}}{\max(N,s)}`.

**Step 4: Render layout**

Panel structure:
- Header with close button.
- Tag label and reason/score badge.
- Loading state.
- Error state.
- Direct hit section for `direct_hit_auxiliaries`.
- Formula section using `KaTeXRender` for primary and secondary formulas.
- Pair list/table:
  - hit icon/color for `is_hit`
  - tag auxiliary label → board auxiliary label
  - similarity with two decimals
- Config `<details>` block showing threshold/blend/weights.

**Step 5: Styling**

Use local scoped styles matching existing dark editorial style:
- fixed width controlled by parent; panel uses `height: 100%`, dark translucent background, border-left.
- readable compact typography.
- green/gray hit indicators.

**Step 6: Run typecheck**

Run:

```bash
cd front && pnpm exec nuxi typecheck
```

Expected: PASS or only unrelated pre-existing errors. Fix component/API type errors before continuing.

## Task 8: TagsPage integration

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Step 1: Import component and state**

Add import:

```ts
import MatchDetailPanel from './MatchDetailPanel.vue'
```

Add state:

```ts
const selectedTagForDetail = ref<BoardArticleTag | null>(null)
```

Clear it when board or filters change:
- In `handleSelectBoard`, set `selectedTagForDetail.value = null`.
- In `handleFilterLabel` and `handleFilterChange`, set it to null.

**Step 2: Add click helper**

Add:

```ts
function toggleMatchDetail(tag: BoardArticleTag) {
  selectedTagForDetail.value = selectedTagForDetail.value?.id === tag.id ? null : tag
}

function isSelectedDetailTag(tag: BoardArticleTag): boolean {
  return selectedTagForDetail.value?.id === tag.id
}
```

**Step 3: Prevent article preview when clicking tag chip**

Change chip from `<span>` to `<button type="button">` or keep span with `role="button"`, but prefer button for accessibility.

Use:

```vue
<button
  v-for="tag in article.filtered_tags"
  :key="tag.id"
  type="button"
  class="tags-timeline-tag-chip"
  :class="{ 'tags-timeline-tag-chip--selected': isSelectedDetailTag(tag) }"
  :style="{ borderColor: matchReasonColor(tag.match_reason) }"
  :title="matchInfoLabel(tag)"
  @click.stop="toggleMatchDetail(tag)"
>
  {{ tag.label }} {{ tag.score.toFixed(2) }}
</button>
```

**Step 4: Change article tab layout to two columns**

Wrap the existing article timeline content in a row container:

```vue
<div v-if="contentTab === 'articles'" class="tags-articles-layout">
  <div class="tags-timeline">
    <!-- existing timeline content -->
  </div>
  <Transition name="match-detail-panel">
    <MatchDetailPanel
      v-if="selectedTagForDetail && selectedBoardId !== null"
      :board-id="selectedBoardId"
      :tag="selectedTagForDetail"
      class="tags-match-detail-panel"
      @close="selectedTagForDetail = null"
    />
  </Transition>
</div>
```

Do not change non-article tabs.

**Step 5: Add CSS**

Add scoped styles:

```css
.tags-articles-layout {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  min-width: 0;
}

.tags-articles-layout .tags-timeline {
  flex: 1;
  min-width: 0;
}

.tags-match-detail-panel {
  width: 320px;
  flex-shrink: 0;
}

.tags-timeline-tag-chip {
  cursor: pointer;
}

.tags-timeline-tag-chip--selected {
  box-shadow: 0 0 0 2px rgba(240, 138, 75, 0.35);
  background: rgba(240, 138, 75, 0.12);
}

.match-detail-panel-enter-active,
.match-detail-panel-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.match-detail-panel-enter-from,
.match-detail-panel-leave-to {
  opacity: 0;
  transform: translateX(12px);
}
```

**Step 6: Run frontend verification**

Run:

```bash
cd front && pnpm lint
cd front && pnpm exec nuxi typecheck
```

Expected: both PASS. Fix any introduced errors.

## Task 9: Frontend build verification and OpenSpec task marking

**Files:**
- Modify: `openspec/changes/board-interaction-overhaul/tasks.md`

**Step 1: Run full frontend verification**

Run exactly:

```bash
cd front && pnpm lint
cd front && pnpm exec nuxi typecheck
cd front && pnpm build
```

Expected: all exit 0. If WSL native binding issues occur, record exact output and identify whether it is environmental or code-related.

**Step 2: Mark frontend tasks complete**

Only after verification passes (or after clearly documented environmental-only blocker accepted by reviewer), update tasks 17.1 through 17.8 from `- [ ]` to `- [x]`.

## Task 10: Docs update and final verification summary

**Files:**
- Modify one or more relevant docs under `docs/reference/` if implementation changes public API or user-visible behavior:
  - `docs/reference/api/semantic-boards.md`
  - `docs/userguide/tags.md`

**Step 1: Update docs minimally**

Document:
- `GET /api/semantic-boards/:id/match-detail/:tagId`
- Response fields: persisted `match_reason`/`score`, current `config`, `direct_hit_auxiliaries`, aggregate metrics, `pairs`.
- Frontend interaction: click article tag chip in board articles tab to open match detail panel.

**Step 2: Run final targeted verification**

Run:

```bash
cd backend-go && go test ./internal/domain/tagging/ -run 'TestComputeMatchDetail|TestDirectHitAuxiliary' -v
cd backend-go && go build ./...
cd front && pnpm lint
cd front && pnpm exec nuxi typecheck
cd front && pnpm build
```

**Step 3: Final report for reviewer**

Report:
- Files changed.
- OpenSpec tasks marked complete.
- Exact verification commands and results.
- Any environmental failures with evidence.
- Note whether GitNexus impact analysis was run or unavailable.

Stop after this report and wait for reviewer acceptance.
