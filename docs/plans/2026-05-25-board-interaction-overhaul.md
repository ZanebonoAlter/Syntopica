# Board Interaction Overhaul 实施计划

> **执行方式:** Subagent-Driven Development — 主线程按 Phase 串行派发子线程，Phase 内独立任务并行。

**Goal:** 修复板块匹配精度、增强交互体验、统一叙事到板块页面

**Architecture:** 后端 Go (Gin/GORM) 新增/改造 4 个 handler、改造叙事生成调度；前端 Vue 3 新增 1 个组件、改造 TagsPage、清理 TopicGraphPage 叙事相关代码

**Tech Stack:** Go 1.24, GORM, Gin, PostgreSQL, Vue 3, Nuxt 4, TypeScript, Tailwind CSS v4

---

## 依赖分析

```
Phase A (后端独立，可并行):
  A1: 匹配精度增强     ─┐
  A2: 升级建议 DTO 增强  ─┤  互不依赖
  A3: 叙事生成取消 scope  ─┘

Phase B (后端 API，依赖 Phase A):
  B1: Board 文章列表 API    ← 依赖 A1 (filtered_tags 需要 match_reason/score)
  B2: Board 叙事时间线 API   ← 依赖 A3 (scope 改造)

Phase C (前端，依赖 Phase B):
  C1: TagsPage 文章列表改造    ← 依赖 B1 (新 API)
  C2: BoardNarrativeTimeline   ← 依赖 B2 (新 API)
  C3: 叙事功能迁移删除          ← 无后端依赖，但逻辑上与 C2 衔接

Phase D (集成验证):
  D1: 后端全量验证
  D2: 前端全量验证
```

---

## Phase A: 后端核心改造（3 个子线程并行）

### A1: 匹配精度增强

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching.go`
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching_test.go`（如存在）
- Modify: `front/app/features/tags/components/MatchingConfigDialog.vue`
- Modify: `front/app/api/semanticBoards.ts`

**Tasks (from tasks.md Section 1):**
- 1.1 `SemanticBoardMatchConfig` 新增 `DirectMaxSimMinHits int`（默认 2）和 `DirectMaxSimMinHitRate float64`（默认 0.3）
- 1.2 `evaluateSemanticBoardMatches` max_sim 分支改为三条件联合判断。**关键**: `scoreSemanticBoardSimilarity` 当前返回 `(hitRate, maxSimilarity)` 不返回 hits，需修改签名增加第三个返回值 `hits int`，或者从 `hitRate * tagAuxiliaryCount` 反算（推荐反算，改动最小：`hits = int(math.Round(hitRate * float64(tagAuxiliaryCount)))`）
- 1.3 `loadConfig` 读取两个新 ai_settings key
- 1.4 Seed 新默认配置
- 1.5 单元测试：N=1/2/3/5 场景、hits 不足拒绝、hit_rate 不足拒绝、N=1 退化
- 1.6 前端 `MatchingConfigDialog.vue` 新增两个配置项；`semanticBoards.ts` 的 `MatchingConfig` type 新增对应字段
- 1.7 验证

**Code Guidance — `evaluateSemanticBoardMatches` 修改点 (当前代码第 136 行附近):**

```go
// 当前:
hitRate, maxSimilarity := scoreSemanticBoardSimilarity(tagVectors, boardVectors, len(tagAuxiliaries), config.SimThreshold)
// ...
case maxSimilarity >= config.DirectMaxSim:
    score = maxSimilarity
    matchReason = "max_sim"

// 改为:
hitRate, maxSimilarity := scoreSemanticBoardSimilarity(tagVectors, boardVectors, len(tagAuxiliaries), config.SimThreshold)
hits := int(math.Round(hitRate * float64(len(tagAuxiliaries))))
minHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))
// ...
case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
    score = maxSimilarity
    matchReason = "max_sim"
```

**Seed 配置 (参考现有 seed 格式):**

```go
// semantic_board_match_direct_max_sim_min_hits = 2
// semantic_board_match_direct_max_sim_min_hit_rate = 0.3
```

**前端 MatchingConfigDialog — 新增字段:**
- `semantic_board_match_direct_max_sim_min_hits` (number input, step=1, min=1, max=5, label: "max_sim 最小命中数")
- `semantic_board_match_direct_max_sim_min_hit_rate` (number input, step=0.05, min=0, max=1, label: "max_sim 最小命中率")

**Verification:** `go test ./internal/domain/tagging/ -run TestEvaluateSemanticBoardMatches -v && go build ./...`

---

### A2: 升级建议 DTO 增强

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go`
- Modify: `front/app/api/semanticBoards.ts`
- Modify: `front/app/features/tags/components/UpgradeSuggestionPanel.vue`

**Tasks (from tasks.md Section 2):**
- 2.1 `semanticBoardUpgradeSuggestionDTO` 新增 `AuxiliaryLabels` 和 `TargetBoardLabel`
- 2.2 `suggestionsToDTO` 批量查询 DB 填充新字段
- 2.3 前端 type 新增字段
- 2.4 前端展示 #id → label
- 2.5 验证

**Code Guidance — `semanticBoardUpgradeSuggestionDTO` 新增字段:**

```go
type semanticBoardUpgradeSuggestionDTO struct {
    Decision          SemanticBoardUpgradeDecision `json:"decision"`
    BoardLabel        string                       `json:"board_label,omitempty"`
    Description       string                       `json:"description,omitempty"`
    AuxiliaryLabelIDs []uint                       `json:"auxiliary_label_ids"`
    AuxiliaryLabels   []struct {
        ID    uint   `json:"id"`
        Label string `json:"label"`
    } `json:"auxiliary_labels"`
    TargetBoardID    *uint  `json:"target_board_id,omitempty"`
    TargetBoardLabel string `json:"target_board_label,omitempty"`
    Reason           string `json:"reason"`
}
```

**Code Guidance — `suggestionsToDTO` 修改:**

当前是直接类型转换 `semanticBoardUpgradeSuggestionDTO(suggestion)`。需改为：
1. 收集所有 `auxiliary_label_ids` 和 `target_board_id`
2. 批量查询 `semantic_labels` 表获取 label
3. 批量查询 `semantic_labels` 表 (label_type="board") 获取 board label
4. 填充新字段

注意：`suggestionsToDTO` 当前接收 `[]SemanticBoardUpgradeSuggestion` 参数，`SemanticBoardUpgradeSuggestion` 是领域模型。需要在转换函数内加 DB 查询（该函数已在 handler 包内，可访问 `database.DB`）。

**前端 `UpgradeSuggestionPanel.vue` 修改点:**

当前约 111 行: `板块 #{{ s.target_board_id }}` → `{{ s.target_board_label || '板块 #' + s.target_board_id }}`
当前约 116 行: `标签 #{{ id }}` → 遍历 `s.auxiliary_labels` 展示 `label.label`

**Verification:** `go test ./internal/domain/tagging/ -run TestUpgrade -v && pnpm lint && pnpm exec nuxi typecheck`

---

### A3: 叙事生成取消 scope

**Files:**
- Modify: `backend-go/internal/domain/narrative/service.go`
- Modify: `backend-go/internal/domain/narrative/collector.go`
- Modify: `backend-go/internal/domain/narrative/board_creation.go`
- Modify: `backend-go/internal/domain/narrative/board_narrative_generator.go`
- Modify: `backend-go/internal/domain/narrative/service_test.go`

**Tasks (from tasks.md Section 5):**
- 5.1 `service.go`: 移除 scope 循环，改为 `GenerateAndSaveForAllBoards`
- 5.1a `collector.go`: `CollectSemanticBoardNarrativeInputs` 移除 scopeType/categoryID 参数和 category JOIN
- 5.1b `board_creation.go`: `matchPreviousSemanticBoard` 移除 scopeType/categoryID 参数
- 5.2 `board_narrative_generator.go`: 废弃 `SaveNarrativesForBoard`，统一 `saveNarrativesWithBoard` scope_type="board"
- 5.3 `LoadBoardEventTags` 不再按 scope 过滤
- 5.4 `service.go`: NarrativeBoard 创建统一 scope
- 5.5 `saveNarrativesWithBoard`: scope_type="board" 使用 `resolveGeneration`
- 5.6 生成测试
- 5.7 验证

**Code Guidance — 关键修改:**

`collector.go` `CollectSemanticBoardNarrativeInputs`:
```go
// 当前签名:
func CollectSemanticBoardNarrativeInputs(date time.Time, scopeType string, categoryID *uint) ([]SemanticBoardNarrativeInput, error)
// 改为:
func CollectSemanticBoardNarrativeInputs(date time.Time) ([]SemanticBoardNarrativeInput, error)
// 删除 if scopeType == feed_category { ... JOIN feeds ... } 分支
```

`board_creation.go` `matchPreviousSemanticBoard`:
```go
// 当前签名:
func matchPreviousSemanticBoard(semanticBoardID uint, date time.Time, scopeType string, categoryID *uint) []uint
// 改为:
func matchPreviousSemanticBoard(semanticBoardID uint, date time.Time) []uint
// WHERE 条件改为: semantic_board_id = ? AND period_date >= ? AND period_date < ?
// 去掉 scope_type 和 scope_category_id 条件
```

`service.go` 新调度:
```go
// GenerateAndSave 改为:
// 1. 遍历所有 active SemanticBoard
// 2. 对每个 board:
//    inputs := CollectSemanticBoardNarrativeInputs(date)  // 返回所有 board 的 inputs
//    按 board ID 找到对应 input
//    board := createBoardFromSemanticBoard(input, date, ScopeSaveOpts{ScopeType: "board"})
//    GenerateNarrativesForBoard + saveNarrativesWithBoard
// 3. runFallbackAssociations + DeriveBoardConnections + feedback + cleanEmptyBoards
```

`board_narrative_generator.go`:
- 标记 `SaveNarrativesForBoard` 为 deprecated 或删除
- `saveNarrativesWithBoard` 中 scope="board" 时的 generation 统一用 `resolveGeneration(out, date)`

**Verification:** `go test ./internal/domain/narrative/ -v && go build ./...`

---

## Phase B: 后端 API 层（2 个子线程并行）

### B1: Board 文章列表 API

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go`
- Modify: `front/app/api/semanticBoards.ts`

**Tasks (from tasks.md Section 3):**
- 3.1 新增 `getBoardArticles` handler + 路由注册
- 3.2 查询逻辑
- 3.3 filtered_tags 批量查询（含 match_reason + score）
- 3.4 返回格式
- 3.5 前端 API client
- 3.6 单元测试
- 3.7 验证

**Code Guidance — Handler 结构:**

```go
// 路由注册 (在 RegisterSemanticBoardRoutes 中):
// boardRoutes.GET("/:id/articles", getBoardArticles)

func getBoardArticles(c *gin.Context) {
    boardID := strconv.ParseUint(c.Param("id"), 10, 64)
    page, per_page := 分页参数
    feed_id, start_date, end_date, auxiliary_label_id := 筛选参数

    // Step 1: 获取属于 board 的 tag IDs
    // SELECT topic_tag_id FROM topic_tag_board_labels WHERE semantic_board_id = boardID

    // Step 2: 查文章
    // SELECT articles.*, feeds.name as feed_name
    // FROM articles
    // JOIN article_topic_tags ON ... AND topic_tag_id IN (board_tag_ids)
    // JOIN feeds ON feeds.id = articles.feed_id
    // WHERE pub_date 筛选 / feed_id 筛选 / auxiliary_label_id 筛选
    // ORDER BY pub_date DESC
    // LIMIT/OFFSET 分页

    // Step 3: filtered_tags 批量查询
    // SELECT att.article_id, tt.id, tt.label, tt.category, tbl.match_reason, tbl.score
    // FROM article_topic_tags att
    // JOIN topic_tags tt ON tt.id = att.topic_tag_id
    // JOIN topic_tag_board_labels tbl ON tbl.topic_tag_id = tt.id AND tbl.semantic_board_id = boardID
    // WHERE att.article_id IN (当前页文章IDs)
    // 按 article_id 分组

    // Step 4: 组装返回
}
```

**前端 type:**
```typescript
export interface BoardArticleTag {
  id: number; label: string; category: string
  match_reason: string; score: number
}
export interface BoardArticle {
  id: number; title: string; url: string; pub_date: string
  feed_name: string; feed_id: number
  filtered_tags: BoardArticleTag[]
  // ... 其他 article 字段
}
```

**Verification:** `go test ./internal/domain/tagging/ -run TestBoardArticles -v`

---

### B2: Board 叙事时间线 API

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go`
- Modify: `front/app/api/semanticBoards.ts`

**Tasks (from tasks.md Section 6):**
- 6.1 新增 `getBoardNarratives` handler + 路由注册
- 6.2 查询逻辑
- 6.3 返回格式（含 related_article_ids, scope_type）
- 6.4 前端 API client
- 6.5 单元测试
- 6.6 验证

**Code Guidance — 查询逻辑:**

```go
// 路由: boardRoutes.GET("/:id/narratives", getBoardNarratives)

func getBoardNarratives(c *gin.Context) {
    boardID := ...
    days := c.DefaultQuery("days", "7")

    // 查询 narrative_boards WHERE semantic_board_id = boardID AND period_date >= now - days
    // JOIN narrative_summaries WHERE summaries.board_id = boards.id
    // 按 period_date DESC 排序

    // 每条叙事返回:
    // id, title, summary, status, related_tags (从 related_tag_ids 解析后查 label)
    // related_article_ids (从 RelatedArticleIDs JSON 解析)
    // scope_type, article_count, period_date
}
```

注意：narrative_summaries 和 narrative_boards 通过 `summaries.board_id = boards.id` 关联。
`NarrativeBoard.SemanticBoardID` 是 board 指向 semantic_board 的外键。

**前端 type:**
```typescript
export interface BoardNarrative {
  id: number; title: string; summary: string; status: string
  related_tags: { id: number; label: string }[]
  related_article_ids: number[]
  scope_type: string; article_count: number; period_date: string
}
```

**Verification:** `go test ./internal/domain/tagging/ -run TestBoardNarratives -v`

---

## Phase C: 前端改造（3 个子线程，C1/C2 可并行，C3 可与 C1 并行）

### C1: TagsPage 文章列表改造

**Depends on:** B1 (board article API 就绪)

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Tasks (from tasks.md Section 4):**
- 4.1 `loadTimelineArticles` 切换到 `getBoardArticles(boardId, params)`
- 4.2 新增 feed 下拉筛选
- 4.3 新增时间范围选择器
- 4.4 文章行展示增强：feed_name + filtered_tags chips (tooltip 含 match_reason + score)
- 4.5 验证

**Code Guidance:**

当前 `loadTimelineArticles` 调用 `articlesApi.getArticles({ concept_id: boardId })`。改为调用 `semanticBoardsApi.getBoardArticles(boardId, params)`。

新增筛选 UI 元素放在 `tags-filter-chips` 区域（当前只有辅助标签过滤按钮），扩展为：
- 辅助标签过滤按钮组（保留现有）
- Feed 下拉 (select/combobox)
- 时间范围选择器 (date input pair)

文章行增强：当前只显示日期+标题，新增：
- feed_name（来源名称，小字灰色）
- filtered_tags chips（每个 chip tooltip 显示匹配信息）

匹配信息可视化：
- `direct_hit` → tooltip: "直接命中 · 1.00"
- `hit_rate` → tooltip: "命中率 · 0.75"
- `max_sim` → tooltip: "相似度 · 0.85"
- `weighted` → tooltip: "综合 · 0.62"

**Verification:** `pnpm lint && pnpm exec nuxi typecheck && pnpm build`

---

### C2: BoardNarrativeTimeline 组件

**Depends on:** B2 (board narrative API 就绪)

**Files:**
- Create: `front/app/features/tags/components/BoardNarrativeTimeline.vue`
- Modify: `front/app/features/tags/components/TagsPage.vue` (嵌入组件)

**Tasks (from tasks.md Section 7):**
- 7.1 新建组件，调用 API
- 7.2 卡片 UI
- 7.3 点击展开文章（使用 related_article_ids）
- 7.4 空状态
- 7.5 加载更早
- 7.6 TagsPage 嵌入
- 7.7 验证

**Code Guidance — 组件结构:**

```vue
<script setup lang="ts">
const props = defineProps<{ boardId: number }>()
const { getBoardNarratives } = useSemanticBoardsApi()

const narratives = ref<BoardNarrative[]>([])
const days = ref(7)
const loading = ref(false)
const expandedId = ref<number | null>(null)
const expandedArticles = ref<Article[]>([])

async function loadNarratives() {
  loading.value = true
  const res = await getBoardNarratives(props.boardId, { days: days.value })
  narratives.value = res
  loading.value = false
}

async function toggleExpand(narrative: BoardNarrative) {
  if (expandedId.value === narrative.id) {
    expandedId.value = null; return
  }
  expandedId.value = narrative.id
  // 用 related_article_ids 批量加载文章
  expandedArticles.value = await loadArticlesByIds(narrative.related_article_ids)
}

function loadMore() {
  days.value += 7
  loadNarratives()
}

watch(() => props.boardId, loadNarratives, { immediate: true })
</script>
```

**Status 颜色映射:**
```typescript
const statusColors: Record<string, string> = {
  emerging: 'bg-green-100 text-green-700',
  continuing: 'bg-blue-100 text-blue-700',
  splitting: 'bg-orange-100 text-orange-700',
  merging: 'bg-purple-100 text-purple-700',
  ending: 'bg-gray-100 text-gray-700',
}
```

**TagsPage 嵌入位置:** 在 BoardCompositionPanel 之后、文章时间线之前：
```html
<!-- BoardCompositionPanel -->
<BoardCompositionPanel ... />
<!-- BoardNarrativeTimeline (新增) -->
<BoardNarrativeTimeline v-if="selectedBoardId" :board-id="selectedBoardId" />
<!-- 文章时间线 -->
<div class="tags-timeline"> ... </div>
```

**Verification:** `pnpm lint && pnpm exec nuxi typecheck && pnpm build`

---

### C3: 叙事功能迁移 — /topics 叙事 tab 删除

**Depends on:** 无后端依赖，可与 C1/C2 并行

**Files:**
- Modify: `front/app/features/topic-graph/components/TopicGraphPage.vue`
- Delete: `front/app/features/topic-graph/components/NarrativePanel.vue`
- Delete: `front/app/features/topic-graph/components/NarrativeBoardCanvas.vue`

**Tasks (from tasks.md Section 8):**
- 8.1 删除 activeTab 状态和叙事 tab 按钮
- 8.2 移除 NarrativePanel import
- 8.2a 删除 NarrativePanel.vue
- 8.2b 删除 NarrativeBoardCanvas.vue
- 8.3 移除叙事相关 ref/函数
- 8.4 验证

**Code Guidance:**

TopicGraphPage.vue (2341 行) 中需要清理的关键点：

1. `const activeTab = ref<'graph' | 'narrative'>('graph')` → 删除
2. Tab 按钮区域（左侧 rail）删除叙事 tab button，只保留图谱 tab
3. `v-else-if="activeTab === 'narrative'"` 整个 template 块删除
4. `import NarrativePanel from './NarrativePanel.vue'` → 删除
5. `handleNarrativeTagSelect` 函数 → 删除
6. `showConceptManager` ref → 删除（如果仅被叙事 tab 使用）
7. 检查 `expandedBoardIds`、`unclassifiedTags` 等 ref 是否仅被叙事使用，如是则删除

**重要**: 删除前先确认 NarrativeBoardCanvas.vue 没有其他引用方：
```bash
grep -rn "NarrativeBoardCanvas" front/ --include="*.vue" --include="*.ts"
grep -rn "NarrativePanel" front/ --include="*.vue" --include="*.ts"
```

**Verification:** `pnpm lint && pnpm exec nuxi typecheck && pnpm build`

---

## Phase D: 集成验证

### D1: 后端全量验证

```bash
cd backend-go && golangci-lint run ./... && go vet ./... && go test ./... && go build ./...
```

### D2: 前端全量验证

```bash
cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build
```

---

## Commit 策略

| Commit | Phase | Message |
|--------|-------|---------|
| 1 | A1 | `feat(tagging): add dual-factor constraint to max_sim matching rule` |
| 2 | A2 | `feat(tagging): enhance upgrade suggestion DTO with label fields` |
| 3 | A3 | `refactor(narrative): unify scope to board-level, remove global/category dispatch` |
| 4 | B1 | `feat(tagging): add board article list API with filtered tags and match info` |
| 5 | B2 | `feat(tagging): add board narrative timeline API` |
| 6 | C1 | `feat(front): enhance TagsPage article list with filters and match info display` |
| 7 | C2 | `feat(front): add BoardNarrativeTimeline component` |
| 8 | C3 | `refactor(front): remove narrative tab from /topics, clean up dead components` |
