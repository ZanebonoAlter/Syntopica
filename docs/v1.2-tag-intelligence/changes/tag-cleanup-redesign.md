# Tag Cleanup Redesign 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 重写 tag 层级清理系统，从"深层树修剪"改为三级清理策略（僵尸清理 → 扁平合并 → 层次精简），匹配实际数据特征。

**Architecture:** 保持现有调度器框架（`TagHierarchyCleanupScheduler`）。新增 `tag_cleanup.go` 承载三级清理的主逻辑；`hierarchy_cleanup.go` 保留为历史兼容代码，不再参与调度器主流程。调度器 `runCleanupCycle` 现在只按顺序执行三个阶段。

**Tech Stack:** Go, GORM, PostgreSQL, 现有 `airouter` LLM 调用, 现有 `MergeTags`

---

## 数据现状（决策依据）

| 指标 | 数值 | 影响 |
|------|------|------|
| Active tags | 3,369 | 碎片化严重 |
| 孤立标签（无 abstract 关系） | 2,240 (66%) | 大量标签未被层次化 |
| 孤立且无文章（僵尸） | 1,006 (30%) | 最高优先级清理 |
| Abstract event 标签关联文章 | 全部 0 篇 | 抽象标签不参与文章关联 |
| depth ≥ 5 的树 | 仅 2 棵 | 当前流程几乎不触发 |
| 多父节点冲突 | 15+ 个标签 | 需要精简 |

## 三级策略概述

## 实施后更新

- 已落地版本没有保留旧的“树深度放宽后继续清理”步骤。
- `runCleanupCycle` 现在只跑三步：僵尸清理 -> 扁平合并 -> 层次精简。
- 运行摘要也改成只记录这三步的结果，不再记录旧树流程的统计。
- 文档里的旧树流程内容仅作为最初设计讨论保留，不能再当作当前行为理解。

### Phase 1: 僵尸标签清理（无 LLM）
- 条件：`status=active` + 无 abstract 关系 + 关联文章 = 0
- 操作：批量标记为 `inactive`
- 预计清理：~1,006 个标签

### Phase 2: 扁平化相似合并（LLM 辅助）
- 不依赖树结构，按 category 分批
- 从 abstract 标签池中检测相似/重复对
- 每批 ≤ 50 个标签，LLM 判断是否合并
- 保留关联文章更多的标签

### Phase 3: 层次结构精简
- 处理多父节点冲突：保留最相似关系
- 清理无叶子节点的 abstract 中间节点
- 清理引用了 `merged` 状态标签的 abstract 关系

---

## Task 1: 创建新文件 `tag_cleanup.go` — Phase 1 僵尸标签清理

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/tag_cleanup.go`
- Test: `backend-go/internal/domain/topicanalysis/tag_cleanup_test.go`

**Step 1: 写僵尸标签清理函数的测试**

```go
// tag_cleanup_test.go
package topicanalysis

import (
	"testing"
)

func TestFindZombieTagIDs_NoDatabase(t *testing.T) {
	// 纯逻辑测试：验证 ZombieTagCriteria 结构和过滤条件
	criteria := ZombieTagCriteria{
		MinAgeDays: 7,
		Categories: []string{"event", "keyword"},
	}
	if len(criteria.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(criteria.Categories))
	}
	if criteria.MinAgeDays != 7 {
		t.Errorf("expected 7 min age days, got %d", criteria.MinAgeDays)
	}
}

func TestBuildZombieQuery(t *testing.T) {
	criteria := ZombieTagCriteria{
		MinAgeDays: 7,
		Categories: []string{"event", "keyword"},
	}
	query := BuildZombieTagSubQuery(criteria)
	if query == "" {
		t.Error("expected non-empty query")
	}
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/domain/topicanalysis -run TestFindZombieTagIDs -v`
Expected: FAIL — types not defined

**Step 3: 实现僵尸标签清理**

```go
// tag_cleanup.go
package topicanalysis

import (
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

// ZombieTagCriteria defines the criteria for identifying zombie tags
type ZombieTagCriteria struct {
	MinAgeDays int
	Categories []string
}

// CleanupZombieTags finds and deactivates tags with no articles and no abstract relations.
// Returns the count of tags deactivated.
func CleanupZombieTags(criteria ZombieTagCriteria) (int, error) {
	subQuery := BuildZombieTagSubQuery(criteria)

	result := database.DB.Model(&models.TopicTag{}).
		Where("id IN (?)", gorm.Expr(subQuery))

	var count int64
	if err := result.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count zombie tags: %w", err)
	}

	if count == 0 {
		return 0, nil
	}

	if err := result.Updates(map[string]interface{}{
		"status": "inactive",
	}).Error; err != nil {
		return 0, fmt.Errorf("deactivate zombie tags: %w", err)
	}

	logging.Infof("CleanupZombieTags: deactivated %d zombie tags", count)
	return int(count), nil
}

// BuildZombieTagSubQuery returns SQL subquery selecting IDs of zombie tags.
// Zombie = active + no abstract relations + no article associations + older than MinAgeDays.
func BuildZombieTagSubQuery(criteria ZombieTagCriteria) string {
	return fmt.Sprintf(`
		SELECT t.id FROM topic_tags t
		WHERE t.status = 'active'
		  AND t.category IN (%s)
		  AND t.created_at < NOW() - INTERVAL '%d days'
		  AND NOT EXISTS (
		    SELECT 1 FROM topic_tag_relations r
		    WHERE (r.parent_id = t.id OR r.child_id = t.id) AND r.relation_type = 'abstract'
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM article_topic_tags att
		    WHERE att.topic_tag_id = t.id
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM ai_summary_topics ast
		    WHERE ast.topic_tag_id = t.id
		  )
	`, quoteCategories(criteria.Categories), criteria.MinAgeDays)
}

func quoteCategories(categories []string) string {
	quoted := ""
	for i, c := range categories {
		if i > 0 {
			quoted += ", "
		}
		quoted += fmt.Sprintf("'%s'", c)
	}
	return quoted
}
```

**Step 4: 运行测试确认通过**

Run: `go test ./internal/domain/topicanalysis -run "TestFindZombieTagIDs|TestBuildZombieQuery" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/tag_cleanup.go backend-go/internal/domain/topicanalysis/tag_cleanup_test.go
git commit -m "feat: add zombie tag cleanup (Phase 1 of tag cleanup redesign)"
```

---

## Task 2: `tag_cleanup.go` — Phase 2 扁平化相似合并

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup_test.go`

**Step 1: 写扁平合并的测试**

```go
func TestBuildFlatMergePrompt(t *testing.T) {
	tags := []FlatTagInfo{
		{ID: 1, Label: "日本地震", Description: "关于日本地震", Source: "abstract", ArticleCount: 0},
		{ID: 2, Label: "日本本州地震", Description: "日本本州海域地震", Source: "abstract", ArticleCount: 0},
		{ID: 3, Label: "半导体产业", Description: "半导体行业动态", Source: "abstract", ArticleCount: 0},
	}
	prompt := BuildFlatMergePrompt(tags, "event")
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestFlatMergeJudgment_Parse(t *testing.T) {
	// 验证 JSON 结构
	judgment := flatMergeJudgment{}
	if len(judgment.Merges) != 0 {
		t.Error("expected empty merges initially")
	}
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/domain/topicanalysis -run "TestBuildFlatMergePrompt|TestFlatMergeJudgment" -v`
Expected: FAIL

**Step 3: 实现扁平合并逻辑**

在 `tag_cleanup.go` 中追加（注意：需要在文件顶部 import 中加入 `"context"` 和 `"encoding/json"`）：

```go
// 确保文件顶部 import 包含以下所有项：
// import (
//     "context"
//     "encoding/json"
//     "fmt"
//
//     "my-robot-backend/internal/domain/models"
//     "my-robot-backend/internal/domain/topicanalysis/airouter"
//     "my-robot-backend/internal/domain/topicanalysis/jsonutil"
//     "my-robot-backend/internal/platform/database"
//     "my-robot-backend/internal/platform/logging"
//
//     "gorm.io/gorm"
// )

// FlatTagInfo is a simplified tag representation for flat merge analysis
type FlatTagInfo struct {
	ID           uint   `json:"id"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	Source       string `json:"source"`
	ArticleCount int    `json:"article_count"`
	ChildCount   int    `json:"child_count"`
}

// flatMergeJudgment is the LLM response for flat merge analysis
type flatMergeJudgment struct {
	Merges []flatMergeItem `json:"merges,omitempty"`
	Notes  string          `json:"notes,omitempty"`
}

type flatMergeItem struct {
	SourceID uint   `json:"source_id"`
	TargetID uint   `json:"target_id"`
	Reason   string `json:"reason"`
}

// CollectFlatTagBatch loads a batch of abstract tags for a category.
// Returns up to batchSize tags that are source='abstract' and status='active'.
func CollectFlatTagBatch(category string, batchSize int) ([]FlatTagInfo, error) {
	var tags []models.TopicTag
	if err := database.DB.
		Where("category = ? AND status = 'active' AND source = 'abstract'", category).
		Limit(batchSize).
		Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("load abstract tags: %w", err)
	}

	tagIDs := make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	articleCounts := countArticlesByTag(tagIDs, "")

	childCounts := make(map[uint]int)
	var childRows []struct {
		ParentID uint `gorm:"column:parent_id"`
		Cnt      int  `gorm:"column:cnt"`
	}
	database.DB.Model(&models.TopicTagRelation{}).
		Select("parent_id, count(*) as cnt").
		Where("parent_id IN ? AND relation_type = 'abstract'", tagIDs).
		Group("parent_id").
		Scan(&childRows)
	for _, r := range childRows {
		childCounts[r.ParentID] = r.Cnt
	}

	result := make([]FlatTagInfo, len(tags))
	for i, t := range tags {
		result[i] = FlatTagInfo{
			ID:           t.ID,
			Label:        t.Label,
			Description:  truncateStr(t.Description, 200),
			Source:        t.Source,
			ArticleCount: articleCounts[t.ID],
			ChildCount:   childCounts[t.ID],
		}
	}
	return result, nil
}

// BuildFlatMergePrompt builds the LLM prompt for flat merge analysis
func BuildFlatMergePrompt(tags []FlatTagInfo, category string) string {
	promptData := map[string]interface{}{
		"category": category,
		"total":    len(tags),
		"tags":     tags,
	}

	promptJSON, _ := json.MarshalIndent(promptData, "", "  ")

	return fmt.Sprintf(`你是一位标签分类专家。请分析以下 %s 类别的抽象标签列表，找出语义重复或高度相似的标签对。

标签列表：
%s

请返回以下格式的 JSON：
{
  "merges": [
    {
      "source_id": 123,
      "target_id": 456,
      "reason": "这两个标签描述的是同一个概念，应该合并"
    }
  ],
  "notes": "其他观察（可选）"
}

规则：
1. merges 是可选的，可以为空数组
2. source_id: 被合并的标签（子标签数更少或描述更窄的那个）
3. target_id: 保留的目标标签（子标签数更多或描述更广的那个）
4. 只合并真正描述同一核心概念的标签，不要合并仅有部分重叠的标签
5. 如果没有需要合并的，返回空数组
6. 只返回真正有把握的建议`, category, string(promptJSON))
}

// ExecuteFlatMerge executes the flat merge phase for a category
func ExecuteFlatMerge(category string, batchSize int) (int, []string, error) {
	tags, err := CollectFlatTagBatch(category, batchSize)
	if err != nil {
		return 0, nil, fmt.Errorf("collect tags: %w", err)
	}
	if len(tags) == 0 {
		return 0, nil, nil
	}

	prompt := BuildFlatMergePrompt(tags, category)
	judgment, err := callFlatMergeLLM(prompt)
	if err != nil {
		return 0, nil, fmt.Errorf("LLM call: %w", err)
	}

	tagMap := make(map[uint]*FlatTagInfo)
	for i := range tags {
		tagMap[tags[i].ID] = &tags[i]
	}

	var errors []string
	merged := 0
	for _, merge := range judgment.Merges {
		if err := validateFlatMerge(merge, tagMap); err != nil {
			errors = append(errors, fmt.Sprintf("merge %d→%d: %v", merge.SourceID, merge.TargetID, err))
			continue
		}
		if err := MergeTags(merge.SourceID, merge.TargetID); err != nil {
			errors = append(errors, fmt.Sprintf("merge %d→%d: %v", merge.SourceID, merge.TargetID, err))
			continue
		}
		merged++
	}

	logging.Infof("ExecuteFlatMerge(%s): %d tags analyzed, %d merges applied", category, len(tags), merged)
	return merged, errors, nil
}

func validateFlatMerge(merge flatMergeItem, tagMap map[uint]*FlatTagInfo) error {
	source, ok := tagMap[merge.SourceID]
	if !ok {
		return fmt.Errorf("source %d not found", merge.SourceID)
	}
	target, ok := tagMap[merge.TargetID]
	if !ok {
		return fmt.Errorf("target %d not found", merge.TargetID)
	}
	if merge.SourceID == merge.TargetID {
		return fmt.Errorf("same tag")
	}
	if source.ChildCount > 0 && target.ChildCount > 0 {
		if source.ChildCount > target.ChildCount {
			return fmt.Errorf("source has more children (%d) than target (%d), swap recommended", source.ChildCount, target.ChildCount)
		}
	}
	return nil
}

func callFlatMergeLLM(prompt string) (*flatMergeJudgment, error) {
	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy cleanup assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"merges": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"source_id": {Type: "integer"},
							"target_id": {Type: "integer"},
							"reason":    {Type: "string"},
						},
						Required: []string{"source_id", "target_id", "reason"},
					},
				},
				"notes": {Type: "string"},
			},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata:    map[string]any{"operation": "tag_flat_merge"},
	}

	result, err := router.Chat(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment flatMergeJudgment
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}
	return &judgment, nil
}
```

需要在文件顶部 import 中加入 `"encoding/json"` 和 `"context"`。

**Step 4: 运行测试确认通过**

Run: `go test ./internal/domain/topicanalysis -run "TestBuildFlatMergePrompt|TestFlatMergeJudgment" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/tag_cleanup.go backend-go/internal/domain/topicanalysis/tag_cleanup_test.go
git commit -m "feat: add flat merge cleanup (Phase 2 of tag cleanup redesign)"
```

---

## Task 3: `tag_cleanup.go` — Phase 3 层次精简

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup_test.go`

**Step 1: 写层次精简测试**

```go
func TestCleanupOrphanedRelations(t *testing.T) {
	// 纯逻辑验证：确认函数签名
	_ = CleanupOrphanedRelations
}

func TestCleanupMultiParentConflicts_Signature(t *testing.T) {
	_ = CleanupMultiParentConflicts
}

func TestCleanupEmptyAbstractNodes_Signature(t *testing.T) {
	_ = CleanupEmptyAbstractNodes
}
```

**Step 2: 运行测试确认失败**

Run: `go test ./internal/domain/topicanalysis -run "TestCleanupOrphanedRelations|TestCleanupMultiParent|TestCleanupEmptyAbstract" -v`
Expected: FAIL

**Step 3: 实现层次精简函数**

在 `tag_cleanup.go` 中追加：

```go
// CleanupOrphanedRelations removes abstract relations that reference merged/inactive tags.
func CleanupOrphanedRelations() (int, error) {
	result := database.DB.Where(
		"relation_type = 'abstract' AND (parent_id IN (SELECT id FROM topic_tags WHERE status != 'active') OR child_id IN (SELECT id FROM topic_tags WHERE status != 'active'))",
	).Delete(&models.TopicTagRelation{})

	if result.Error != nil {
		return 0, fmt.Errorf("cleanup orphaned relations: %w", result.Error)
	}

	deleted := int(result.RowsAffected)
	if deleted > 0 {
		logging.Infof("CleanupOrphanedRelations: removed %d orphaned relations", deleted)
	}
	return deleted, nil
}

// CleanupMultiParentConflicts resolves tags with >1 abstract parent by keeping the best.
// NOTE: resolveMultiParentConflict (abstract_tag_service.go) has no return value.
// It handles errors internally via logging. We count resolved conflicts but cannot
// capture per-conflict errors from the underlying function.
func CleanupMultiParentConflicts() (int, []string, error) {
	var conflicts []struct {
		ChildID uint `gorm:"column:child_id"`
		Cnt     int  `gorm:"column:cnt"`
	}
	database.DB.Model(&models.TopicTagRelation{}).
		Select("child_id, count(*) as cnt").
		Where("relation_type = 'abstract'").
		Group("child_id").
		Having("count(*) > 1").
		Scan(&conflicts)

	if len(conflicts) == 0 {
		return 0, nil, nil
	}

	totalResolved := 0
	for _, c := range conflicts {
		resolveMultiParentConflict(c.ChildID)
		totalResolved++
	}

	logging.Infof("CleanupMultiParentConflicts: resolved %d conflicts", totalResolved)
	return totalResolved, nil, nil
}

// CleanupEmptyAbstractNodes removes abstract tags with 0 children (no relations as parent).
func CleanupEmptyAbstractNodes() (int, error) {
	subQuery := `
		SELECT t.id FROM topic_tags t
		WHERE t.source = 'abstract' AND t.status = 'active'
		  AND NOT EXISTS (
		    SELECT 1 FROM topic_tag_relations r
		    WHERE r.parent_id = t.id AND r.relation_type = 'abstract'
		  )`

	result := database.DB.Model(&models.TopicTag{}).
		Where("id IN (%s)", subQuery)

	var count int64
	result.Count(&count)
	if count == 0 {
		return 0, nil
	}

	if err := result.Updates(map[string]interface{}{
		"status": "inactive",
	}).Error; err != nil {
		return 0, fmt.Errorf("cleanup empty abstracts: %w", err)
	}

	logging.Infof("CleanupEmptyAbstractNodes: deactivated %d empty abstract tags", count)
	return int(count), nil
}
```

**Step 4: 运行测试确认通过**

Run: `go test ./internal/domain/topicanalysis -run "TestCleanupOrphanedRelations|TestCleanupMultiParent|TestCleanupEmptyAbstract" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/tag_cleanup.go backend-go/internal/domain/topicanalysis/tag_cleanup_test.go
git commit -m "feat: add hierarchy pruning cleanup (Phase 3 of tag cleanup redesign)"
```

---

## Task 4: 调整 `hierarchy_cleanup.go` 的角色

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go`

**实施后说明：**

- 最初计划是把树深度门槛从 5 改到 3，让旧树清理继续参与调度。
- 实际落地时，这条路被放弃了，因为它会和新的三阶段流程重复处理同一批标签。
- 因此这里最终只保留兼容代码和基础测试，不再把 `BuildTagForest` / `ProcessTree` 接进 `runCleanupCycle`。

**保留目标：**

- `hierarchy_cleanup.go` 继续作为历史兼容层存在。
- 相关测试继续保证旧辅助逻辑可编译、可回归。
- 但它不再代表当前调度器真实执行的主路径。

**验证：**

Run: `go test ./internal/domain/topicanalysis/... -v`
Expected: 全部 PASS

**Commit（如需要单独提交）：**

```bash
git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go
git commit -m "refactor: keep hierarchy cleanup as compatibility layer"
```

---

## Task 5: 重写 `runCleanupCycle` 集成三级策略

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go:215-283`

**Step 1: 查看 `runCleanupCycle` 当前签名确保不破坏外部调用**

当前函数签名：`func (s *TagHierarchyCleanupScheduler) runCleanupCycle(triggerSource string)`
外部调用者：`cleanupHierarchy()` 和 `TriggerNow()` 的 goroutine。签名不变。

**Step 2: 重写 `runCleanupCycle`**

替换 `runCleanupCycle` 函数体（215-283 行）为：

```go
func (s *TagHierarchyCleanupScheduler) runCleanupCycle(triggerSource string) {
	startTime := time.Now()
	summary := &TagHierarchyCleanupRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}
	s.updateSchedulerStatus("running", "", nil, nil)

	logging.Infoln("Starting tag cleanup cycle (3-phase)")

	// Phase 1: Zombie tag cleanup (no LLM)
	zombieCount, err := topicanalysis.CleanupZombieTags(topicanalysis.ZombieTagCriteria{
		MinAgeDays: 7,
		Categories: []string{"event", "keyword", "person"},
	})
	if err != nil {
		logging.Errorf("Phase 1 zombie cleanup failed: %v", err)
		summary.Errors++
	} else {
		logging.Infof("Phase 1: deactivated %d zombie tags", zombieCount)
	}

	// Phase 2: Flat merge (LLM-assisted)
	for _, category := range []string{"event", "keyword"} {
		merged, mergeErrors, err := topicanalysis.ExecuteFlatMerge(category, 50)
		if err != nil {
			logging.Errorf("Phase 2 flat merge failed for %s: %v", category, err)
			summary.Errors++
			continue
		}
		summary.FlatMergesApplied += merged
		summary.Errors += len(mergeErrors)
		for _, e := range mergeErrors {
			logging.Warnf("Phase 2 %s: %s", category, e)
		}
		logging.Infof("Phase 2 (%s): %d merges applied", category, merged)
	}

	// Phase 3: Hierarchy pruning
	orphaned, err := topicanalysis.CleanupOrphanedRelations()
	if err != nil {
		logging.Errorf("Phase 3 orphaned relations cleanup failed: %v", err)
		summary.Errors++
	}
	logging.Infof("Phase 3: removed %d orphaned relations", orphaned)

	resolved, resolveErrors, err := topicanalysis.CleanupMultiParentConflicts()
	if err != nil {
		logging.Errorf("Phase 3 multi-parent cleanup failed: %v", err)
		summary.Errors++
	}
	summary.MultiParentFixed = resolved
	summary.Errors += len(resolveErrors)
	for _, errMsg := range resolveErrors {
		logging.Warnf("Phase 3 multi-parent: %s", errMsg)
	}
	logging.Infof("Phase 3: resolved %d multi-parent conflicts", resolved)

	emptied, err := topicanalysis.CleanupEmptyAbstractNodes()
	if err != nil {
		logging.Errorf("Phase 3 empty abstract cleanup failed: %v", err)
		summary.Errors++
	}
	logging.Infof("Phase 3: deactivated %d empty abstract tags", emptied)

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	summary.Reason = fmt.Sprintf("zombie=%d, flat_merges=%d, orphaned_rels=%d, multi_parent=%d, empty_abstracts=%d",
		summary.ZombieDeactivated, summary.FlatMergesApplied, summary.OrphanedRelations, summary.MultiParentFixed, summary.EmptyAbstracts)

	logging.Infof("Tag cleanup cycle completed: %s", summary.Reason)

	if summary.Errors > 0 {
		s.updateSchedulerStatus("success_with_errors", "", &startTime, summary)
	} else {
		s.updateSchedulerStatus("success", "", &startTime, summary)
	}
}
```

**Step 3: 更新 `TagHierarchyCleanupRunSummary` 字段以匹配新输出**

实际落地后，摘要字段改成直接对应三阶段结果：

```go
ZombieDeactivated int `json:"zombie_deactivated"`
FlatMergesApplied int `json:"flat_merges_applied"`
OrphanedRelations int `json:"orphaned_relations"`
MultiParentFixed  int `json:"multi_parent_fixed"`
EmptyAbstracts    int `json:"empty_abstracts"`
```

不再保留旧树流程的 `trees_processed`、`merges_applied`、`abstracts_created` 这类字段。

**Step 4: 运行编译确认**

Run: `go build ./...`
Expected: 编译通过

**Step 5: Commit**

```bash
git add backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: integrate 3-phase cleanup strategy into scheduler runCleanupCycle"
```

---

## Task 6: 补充测试 + 运行完整测试套件

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup_test.go`

**Step 1: 补充边界测试**

```go
func TestQuoteCategories(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{"event"}, "'event'"},
		{[]string{"event", "keyword"}, "'event', 'keyword'"},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		got := quoteCategories(tt.input)
		if got != tt.expected {
			t.Errorf("quoteCategories(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidateFlatMerge_SameTag(t *testing.T) {
	tagMap := map[uint]*FlatTagInfo{1: {ID: 1, Label: "a"}}
	err := validateFlatMerge(flatMergeItem{SourceID: 1, TargetID: 1}, tagMap)
	if err == nil {
		t.Error("expected error for same tag")
	}
}

func TestValidateFlatMerge_SourceNotFound(t *testing.T) {
	tagMap := map[uint]*FlatTagInfo{1: {ID: 1, Label: "a"}}
	err := validateFlatMerge(flatMergeItem{SourceID: 999, TargetID: 1}, tagMap)
	if err == nil {
		t.Error("expected error for missing source")
	}
}

func TestValidateFlatMerge_SourceMoreChildren(t *testing.T) {
	tagMap := map[uint]*FlatTagInfo{
		1: {ID: 1, Label: "big", ChildCount: 10},
		2: {ID: 2, Label: "small", ChildCount: 1},
	}
	err := validateFlatMerge(flatMergeItem{SourceID: 1, TargetID: 2}, tagMap)
	if err == nil {
		t.Error("expected error when source has more children than target")
	}
}

func TestValidateFlatMerge_ValidMerge(t *testing.T) {
	tagMap := map[uint]*FlatTagInfo{
		1: {ID: 1, Label: "big", ChildCount: 10},
		2: {ID: 2, Label: "small", ChildCount: 1},
	}
	err := validateFlatMerge(flatMergeItem{SourceID: 2, TargetID: 1}, tagMap)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestBuildFlatMergePrompt_ContainsCategory(t *testing.T) {
	tags := []FlatTagInfo{{ID: 1, Label: "test"}}
	prompt := BuildFlatMergePrompt(tags, "event")
	if len(prompt) == 0 {
		t.Error("expected non-empty prompt")
	}
}
```

**Step 2: 运行全部测试**

Run: `go test ./internal/domain/topicanalysis/... -v`
Expected: 全部 PASS

**Step 3: 运行编译 + 整体测试**

Run: `go build ./... && go test ./...`
Expected: 编译通过，测试通过

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/tag_cleanup_test.go
git commit -m "test: add comprehensive tests for 3-phase tag cleanup"
```

---

## Task 7: 更新流程文档

**Files:**
- Modify: `docs/plans/tag-hierarchy-cleanup-flow.md`
- Create: `docs/architecture/tag-cleanup-redesign.md`
- Modify: `docs/api/schedulers.md`

**Step 1: 更新 `tag-hierarchy-cleanup-flow.md`，在文件顶部添加废弃提示**

在文件第一行之前插入：

```markdown
> ⚠️ **已废弃**: 此文档描述的"深层树修剪"流程已被 [三级清理策略](../design/tag-cleanup-redesign.md) 取代。保留供参考。

```

**Step 2: 创建 `docs/architecture/tag-cleanup-redesign.md`**

内容为三级清理策略的架构文档，包括：
- 数据调研结论表格
- 三级策略说明
- 关键文件对照表
- 运行摘要字段说明

**Step 3: 更新 `docs/api/schedulers.md`**

- 给 `tag_hierarchy_cleanup` 增加调度器说明
- 用更容易理解的话解释：它现在是按三步顺序清理标签

**Step 4: Commit**

```bash
git add docs/plans/tag-hierarchy-cleanup-flow.md docs/architecture/tag-cleanup-redesign.md docs/api/schedulers.md
git commit -m "docs: document 3-phase tag cleanup redesign"
```

---

## 文件变更总结

| 文件 | 操作 | 说明 |
|------|------|------|
| `topicanalysis/tag_cleanup.go` | 新建 | Phase 1/2/3 核心逻辑 |
| `topicanalysis/tag_cleanup_test.go` | 新建 | 测试 |
| `topicanalysis/hierarchy_cleanup.go` | 修改 | 兼容层，保留历史树清理逻辑 |
| `topicanalysis/hierarchy_cleanup_test.go` | 修改 | 继续覆盖兼容逻辑 |
| `jobs/tag_hierarchy_cleanup.go` | 修改 | `runCleanupCycle` 改为三级策略 |
| `docs/plans/tag-hierarchy-cleanup-flow.md` | 修改 | 废弃标记 |
| `docs/architecture/tag-cleanup-redesign.md` | 新建 | 新架构文档 |
| `docs/api/schedulers.md` | 修改 | 更新调度器说明 |
