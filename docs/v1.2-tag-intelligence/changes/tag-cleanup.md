# 标签体系瘦身与治理 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 active 标签从 5580 个压缩到 1500-2000 个，清理垃圾标签并收紧源头，使标签体系可管理。

**Architecture:** 分三波执行——P0 批量清理现有垃圾（纯 SQL + 后端函数，无 LLM），P1 收紧提取源头（prompt 和阈值调整），P2 增强归集能力。每波独立可交付。

**Tech Stack:** Go (Gin/GORM), PostgreSQL (pgvector), LLM prompt engineering

**当前数据快照（2026-04-28）：**

| 指标 | 数值 |
|------|------|
| Active 标签总数 | 5580 |
| keyword | 3959 (71%) |
| event | 1297 (23%) |
| person | 320 (6%) |
| 0 文章标签 | 1507 (keyword 995, event 409, person 103) |
| 1 文章标签 | 3303 (keyword 2442, event 706, person 155) |
| 未归入抽象树 | 4256 (keyword 3187, event 807, person 262) |
| 文章打标覆盖率 | 20.7% (777/3757) |
| 质量分未计算 | 1591 |
| 0 文章标签中 95%+ quality_score < 0.15 | 1451/1507 |

---

## Task 1: 批量清理零文章标签（Phase 0: 一次性 SQL 清理）

**目的：** 立即删除 1507 个没有任何文章关联的僵尸标签，释放系统负载。

**Files:**
- Create: `backend-go/cmd/cleanup-tags/main.go` (一次性清理命令)
- Reference: `backend-go/internal/domain/topicanalysis/tag_cleanup.go` (现有 zombie 清理逻辑)
- Reference: `backend-go/internal/domain/models/topic_tag.go` (TopicTag 模型)

**Step 1: 编写一次性清理命令**

创建 `backend-go/cmd/cleanup-tags/main.go`：

```go
package main

import (
	"fmt"
	"os"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

func main() {
	database.Init()
	logging.Init()

	dryRun := os.Getenv("DRY_RUN") != "false"
	if dryRun {
		fmt.Println("=== DRY RUN (set DRY_RUN=false to execute) ===")
	}

	// Phase A: Deactivate tags with zero article associations
	zeroArticleCleanup(dryRun)

	// Phase B: Deactivate single-article low-quality keyword tags
	singleArticleLowQualityCleanup(dryRun)

	fmt.Println("Done.")
}

func zeroArticleCleanup(dryRun bool) {
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = 'active'").
		Where("(kind != 'abstract' AND source != 'abstract')").
		Where("NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id)").
		Where("NOT EXISTS (SELECT 1 FROM ai_summary_topics ast WHERE ast.topic_tag_id = topic_tags.id)")

	var count int64
	query.Count(&count)
	fmt.Printf("\n[Phase A] Zero-article tags to deactivate: %d\n", count)

	if count == 0 {
		return
	}

	// Show breakdown by category
	type catCount struct {
		Category string
		Kind     string
		Cnt      int64
	}
	var breakdown []catCount
	database.DB.Model(&models.TopicTag{}).
		Select("category, kind, count(*) as cnt").
		Where("status = 'active'").
		Where("(kind != 'abstract' AND source != 'abstract')").
		Where("NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id)").
		Where("NOT EXISTS (SELECT 1 FROM ai_summary_topics ast WHERE ast.topic_tag_id = topic_tags.id)").
		Group("category, kind").
		Order("cnt DESC").
		Scan(&breakdown)
	for _, b := range breakdown {
		fmt.Printf("  %s/%s: %d\n", b.Category, b.Kind, b.Cnt)
	}

	if !dryRun {
		// Also delete their embeddings and hierarchy relations first
		var tagIDs []uint
		query.Pluck("id", &tagIDs)

		// Delete embeddings
		database.DB.Where("topic_tag_id IN ?", tagIDs).Delete(&models.TopicTagEmbedding{})
		fmt.Printf("  Deleted embeddings for %d tags\n", len(tagIDs))

		// Delete hierarchy relations where these tags are children
		database.DB.Where("child_id IN ? AND relation_type = 'abstract'", tagIDs).Delete(&models.TopicTagRelation{})
		// Delete relations where these tags are parents (shouldn't happen for non-abstract, but safety)
		database.DB.Where("parent_id IN ? AND relation_type = 'abstract'", tagIDs).Delete(&models.TopicTagRelation{})

		// Deactivate
		result := query.Updates(map[string]interface{}{"status": "inactive"})
		fmt.Printf("  Deactivated %d tags\n", result.RowsAffected)
	}
}

func singleArticleLowQualityCleanup(dryRun bool) {
	// Single-article keyword tags with quality_score < 0.15
	// These are essentially noise - one-off keywords that will never be reused
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = 'active'").
		Where("category = 'keyword'").
		Where("kind = 'keyword'").
		Where("quality_score < 0.15").
		Where("(SELECT count(*) FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id) = 1")

	var count int64
	query.Count(&count)
	fmt.Printf("\n[Phase B] Single-article low-quality keyword tags (score<0.15): %d\n", count)

	if count == 0 {
		return
	}

	if !dryRun {
		var tagIDs []uint
		query.Pluck("id", &tagIDs)

		// Delete embeddings
		database.DB.Where("topic_tag_id IN ?", tagIDs).Delete(&models.TopicTagEmbedding{})

		// Delete hierarchy relations
		database.DB.Where("child_id IN ? AND relation_type = 'abstract'", tagIDs).Delete(&models.TopicTagRelation{})

		// Deactivate
		result := query.Updates(map[string]interface{}{"status": "inactive"})
		fmt.Printf("  Deactivated %d tags\n", result.RowsAffected)
	}
}
```

**Step 2: DRY RUN 验证**

Run: `cd backend-go && DRY_RUN=true go run ./cmd/cleanup-tags`

Expected: 输出统计数字，不修改数据。Phase A 应约 1507，Phase B 应约 55。

**Step 3: 执行清理**

Run: `cd backend-go && DRY_RUN=false go run ./cmd/cleanup-tags`

Expected: 两个 Phase 各输出 deactivated 数量。

**Step 4: 验证清理结果**

```bash
docker exec zanebono-rssreader-pgvector psql -U postgres -d rss_reader -c \
  "SELECT count(*) as total, count(*) FILTER (WHERE status='active') as active, count(*) FILTER (WHERE status='inactive') as inactive FROM topic_tags;"
```

Expected: active 应降至约 4000 左右。

**Step 5: Commit**

```bash
git add backend-go/cmd/cleanup-tags/
git commit -m "feat: add one-off tag cleanup command for zero-article and low-quality tags"
```

---

## Task 2: 放宽 Phase 1 Zombie 清理条件

**目的：** 让定时清理更激进，防止垃圾标签再次积累。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup.go:18-59` (zombie 条件)
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go:248-259` (Phase 1 调用参数)
- Test: `backend-go/internal/domain/topicanalysis/tag_cleanup_test.go`

**Step 1: 写测试**

在 `backend-go/internal/domain/topicanalysis/tag_cleanup_test.go` 中：

```go
func TestCleanupZombieTags_RelaxedCriteria(t *testing.T) {
	// Verify that MinAgeDays=3 works (reduced from 7)
	// Verify that tags WITH abstract relations but WITHOUT articles ARE cleaned
	// Verify that tags WITH articles are NOT cleaned
}
```

**Step 2: 运行测试验证失败**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestCleanupZombieTags_RelaxedCriteria -v`
Expected: FAIL

**Step 3: 修改 zombie 清理逻辑**

在 `tag_cleanup.go` 中新增一个函数 `CleanupZeroArticleTags`，不要求"无关系"条件，只要求零文章 + 非摘要引用：

```go
func CleanupZeroArticleTags(categories []string) (int, error) {
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = 'active'").
		Where("(kind != 'abstract' AND source != 'abstract')").
		Where("category IN ?", categories).
		Where("NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id)").
		Where("NOT EXISTS (SELECT 1 FROM ai_summary_topics ast WHERE ast.topic_tag_id = topic_tags.id)")

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count zero-article tags: %w", err)
	}
	if count == 0 {
		return 0, nil
	}

	// Collect IDs first for cleanup
	var tagIDs []uint
	query.Pluck("id", &tagIDs)

	// Delete embeddings
	database.DB.Where("topic_tag_id IN ?", tagIDs).Delete(&models.TopicTagEmbedding{})

	// Delete child relations
	database.DB.Where("child_id IN ? AND relation_type = 'abstract'", tagIDs).Delete(&models.TopicTagRelation{})

	if err := query.Updates(map[string]interface{}{"status": "inactive"}).Error; err != nil {
		return 0, fmt.Errorf("deactivate zero-article tags: %w", err)
	}

	logging.Infof("CleanupZeroArticleTags: deactivated %d tags", count)
	return int(count), nil
}
```

同时在 `tag_cleanup.go` 新增 `CleanupLowQualitySingleArticleTags`：

```go
func CleanupLowQualitySingleArticleTags(category string, maxScore float64) (int, error) {
	query := database.DB.Model(&models.TopicTag{}).
		Where("status = 'active'").
		Where("category = ?", category).
		Where("kind != 'abstract' AND source != 'abstract'").
		Where("quality_score < ?", maxScore).
		Where("(SELECT count(*) FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id) = 1")

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count low-quality single-article tags: %w", err)
	}
	if count == 0 {
		return 0, nil
	}

	var tagIDs []uint
	query.Pluck("id", &tagIDs)

	database.DB.Where("topic_tag_id IN ?", tagIDs).Delete(&models.TopicTagEmbedding{})
	database.DB.Where("child_id IN ? AND relation_type = 'abstract'", tagIDs).Delete(&models.TopicTagRelation{})

	if err := query.Updates(map[string]interface{}{"status": "inactive"}).Error; err != nil {
		return 0, fmt.Errorf("deactivate low-quality single-article tags: %w", err)
	}

	logging.Infof("CleanupLowQualitySingleArticleTags(%s, <%.2f): deactivated %d tags", category, maxScore, count)
	return int(count), nil
}
```

**Step 4: 在 scheduler 中调用新清理函数**

修改 `tag_hierarchy_cleanup.go` 的 `runCleanupCycle`，在 Phase 1 后插入 Phase 1.5：

```go
// Phase 1.5: Zero-article tag cleanup (no LLM, more aggressive than Phase 1)
for _, category := range []string{"event", "keyword", "person"} {
    zeroCount, err := topicanalysis.CleanupZeroArticleTags([]string{category})
    if err != nil {
        logging.Errorf("Phase 1.5 zero-article cleanup failed for %s: %v", category, err)
        summary.Errors++
    } else {
        summary.ZombieDeactivated += zeroCount
        logging.Infof("Phase 1.5 (%s): deactivated %d zero-article tags", category, zeroCount)
    }
}

// Phase 1.6: Low-quality single-article keyword cleanup (no LLM)
lqCount, err := topicanalysis.CleanupLowQualitySingleArticleTags("keyword", 0.15)
if err != nil {
    logging.Errorf("Phase 1.6 low-quality keyword cleanup failed: %v", err)
    summary.Errors++
} else {
    summary.ZombieDeactivated += lqCount
    logging.Infof("Phase 1.6: deactivated %d low-quality single-article keyword tags", lqCount)
}
```

**Step 5: 运行测试**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -v -run TestCleanupZombieTags`
Expected: PASS

**Step 6: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: 成功

**Step 7: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/tag_cleanup.go backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: add aggressive zero-article and low-quality tag cleanup (Phase 1.5/1.6)"
```

---

## Task 3: 收紧 Keyword 提取 Prompt 和数量限制

**目的：** 从源头减少 keyword 标签的爆炸性增长。

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/extractor_enhanced.go:230-269` (提取 prompt)
- Modify: `backend-go/internal/domain/topicextraction/article_tagger.go:16` (maxArticleTags 常量)
- Test: `backend-go/internal/domain/topicextraction/extractor_test.go`

**Step 1: 减少每篇文章最大标签数**

修改 `backend-go/internal/domain/topicextraction/article_tagger.go:16`：

```go
// 从 8 降到 5
const maxArticleTags = 5
```

**Step 2: 收紧提取 prompt**

修改 `buildExtractionSystemPrompt()` 中关于数量的指令，将：

```
- 最多返回 8 个标签
```

改为：

```
- 最多返回 5 个标签，其中 keyword 类最多 3 个
- 宁少勿多：如果文章只聚焦一个话题，2-3 个标签就够了
```

同时强化 keyword 的提取门槛，在 keyword 说明后追加：

```
  - keyword 类标签必须是具有持久辨识度的实体或术语，不接受只在一篇文章出现的临时性描述词
  - 如果一个 keyword 只在单篇文章中有意义，不要提取它
```

**Step 3: 运行现有测试确保通过**

Run: `cd backend-go && go test ./internal/domain/topicextraction/... -v`
Expected: 全部 PASS（prompt 变更不影响测试逻辑）

注意：`metadata_test.go:152` 中有 `limited tag count = %d, want 8` 的断言，需要更新为 `want 5`。

**Step 4: 更新受影响的测试**

找到 `metadata_test.go` 中引用 `maxArticleTags=8` 的断言，改为 5。

**Step 5: 验证**

Run: `cd backend-go && go test ./internal/domain/topicextraction/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add backend-go/internal/domain/topicextraction/
git commit -m "feat: reduce max tags per article from 8 to 5, tighten keyword extraction prompt"
```

---

## Task 4: 提高 Keyword 类别的匹配阈值

**目的：** 减少 keyword 标签的碎片化，让更多相似 keyword 被合并。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go` (FindSimilarTags 或 TagMatch 中的阈值逻辑)
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` (findOrCreateTag 中的阈值使用)
- Test: `backend-go/internal/domain/topicextraction/tagger_test.go` 或 `topicanalysis/embedding_test.go`

**Step 1: 了解当前阈值配置**

阅读 `embedding.go` 中 `TagMatch` 函数和 `FindSimilarTags` 的阈值参数。当前 `LowSimilarity = 0.78` 是全局的。

**Step 2: 为 keyword 类别使用更高阈值**

在 `tagger.go` 的 `findOrCreateTag` 中，根据 category 使用不同阈值：

```go
// 当前: candidates 的判断都走统一阈值
// 改为: keyword 类别的搜索阈值提高到 0.85

searchThreshold := topicanalysis.LowSimilarity // 默认 0.78
if category == "keyword" {
    searchThreshold = 0.85
}
```

如果 `TagMatch` 接受阈值参数，传入更高的值；否则在 `findOrCreateTag` 中对 keyword 的候选结果做二次过滤。

**Step 3: 写测试**

测试 keyword 类别使用 0.85 阈值、event 保持 0.78。

**Step 4: 验证**

Run: `cd backend-go && go test ./internal/domain/topicextraction/... ./internal/domain/topicanalysis/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicextraction/tagger.go backend-go/internal/domain/topicanalysis/embedding.go
git commit -m "feat: raise keyword similarity threshold to 0.85 to reduce fragmentation"
```

---

## Task 5: 用 Quality Score 驱动自动清理

**目的：** 让质量分未计算的标签得到评分，并在 scheduler 中用分数驱动自动清理。

**Files:**
- Modify: `backend-go/internal/jobs/tag_quality_score.go` (确保评分覆盖所有 active 标签)
- Modify: `backend-go/internal/domain/topicextraction/quality_score.go` (检查评分逻辑是否排除了某些标签)
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go` (在 Phase 1.6 后加入 quality-score 清理)

**Step 1: 调查 1591 个未评分标签的原因**

检查 `quality_score.go` 中 `CalculateQualityScores` 是否只处理了部分 category，或有其他过滤条件。

**Step 2: 修复评分覆盖率**

确保所有 `status='active'` 且 `(kind != 'abstract' AND source != 'abstract')` 的标签都参与评分。

**Step 3: 在清理 scheduler 中增加质量分驱动清理**

在 Phase 1.6 后加入 Phase 1.7，清理 `quality_score = 0` 且年龄 > 3 天的标签（这些是"评分系统都忽略了"的标签）。

**Step 4: 验证**

Run: `cd backend-go && go test ./internal/domain/topicextraction/... -v -run TestQuality`
Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicextraction/quality_score.go backend-go/internal/jobs/tag_quality_score.go backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: improve quality score coverage and add score-driven auto-cleanup"
```

---

## Task 6: 提高离线聚类的 batch size

**目的：** 让离线聚类能覆盖更多未分类标签。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/config_service.go` (ClusterConfig 默认值)
- Reference: `backend-go/internal/domain/topicanalysis/tag_clustering.go:114` (使用 cfg.MaxTags)

**Step 1: 修改默认配置**

在 `config_service.go` 中将 `cluster_max_tags` 默认值从 200 改为 500：

```go
// 从 200 提高到 500
MaxTags: 500,
```

**Step 2: 运行测试**

Run: `cd backend-go && go test ./internal/domain/topicanalysis/... -v`
Expected: PASS

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/config_service.go
git commit -m "feat: increase cluster_max_tags from 200 to 500 for broader offline clustering"
```

---

## Task 7: 更新文档

**目的：** 反映所有改动到项目文档。

**Files:**
- Modify: `docs/guides/tagging-flow.md` (更新 Phase 描述、阈值变更)
- Modify: `docs/guides/topic-graph.md` (更新质量分、清理策略描述)

**Step 1: 更新 tagging-flow.md**

- Phase 1 描述中加入 Phase 1.5 (零文章清理) 和 Phase 1.6 (低质量单文章 keyword 清理)
- 更新阈值表格，keyword 的 LowSimilarity 改为 0.85
- 更新 `maxArticleTags` 从 8 改为 5
- 更新 `cluster_max_tags` 从 200 改为 500

**Step 2: 更新 topic-graph.md**

- 更新质量分章节，反映新的自动清理阈值
- 更新未分类标签离线聚类章节中的默认配置

**Step 3: Commit**

```bash
git add docs/guides/tagging-flow.md docs/guides/topic-graph.md
git commit -m "docs: update tagging flow and topic graph docs for tag cleanup changes"
```

---

## 预期效果

执行完 Task 1-2 后的一次性清理效果：

| 清理项 | 预计消除 |
|--------|---------|
| 零文章标签 | ~1507 |
| 单文章低质量 keyword | ~55 |
| **合计** | **~1562** |

执行完 Task 3-6 后的持续效果：

- 每篇文章最多 5 个标签（原 8 个），keyword 最多 3 个
- keyword 匹配阈值 0.85（原 0.78），更积极地合并
- 离线聚类 batch 从 200 提升到 500，覆盖更多未分类标签
- 定时清理更激进，不会再积累大量僵尸标签

**最终目标：** active 标签从 5580 降至 1500-2000，后续增量得到控制。

---

## 执行顺序

```
Task 1 (一次性SQL清理) → 验证 → Commit
    ↓
Task 2 (scheduler Phase 1.5/1.6) → 测试 → Commit
    ↓
Task 3 (prompt + 数量限制) → 测试 → Commit
    ↓
Task 4 (keyword 阈值) → 测试 → Commit
    ↓
Task 5 (quality score) → 测试 → Commit
    ↓
Task 6 (cluster batch) → 测试 → Commit
    ↓
Task 7 (文档更新) → Commit
```

每个 Task 独立可回滚。Task 1 是立竿见影的一次性操作，Task 2-6 是持续防护。
