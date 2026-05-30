# matching-quality-and-daily-report-redesign Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** 将方向校验扩展到所有匹配规则，文章按质量分层排序，日报管线精简（质量筛选+聚类数限制+去dynamics），前端改为多页报纸布局。

**Architecture:** P1-P3 为后端独立任务可并行（P1 方向校验、P2 文章排序、P3.1-3.4 日报精简），P3.5 依赖 P3.1（需要 TagInput 有 MatchReason/Score），P4 前端依赖 P3.5（需要后端返回 best_tier/avg_score）。

**Tech Stack:** Go (Gin/GORM), Vue 3 (Nuxt 4), Tailwind CSS v4, PostgreSQL + pgvector

---

## Task 1: P1 方向校验扩展

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching.go:212-234`
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching_test.go:553-586`

**Step 1: 移动方向校验代码**

在 `semantic_board_matching.go` 的 `evaluateSemanticBoardMatches` 函数中，将方向校验从 `case max_sim` 内部移到 switch 语句之后。

当前代码（lines 212-234）:
```go
switch {
case hitRate > config.DirectHitRate:
    score = config.HitRateSimBlend*maxSimilarity + (1-config.HitRateSimBlend)*hitRate
    matchReason = "hit_rate"
case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
    score = maxSimilarity
    matchReason = "max_sim"
    if minHits < config.DirectMaxSimMinHits {
        downgraded = true
    }
    // Direction check: only for max_sim
    if len(tagEmbedding) > 0 {
        if boardEmb, ok := boardEmbeddings[boardID]; ok && len(boardEmb) > 0 {
            dirSim := cosineSimilarity(tagEmbedding, boardEmb)
            if dirSim < config.DirectionSimThreshold {
                directionMismatch = true
            }
        }
    }
case weighted >= config.WeightedThreshold:
    score = weighted
    matchReason = "weighted"
}
```

改为:
```go
switch {
case hitRate > config.DirectHitRate:
    score = config.HitRateSimBlend*maxSimilarity + (1-config.HitRateSimBlend)*hitRate
    matchReason = "hit_rate"
case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
    score = maxSimilarity
    matchReason = "max_sim"
    if minHits < config.DirectMaxSimMinHits {
        downgraded = true
    }
case weighted >= config.WeightedThreshold:
    score = weighted
    matchReason = "weighted"
}

// Direction check: applies to all match reasons except direct_hit
if matchReason != "" && matchReason != "direct_hit" {
    if len(tagEmbedding) > 0 {
        if boardEmb, ok := boardEmbeddings[boardID]; ok && len(boardEmb) > 0 {
            dirSim := cosineSimilarity(tagEmbedding, boardEmb)
            if dirSim < config.DirectionSimThreshold {
                directionMismatch = true
            }
        }
    }
}
```

注意：`direct_hit` 是在 switch 之前单独处理的（通过 `overlapCount >= config.DirectHitMinOverlap`），不走 switch，所以 `matchReason != "direct_hit"` 检查是安全的。

**Step 2: 更新测试**

当前测试 `TestEvaluateSemanticBoardMatches_DirectionCheck` 最后一个子测试 `"non-max_sim match does not run direction check"` (lines 553-586) 断言 hit_rate 匹配时 `DirectionMismatch=false`。需要改为断言 `DirectionMismatch=true`（因为方向正交），并重命名。

同时新增测试：
- `"weighted match with orthogonal embeddings → direction mismatch"` — weighted 匹配 + 正交 embedding → DirectionMismatch=true

**Step 3: 运行测试验证**

```bash
cd backend-go && go test ./internal/domain/tagging -run TestEvaluateSemanticBoardMatches_DirectionCheck -v
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_matching.go backend-go/internal/domain/tagging/semantic_board_matching_test.go
git commit -m "feat(tagging): extend direction check to hit_rate and weighted match reasons"
```

---

## Task 2: P2 文章排序优先级

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go:383,427-443`

**Step 1: 添加 tier 计算函数**

在 `semantic_board_handler.go` 中添加辅助函数（放在 `getBoardArticles` 函数附近）：

```go
func matchTier(matchReason string, downgraded bool) int {
    switch {
    case matchReason == "direct_hit":
        return 0
    case matchReason == "hit_rate":
        return 1
    case matchReason == "max_sim" && !downgraded:
        return 2
    default: // max_sim(downgraded) or weighted
        return 3
    }
}
```

**Step 2: 修改 getBoardArticles 排序逻辑**

当前排序在 SQL 层面（line 383）：
```go
query = query.Order("articles.pub_date DESC").Offset(offset).Limit(perPage)
```

改为去掉 SQL 排序，在 Go 端排序。

1. 移除 SQL ORDER BY，改为按 ID 排序（保证确定性）：
```go
query = query.Order("articles.id ASC").Offset(offset).Limit(perPage)
```

2. 在组装 filtered_tags 之后（line ~443 之后），添加 Go 端排序逻辑：

```go
// Calculate best tier per article from filtered tags
articleBestTier := make(map[uint]int)
articleBestScore := make(map[uint]float64)
for _, ft := range filteredTags {
    t := matchTier(ft.MatchReason, ft.Downgraded)
    if existing, ok := articleBestTier[ft.ArticleID]; !ok || t < existing {
        articleBestTier[ft.ArticleID] = t
        articleBestScore[ft.ArticleID] = ft.Score
    } else if t == existing && ft.Score > articleBestScore[ft.ArticleID] {
        articleBestScore[ft.ArticleID] = ft.Score
    }
}

// Sort articles by (tier ASC, score DESC, pub_date DESC)
sort.SliceStable(articles, func(i, j int) bool {
    ti, tj := articleBestTier[articles[i].ID], articleBestTier[articles[j].ID]
    if ti != tj {
        return ti < tj
    }
    si, sj := articleBestScore[articles[i].ID], articleBestScore[articles[j].ID]
    if si != sj {
        return si > sj
    }
    return articles[i].PubDate.After(articles[j].PubDate)
})
```

注意：`articles` 的类型需要确认——应该是 `[]models.Article` 或类似的 slice，有 `ID` 和 `PubDate` 字段。`sort` 包需要 import `"sort"`。

**Step 3: 验证构建**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_handler.go
git commit -m "feat(tagging): sort board articles by match quality tier instead of pure time"
```

---

## Task 3: P3.1 collectBoardTags 携带匹配质量 + P3.1.2 TagInput 扩展

**Files:**
- Modify: `backend-go/internal/domain/daily_report/models.go:77-84` (TagInput struct)
- Modify: `backend-go/internal/domain/daily_report/generator.go:390-463` (collectBoardTags)

**Step 1: 扩展 TagInput**

在 `models.go` 的 `TagInput` struct 中添加两个字段：

```go
type TagInput struct {
    ID           uint    `json:"id"`
    Label        string  `json:"label"`
    Category     string  `json:"category"`
    Description  string  `json:"description"`
    ArticleCount int     `json:"article_count"`
    Source       string  `json:"source"`
    MatchReason  string  `json:"match_reason"`
    Score        float64 `json:"score"`
}
```

**Step 2: 修改 collectBoardTags 主查询**

在 `generator.go` 的 `collectBoardTags` 函数中：

1. 主查询的 `Select` 子句增加 `topic_tag_board_labels.match_reason` 和 `topic_tag_board_labels.score`（别名为 `match_reason` 和 `score`）

2. 扫描结果时填充 `TagInput.MatchReason` 和 `TagInput.Score`

当前 Select（约 lines 404-408）：
```go
Select(`topic_tags.id, topic_tags.label, topic_tags.category, topic_tags.description, topic_tags.source, COUNT(DISTINCT articles.id) AS article_count`)
```

改为：
```go
Select(`topic_tags.id, topic_tags.label, topic_tags.category, topic_tags.description, topic_tags.source, topic_tag_board_labels.match_reason, topic_tag_board_labels.score, COUNT(DISTINCT articles.id) AS article_count`)
```

扫描时需要增加对应字段。

**Step 3: 修改 fallback 路径**

fallback 路径（约 lines 436-463）调用 `matcher.MatchTopicTag()`，返回的 `SemanticBoardMatchResult` 有 `MatchReason` 和 `Score` 字段。需要在 fallback 标签纳入时设置 `TagInput.MatchReason` 和 `TagInput.Score`。

**Step 4: 验证构建**

```bash
cd backend-go && go build ./...
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/daily_report/models.go backend-go/internal/domain/daily_report/generator.go
git commit -m "feat(daily_report): carry match_reason and score in collectBoardTags"
```

---

## Task 4: P3.2 质量筛选层

**Files:**
- Modify: `backend-go/internal/domain/daily_report/generator.go` (GenerateDailyReport, 在聚类前)

**Step 1: 添加筛选函数**

在 `generator.go` 中添加质量筛选函数：

```go
func filterTagsByQuality(tags []TagInput) []TagInput {
    // Step 1: Filter out direction_mismatch (already done in collectBoardTags query,
    // but as a safety net)
    var noMismatch []TagInput
    // Note: direction_mismatch is filtered at SQL level in collectBoardTags,
    // so this step is a no-op for the main path. But if future code passes tags
    // from other sources, this would catch it.

    // Step 2: Separate by match reason quality
    var kept []TagInput
    var weighted []TagInput
    for _, t := range tags {
        switch t.MatchReason {
        case "direct_hit", "hit_rate", "max_sim":
            kept = append(kept, t)
        case "weighted":
            weighted = append(weighted, t)
        default:
            // Unknown match reason, treat as low quality
            weighted = append(weighted, t)
        }
    }

    // Step 3: If kept < 10, pull back weighted tags
    if len(kept) < 10 {
        kept = append(kept, weighted...)
    }

    // Step 4: If kept > 30, truncate by quality
    if len(kept) > 30 {
        sort.SliceStable(kept, func(i, j int) bool {
            ti := matchTier(kept[i].MatchReason, false) // simplified: no downgraded info at this level
            tj := matchTier(kept[j].MatchReason, false)
            if ti != tj {
                return ti < tj
            }
            return kept[i].Score > kept[j].Score
        })
        kept = kept[:30]
    }

    return kept
}
```

注意：`matchTier` 已在 Task 2 中定义在 `semantic_board_handler.go`。这里需要决定：
- 方案 A: 将 `matchTier` 提取到共享位置（如 `models.go` 或新文件）
- 方案 B: 在 `daily_report` 包内重新定义

推荐方案 A：将 `matchTier` 移到 `semantic_board_matching.go` 中作为 exported 函数 `MatchTier`。

**Step 2: 在 GenerateDailyReport 中调用**

在 `GenerateDailyReport` 函数中，`DeduplicateTags` 之后、`ClusterTags` 之前，插入：

```go
tags = filterTagsByQuality(tags)
if len(tags) == 0 {
    return BoardDailyReport{}, nil, nil // or return an appropriate error/empty state
}
```

**Step 3: 验证**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/daily_report/generator.go
git commit -m "feat(daily_report): add quality filtering layer before clustering"
```

---

## Task 5: P3.3 聚类数限制

**Files:**
- Modify: `backend-go/internal/domain/daily_report/cluster.go:9-26,28-88`

**Step 1: 修改 ClusterTags 接受动态 prompt 约束**

将 `clusterSystemPrompt` 从 const 改为接受参数的函数：

```go
func buildClusterSystemPrompt(tagCount int) string {
    base := `你是一名专业的事件分组分析师。你的任务是将一组事件标签按照它们所描述的核心事件进行分组。

分组规则：
1. 属于同一核心事件的标签归入一组
2. 每组 2-8 个标签；如果某个标签找不到同类，可以单独成组
3. 分组粒度：比"同一主题"更细，比"完全相同"更宽。例如"某公司发布财报"和"该公司CEO变动"虽然相关但应分属不同组
4. 每组给出一个简洁的中文组名（不超过20字）
5. 必须确保每个输入标签恰好出现在一个组中`

    if tagCount > 25 {
        base += "\n6. 标签数量较多，请分成 8-15 组，合并关联性强的小事件"
    } else if tagCount > 15 {
        base += "\n6. 请分成 6-12 组"
    }
    // tagCount <= 15: no limit, natural grouping

    return base
}
```

**Step 2: 修改 ClusterTags 调用**

在 `ClusterTags` 函数中，将 `clusterSystemPrompt` 替换为 `buildClusterSystemPrompt(len(tags))`。

**Step 3: 验证构建**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/daily_report/cluster.go
git commit -m "feat(daily_report): add dynamic cluster count limits based on tag count"
```

---

## Task 6: P3.4 去掉板块动态

**Files:**
- Modify: `backend-go/internal/domain/daily_report/generator.go` (GenerateDailyReport 并发逻辑)

**Step 1: 移除 Call B**

在 `GenerateDailyReport` 函数中：

1. 删除 `dynamicsCh` channel 和 `dynamicsResult` 类型
2. 删除 `GenerateDynamics` 的 goroutine 调用
3. 删除从 `dynamicsCh` 接收结果的代码
4. 将 `report.Dynamics = dynamicsData` 改为 `report.Dynamics = ""`
5. 将 `report.GenerationPromptVersion` 改为 `"2.0"`

并发结构从：
```
Call A (highlights)  +  Call B (dynamics)  +  Call C×K (threads)
```
变为：
```
Call A (highlights)  +  Call C×K (threads)
```

**Step 2: 清理 GenerateDynamics 函数**

如果 `GenerateDynamics` 没有其他调用者，可以删除整个函数。如果有，保留但不再调用。

**Step 3: 验证**

```bash
cd backend-go && go build ./... && go test ./internal/domain/daily_report/...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/daily_report/generator.go
git commit -m "feat(daily_report): remove GenerateDynamics call, simplify pipeline"
```

---

## Task 7: P3.5 聚类排序字段

**Files:**
- Modify: `backend-go/internal/domain/daily_report/models.go:35-44` (DailyReportSection)
- Modify: `backend-go/internal/domain/daily_report/generator.go` (报告组装)

**Step 1: 扩展 DailyReportSection**

在 `DailyReportSection` struct 中添加：

```go
type DailyReportSection struct {
    ID            uint      `gorm:"primarykey" json:"id"`
    ReportID      uint      `gorm:"index;not null" json:"report_id"`
    ClusterIndex  int       `json:"cluster_index"`
    ClusterLabel  string    `gorm:"size:200" json:"cluster_label"`
    ClusterTagIDs JSON      `gorm:"type:jsonb" json:"cluster_tag_ids"`
    Threads       JSON      `gorm:"type:jsonb" json:"threads"`
    ArticleCount  int       `json:"article_count"`
    BestTier      int       `gorm:"default:0" json:"best_tier"`
    AvgScore      float64   `gorm:"default:0" json:"avg_score"`
    CreatedAt     time.Time `json:"created_at"`
}
```

**Step 2: 计算并填充字段**

在 `GenerateDailyReport` 组装 sections 时，对每个 section：

```go
// Calculate best_tier and avg_score from tags in this cluster
tagIDSet := make(map[uint]bool)
for _, tid := range cluster.TagIDs {
    tagIDSet[tid] = true
}

bestTier := 4 // worst possible
totalScore := 0.0
matchCount := 0
for _, t := range tags {
    if tagIDSet[t.ID] {
        tier := MatchTier(t.MatchReason, false) // simplified
        if tier < bestTier {
            bestTier = tier
        }
        totalScore += t.Score
        matchCount++
    }
}
avgScore := 0.0
if matchCount > 0 {
    avgScore = totalScore / float64(matchCount)
}

section := DailyReportSection{
    // ... existing fields ...
    BestTier: bestTier,
    AvgScore: avgScore,
}
```

**Step 3: 验证构建**

```bash
cd backend-go && go build ./... && go test ./internal/domain/daily_report/...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/daily_report/models.go backend-go/internal/domain/daily_report/generator.go
git commit -m "feat(daily_report): add best_tier and avg_score to DailyReportSection"
```

---

## Task 8: P4.1 前端数据适配

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Step 1: 处理 dynamics 为空**

找到渲染 "板块动态" 的区域（约 lines 289-293），当前已有 `v-if="currentPg.dynamics"` 条件，无需修改。确认模板中 `<template v-if="currentPg.dynamics">` 存在即可。

**Step 2: 聚类按 best_tier + avg_score 排序**

在 `pages` computed 中，sections 需要按 `best_tier ASC, avg_score DESC` 排序后再分页。

当前 `pages` computed（约 lines 33-46）是一页一个 section。改为先排序再分页：

```typescript
const sortedSections = [...report.sections].sort((a, b) => {
    if (a.best_tier !== b.best_tier) return a.best_tier - b.best_tier
    return b.avg_score - a.avg_score
})
```

**Step 3: Commit**

```bash
git add front/app/features/tags/components/BoardDailyReportTimeline.vue
git commit -m "feat(front): sort report sections by best_tier and avg_score"
```

---

## Task 9: P4.2 多页报纸布局

**Files:**
- Modify: `front/app/features/tags/components/BoardDailyReportTimeline.vue`

**Step 1: 重构 pages computed**

将 `pages` 从 "每页一个 section" 改为 "每页多个聚类格子"：

```typescript
// 第1页: overview + top-4 sections
// 第2+页: 本页热点 + top-5 sections
const PAGE1_CAPACITY = 4
const PAGE_N_CAPACITY = 5

const pages = computed(() => {
    // ... 对于当前选中报告
    const sorted = [...sections].sort((a, b) => {
        if (a.best_tier !== b.best_tier) return a.best_tier - b.best_tier
        return b.avg_score - a.avg_score
    })

    const result = []

    // Page 1: overview + first 4 sections
    const page1Sections = sorted.slice(0, PAGE1_CAPACITY)
    result.push({
        type: 'overview',
        highlights: report.highlights,
        dynamics: report.dynamics,
        sections: page1Sections
    })

    // Remaining pages
    let idx = PAGE1_CAPACITY
    while (idx < sorted.length) {
        const end = Math.min(idx + PAGE_N_CAPACITY, sorted.length)
        result.push({
            type: 'content',
            sections: sorted.slice(idx, end)
        })
        idx = end
    }

    return result
})
```

**Step 2: 修改 NewspaperPage 类型**

更新类型定义，`overview` 页增加 `sections` 字段，新增 `content` 页类型。

**Step 3: 重写模板**

- Overview 页：保留报头 + highlights，去掉 dynamics 区域（已由 v-if 处理），增加聚类格子网格
- Content 页：页头 + 本页热点（sections[0] 的概要）+ 聚类格子网格
- 聚类格子组件：名称 + 文章数 + 叙事列表（最多 3 条，超出折叠）

**Step 4: CSS Grid 布局**

为聚类格子区域添加 CSS Grid：
```css
.np-cluster-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 1rem;
}
```

**Step 5: 验证**

```bash
cd front && pnpm lint
```

**Step 6: Commit**

```bash
git add front/app/features/tags/components/BoardDailyReportTimeline.vue
git commit -m "feat(front): multi-page newspaper layout with clustered sections"
```

---

## Task 10: P5 全量验证

**Step 1: 后端验证**
```bash
cd backend-go && go build ./... && go vet ./... && go test ./internal/domain/tagging/... ./internal/domain/daily_report/...
```

**Step 2: 前端验证**
```bash
# lint 在 WSL
cd front && pnpm lint
# typecheck + build 在 Windows cmd
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
```
