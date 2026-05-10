# Tag Cleanup LLM Budget & Timeout 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 `TagHierarchyCleanupScheduler` 增加 LLM 调用预算控制、执行超时机制和 Phase 4/5 跨 task 批量合并，并将关键指标暴露到前端 scheduler 面板。

**Architecture:** 在 `runCleanupCycle` 层面引入 `CleanupBudget` 控制 LLM 调用次数和总超时。Phase 4 从逐 task 调 LLM 改为跨 task 批量 prompt（一次 LLM 调用处理多个 abstract tag 的 narrower candidates）。Phase 5 从逐 task 调 LLM 改为跨 task 批量 label/description 生成。Phase 6 加 per-category 上限。所有 LLM 调用走 `budget.consume()` 统一计数，超预算时优雅中断。

**Tech Stack:** Go, Gin, GORM, PostgreSQL, airouter (LLM router), Vue 3 + TypeScript (frontend)

---

## 当前 LLM 调用链路

| Phase | 当前模式 | 最坏次数 | 优化后 |
|-------|---------|---------|--------|
| 1 Zombie | 无 LLM | 0 | 0 |
| 2 Flat Merge | 每 category 1 次批量 | 2 | 2 |
| 3 Hierarchy Pruning | 无 LLM | 0 | 0 |
| 4 Adopt Narrower | 逐 task，每 task 1 次 LLM | **50** | ≤ 10 (跨 task 合并) |
| 5 Abstract Update | 逐 task，每 task 1 次 LLM | **50** | ≤ 10 (跨 task 合并) |
| 6 Tree Review | 大树逐棵+小树批审，无上限 | **∞** | ≤ 30 (per-cat 10) |
| 7 Desc Backfill | 已批量 (10/batch) | 5 | 5 |
| **合计** | | **107+** | **≤ 57** |

---

### Task 1: 新增 `CleanupBudget` 结构体

**Files:**
- Create: `backend-go/internal/jobs/cleanup_budget.go`
- Test: `backend-go/internal/jobs/cleanup_budget_test.go`

**Step 1: 编写 budget 测试**

```go
package jobs

import (
	"testing"
	"time"
)

func TestCleanupBudget_Consume(t *testing.T) {
	b := NewCleanupBudget(5, 30*time.Minute)
	for i := 0; i < 5; i++ {
		if !b.Consume() {
			t.Fatalf("consume %d should succeed", i+1)
		}
	}
	if b.Consume() {
		t.Fatal("6th consume should fail")
	}
}

func TestCleanupBudget_ConsumeForPhase(t *testing.T) {
	b := NewCleanupBudget(100, 30*time.Minute)
	b.SetPhaseQuota("phase4", 3)
	b.SetPhaseQuota("phase5", 3)
	b.SetPhaseQuota("phase6", 10)

	for i := 0; i < 3; i++ {
		if !b.ConsumeForPhase("phase4") {
			t.Fatalf("phase4 consume %d should succeed", i+1)
		}
	}
	if b.ConsumeForPhase("phase4") {
		t.Fatal("phase4 4th consume should fail (quota=3)")
	}

	if !b.ConsumeForPhase("phase5") {
		t.Fatal("phase5 should still succeed (different quota)")
	}
}

func TestCleanupBudget_Timeout(t *testing.T) {
	b := NewCleanupBudget(100, 50*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	if !b.IsTimedOut() {
		t.Fatal("should be timed out")
	}
	if b.Consume() {
		t.Fatal("consume after timeout should fail")
	}
}

func TestCleanupBudget_Stats(t *testing.T) {
	b := NewCleanupBudget(100, 30*time.Minute)
	b.SetPhaseQuota("phase6", 2)
	b.ConsumeForPhase("phase4")
	b.ConsumeForPhase("phase4")
	b.ConsumeForPhase("phase6")

	stats := b.Stats()
	if stats.TotalConsumed != 3 {
		t.Fatalf("expected 3 consumed, got %d", stats.TotalConsumed)
	}
	if stats.PhaseConsumed["phase6"] != 1 {
		t.Fatalf("expected phase6=1, got %d", stats.PhaseConsumed["phase6"])
	}
}
```

**Step 2: 运行测试确认失败**

```bash
cd backend-go && go test ./internal/jobs -run TestCleanupBudget -v
```

Expected: FAIL (NewCleanupBudget undefined)

**Step 3: 实现 `CleanupBudget`**

```go
package jobs

import (
	"sync"
	"sync/atomic"
	"time"
)

type CleanupBudgetStats struct {
	TotalConsumed int              `json:"total_consumed"`
	TotalBudget   int              `json:"total_budget"`
	PhaseConsumed map[string]int   `json:"phase_consumed"`
	PhaseBudget   map[string]int   `json:"phase_budget"`
	TimedOut      bool             `json:"timed_out"`
}

type CleanupBudget struct {
	totalBudget  atomic.Int32
	consumed     atomic.Int32
	deadline     time.Time
	mu           sync.Mutex
	phaseQuota   map[string]int
	phaseUsed    map[string]int
	timedOut     atomic.Bool
}

func NewCleanupBudget(totalBudget int, timeout time.Duration) *CleanupBudget {
	b := &CleanupBudget{
		deadline:   time.Now().Add(timeout),
		phaseQuota: make(map[string]int),
		phaseUsed:  make(map[string]int),
	}
	b.totalBudget.Store(int32(totalBudget))
	return b
}

func (b *CleanupBudget) SetPhaseQuota(phase string, quota int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.phaseQuota[phase] = quota
}

func (b *CleanupBudget) Consume() bool {
	if b.checkTimeout() {
		return false
	}
	for {
		current := b.consumed.Load()
		if current >= b.totalBudget.Load() {
			return false
		}
		if b.consumed.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (b *CleanupBudget) ConsumeForPhase(phase string) bool {
	if !b.Consume() {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.phaseUsed[phase]++
	return true
}

func (b *CleanupBudget) IsTimedOut() bool {
	return b.checkTimeout()
}

func (b *CleanupBudget) Stats() CleanupBudgetStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	phaseConsumed := make(map[string]int)
	for k, v := range b.phaseUsed {
		phaseConsumed[k] = v
	}
	phaseBudget := make(map[string]int)
	for k, v := range b.phaseQuota {
		phaseBudget[k] = v
	}
	return CleanupBudgetStats{
		TotalConsumed: int(b.consumed.Load()),
		TotalBudget:   int(b.totalBudget.Load()),
		PhaseConsumed: phaseConsumed,
		PhaseBudget:   phaseBudget,
		TimedOut:      b.timedOut.Load(),
	}
}

func (b *CleanupBudget) checkTimeout() bool {
	if b.timedOut.Load() {
		return true
	}
	if time.Now().After(b.deadline) {
		b.timedOut.Store(true)
		return true
	}
	return false
}
```

**Step 4: 运行测试确认通过**

```bash
cd backend-go && go test ./internal/jobs -run TestCleanupBudget -v
```

Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/jobs/cleanup_budget.go backend-go/internal/jobs/cleanup_budget_test.go
git commit -m "feat(tag-cleanup): add CleanupBudget for LLM call budgeting and timeout"
```

---

### Task 2: Phase 4 — 跨 task 批量 narrower 判断

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/queue_batch_processor.go`
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_judgment.go`
- Test: `backend-go/internal/domain/topicanalysis/queue_batch_processor_test.go`

**Step 1: 编写批量 adopt narrower 测试**

在 `queue_batch_processor_test.go` 中：

```go
package topicanalysis

import (
	"testing"
)

func TestBatchAdoptNarrowerBatching(t *testing.T) {
	tasks := []adoptTaskWithCandidates{
		{AbstractTagID: 1, Label: "AI", Candidates: []string{"ML", "DL"}},
		{AbstractTagID: 2, Label: "Cloud", Candidates: []string{"AWS", "GCP"}},
		{AbstractTagID: 3, Label: "Security", Candidates: []string{}},
	}

	batches := groupAdoptTasksByCategory(tasks, 2)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	// batch 1: tag 1 + tag 2 (has candidates)
	// batch 2: tag 3 has no candidates, skipped
	totalTasks := 0
	for _, b := range batches {
		totalTasks += len(b)
	}
	if totalTasks != 2 {
		t.Fatalf("expected 2 tasks with candidates in batches, got %d", totalTasks)
	}
}
```

**Step 2: 运行测试确认失败**

```bash
cd backend-go && go test ./internal/domain/topicanalysis -run TestBatchAdoptNarrower -v
```

**Step 3: 实现批量 adopt narrower**

在 `queue_batch_processor.go` 中新增：

```go
type adoptTaskWithCandidates struct {
	AbstractTagID uint
	Label         string
	Category      string
	Candidates    []TagCandidate
	TaskModel     models.AdoptNarrowerQueue
}

func groupAdoptTasksByCategory(tasks []adoptTaskWithCandidates, batchSize int) [][]adoptTaskWithCandidates {
	var withCandidates []adoptTaskWithCandidates
	for _, t := range tasks {
		if len(t.Candidates) > 0 {
			withCandidates = append(withCandidates, t)
		}
	}
	var batches [][]adoptTaskWithCandidates
	for i := 0; i < len(withCandidates); i += batchSize {
		end := i + batchSize
		if end > len(withCandidates) {
			end = len(withCandidates)
		}
		batches = append(batches, withCandidates[i:end])
	}
	return batches
}

type batchAdoptJudgment struct {
	Results []struct {
		AbstractTagID uint   `json:"abstract_tag_id"`
		NarrowerIDs   []uint `json:"narrower_ids"`
	} `json:"results"`
}

func batchJudgeAdoptNarrower(ctx context.Context, batch []adoptTaskWithCandidates) (*batchAdoptJudgment, error) {
	if len(batch) == 0 {
		return nil, nil
	}

	var entries []string
	for i, t := range batch {
		var candParts []string
		for _, c := range t.Candidates {
			candParts = append(candParts, fmt.Sprintf("%q (相似度: %.4f)", c.Tag.Label, c.Similarity))
		}
		entries = append(entries, fmt.Sprintf("%d. 抽象标签 %q (ID:%d): 候选 [%s]",
			i+1, t.Label, t.AbstractTagID, strings.Join(candParts, ", ")))
	}

	prompt := fmt.Sprintf(`判断以下多个抽象标签各自应该收养哪些候选作为更窄概念子标签。

抽象标签及候选:
%s

规则:
- 对每个抽象标签，判断哪些候选是其更窄（更具体）的概念
- 如果候选与抽象标签同级或更宽泛，不选
- 如果候选的子标签与抽象标签的子标签高度重叠，不选
- 可以选择零个、一个或多个

返回 JSON: {"results": [{"abstract_tag_id": ID, "narrower_ids": [候选标签ID列表]}, ...]}`,
		strings.Join(entries, "\n"))

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"results": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"abstract_tag_id": {Type: "integer"},
							"narrower_ids":    {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
						},
						Required: []string{"abstract_tag_id", "narrower_ids"},
					},
				},
			},
			Required: []string{"results"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":    "adopt_narrower_batch",
			"batch_size":   len(batch),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("batch adopt narrower LLM: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment batchAdoptJudgment
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse batch adopt response: %w", err)
	}
	return &judgment, nil
}
```

**Step 4: 重写 `ProcessPendingAdoptNarrowerTasks` 使用批量路径**

```go
func ProcessPendingAdoptNarrowerTasks() (int, error) {
	var tasks []models.AdoptNarrowerQueue
	if err := database.DB.
		Where("status = ?", models.AdoptNarrowerQueueStatusPending).
		Order("created_at ASC").
		Limit(50).
		Find(&tasks).Error; err != nil {
		return 0, err
	}

	if len(tasks) == 0 {
		return 0, nil
	}

	logging.Infof("adopt narrower batch: found %d pending tasks", len(tasks))

	es := NewEmbeddingService()

	var enriched []adoptTaskWithCandidates
	for _, task := range tasks {
		var abstractTag models.TopicTag
		if err := database.DB.First(&abstractTag, task.AbstractTagID).Error; err != nil {
			markAdoptNarrowerFailed(task.ID, err.Error())
			continue
		}

		candidates, err := es.FindSimilarAbstractTags(context.Background(), task.AbstractTagID, abstractTag.Category, 0)
		if err != nil {
			markAdoptNarrowerFailed(task.ID, err.Error())
			continue
		}

		thresholds := es.GetThresholds()
		var eligible []TagCandidate
		for _, c := range candidates {
			if c.Tag != nil && c.Similarity >= thresholds.LowSimilarity {
				eligible = append(eligible, c)
			}
		}

		enriched = append(enriched, adoptTaskWithCandidates{
			AbstractTagID: task.AbstractTagID,
			Label:         abstractTag.Label,
			Category:      abstractTag.Category,
			Candidates:    eligible,
			TaskModel:     task,
		})
	}

	// Fallback: single-candidate tasks use original single LLM path
	// Batch: multi-candidate tasks grouped and sent to batch LLM
	batchSize := 5
	batches := groupAdoptTasksByCategory(enriched, batchSize)

	processed := 0
	for _, batch := range batches {
		judgment, err := batchJudgeAdoptNarrower(context.Background(), batch)
		if err != nil {
			logging.Warnf("adopt narrower batch LLM failed: %v", err)
			for _, t := range batch {
				markAdoptNarrowerFailed(t.TaskModel.ID, err.Error())
			}
			continue
		}

		judgmentMap := make(map[uint][]uint)
		for _, r := range judgment.Results {
			judgmentMap[r.AbstractTagID] = r.NarrowerIDs
		}

		for _, t := range batch {
			narrowerIDs, ok := judgmentMap[t.AbstractTagID]
			if !ok {
				narrowerIDs = []uint{}
			}

			adopted := 0
			for _, cid := range narrowerIDs {
				if err := reparentOrLinkAbstractChild(context.Background(), cid, t.AbstractTagID); err != nil {
					logging.Warnf("adopt narrower batch: failed to link %d under %d: %v", cid, t.AbstractTagID, err)
					continue
				}
				adopted++
			}

			if adopted > 0 {
				EnqueueAbstractTagUpdate(t.AbstractTagID, "adopted_narrower_children")
			}

			now := time.Now()
			database.DB.Model(&models.AdoptNarrowerQueue{}).
				Where("id = ?", t.TaskModel.ID).
				Updates(map[string]interface{}{
					"status":       models.AdoptNarrowerQueueStatusCompleted,
					"completed_at": now,
				})
			processed++
		}
	}

	logging.Infof("adopt narrower batch: processed %d/%d tasks", processed, len(tasks))
	return processed, nil
}
```

**Step 5: 运行测试**

```bash
cd backend-go && go test ./internal/domain/topicanalysis -run TestBatchAdoptNarrower -v
```

**Step 6: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/queue_batch_processor.go
git commit -m "perf(tag-cleanup): batch adopt narrower LLM — 50 calls → ~10 batches"
```

---

### Task 3: Phase 5 — 跨 task 批量 abstract label/description 生成

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go`
- Modify: `backend-go/internal/domain/topicanalysis/queue_batch_processor.go`

**Step 1: 新增批量 label/description 生成函数**

在 `abstract_tag_update_queue.go` 中新增：

```go
type batchLabelDescResult struct {
	Results []struct {
		AbstractTagID uint   `json:"abstract_tag_id"`
		Label         string `json:"label"`
		Description   string `json:"description"`
	} `json:"results"`
}

func batchRegenerateLabelsAndDescriptions(ctx context.Context, entries []abstractTagWithChildren) (*batchLabelDescResult, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	var parts []string
	for i, e := range entries {
		var childParts []string
		for _, c := range e.Children {
			childParts = append(childParts, fmt.Sprintf("- %q", c.Label))
		}
		parts = append(parts, fmt.Sprintf(`%d. 抽象标签 %q (ID:%d, 类别:%s, 当前描述:%s)
子标签:
%s`,
			i+1, e.Tag.Label, e.Tag.ID, e.Tag.Category,
			truncateStr(e.Tag.Description, 100),
			strings.Join(childParts, "\n")))
	}

	prompt := fmt.Sprintf(`为以下多个抽象标签重新生成 label 和 description。

抽象标签列表:
%s

规则:
- label: 概括所有子标签（1-160字）。保持当前 label 如果仍然准确
- description: 中文，1-2 句话，客观说明，500 字以内
- person 类标签说明人物身份
- event 类标签说明事件经过
- keyword 类标签说明概念领域

返回 JSON: {"results": [{"abstract_tag_id": ID, "label": "标签", "description": "描述"}, ...]}`,
		strings.Join(parts, "\n\n"))

	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"results": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"abstract_tag_id": {Type: "integer"},
							"label":           {Type: "string"},
							"description":     {Type: "string"},
						},
						Required: []string{"abstract_tag_id", "label", "description"},
					},
				},
			},
			Required: []string{"results"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":  "abstract_label_desc_batch",
			"batch_size": len(entries),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("batch label/desc LLM: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment batchLabelDescResult
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse batch label/desc response: %w", err)
	}
	return &judgment, nil
}
```

**Step 2: 重写 `ProcessPendingAbstractTagUpdateTasks` 使用批量路径**

在 `queue_batch_processor.go` 中新增辅助类型和重写函数：

```go
type abstractTagWithChildren struct {
	Tag      models.TopicTag
	Children []models.TopicTag
	TaskID   uint
}

func ProcessPendingAbstractTagUpdateTasks() (int, error) {
	var tasks []models.AbstractTagUpdateQueue
	if err := database.DB.
		Where("status = ?", models.AbstractTagUpdateQueueStatusPending).
		Order("created_at ASC").
		Limit(50).
		Find(&tasks).Error; err != nil {
		return 0, err
	}

	if len(tasks) == 0 {
		return 0, nil
	}

	logging.Infof("abstract tag update batch: found %d pending tasks", len(tasks))

	svc := NewAbstractTagUpdateQueueService(nil)

	var entries []abstractTagWithChildren
	for _, task := range tasks {
		var tag models.TopicTag
		if err := database.DB.First(&tag, task.AbstractTagID).Error; err != nil {
			markAbstractTagUpdateFailed(task.ID, err.Error())
			continue
		}

		children, err := svc.loadChildren(task.AbstractTagID)
		if err != nil {
			markAbstractTagUpdateFailed(task.ID, err.Error())
			continue
		}
		if len(children) == 0 {
			now := time.Now()
			database.DB.Model(&models.AbstractTagUpdateQueue{}).
				Where("id = ?", task.ID).
				Updates(map[string]interface{}{
					"status":       models.AbstractTagUpdateQueueStatusCompleted,
					"completed_at": now,
				})
			continue
		}

		entries = append(entries, abstractTagWithChildren{
			Tag:      tag,
			Children: children,
			TaskID:   task.ID,
		})
	}

	// 批量处理，每批最多 5 个 abstract tag
	batchSize := 5
	processed := 0
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		judgment, err := batchRegenerateLabelsAndDescriptions(context.Background(), batch)
		if err != nil {
			logging.Warnf("abstract tag update batch LLM failed: %v", err)
			for _, e := range batch {
				markAbstractTagUpdateFailed(e.TaskID, err.Error())
			}
			continue
		}

		resultMap := make(map[uint]struct {
			Label       string
			Description string
		})
		if judgment != nil {
			for _, r := range judgment.Results {
				resultMap[r.AbstractTagID] = struct {
					Label       string
					Description string
				}{Label: r.Label, Description: r.Description}
			}
		}

		for _, e := range batch {
			r, ok := resultMap[e.Tag.ID]
			if !ok {
				markAbstractTagUpdateFailed(e.TaskID, "no result in batch response")
				continue
			}

			updates := map[string]interface{}{}
			if r.Description != "" && r.Description != e.Tag.Description {
				updates["description"] = r.Description
			}

			if r.Label != "" && r.Label != e.Tag.Label {
				newSlug := topictypes.Slugify(r.Label)
				if newSlug != "" && newSlug != e.Tag.Slug && !isAbstractRoot(database.DB, e.Tag.ID) {
					var conflictCount int64
					database.DB.Model(&models.TopicTag{}).
						Where("slug = ? AND id != ? AND status = ?", newSlug, e.Tag.ID, "active").
						Count(&conflictCount)
					if conflictCount == 0 {
						updates["label"] = r.Label
						updates["slug"] = newSlug
					}
				}
			}

			if len(updates) > 0 {
				if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", e.Tag.ID).Updates(updates).Error; err != nil {
					markAbstractTagUpdateFailed(e.TaskID, err.Error())
					continue
				}
			}

			// 重新生成 embedding
			embSvc := NewEmbeddingService()
			emb, err := embSvc.GenerateEmbedding(context.Background(), &e.Tag, EmbeddingTypeIdentity)
			if err == nil {
				emb.TopicTagID = e.Tag.ID
				embSvc.SaveEmbedding(emb)
			}

			now := time.Now()
			database.DB.Model(&models.AbstractTagUpdateQueue{}).
				Where("id = ?", e.TaskID).
				Updates(map[string]interface{}{
					"status":       models.AbstractTagUpdateQueueStatusCompleted,
					"completed_at": now,
				})
			processed++
		}
	}

	logging.Infof("abstract tag update batch: processed %d/%d tasks", processed, len(tasks))
	return processed, nil
}
```

**Step 3: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go backend-go/internal/domain/topicanalysis/queue_batch_processor.go
git commit -m "perf(tag-cleanup): batch abstract tag update LLM — 50 calls → ~10 batches"
```

---

### Task 4: Phase 6 — 添加 per-category LLM 调用上限

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`

**Step 1: 修改 `ReviewHierarchyTrees` 签名接受 budget**

```go
func ReviewHierarchyTrees(category string, windowDays int, budget *CleanupBudget) (*TreeReviewResult, error) {
```

> 注意: `CleanupBudget` 定义在 `jobs` 包中，`topicanalysis` 包不能直接引用 `jobs`（会造成循环依赖）。
> 因此将 `CleanupBudget` 改为接口：

在 `cleanup_budget.go` 中新增接口定义：

```go
type LLMBudget interface {
	ConsumeForPhase(phase string) bool
	IsTimedOut() bool
}
```

确保 `CleanupBudget` 满足此接口（它已有这两个方法，无需改动）。

**Step 2: 修改 `reviewForestBatched` 和 `reviewOneTree` 使用 budget**

```go
func reviewForestBatched(forest []*TreeNode, category string, result *TreeReviewResult, budget LLMBudget) {
	var smallTrees []*TreeNode
	for _, root := range forest {
		if budget.IsTimedOut() {
			return
		}
		if countNodes(root) <= smallTreeThreshold {
			smallTrees = append(smallTrees, root)
		} else {
			for _, tree := range splitReviewTrees(root, 50) {
				if !budget.ConsumeForPhase("phase6") {
					logging.Warnf("Phase 6: LLM budget exhausted, stopping tree review")
					return
				}
				reviewOneTree(tree, category, result)
			}
		}
	}

	if len(smallTrees) == 0 {
		return
	}

	batch := mergeSmallTreesForReview(smallTrees, 5, 100)
	for _, group := range batch {
		if budget.IsTimedOut() {
			return
		}
		if !budget.ConsumeForPhase("phase6") {
			logging.Warnf("Phase 6: LLM budget exhausted, stopping small tree batch review")
			return
		}
		reviewOneTree(group, category, result)
	}
}
```

**Step 3: 更新 `ReviewHierarchyTrees` 传递 budget**

```go
func ReviewHierarchyTrees(category string, windowDays int, budget LLMBudget) (*TreeReviewResult, error) {
	forest, err := BuildTagForest(category, 2)
	if err != nil {
		return nil, fmt.Errorf("build forest: %w", err)
	}
	if len(forest) == 0 {
		return &TreeReviewResult{}, nil
	}

	forest = filterTreesWithRecentRelations(forest, windowDays)
	if len(forest) == 0 {
		return &TreeReviewResult{}, nil
	}

	result := &TreeReviewResult{}
	reviewForestBatched(forest, category, result, budget)
	return result, nil
}
```

**Step 4: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 5: 运行已有测试**

```bash
cd backend-go && go test ./internal/domain/topicanalysis -v -run TestReview
```

需要更新测试中 `ReviewHierarchyTrees` 的调用签名，传入 `nil` budget（nil 表示无限制，在 `reviewForestBatched` 中加 nil 检查）：

```go
// 在 reviewForestBatched 开头加 nil 检查
func reviewForestBatched(forest []*TreeNode, category string, result *TreeReviewResult, budget LLMBudget) {
	canConsume := func() bool {
		if budget == nil {
			return true
		}
		return budget.ConsumeForPhase("phase6")
	}
	isTimedOut := func() bool {
		if budget == nil {
			return false
		}
		return budget.IsTimedOut()
	}
	// ... 使用 canConsume() 和 isTimedOut() 替代直接调用 budget
```

**Step 6: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/jobs/cleanup_budget.go
git commit -m "feat(tag-cleanup): add per-category LLM call budget for Phase 6 tree review"
```

---

### Task 5: 集成 budget 到 `runCleanupCycle`，加全局超时

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go`

**Step 1: 修改 `runCleanupCycle` 使用 budget**

在 `runCleanupCycle` 开头创建 budget 并在各阶段传递：

```go
func (s *TagHierarchyCleanupScheduler) runCleanupCycle(triggerSource string) {
	startTime := time.Now()
	summary := &TagHierarchyCleanupRunSummary{
		TriggerSource: triggerSource,
		StartedAt:     startTime.Format(time.RFC3339),
	}

	budget := NewCleanupBudget(60, 30*time.Minute)
	budget.SetPhaseQuota("phase6", 10)

	s.updateSchedulerStatus("running", "", nil, nil)
	logging.Infoln("Starting tag cleanup cycle (7-phase) with LLM budget")

	// Phase 1: Zombie (no LLM) — unchanged
	// ...

	// Phase 2: Flat merge — 2 LLM calls
	for _, category := range []string{"event", "keyword"} {
		if budget.IsTimedOut() {
			summary.Errors++
			logging.Warnf("Phase 2: budget timeout, skipping %s", category)
			break
		}
		if !budget.ConsumeForPhase("phase2") {
			break
		}
		merged, mergeErrors, err := topicanalysis.ExecuteFlatMerge(category, 50)
		// ... unchanged
	}

	// Phase 3: Hierarchy pruning (no LLM) — unchanged
	// ...

	// Phase 4: Adopt narrower — budget-controlled (batch LLM already in queue_batch_processor)
	if !budget.IsTimedOut() {
		adopted, err := topicanalysis.ProcessPendingAdoptNarrowerTasks()
		// ... unchanged
	}

	// Phase 5: Abstract update — budget-controlled (batch LLM already in queue_batch_processor)
	if !budget.IsTimedOut() {
		updated, err := topicanalysis.ProcessPendingAbstractTagUpdateTasks()
		// ... unchanged
	}

	// Phase 6: Tree review — per-category budget
	for _, category := range []string{"event", "keyword", "person"} {
		if budget.IsTimedOut() {
			summary.Errors++
			logging.Warnf("Phase 6: budget timeout, skipping %s", category)
			break
		}
		reviewResult, reviewErr := topicanalysis.ReviewHierarchyTrees(category, 14, budget)
		// ... unchanged
	}

	// Phase 7: Description backfill — budget-controlled
	if !budget.IsTimedOut() {
		backfilled, err := topicextraction.BackfillMissingDescriptions()
		// ... unchanged
	}

	// Record budget stats
	budgetStats := budget.Stats()
	summary.LLMCallsTotal = budgetStats.TotalConsumed
	summary.LLMBudgetTotal = budgetStats.TotalBudget
	summary.TimedOut = budgetStats.TimedOut

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	// ... rest unchanged
}
```

**Step 2: 更新 `TagHierarchyCleanupRunSummary` 新增字段**

```go
type TagHierarchyCleanupRunSummary struct {
	// ... existing fields ...
	LLMCallsTotal     int  `json:"llm_calls_total"`
	LLMBudgetTotal    int  `json:"llm_budget_total"`
	TimedOut          bool `json:"timed_out"`
}
```

**Step 3: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 4: 运行全部 jobs 测试**

```bash
cd backend-go && go test ./internal/jobs -v
```

**Step 5: Commit**

```bash
git add backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat(tag-cleanup): integrate LLM budget and 30min timeout into runCleanupCycle"
```

---

### Task 6: 前端 scheduler 面板展示 tag_hierarchy_cleanup 运行详情

**Files:**
- Modify: `front/app/components/dialog/GlobalSettingsDialog.vue`
- Modify: `front/app/utils/schedulerMeta.ts`
- Modify: `front/app/types/scheduler.ts`

**Step 1: 更新 `schedulerMeta.ts` — 添加 tag_hierarchy_cleanup 的展示配置**

```typescript
// 在 getSchedulerDisplayName 中添加:
'tag_hierarchy_cleanup': '标签清理',

// 在 getSchedulerIcon 中添加:
'tag_hierarchy_cleanup': 'mdi:tag-remove-outline',

// 在 getSchedulerColor 中添加:
'tag_hierarchy_cleanup': 'from-violet-500 to-purple-600',
```

**Step 2: 更新 `GlobalSettingsDialog.vue` — 添加 tag_hierarchy_cleanup 的 run summary 展示**

在 scheduler card 模板中，仿照 `auto_refresh` 的 summary 展示模式，为 `tag_hierarchy_cleanup` 新增展示区块：

```vue
<!-- 在 scheduler.database_state 区块后面添加 -->
<div
  v-if="scheduler.name === 'tag_hierarchy_cleanup' && scheduler.last_run_summary"
  class="border-t border-gray-100 p-4 bg-gray-50/50"
>
  <div class="grid grid-cols-4 gap-3 text-center">
    <div>
      <div class="text-xs text-gray-500">僵尸清理</div>
      <div class="mt-1 text-xl font-bold text-gray-900">{{ scheduler.last_run_summary.zombie_deactivated ?? 0 }}</div>
    </div>
    <div>
      <div class="text-xs text-gray-500">平级合并</div>
      <div class="mt-1 text-xl font-bold text-sky-700">{{ scheduler.last_run_summary.flat_merges_applied ?? 0 }}</div>
    </div>
    <div>
      <div class="text-xs text-gray-500">树审查</div>
      <div class="mt-1 text-xl font-bold text-emerald-600">{{ scheduler.last_run_summary.trees_reviewed ?? 0 }}</div>
    </div>
    <div>
      <div class="text-xs text-gray-500">LLM 调用</div>
      <div class="mt-1 text-xl font-bold" :class="scheduler.last_run_summary.timed_out ? 'text-rose-600' : 'text-violet-600'">
        {{ scheduler.last_run_summary.llm_calls_total ?? '-' }}
        <span class="text-xs text-gray-400">/ {{ scheduler.last_run_summary.llm_budget_total ?? '-' }}</span>
      </div>
    </div>
  </div>
  <div v-if="scheduler.last_run_summary.timed_out" class="mt-2 text-xs text-rose-600">
    ⚠ 执行超时，部分阶段未完成
  </div>
</div>
```

**Step 3: 更新 `SchedulerLastRunSummary` 类型添加新字段**

在 `front/app/types/scheduler.ts` 的 `SchedulerLastRunSummary` 中添加：

```typescript
// tag_hierarchy_cleanup fields
zombie_deactivated?: number
flat_merges_applied?: number
orphaned_relations?: number
trees_reviewed?: number
merges_applied?: number
moves_applied?: number
adopt_narrower_processed?: number
abstract_update_processed?: number
description_backfilled?: number
llm_calls_total?: number
llm_budget_total?: number
timed_out?: boolean
```

**Step 4: 前端编译验证**

```bash
cd front && pnpm exec nuxi typecheck
```

**Step 5: Commit**

```bash
git add front/app/components/dialog/GlobalSettingsDialog.vue front/app/utils/schedulerMeta.ts front/app/types/scheduler.ts
git commit -m "feat(ui): show tag cleanup run summary with LLM budget stats in scheduler panel"
```

---

## 实施顺序与依赖

```
Task 1 (CleanupBudget) ──┐
                          ├── Task 5 (集成到 runCleanupCycle)
Task 2 (Phase 4 batch) ──┤
Task 3 (Phase 5 batch) ──┤
Task 4 (Phase 6 budget) ─┘
                          │
                          └── Task 6 (前端展示)
```

Task 1-4 可并行开发，Task 5 依赖 Task 1+4，Task 6 依赖 Task 5。

## 验证清单

- [ ] `go build ./...` 编译通过
- [ ] `go test ./internal/jobs/... -v` 通过
- [ ] `go test ./internal/domain/topicanalysis/... -v` 通过
- [ ] `pnpm exec nuxi typecheck` 前端类型检查通过
- [ ] 手动触发 `tag_hierarchy_cleanup`，观察 summary 中 `llm_calls_total` 和 `timed_out` 字段
- [ ] 前端 GlobalSettingsDialog scheduler 面板中 tag_hierarchy_cleanup 卡片展示正常
