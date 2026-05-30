# Section Lifecycle UI 实现计划

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 为日报的 section（聚类）增加独立生命周期，用 Jaccard 相似度匹配跨天话题，前端从 thread 粒度改为 section 粒度展示。

**Architecture:** 后端 generator 在所有 cluster 生成完毕后、保存前，通过 `cluster_tag_ids` 的 Jaccard 相似度匹配前一天 section，设置 `prev_section_id` 和 `status`。前端 BoardThreadBrowser 改为 section Gantt，ThreadLineagePanel 改为 SectionLifecyclePanel，cluster card 内线索折叠。

**Tech Stack:** Go (Gin/GORM), Vue 3 (Nuxt 4), TypeScript, Tailwind CSS v4, PostgreSQL (pgvector)

---

## Task 1: 后端模型与 Migration

**Files:**
- Modify: `backend-go/internal/domain/daily_report/models.go:36-47`
- Create: migration (GORM AutoMigrate 即可，项目无独立 migration 文件)
- Modify: `backend-go/internal/domain/daily_report/repository.go:16-87` (SaveReport upsert 清理)

**Step 1: 给 DailyReportSection 模型加字段**

在 `backend-go/internal/domain/daily_report/models.go` 的 `DailyReportSection` 结构体中，`CreatedAt` 之前添加：

```go
Status         string    `gorm:"size:20;default:emerging" json:"status"`
PrevSectionID  *uint     `json:"prev_section_id,omitempty"`
```

**Step 2: 在 SaveReport 中增加 prev_section_id 悬空清理**

在 `repository.go` 的 `SaveReport` 函数 upsert 分支中，在删除旧 section 之前（约第 44-53 行区域），增加与 `prev_thread_id` 对称的清理：

```go
// Nullify downstream prev_section_id references before deleting old sections
if err := tx.Model(&DailyReportSection{}).
    Where("prev_section_id IN (SELECT id FROM daily_report_sections WHERE report_id = ?)", existing.ID).
    Update("prev_section_id", nil).Error; err != nil {
    return fmt.Errorf("nullify downstream prev_section_id: %w", err)
}
```

位置：在现有 `prev_thread_id` 清理块之后、删除旧 threads 之前。

**Step 3: 确认 AutoMigrate 包含新字段**

检查 `cmd/server/main.go` 或 runtime 初始化代码中是否已有 `DailyReportSection` 的 AutoMigrate。项目使用 GORM AutoMigrate，应自动处理新列。确认即可。

**Step 4: 验证**

```bash
cd backend-go && go build ./...
```

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(daily-report): add status/prev_section_id to DailyReportSection model + SaveReport cleanup"
```

---

## Task 2: 后端 Section 匹配逻辑

**Files:**
- Modify: `backend-go/internal/domain/daily_report/generator.go`
  - 新增 `findPreviousSections` 函数
  - 新增 `matchPreviousSections` 函数
  - 修改 `GenerateDailyReport` 在 WaitGroup 之后集成调用

**Step 1: 新增 findPreviousSections 函数**

在 `generator.go` 中（`findPreviousReport` 附近）新增：

```go
// findPreviousSections loads all sections (with ClusterTagIDs) from the most recent
// report before the given date. Used for section-level lifecycle matching.
func findPreviousSections(boardID uint, date time.Time) []DailyReportSection {
    var report BoardDailyReport
    err := database.DB.Where(
        "semantic_board_id = ? AND period_date < ? AND status = ?",
        boardID, normalizeReportDate(date).Format("2006-01-02"), "completed",
    ).Order("period_date DESC").
        Preload("Sections").
        First(&report).Error
    if err != nil {
        return nil
    }
    return report.Sections
}
```

**Step 2: 新增 matchPreviousSections 函数**

在 `generator.go` 中（`matchPreviousThreads` 附近）新增：

```go
// matchPreviousSections matches each today's section against yesterday's sections
// using cluster_tag_ids Jaccard similarity. Sets PrevSectionID and Status.
// Threshold: intersection >= 2 OR Jaccard >= 0.3.
// Many-to-one is allowed (topic "splitting" is expected).
func matchPreviousSections(sections []DailyReportSection, prevSections []DailyReportSection) {
    if len(sections) == 0 || len(prevSections) == 0 {
        return
    }

    type prevEntry struct {
        id        uint
        clusterTagIDs []uint
    }
    var prevList []prevEntry
    for i := range prevSections {
        ps := &prevSections[i]
        var tagIDs []uint
        if ps.ClusterTagIDs != nil {
            _ = json.Unmarshal(ps.ClusterTagIDs, &tagIDs)
        }
        prevList = append(prevList, prevEntry{id: ps.ID, clusterTagIDs: tagIDs})
    }

    for i := range sections {
        sec := &sections[i]
        bestIdx := -1
        bestScore := 0.0

        var curTagIDs []uint
        if sec.ClusterTagIDs != nil {
            _ = json.Unmarshal(sec.ClusterTagIDs, &curTagIDs)
        }

        for j, prev := range prevList {
            intersection := countTagOverlap(curTagIDs, prev.clusterTagIDs)
            if intersection == 0 {
                continue
            }
            union := len(curTagIDs) + len(prev.clusterTagIDs) - intersection
            jaccard := float64(intersection) / float64(union)

            if intersection >= 2 || jaccard >= 0.3 {
                if jaccard > bestScore {
                    bestScore = jaccard
                    bestIdx = j
                }
            }
        }

        if bestIdx >= 0 {
            sec.PrevSectionID = &prevList[bestIdx].id
            sec.Status = "continuing"
        }
        // else: keeps default "emerging"
    }
}
```

**Step 3: 在 GenerateDailyReport 中集成**

在 `generator.go` 的 `GenerateDailyReport` 函数中，找到 WaitGroup 等待完成后的位置（所有 cluster 的 threads 都已生成），在调用 `SaveReport` 之前，加入：

```go
// Section lifecycle matching
prevSections := findPreviousSections(boardID, startOfDay)
matchPreviousSections(sections, prevSections)
```

需要确认 `sections` 变量在 WaitGroup 之后已经完整组装。在现有代码中，sections 在 goroutine 外创建并在 WaitGroup 后赋值 ClusterTagIDs 等，匹配应在此时执行。

**Step 4: 验证**

```bash
cd backend-go && go build ./...
```

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(daily-report): add section lifecycle matching via cluster_tag_ids Jaccard similarity"
```

---

## Task 3: 后端 Section Timeline API

**Files:**
- Modify: `backend-go/internal/domain/daily_report/repository.go` — 新增查询函数
- Modify: `backend-go/internal/domain/daily_report/handler.go` — 新增 handler
- Modify: `backend-go/internal/app/router.go` — 注册路由

**Step 1: 定义 SectionTimelineNode 和查询函数**

在 `repository.go` 中新增：

```go
// SectionTimelineNode represents a section in a timeline view.
type SectionTimelineNode struct {
    ID            uint      `json:"id"`
    ReportID      uint      `json:"report_id"`
    PeriodDate    time.Time `json:"period_date"`
    ClusterLabel  string    `json:"cluster_label"`
    Status        string    `json:"status"`
    ArticleCount  int       `json:"article_count"`
    ThreadCount   int       `json:"thread_count"`
    PrevSectionID *uint     `json:"prev_section_id,omitempty"`
}

// GetBoardSectionTimeline fetches all sections for a board within a date range.
func GetBoardSectionTimeline(boardID uint, days int) ([]SectionTimelineNode, error) {
    if days <= 0 {
        days = 30
    }
    if days > 90 {
        days = 90
    }
    var nodes []SectionTimelineNode
    err := database.DB.Raw(`
        SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label,
               COALESCE(ds.status, 'emerging') AS status,
               ds.article_count,
               (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = ds.id) AS thread_count,
               ds.prev_section_id
        FROM daily_report_sections ds
        JOIN board_daily_reports bdr ON bdr.id = ds.report_id
        WHERE bdr.semantic_board_id = ?
          AND bdr.period_date >= NOW() - ? * INTERVAL '1 day'
          AND bdr.status = 'completed'
        ORDER BY bdr.period_date DESC, ds.id ASC
    `, boardID, days).Scan(&nodes).Error
    if err != nil {
        return nil, fmt.Errorf("get board section timeline: %w", err)
    }

    // Derive ending status
    if len(nodes) > 0 {
        latestDate := nodes[0].PeriodDate // first node is latest date (DESC order)
        pointedIDs := make(map[uint]bool)
        for _, n := range nodes {
            if n.PrevSectionID != nil {
                pointedIDs[*n.PrevSectionID] = true
            }
        }
        for i := range nodes {
            if nodes[i].Status != "emerging" && nodes[i].Status != "continuing" {
                continue
            }
            if !pointedIDs[nodes[i].ID] && !isSameDay(nodes[i].PeriodDate, latestDate) {
                nodes[i].Status = "ending"
            }
        }
    }

    return nodes, nil
}

func isSameDay(a, b time.Time) bool {
    return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
```

**Step 2: 定义 GetSectionLifecycle 查询**

```go
// GetSectionLifecycle fetches the full lifecycle chain for a section using recursive CTE.
func GetSectionLifecycle(sectionID uint) ([]SectionTimelineNode, error) {
    var nodes []SectionTimelineNode
    err := database.DB.Raw(`
        WITH RECURSIVE chain AS (
            -- Base: the target section
            SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label,
                   COALESCE(ds.status, 'emerging') AS status,
                   ds.article_count,
                   (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = ds.id) AS thread_count,
                   ds.prev_section_id
            FROM daily_report_sections ds
            JOIN board_daily_reports bdr ON bdr.id = ds.report_id
            WHERE ds.id = ?

            UNION ALL

            -- Walk up to ancestors via prev_section_id
            SELECT parent.id, parent.report_id, bdr.period_date, parent.cluster_label,
                   COALESCE(parent.status, 'emerging') AS status,
                   parent.article_count,
                   (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = parent.id) AS thread_count,
                   parent.prev_section_id
            FROM daily_report_sections parent
            JOIN chain c ON c.prev_section_id = parent.id
            JOIN board_daily_reports bdr ON bdr.id = parent.report_id
        )
        SELECT * FROM chain ORDER BY period_date ASC
    `, sectionID).Scan(&nodes).Error
    if err != nil {
        return nil, fmt.Errorf("get section lifecycle: %w", err)
    }

    // Also find descendants: sections whose prev_section_id points to any chain member
    if len(nodes) > 0 {
        chainIDs := make([]uint, len(nodes))
        for i, n := range nodes {
            chainIDs[i] = n.ID
        }
        var descendants []SectionTimelineNode
        // Recursive find: any section pointing to chain member that's not already in chain
        err = database.DB.Raw(`
            WITH RECURSIVE kids AS (
                SELECT ds.id, ds.report_id, bdr.period_date, ds.cluster_label,
                       COALESCE(ds.status, 'emerging') AS status,
                       ds.article_count,
                       (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = ds.id) AS thread_count,
                       ds.prev_section_id
                FROM daily_report_sections ds
                JOIN board_daily_reports bdr ON bdr.id = ds.report_id
                WHERE ds.prev_section_id = ANY(?)

                UNION ALL

                SELECT child.id, child.report_id, bdr.period_date, child.cluster_label,
                       COALESCE(child.status, 'emerging') AS status,
                       child.article_count,
                       (SELECT COUNT(*) FROM daily_report_threads t WHERE t.section_id = child.id) AS thread_count,
                       child.prev_section_id
                FROM daily_report_sections child
                JOIN kids k ON k.id = child.prev_section_id
                JOIN board_daily_reports bdr ON bdr.id = child.report_id
            )
            SELECT * FROM kids ORDER BY period_date ASC
        `, chainIDs).Scan(&descendants).Error
        if err == nil {
            // Merge and deduplicate
            existing := make(map[uint]bool)
            for _, n := range nodes {
                existing[n.ID] = true
            }
            for _, d := range descendants {
                if !existing[d.ID] {
                    nodes = append(nodes, d)
                    existing[d.ID] = true
                }
            }
            // Re-sort by period_date
            sort.Slice(nodes, func(i, j int) bool {
                return nodes[i].PeriodDate.Before(nodes[j].PeriodDate)
            })
        }
    }

    return nodes, nil
}
```

**Step 3: 新增 handler 函数**

在 `handler.go` 中新增：

```go
func GetBoardSectionTimeline(c *gin.Context) {
    boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid board id"})
        return
    }
    days, _ := strconv.Atoi(c.DefaultQuery("days", "14"))

    nodes, err := daily_report.GetBoardSectionTimeline(uint(boardID), days)
    if err != nil {
        logging.Errorf("get board section timeline: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get section timeline"})
        return
    }
    if nodes == nil {
        nodes = []daily_report.SectionTimelineNode{}
    }
    c.JSON(http.StatusOK, gin.H{"sections": nodes})
}

func GetSectionLifecycle(c *gin.Context) {
    sectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid section id"})
        return
    }

    nodes, err := daily_report.GetSectionLifecycle(uint(sectionID))
    if err != nil {
        logging.Errorf("get section lifecycle: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get section lifecycle"})
        return
    }
    if nodes == nil {
        nodes = []daily_report.SectionTimelineNode{}
    }
    c.JSON(http.StatusOK, gin.H{"chain": nodes})
}
```

注意：handler 函数签名和参数需参照现有 `getBoardThreadTimeline` 和 `getThreadLineage` handler 的写法（可能在 `handler.go` 中是方法或独立函数，取决于现有风格）。

**Step 4: 注册路由**

在 `router.go` 中，找到 daily-report 路由组，添加：

```go
semanticBoards.GET("/:id/section-timeline", daily_report.GetBoardSectionTimeline)
dailyReports.GET("/sections/:id/lifecycle", daily_report.GetSectionLifecycle)
```

**Step 5: 验证**

```bash
cd backend-go && go build ./...
```

**Step 6: Commit**

```bash
git add -A && git commit -m "feat(daily-report): add section timeline and lifecycle API endpoints"
```

---

## Task 4: 前端 API 层

**Files:**
- Modify: `front/app/api/dailyReports.ts`

**Step 1: 新增接口类型**

在 `dailyReports.ts` 中新增：

```typescript
export interface SectionTimelineNode {
  id: number
  report_id: number
  period_date: string
  cluster_label: string
  status: string
  article_count: number
  thread_count: number
  prev_section_id: number | null
}

export interface SectionLifecycleNode {
  id: number
  report_id: number
  period_date: string
  cluster_label: string
  status: string
  article_count: number
  thread_count: number
  prev_section_id: number | null
}
```

**Step 2: DailyReportSection 接口加字段**

在 `DailyReportSection` 接口中添加 `status` 和 `prev_section_id`：

```typescript
export interface DailyReportSection {
  id: number
  cluster_index: number
  cluster_label: string
  cluster_tag_ids: number[]
  threads: DailyReportThread[]
  article_count: number
  best_tier: number
  avg_score: number
  status: string          // NEW
  prev_section_id: number | null  // NEW
}
```

**Step 3: 新增 API 方法**

在 `useDailyReportsApi` 中新增：

```typescript
async function getBoardSectionTimeline(boardId: number, days?: number): Promise<ApiResponse<{ sections: SectionTimelineNode[] }>> {
  const query = days ? `?days=${days}` : ''
  return apiClient.get(`/semantic-boards/${boardId}/section-timeline${query}`)
}

async function getSectionLifecycle(sectionId: number): Promise<ApiResponse<{ chain: SectionLifecycleNode[] }>> {
  return apiClient.get(`/daily-reports/sections/${sectionId}/lifecycle`)
}
```

并在 return 对象中导出。

**Step 4: 验证**

```bash
cd front && pnpm lint
```

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(daily-report): add section timeline/lifecycle API types and methods"
```

---

## Task 5: 前端报纸 Modal 改造（cluster card 折叠 + section 状态徽章）

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Step 1: cluster card 添加 section 状态徽章**

在每个 cluster card 的 header 区域，添加状态徽章组件。根据 `section.status` 显示颜色：
- `emerging` → 绿色（`bg-emerald-500/20 text-emerald-400`）
- `continuing` → 蓝色（`bg-blue-500/20 text-blue-400`）
- `ending` → 灰色（`bg-gray-500/20 text-gray-400`）

**Step 2: 线索默认折叠**

改造 cluster card 内容区域：
- 默认只显示 `section.status` 徽章 + 「N 条线索 ▸」文本
- 点击展开后显示线索列表（title + summary + 文章图标）
- 移除 thread 级别的状态徽章

**Step 3: section header 点击事件**

cluster card 的 header 区域（名称+状态）点击时 emit 事件打开 SectionLifecyclePanel（与 Task 6 配合）。

**Step 4: 验证**

```bash
cd front && pnpm lint
```

**Step 5: Commit**

```bash
git add -A && git commit -m "feat(daily-report): collapse threads in cluster card, add section status badge"
```

---

## Task 6: 前端 SectionLifecyclePanel（改造 ThreadLineagePanel）

**Files:**
- Rename: `ThreadLineagePanel.vue` → `SectionLifecyclePanel.vue`
- Modify: `front/app/features/tags/components/SectionLifecyclePanel.vue`

**Step 1: 改造组件**

将 `ThreadLineagePanel.vue` 改造为 `SectionLifecyclePanel.vue`：

- Props 改为 `{ sectionId: number, visible: boolean }`
- 数据源改为 `getSectionLifecycle(sectionId)`
- 链中每个节点显示：日期、聚类名称、状态徽章、文章数、线索数
- 当前 section 高亮
- 节点间竖线连接

**Step 2: 面板定位**

- `position: fixed; right: 0; top: 0; width: 320px; height: 100vh; z-index: 50`
- 不移动 Modal，两者独立滚动

**Step 3: 交互**

- 面板内点击节点 → emit `navigateToSection(node)` 事件
- ✕ 按钮 → emit `close`
- 关闭 Modal 时同时关闭面板

**Step 4: 更新引用**

在 `BoardDailyReportTimeline.vue` 中将 `ThreadLineagePanel` 引用改为 `SectionLifecyclePanel`。

**Step 5: 验证**

```bash
cd front && pnpm lint
```

**Step 6: Commit**

```bash
git add -A && git commit -m "feat(daily-report): create SectionLifecyclePanel replacing ThreadLineagePanel"
```

---

## Task 7: 前端话题总览（BoardThreadBrowser 改造）

**Files:**
- Modify: `front/app/features/tags/components/BoardThreadBrowser.vue`

**Step 1: 数据源替换**

- 从 `getBoardThreadTimeline` 改为 `getBoardSectionTimeline`
- 类型从 `ThreadLineageNode` 改为 `SectionTimelineNode`
- `buildChains` 逻辑适配：用 `prev_section_id` 串联

**Step 2: 行和节点改造**

- 行代表 section 生命周期（`prev_section_id` 串联）
- 每行显示 `cluster_label`
- 节点颜色：emerging=绿, continuing=蓝, ending=灰
- 移除 splitting/merging 状态色

**Step 3: 点击交互**

- 点击圆点 → 打开该 section 的日报 Modal + SectionLifecyclePanel
- emit 事件与父组件配合

**Step 4: 文案更新**

- 标题从「线程时间线」改为「话题总览」

**Step 5: 验证**

```bash
cd front && pnpm lint
```

**Step 6: Commit**

```bash
git add -A && git commit -m "feat(daily-report): convert BoardThreadBrowser to section-level topic overview"
```

---

## Task 8: 清理与端到端验证

**Files:**
- Various

**Step 1: 确认旧 API 保留**

确认 `getBoardThreadTimeline` 前端不再有调用点，后端路由保留（向后兼容）。

**Step 2: 前端编译验证**

```bash
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
```

**Step 3: 后端全量验证**

```bash
cd backend-go && golangci-lint run ./... && go vet ./... && go test ./internal/domain/daily_report/... && go build ./...
```

**Step 4: Commit**

```bash
git add -A && git commit -m "chore(daily-report): cleanup and e2e verification for section lifecycle"
```

---

## Task Dependency Graph

```
Task 1 (model + SaveReport)
  ↓
Task 2 (section matching logic)
  ↓
Task 3 (API endpoints) ←── 依赖 Task 1, 2
  ↓
Task 4 (frontend API types) ←── 可与 Task 3 并行
  ↓
Task 5 (newspaper modal) ←── 依赖 Task 4
Task 6 (lifecycle panel) ←── 依赖 Task 4
Task 7 (topic overview)  ←── 依赖 Task 4
  ↓
Task 8 (cleanup + verify) ←── 依赖全部
```

**可并行的分组：**
- **Group A (后端)**: Task 1 → Task 2 → Task 3（串行）
- **Group B (前端 API)**: Task 4（可与 Task 2/3 并行）
- **Group C (前端 UI)**: Task 5 + Task 6 + Task 7（依赖 Task 4，三者可并行）
- **Group D**: Task 8（最后）
