# Board Interaction Overhaul — 剩余任务实施计划

> **REQUIRED SUB-SKILL:** Use the subagent-driven-development skill to implement tasks in parallel.

**Goal:** 完成 board-interaction-overhaul 变更中剩余的 61 个任务（G9/G11/G12/G13/G14）

**Architecture:** 4 个独立组可并行执行，日报系统是最大的新增模块（new domain `daily_report`），最后做集成验证

**Tech Stack:** Go (Gin/GORM) + Vue 3 (Nuxt 4, TypeScript, Tailwind CSS v4)

---

## 并行批次 1（独立，无依赖）

### Stream A: G9 — TagsPage 内容 Tab 切换 (Tasks #53-56)

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Context:** TagsPage 当前有 `contentTab` ref 但 type 是 `'composition' | 'narratives' | 'articles'`，需改为 `'composition' | 'daily-reports' | 'articles'`。需确认当前模板中 tab 的实现状态。

**Task 53:** TagsPage 新增/修改 `contentTab` ref 为 `'composition' | 'daily-reports' | 'articles'`，默认 `'composition'`。选中 board 时显示 Tab 栏（板块内容 / 日报 / 文章），按 tab 切换。

**Task 54:** BoardCompositionPanel、BoardDailyReportTimeline（待 G13 创建）、文章时间线各区域用 `v-if="contentTab === 'xxx'"` 控制显隐。注意：BoardDailyReportTimeline 组件在 G13 才创建，此处先用 `v-if` 预留位置。

**Task 55:** Tab 栏样式——简洁横向 tab，暗色风格，选中态用 accent 色。

**Task 56:** 验证：`pnpm lint` + `pnpm exec nuxi typecheck`

**Verification:**
```bash
cd front && pnpm lint && pnpm exec nuxi typecheck
```

---

### Stream B: G11 — Refresh 并行化 (Tasks #64-67)

**Files:**
- Modify: `backend-go/internal/domain/feed/handler.go` (refreshAllFeedsWorker)
- Modify: `front/app/features/shell/components/FeedLayoutShell.vue` (onMounted)

**Context:**
- 当前 `refreshAllFeedsWorker` 是串行 for 循环，需改为 `sync.WaitGroup` + `chan struct{}(cap=3)` semaphore
- 当前 `onMounted` 是 `await fetchFeeds(); await loadArticles(); await fetchGlobalUnreadCount(); await loadWatchedTags()` 串行

**Task 64 (Backend):** 改造 `refreshAllFeedsWorker`：
```go
func refreshAllFeedsWorker(feedIDs []uint) {
    feedService := NewFeedService()
    sem := make(chan struct{}, 3)
    var wg sync.WaitGroup

    for _, id := range feedIDs {
        wg.Add(1)
        sem <- struct{}{}
        go func(feedID uint) {
            defer wg.Done()
            defer func() { <-sem }()
            defer func() {
                if r := recover(); r != nil {
                    logging.Errorf("[refresh-all] PANIC refreshing feed %d: %v", feedID, r)
                    resetFeedStatus(feedID, fmt.Sprintf("panic: %v", r))
                }
            }()
            if err := feedService.RefreshFeed(context.Background(), feedID); err != nil {
                logging.Errorf("[refresh-all] Error refreshing feed %d: %v", feedID, err)
                resetFeedStatus(feedID, err.Error())
            }
        }(id)
    }
    wg.Wait()
}
```

**Task 65:** 验证：`go test ./internal/domain/feed/ -run TestRefreshAll -v` + `go build ./...`

**Task 66 (Frontend):** 改造 `onMounted` 为两波 Promise.all：
```typescript
onMounted(async () => {
  // Wave 1: no dependencies between these two
  const [,] = await Promise.allSettled([
    fetchFeeds(),
    loadWatchedTags(),
  ])
  // Wave 2: may depend on feeds list
  await Promise.allSettled([
    loadArticles(),
    fetchGlobalUnreadCount(),
  ])
})
```
注意：用 `Promise.allSettled` 而非 `Promise.all`，确保部分失败不影响另一部分。

**Task 67:** 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

**Verification:**
```bash
cd backend-go && go build ./...
cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build
```

---

### Stream C: G12 — 匹配得分可视化 (Tasks #68-72)

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Context:** 当前文章行的 tag chips 只显示 label + tooltip (match_reason + score)。需要：颜色编码 + 分数文字 + 文章行最强匹配信息。

**Task 68:** Tag chip 颜色样式。每个 tag chip 根据 `match_reason` 添加颜色：
- `direct_hit` → `#22c55e` (绿)
- `hit_rate` → `#3b82f6` (蓝)
- `max_sim` → `#f59e0b` (橙)
- `weighted` → `#94a3b8` (灰)
颜色应用于 chip 的 `border-color` 或 `background-color`。

**Task 69:** Chip 内显示分数文字，格式 `[标签名 0.85]`。score 保留两位小数。在 template 的 tag chip 区域修改展示格式。

**Task 70:** 文章行右侧 end 处显示最强匹配信息：从 `article.filtered_tags` 中取 score 最高的 tag，显示匹配方式中文名 + 最高分数。

**Task 71:** 新增两个工具函数：
```typescript
function matchReasonColor(reason: string): string {
  const colors: Record<string, string> = {
    direct_hit: '#22c55e',
    hit_rate: '#3b82f6',
    max_sim: '#f59e0b',
    weighted: '#94a3b8',
  }
  return colors[reason] || '#94a3b8'
}

function matchInfoLabel(tag: BoardArticleTag): string {
  const labels: Record<string, string> = {
    direct_hit: '直接命中',
    hit_rate: '命中率',
    max_sim: '相似度',
    weighted: '综合',
  }
  return `${labels[tag.match_reason] || tag.match_reason} ${tag.score.toFixed(2)}`
}
```

**Task 72:** 验证：`pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build`

**Verification:**
```bash
cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm build
```

---

### Stream D: G13 Backend — 日报系统后端 (Tasks #73-93)

这是最大的新增模块，需要创建 `backend-go/internal/domain/daily_report/` 新 domain。

#### D1: 数据模型 (Tasks #73-74)

**Files:**
- Create: `backend-go/internal/domain/daily_report/models.go`
- Modify: `backend-go/internal/app/runtime.go` (AutoMigrate)

**models.go:**
```go
package daily_report

import (
    "time"
    "gorm.io/gorm"
)

type BoardDailyReport struct {
    ID                     uint           `gorm:"primarykey" json:"id"`
    SemanticBoardID        uint           `gorm:"index;not null" json:"semantic_board_id"`
    PeriodDate             time.Time      `gorm:"type:date;not null" json:"period_date"`
    Title                  string         `json:"title"`
    Summary                string         `json:"summary"`
    Highlights             JSON           `gorm:"type:jsonb" json:"highlights"`
    Dynamics               string         `gorm:"type:text" json:"dynamics"`
    ArticleCount           int            `json:"article_count"`
    EventTagCount          int            `json:"event_tag_count"`
    ClusterCount           int            `json:"cluster_count"`
    Status                 string         `gorm:"size:20;default:generating" json:"status"`
    RawClusters            JSON           `gorm:"type:jsonb" json:"raw_clusters,omitempty"`
    PrevReportID           *uint          `json:"prev_report_id,omitempty"`
    GenerationPromptVersion string        `gorm:"size:20" json:"generation_prompt_version,omitempty"`
    CreatedAt              time.Time      `json:"created_at"`
    UpdatedAt              time.Time      `json:"updated_at"`
}

type DailyReportSection struct {
    ID            uint      `gorm:"primarykey" json:"id"`
    ReportID      uint      `gorm:"index;not null" json:"report_id"`
    ClusterIndex  int       `json:"cluster_index"`
    ClusterLabel  string    `gorm:"size:200" json:"cluster_label"`
    ClusterTagIDs JSON      `gorm:"type:jsonb" json:"cluster_tag_ids"`
    Threads       JSON      `gorm:"type:jsonb" json:"threads"`
    ArticleCount  int       `json:"article_count"`
    CreatedAt     time.Time `json:"created_at"`
}

type JSON []byte  // implement gorm.Valuer, sql.Scanner
```

AutoMigrate 注册到 runtime.go。

#### D2: 去重模块 (Tasks #75-76)

**Files:**
- Create: `backend-go/internal/domain/daily_report/dedup.go`
- Create: `backend-go/internal/domain/daily_report/dedup_test.go`

去重输入是 event tags（来自 narrative domain 的 `TagInput` 或类似结构），需要知道 tag → article IDs 的映射。两个规则：(1) 文章集合完全相同 → 合并；(2) article_count=1 且关联同一文章 → 合并。

#### D3: LLM 分组模块 (Tasks #77-79)

**Files:**
- Create: `backend-go/internal/domain/daily_report/cluster.go`
- Create: `backend-go/internal/domain/daily_report/cluster_test.go`

使用 `airouter.NewAIRouter()` 调用 LLM，temperature=0.1，JSON schema 约束输出 `[{group_name, tag_ids[]}]`。

#### D4: 生成模块 (Tasks #80-85)

**Files:**
- Create: `backend-go/internal/domain/daily_report/generator.go`

编排流水线：收集 → 去重 → 分组 → 并行生成(Call A + B + C×K) → 组装 → 存储。
需要复用 narrative domain 的一些基础设施（如 `LoadBoardEventTags`、airouter）。

#### D5: 存储模块 (Tasks #86-88)

**Files:**
- Create: `backend-go/internal/domain/daily_report/repository.go`

SaveReport / GetReport / ListReports。

#### D6: API (Tasks #89-92)

**Files:**
- Create: `backend-go/internal/domain/daily_report/handler.go`
- Modify: `backend-go/internal/app/router.go` (注册新路由)

POST /api/daily-reports/generate（异步）、GET /api/semantic-boards/:id/daily-reports、GET /api/daily-reports/:id。

#### D7: 定时任务 (Tasks #93-94)

**Files:**
- Create: `backend-go/internal/jobs/daily_report.go`
- Modify: `backend-go/internal/jobs/narrative_summary.go`（或替换）

复用 scheduler_tasks 表的 narrative_summary 任务。

---

## 并行批次 2（依赖批次 1 的部分结果）

### Stream E: G13 Frontend — 日报系统前端 (Tasks #94-102)

**依赖:** Stream A (G9 Tab 切换) + Stream D (G13 后端 API)

#### E1: API Client (Task #94)

**Files:**
- Create: `front/app/api/dailyReports.ts`

```typescript
// generateDailyReport(params), getBoardDailyReports(boardId, params), getDailyReportDetail(id)
```

#### E2: WS Composable (Task #95)

**Files:**
- Create: `front/app/composables/useDailyReportProgress.ts`

参考 `useTagWebSocket.ts`、`useWebSocketRebuild.ts` 的模式。

#### E3: BoardDailyReportTimeline 组件 (Tasks #96-102)

**Files:**
- Create: `front/app/features/tags/components/BoardDailyReportTimeline.vue`
- Modify: `front/app/features/tags/components/TagsPage.vue`
- Modify: `front/app/features/tags/components/NarrativeGenerateDialog.vue`

替代 BoardNarrativeTimeline，展示结构化日报。

---

## 顺序批次 3

### Stream F: G13 旧系统废弃 (Tasks #103-105)

**Files:**
- Modify: `backend-go/internal/domain/narrative/service.go` (标注 deprecated)
- Modify: `backend-go/internal/jobs/narrative_summary.go` (切换到日报生成)

### Stream G: G13 + G14 验证 (Tasks #106-120)

全量构建、lint、测试验证。

---

## 执行策略

### Batch 1（并行，4 个子线程）
1. **Subagent A**: G9 Tasks #53-56 (TagsPage Tab)
2. **Subagent B**: G11 Tasks #64-67 (Refresh 并行化)
3. **Subagent C**: G12 Tasks #68-72 (匹配得分可视化)
4. **Subagent D**: G13 Backend Tasks #73-93 (日报后端)

### Batch 2（并行，依赖 Batch 1）
5. **Subagent E**: G13 Frontend Tasks #94-102 (日报前端)

### Batch 3（顺序）
6. G13 旧系统废弃 Tasks #103-105
7. G13 验证 Tasks #106-108
8. G14 集成验证 Tasks #109-120（手动验证为主）

### 验收标准
- 每个子线程完成后在 tasks.md 中标记对应任务 `[x]`
- 所有 `go build ./...` + `go test ./...` + `pnpm lint` + `pnpm exec nuxi typecheck` + `pnpm build` 通过
- 无 linter 错误
