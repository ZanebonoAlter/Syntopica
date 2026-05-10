# LLM 批量优化：resolve_multi_parent 队列化 + tag_description_person 简化 + 熵增控制

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将实时路径的 `resolveMultiParentConflict` 单条调用改为队列批量处理，简化 `tag_description_person` 的 prompt 降低单次耗时，并在 `callLLMForTagJudgment` 的 abstract 判断中加入熵增控制。

**Architecture:** 复用项目现有的队列表模式（如 `adopt_narrower_queues`），新建 `multi_parent_resolve_queues` 表；`linkAbstractParentChild` 等调用点改为入队；队列 worker 积累后调用已有的 `batchResolveMultiParentConflicts`。`tag_description_person` 精简 prompt、截断上下文、移除过度重试。`callLLMForTagJudgment` 的 abstract 判断增加 children 必须包含 newLabel 的约束。

**Tech Stack:** Go, GORM, PostgreSQL, 现有 airouter

**预期收益：**
- `resolve_multi_parent`: 125 次/192min → ~10-15 次/15-25min（~85% 耗时减少）
- `tag_description_person`: 30 次/143min → 30 次/~60min（~60% 耗时减少，简化 prompt 后单次更快）
- 熵增控制：减少不必要的抽象标签创建，间接减少所有下游 LLM 调用

---

### Task 1: 新建 `multi_parent_resolve_queues` 数据库迁移

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`
- Create: `backend-go/internal/domain/models/multi_parent_resolve_queue.go`

**Step 1: 创建 model 文件**

```go
// backend-go/internal/domain/models/multi_parent_resolve_queue.go
package models

import "time"

const (
	MultiParentResolveStatusPending    = "pending"
	MultiParentResolveStatusProcessing = "processing"
	MultiParentResolveStatusCompleted  = "completed"
	MultiParentResolveStatusFailed     = "failed"
)

type MultiParentResolveQueue struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	ChildTagID  uint       `gorm:"not null;index" json:"child_tag_id"`
	Source      string     `gorm:"size:50;not null" json:"source"`
	Status      string     `gorm:"size:20;not null;default:pending;index" json:"status"`
	ErrorMessage string    `gorm:"type:text" json:"error_message"`
	RetryCount  int        `gorm:"default:0" json:"retry_count"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

func (MultiParentResolveQueue) TableName() string {
	return "multi_parent_resolve_queues"
}
```

**Step 2: 添加迁移**

在 `postgres_migrations.go` 的 `postgresMigrations()` 返回数组末尾（`20260426_0001` 之后）追加：

```go
{
	Version:     "20260427_0001",
	Description: "Create multi_parent_resolve_queues table with partial unique index for dedup.",
	Up: func(db *gorm.DB) error {
		if err := db.Exec(`CREATE TABLE IF NOT EXISTS multi_parent_resolve_queues (
			id SERIAL PRIMARY KEY,
			child_tag_id INTEGER NOT NULL,
			source VARCHAR(50) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'pending',
			error_message TEXT,
			retry_count INTEGER DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			started_at TIMESTAMP,
			completed_at TIMESTAMP,
			CONSTRAINT fk_mprq_child_tag FOREIGN KEY (child_tag_id) REFERENCES topic_tags(id) ON DELETE CASCADE
		)`).Error; err != nil {
			return fmt.Errorf("create multi_parent_resolve_queues table: %w", err)
		}
		if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_mprq_active ON multi_parent_resolve_queues (child_tag_id) WHERE status IN ('pending', 'processing')`).Error; err != nil {
			return fmt.Errorf("create mprq active unique index: %w", err)
		}
		return nil
	},
},
```

**Step 3: 注册到 AutoMigrate**

在 `backend-go/internal/platform/database/migrator.go` 的 `autoMigrateModels` 中追加 `&models.MultiParentResolveQueue{}`。

**Step 4: 启动验证**

```bash
cd backend-go && go build ./...
```

Expected: 编译通过

**Step 5: Commit**

```bash
git add backend-go/internal/domain/models/multi_parent_resolve_queue.go backend-go/internal/platform/database/postgres_migrations.go backend-go/internal/platform/database/migrator.go
git commit -m "feat: add multi_parent_resolve_queues table and model"
```

---

### Task 2: 实现 `EnqueueMultiParentResolve` 和队列 worker

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/multi_parent_resolve_queue.go`

**Step 1: 编写入队和 worker 逻辑**

复用 `adopt_narrower_queue.go` 的模式。关键功能：
- `EnqueueMultiParentResolve(childTagID uint, source string)` — 去重入队
- `ProcessPendingMultiParentResolveTasks() (int, error)` — 批量处理所有 pending 任务
  - 收集 pending 任务（FOR UPDATE 锁定）
  - 加载每个 child 的多父冲突信息（复用 `resolveMultiParentConflict` 中加载 relations 的逻辑）
  - 过滤出真正有多父冲突的 child（len(relations) > 1）
  - 调用已有的 `batchResolveMultiParentConflicts(conflicts)`
  - 更新任务状态

```go
// multi_parent_resolve_queue.go
package topicanalysis

import (
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func EnqueueMultiParentResolve(childTagID uint, source string) {
	if childTagID == 0 || database.DB == nil {
		return
	}
	var activeCount int64
	database.DB.Model(&models.MultiParentResolveQueue{}).
		Where("child_tag_id = ? AND status IN ?", childTagID, []string{
			models.MultiParentResolveStatusPending,
			models.MultiParentResolveStatusProcessing,
		}).Count(&activeCount)
	if activeCount > 0 {
		return
	}
	task := models.MultiParentResolveQueue{
		ChildTagID: childTagID,
		Source:     source,
		Status:     models.MultiParentResolveStatusPending,
	}
	if err := database.DB.Create(&task).Error; err != nil {
		logging.Warnf("EnqueueMultiParentResolve: failed for child %d: %v", childTagID, err)
	}
}

func ProcessPendingMultiParentResolveTasks() (int, error) {
	var tasks []models.MultiParentResolveQueue
	if err := database.DB.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("status = ?", models.MultiParentResolveStatusPending).
		Order("id ASC").
		Limit(50).
		Find(&tasks).Error; err != nil || len(tasks) == 0 {
		return 0, err
	}

	now := time.Now()
	for i := range tasks {
		tasks[i].Status = models.MultiParentResolveStatusProcessing
		tasks[i].StartedAt = &now
		database.DB.Save(&tasks[i])
	}

	var conflicts []multiParentConflict
	var completedIDs []uint
	for _, task := range tasks {
		var relations []models.TopicTagRelation
		if err := database.DB.
			Where("child_id = ? AND relation_type = ?", task.ChildTagID, "abstract").
			Preload("Parent").
			Find(&relations).Error; err != nil {
			markMPTaskFailed(task.ID, fmt.Sprintf("load relations: %v", err))
			continue
		}
		if len(relations) <= 1 {
			completedIDs = append(completedIDs, task.ID)
			continue
		}
		var childTag models.TopicTag
		if err := database.DB.First(&childTag, task.ChildTagID).Error; err != nil {
			markMPTaskFailed(task.ID, fmt.Sprintf("load child: %v", err))
			continue
		}
		var parents []parentWithInfo
		for _, r := range relations {
			if r.Parent != nil {
				parents = append(parents, parentWithInfo{RelationID: r.ID, Parent: r.Parent})
			}
		}
		if len(parents) <= 1 {
			completedIDs = append(completedIDs, task.ID)
			continue
		}
		conflicts = append(conflicts, multiParentConflict{
			ChildID: task.ChildTagID,
			Parents: parents,
			Child:   &childTag,
		})
	}

	if len(conflicts) > 0 {
		resolved, errs := batchResolveMultiParentConflicts(conflicts)
		logging.Infof("ProcessPendingMultiParentResolveTasks: batch resolved %d/%d conflicts, %d errors",
			resolved, len(conflicts), len(errs))
	}

	for _, id := range completedIDs {
		database.DB.Model(&models.MultiParentResolveQueue{}).Where("id = ?", id).
			Updates(map[string]any{"status": models.MultiParentResolveStatusCompleted, "completed_at": time.Now()})
	}

	for _, c := range conflicts {
		database.DB.Model(&models.MultiParentResolveQueue{}).
			Where("child_tag_id = ? AND status = ?", c.ChildID, models.MultiParentResolveStatusProcessing).
			Updates(map[string]any{"status": models.MultiParentResolveStatusCompleted, "completed_at": time.Now()})
	}

	return len(conflicts), nil
}

func markMPTaskFailed(id uint, errMsg string) {
	database.DB.Model(&models.MultiParentResolveQueue{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":        models.MultiParentResolveStatusFailed,
			"error_message": errMsg,
		})
}
```

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/multi_parent_resolve_queue.go
git commit -m "feat: add EnqueueMultiParentResolve and batch queue worker"
```

---

### Task 3: 替换实时调用为入队

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_hierarchy.go` (line 289-291)
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` (line 335-338)
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go` (line 1132-1141)

**Step 1: 替换 `linkAbstractParentChild` 中的调用**

将 `abstract_tag_hierarchy.go:289-291`：
```go
go func(id uint) {
    _, _ = resolveMultiParentConflict(id)
}(childID)
```
改为：
```go
go EnqueueMultiParentResolve(childID, "linkAbstractParentChild")
```

**Step 2: 替换 `processAbstractJudgment` 中的调用**

将 `abstract_tag_service.go:335-338`：
```go
for _, child := range abstractChildren {
    go func(childID uint) {
        _, _ = resolveMultiParentConflict(childID)
    }(child.ID)
}
```
改为：
```go
for _, child := range abstractChildren {
    EnqueueMultiParentResolve(child.ID, "processAbstractJudgment")
}
```

**Step 3: 替换 `hierarchy_cleanup.go` 中的调用**

将 `hierarchy_cleanup.go:1132-1141`：
```go
for _, child := range abstractChildren {
    go func(childID uint) {
        defer func() {
            if r := recover(); r != nil {
                logging.Warnf("Hierarchy cleanup: multi-parent conflict task panic for child tag %d: %v", childID, r)
            }
        }()
        _, _ = resolveMultiParentConflict(childID)
    }(child.ID)
}
```
改为：
```go
for _, child := range abstractChildren {
    EnqueueMultiParentResolve(child.ID, "createAbstractTagDirectly")
}
```

**Step 4: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_hierarchy.go backend-go/internal/domain/topicanalysis/abstract_tag_service.go backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go
git commit -m "refactor: replace inline resolveMultiParentConflict with queue enqueue"
```

---

### Task 4: 将队列 worker 集成到调度器和实时 worker

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/queue_batch_processor.go`（如已有实时 worker）

**Step 1: 在 Phase 3 层级修剪中使用队列**

在 `tag_hierarchy_cleanup.go` 的 Phase 3（层级修剪）执行完 `CleanupMultiParentConflicts` 后，追加调用 `ProcessPendingMultiParentResolveTasks()` 处理所有入队的冲突。

**Step 0: 在 `TagHierarchyCleanupRunSummary` 中新增字段**

在 `backend-go/internal/jobs/tag_hierarchy_cleanup.go` 的 `TagHierarchyCleanupRunSummary` 结构体中，在 `MultiParentFixed` 字段后追加：
```go
	QueuedMultiParentResolved int `json:"queued_multi_parent_resolved"`
```

**Step 1: 在 Run 方法中 Phase 3 后面增加队列处理**

在 Phase 3 的 `CleanupEmptyAbstractNodes` 之后、Phase 4 之前，插入：
```go
	// Phase 3.5: Process queued multi-parent resolves (from real-time enqueue)
	if budget.IsTimedOut() {
		logging.Infoln("Phase 3.5: budget timed out, skipping queued multi-parent resolve")
	} else {
		resolved, err := topicanalysis.ProcessPendingMultiParentResolveTasks()
		if err != nil {
			logging.Warnf("Phase 3.5 multi-parent resolve: %v", err)
			summary.Errors++
		} else {
			summary.QueuedMultiParentResolved = resolved
			if resolved > 0 {
				logging.Infof("Phase 3.5: resolved %d queued multi-parent conflicts", resolved)
			}
		}
	}
```

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: integrate multi-parent resolve queue into scheduler phase 3.5"
```

---

### Task 5: 简化 `tag_description_person` prompt 和重试逻辑

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` (line 706-830)

**Step 1: 简化 prompt 和降低重试**

修改 `generatePersonTagDescription` 函数：

1. **截断上下文**：400 字符 → 200 字符（只需识别人物身份，不需要长文）
2. **简化 prompt**：移除结构化属性提取（country/organization/role/domains），改为只生成简洁 description（和 keyword/event 标签一致的风格）。结构化属性由 `BackfillPersonMetadata` 异步补充，不在创建时阻塞。
3. **降低重试**：maxRetries 从 4 降到 2
4. **移除 metadata 写入**：只更新 description，不再同时写入 metadata

新的 `generatePersonTagDescription` 函数体：

```go
func generatePersonTagDescription(tagID uint, label, articleContext string) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("generatePersonTagDescription panic for tag %d: %v", tagID, r)
		}
	}()

	runes := []rune(articleContext)
	if len(runes) > 200 {
		articleContext = string(runes[:200])
	}

	router := airouter.NewRouter()

	prompt := fmt.Sprintf(`为这个人物标签生成简短描述（中文，1-2句话，200字以内）。
标签: %q
上下文: %s

要求：客观说明此人身份、职务或所属机构，不要描述具体行为。
返回 JSON: {"description": "描述"}`, label, articleContext)

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是一个标签分类助手，只输出合法JSON。"},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"description": {Type: "string", Description: "人物标签的中文简短描述"},
			},
			Required: []string{"description"},
		},
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation": "tag_description_person",
			"tag_id":    tagID,
			"tag_label": label,
		},
	}

	const maxRetries = 2
	var desc string
	var success bool

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := router.Chat(context.Background(), req)
		if err != nil {
			logging.Warnf("Person description LLM call failed for tag %d (attempt %d/%d): %v", tagID, attempt, maxRetries, err)
			continue
		}

		var parsed struct {
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil || parsed.Description == "" {
			logging.Warnf("Failed to parse person description for tag %d (attempt %d/%d)", tagID, attempt, maxRetries)
			continue
		}

		desc = parsed.Description
		success = true
		break
	}

	if !success {
		logging.Warnf("Failed to generate person description for tag %d after %d attempts", tagID, maxRetries)
		return
	}

	if len([]rune(desc)) > 200 {
		desc = string([]rune(desc)[:200])
	}

	if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tagID).Update("description", desc).Error; err != nil {
		logging.Warnf("Failed to save description for person tag %d: %v", tagID, err)
		return
	}

	qs := getEmbeddingQueueService()
	if err := qs.Enqueue(tagID); err != nil {
		logging.Warnf("Failed to enqueue re-embedding after person description update for tag %d: %v", tagID, err)
	}
}
```

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicextraction/tagger.go
git commit -m "perf: simplify person description prompt, reduce retries from 4 to 2, drop inline metadata extraction"
```

---

### Task 6: 熵增控制 — abstract 判断必须包含 newLabel

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_judgment.go` (prompt 和校验)
- Modify: `backend-go/internal/domain/topicextraction/tagger.go`（如 batch 路径也需要校验）

**Step 1: 在 prompt 中增加约束**

在 `buildTagJudgmentPrompt`（`abstract_tag_judgment.go:464` 附近）的 ABSTRACTS 规则段落追加：

```
- CRITICAL: Each abstract group's "children" array MUST contain the new tag "%s" (the tag being classified).
  If the new tag doesn't belong in any abstract group with the candidates, do not create an abstract.
```

在 `abstract_tag_judgment.go:472` 附近，追加到 abstracts 规则块中（在 `- Each candidate should appear in at most ONE abstract group` 之后）：

```go
fmt.Sprintf("- CRITICAL: Each abstract group's \"children\" array MUST contain the new tag \"%s\". If the new tag does not belong in any group with the candidates, put all candidates in none.", newLabel)
```

**Step 2: 在代码校验中增加保护**

在 `parseTagJudgmentResponse` 的返回前（或在 `ensureNewLabelCandidateInAbstractJudgment` 已有逻辑中），增加校验：如果 abstract 的 children 不包含 newLabel 对应的 candidate label，将该 abstract 的所有 children 移到 none，并记录 warning。

查看 `ensureNewLabelCandidateInAbstractJudgment`（`abstract_tag_judgment.go:80`）确认它是否已处理这个逻辑。如果已处理，只需确保 prompt 约束更强即可。如果未完全覆盖，补充：

在 `ensureNewLabelCandidateInAbstractJudgment` 末尾，对每个 abstract 检查 children 是否包含 newLabel。不包含的，将其 children 全部移入 `judgment.None` 并从 `judgment.Abstracts` 中移除。

**Step 3: 对 `batch_tag_judgment.go` 的 batch prompt 做同样约束**

在 `buildBatchTagJudgmentPrompt` 中，对每个 tag 的 abstract 规则加上同样的约束：children 必须包含该 new tag label。

**Step 4: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 5: 运行相关测试**

```bash
cd backend-go && go test ./internal/domain/topicanalysis/... -v -run "TestCleanupMultiParent\|TestBatchJudge\|TestTagJudgment\|TestAbstract"
```

**Step 6: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_judgment.go backend-go/internal/domain/topicanalysis/batch_tag_judgment.go
git commit -m "feat: add entropy control — abstract children must include newLabel"
```

---

### Task 7: 更新文档

**Files:**
- Modify: `docs/guides/tagging-flow.md`

**Step 1: 更新 tagging-flow.md**

在第 7 节"父子链接与多父冲突"中，补充说明实时路径已改为入队：
- `linkAbstractParentChild` 现在调用 `EnqueueMultiParentResolve` 入队而非直接调用
- 队列在 Phase 3.5 批量处理

在第 3 节或第 1 节中补充说明：
- person 标签 description 简化为异步生成，不再提取结构化属性
- abstract 判断增加熵增控制：children 必须包含 newLabel

**Step 2: Commit**

```bash
git add docs/guides/tagging-flow.md
git commit -m "docs: update tagging-flow with queue-based multi-parent resolve and entropy control"
```

---

### Task 8: 全量编译和测试验证

**Step 1: 后端全量编译**

```bash
cd backend-go && go build ./...
```

**Step 2: 运行 topicanalysis 包测试**

```bash
cd backend-go && go test ./internal/domain/topicanalysis/... -v
```

**Step 3: 运行 topicextraction 包测试**

```bash
cd backend-go && go test ./internal/domain/topicextraction/... -v
```

**Step 4: 运行 jobs 包测试**

```bash
cd backend-go && go test ./internal/jobs/... -v
```

**Step 5: 启动后端确认迁移正常运行**

```bash
cd backend-go && go run cmd/server/main.go
```

观察日志确认：
- `multi_parent_resolve_queues` 表创建成功
- 无 panic 或报错
