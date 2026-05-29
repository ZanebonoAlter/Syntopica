# 高层级标签跨层清理定时任务 - 实现计划

> **Goal:** 实现一个定时任务，自动扫描深度 ≥5 的标签树，通过 LLM 判断跨层语义重复并合并标签。

> **Architecture:** 在 `backend-go/internal/jobs/` 新增调度器，核心逻辑在 `topicanalysis` 包。每棵树递归分割（每批 ≤50 标签），串行送 LLM，串行执行合并。

> **Tech Stack:** Go, GORM, pgvector, LLM (CapabilityTopicTagging)

---

## Task 1: 创建核心清理逻辑文件

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`
- Test: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go`

**Step 1: 定义数据结构**

```go
// TreeNode 表示标签树中的一个节点
type TreeNode struct {
    Tag          *models.TopicTag
    Depth        int
    Children     []*TreeNode
    Parent       *TreeNode
    ArticleCount int
}

// TagTreeInfo 用于 LLM prompt
type TagTreeInfo struct {
    ID           uint   `json:"id"`
    Label        string `json:"label"`
    Description  string `json:"description"`
    Depth        int    `json:"depth"`
    ArticleCount int    `json:"article_count"`
    ChildrenIDs  []uint `json:"children_ids"`
    ParentID     *uint  `json:"parent_id,omitempty"`
}

// LLMMergeSuggestion LLM 返回的合并建议
type LLMMergeSuggestion struct {
    SourceID   uint    `json:"source_id"`
    TargetID   uint    `json:"target_id"`
    Reason     string  `json:"reason"`
    Confidence float64 `json:"confidence"`
}

// LLMCleanupResponse LLM 返回的完整响应
type LLMCleanupResponse struct {
    Merges []LLMMergeSuggestion `json:"merges"`
    Notes  string               `json:"notes,omitempty"`
}

// CleanupResult 单次清理结果
type CleanupResult struct {
    TreeRootID    uint
    TreeRootLabel string
    TagsProcessed int
    MergesFound   int
    MergesApplied int
    Errors        []string
}
```

**Step 2: 实现树构建函数**

```go
// BuildTagForest 构建所有标签树森林
func BuildTagForest(category string) ([]*TreeNode, error) {
    // 1. 加载所有 abstract 关系
    // 2. 构建 parent->children 映射
    // 3. 找出所有根节点（没有 parent 的节点）
    // 4. 递归构建每棵树
    // 5. 计算每棵树的深度
    // 6. 过滤出 depth >= 5 的树
}

// calculateTreeDepth 计算树的最大深度
func calculateTreeDepth(node *TreeNode) int {
    if len(node.Children) == 0 {
        return 1
    }
    maxChildDepth := 0
    for _, child := range node.Children {
        d := calculateTreeDepth(child)
        if d > maxChildDepth {
            maxChildDepth = d
        }
    }
    return maxChildDepth + 1
}
```

**Step 3: 实现递归分割逻辑**

```go
// ProcessTree 递归处理标签树
func ProcessTree(node *TreeNode) (*CleanupResult, error) {
    // 1. 收集树中所有标签
    tags := collectAllTags(node)
    
    // 2. 如果标签数 <= 50，直接处理
    if len(tags) <= 50 {
        return processBatch(node, tags)
    }
    
    // 3. 否则，对每个一级子树递归处理
    result := &CleanupResult{TreeRootID: node.Tag.ID, TreeRootLabel: node.Tag.Label}
    for _, child := range node.Children {
        childResult, err := ProcessTree(child)
        if err != nil {
            result.Errors = append(result.Errors, err.Error())
            continue
        }
        // 合并结果
        result.TagsProcessed += childResult.TagsProcessed
        result.MergesFound += childResult.MergesFound
        result.MergesApplied += childResult.MergesApplied
        result.Errors = append(result.Errors, childResult.Errors...)
    }
    
    return result, nil
}
```

**Step 4: 实现批量处理逻辑**

```go
// processBatch 处理一批标签（<=50）
func processBatch(root *TreeNode, tags []*TreeNode) (*CleanupResult, error) {
    // 1. 构建 prompt
    prompt := buildCleanupPrompt(root, tags)
    
    // 2. 调用 LLM
    response, err := callCleanupLLM(prompt)
    if err != nil {
        return nil, err
    }
    
    // 3. 执行合并
    result := &CleanupResult{
        TreeRootID:    root.Tag.ID,
        TreeRootLabel: root.Tag.Label,
        TagsProcessed: len(tags),
    }
    
    for _, merge := range response.Merges {
        result.MergesFound++
        
        // 验证合并建议
        if err := validateMergeSuggestion(merge, tags); err != nil {
            result.Errors = append(result.Errors, err.Error())
            continue
        }
        
        // 执行合并
        if err := MergeTags(merge.SourceID, merge.TargetID); err != nil {
            result.Errors = append(result.Errors, err.Error())
            continue
        }
        
        result.MergesApplied++
    }
    
    return result, nil
}
```

**Step 5: 实现 Prompt 构建**

```go
// buildCleanupPrompt 构建 LLM prompt
func buildCleanupPrompt(root *TreeNode, tags []*TreeNode) string {
    // 收集树信息
    treeInfo := map[string]interface{}{
        "root_label":  root.Tag.Label,
        "max_depth":   calculateTreeDepth(root),
        "total_tags":  len(tags),
        "category":    root.Tag.Category,
    }
    
    // 收集标签信息
    var tagInfos []TagTreeInfo
    for _, tag := range tags {
        info := TagTreeInfo{
            ID:           tag.Tag.ID,
            Label:        tag.Tag.Label,
            Description:  tag.Tag.Description,
            Depth:        tag.Depth,
            ArticleCount: tag.ArticleCount,
        }
        for _, child := range tag.Children {
            info.ChildrenIDs = append(info.ChildrenIDs, child.Tag.ID)
        }
        if tag.Parent != nil {
            pid := tag.Parent.Tag.ID
            info.ParentID = &pid
        }
        tagInfos = append(tagInfos, info)
    }
    
    // 构建 JSON prompt
    promptData := map[string]interface{}{
        "tree_info": treeInfo,
        "tags":      tagInfos,
        "rules": []string{
            "只检查非相邻层级的标签对（depth 差 >= 2）",
            "如果两个标签描述的核心概念相同或高度重叠，建议合并",
            "优先保留更上层（depth 更小）的标签",
            "直接父子关系（depth 差 = 1）不要动",
            "返回的每个 merge 必须明确 source_id（被合并的）和 target_id（保留的）",
            "source 和 target 不能是直接父子关系",
        },
    }
    
    promptJSON, _ := json.MarshalIndent(promptData, "", "  ")
    
    return fmt.Sprintf(`你是一位标签分类专家。请分析以下标签树，找出跨层语义重复的标签并建议合并。

%s

请返回以下格式的 JSON：
{
  "merges": [
    {
      "source_id": 123,
      "target_id": 456,
      "reason": "简要说明为什么这两个标签应该合并",
      "confidence": 0.95
    }
  ],
  "notes": "其他观察（可选）"
}

注意：
- merges 数组可以为空
- confidence 范围 0-1
- 只返回真正有把握的建议`, string(promptJSON))
}
```

**Step 6: 实现 LLM 调用**

```go
// callCleanupLLM 调用 LLM 进行清理判断
func callCleanupLLM(prompt string) (*LLMCleanupResponse, error) {
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
                            "source_id":  {Type: "integer"},
                            "target_id":  {Type: "integer"},
                            "reason":     {Type: "string"},
                            "confidence": {Type: "number"},
                        },
                        Required: []string{"source_id", "target_id", "reason"},
                    },
                },
                "notes": {Type: "string"},
            },
        },
        Temperature: func() *float64 { f := 0.2; return &f }(),
        Metadata: map[string]any{
            "operation": "tag_hierarchy_cleanup",
        },
    }
    
    result, err := router.Chat(context.Background(), req)
    if err != nil {
        return nil, fmt.Errorf("LLM call failed: %w", err)
    }
    
    var response LLMCleanupResponse
    if err := json.Unmarshal([]byte(result.Content), &response); err != nil {
        return nil, fmt.Errorf("parse LLM response: %w", err)
    }
    
    return &response, nil
}
```

**Step 7: 实现验证逻辑**

```go
// validateMergeSuggestion 验证合并建议是否合法
func validateMergeSuggestion(merge LLMMergeSuggestion, tags []*TreeNode) error {
    // 查找 source 和 target
    var sourceNode, targetNode *TreeNode
    for _, tag := range tags {
        if tag.Tag.ID == merge.SourceID {
            sourceNode = tag
        }
        if tag.Tag.ID == merge.TargetID {
            targetNode = tag
        }
    }
    
    if sourceNode == nil {
        return fmt.Errorf("source tag %d not found in batch", merge.SourceID)
    }
    if targetNode == nil {
        return fmt.Errorf("target tag %d not found in batch", merge.TargetID)
    }
    
    // 检查是否是同一个标签
    if merge.SourceID == merge.TargetID {
        return fmt.Errorf("source and target are the same tag")
    }
    
    // 检查是否都是 active 状态
    if sourceNode.Tag.Status != "active" || targetNode.Tag.Status != "active" {
        return fmt.Errorf("one or both tags are not active")
    }
    
    // 检查是否是直接父子关系
    if isDirectParentChild(sourceNode, targetNode) {
        return fmt.Errorf("direct parent-child relationship, skipping")
    }
    
    // 检查 depth 差是否 >= 2
    depthDiff := abs(sourceNode.Depth - targetNode.Depth)
    if depthDiff < 2 {
        return fmt.Errorf("depth difference < 2, skipping")
    }
    
    return nil
}

// isDirectParentChild 检查两个节点是否是直接父子关系
func isDirectParentChild(a, b *TreeNode) bool {
    if a.Parent == b || b.Parent == a {
        return true
    }
    return false
}
```

**Step 8: 编写测试**

```go
func TestCalculateTreeDepth(t *testing.T) {
    // 构建测试树
    root := &TreeNode{Tag: &models.TopicTag{ID: 1}, Depth: 1}
    child1 := &TreeNode{Tag: &models.TopicTag{ID: 2}, Depth: 2, Parent: root}
    child2 := &TreeNode{Tag: &models.TopicTag{ID: 3}, Depth: 2, Parent: root}
    grandchild := &TreeNode{Tag: &models.TopicTag{ID: 4}, Depth: 3, Parent: child1}
    
    root.Children = []*TreeNode{child1, child2}
    child1.Children = []*TreeNode{grandchild}
    
    depth := calculateTreeDepth(root)
    if depth != 3 {
        t.Errorf("expected depth 3, got %d", depth)
    }
}

func TestValidateMergeSuggestion(t *testing.T) {
    // 测试直接父子关系应该被拒绝
    parent := &TreeNode{Tag: &models.TopicTag{ID: 1, Status: "active"}, Depth: 1}
    child := &TreeNode{Tag: &models.TopicTag{ID: 2, Status: "active"}, Depth: 2, Parent: parent}
    parent.Children = []*TreeNode{child}
    
    tags := []*TreeNode{parent, child}
    
    merge := LLMMergeSuggestion{SourceID: 2, TargetID: 1}
    err := validateMergeSuggestion(merge, tags)
    if err == nil {
        t.Error("expected error for direct parent-child merge")
    }
}
```

**Run Tests:**
```bash
cd backend-go
go test ./internal/domain/topicanalysis -run TestCalculateTreeDepth -v
go test ./internal/domain/topicanalysis -run TestValidateMergeSuggestion -v
```

**Commit:**
```bash
git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go
git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go
git commit -m "feat: add tag hierarchy cleanup core logic"
```

---

## Task 2: 创建调度器文件

**Files:**
- Create: `backend-go/internal/jobs/tag_hierarchy_cleanup.go`

**Step 1: 定义调度器结构**

参照 `tag_quality_score.go` 的结构：

```go
package jobs

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "github.com/robfig/cron/v3"
    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/domain/topicanalysis"
    "my-robot-backend/internal/platform/database"
    "my-robot-backend/internal/platform/logging"
    "my-robot-backend/internal/platform/tracing"
)

type TagHierarchyCleanupScheduler struct {
    cron           *cron.Cron
    checkInterval  time.Duration
    isRunning      bool
    executionMutex sync.Mutex
    isExecuting    bool
}

type TagHierarchyCleanupRunSummary struct {
    TriggerSource    string `json:"trigger_source"`
    StartedAt        string `json:"started_at"`
    FinishedAt       string `json:"finished_at"`
    TreesProcessed   int    `json:"trees_processed"`
    TagsProcessed    int    `json:"tags_processed"`
    MergesFound      int    `json:"merges_found"`
    MergesApplied    int    `json:"merges_applied"`
    Errors           int    `json:"errors"`
    Reason           string `json:"reason"`
}
```

**Step 2: 实现调度器方法**

```go
func NewTagHierarchyCleanupScheduler(checkInterval int) *TagHierarchyCleanupScheduler {
    return &TagHierarchyCleanupScheduler{
        cron:          cron.New(),
        checkInterval: time.Duration(checkInterval) * time.Second,
    }
}

func (s *TagHierarchyCleanupScheduler) Start() error {
    if s.isRunning {
        return fmt.Errorf("tag-hierarchy-cleanup scheduler already running")
    }

    s.initSchedulerTask()
    scheduleExpr := fmt.Sprintf("@every %ds", int64(s.checkInterval.Seconds()))
    if _, err := s.cron.AddFunc(scheduleExpr, s.cleanupHierarchy); err != nil {
        return fmt.Errorf("failed to schedule tag-hierarchy-cleanup: %w", err)
    }

    s.cron.Start()
    s.isRunning = true
    logging.Infof("Tag-hierarchy-cleanup scheduler started with interval: %v", s.checkInterval)
    return nil
}

func (s *TagHierarchyCleanupScheduler) Stop() {
    if !s.isRunning {
        return
    }
    s.cron.Stop()
    s.isRunning = false
    logging.Infoln("Tag-hierarchy-cleanup scheduler stopped")
}
```

**Step 3: 实现清理循环**

```go
func (s *TagHierarchyCleanupScheduler) cleanupHierarchy() {
    tracing.TraceSchedulerTick("tag_hierarchy_cleanup", "cron", func(ctx context.Context) {
        _ = ctx
        if !s.executionMutex.TryLock() {
            logging.Infoln("Tag hierarchy cleanup already in progress, skipping")
            return
        }
        s.isExecuting = true
        defer func() {
            s.executionMutex.Unlock()
            s.isExecuting = false
            if r := recover(); r != nil {
                logging.Errorf("PANIC in cleanupHierarchy: %v", r)
                s.updateSchedulerStatus("idle", fmt.Sprintf("Panic: %v", r), nil, nil)
            }
        }()

        s.runCleanupCycle("scheduled")
    })
}

func (s *TagHierarchyCleanupScheduler) runCleanupCycle(triggerSource string) {
    startTime := time.Now()
    summary := &TagHierarchyCleanupRunSummary{
        TriggerSource: triggerSource,
        StartedAt:     startTime.Format(time.RFC3339),
    }
    s.updateSchedulerStatus("running", "", nil, nil)

    // 处理 event 和 keyword 两个 category
    categories := []string{"event", "keyword"}
    
    for _, category := range categories {
        logging.Infof("Starting hierarchy cleanup for category: %s", category)
        
        // 构建标签森林
        forest, err := topicanalysis.BuildTagForest(category)
        if err != nil {
            logging.Errorf("Failed to build tag forest for %s: %v", category, err)
            summary.Errors++
            continue
        }
        
        logging.Infof("Found %d trees for category %s", len(forest), category)
        
        // 处理每棵树（串行）
        for _, tree := range forest {
            result, err := topicanalysis.ProcessTree(tree)
            if err != nil {
                logging.Errorf("Failed to process tree rooted at %s: %v", tree.Tag.Label, err)
                summary.Errors++
                continue
            }
            
            summary.TreesProcessed++
            summary.TagsProcessed += result.TagsProcessed
            summary.MergesFound += result.MergesFound
            summary.MergesApplied += result.MergesApplied
            summary.Errors += len(result.Errors)
            
            logging.Infof("Tree %s: processed %d tags, %d merges found, %d applied",
                result.TreeRootLabel, result.TagsProcessed, result.MergesFound, result.MergesApplied)
        }
    }

    summary.FinishedAt = time.Now().Format(time.RFC3339)
    summary.Reason = fmt.Sprintf("processed %d trees, %d tags, applied %d merges",
        summary.TreesProcessed, summary.TagsProcessed, summary.MergesApplied)
    s.updateSchedulerStatus("success", "", &startTime, summary)
}
```

**Step 4: 实现状态管理**

参照 `tag_quality_score.go` 中的 `updateSchedulerStatus` 方法。

**Commit:**
```bash
git add backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: add tag hierarchy cleanup scheduler"
```

---

## Task 3: 注册调度器到运行时

**Files:**
- Modify: `backend-go/internal/app/runtime.go`
- Modify: `backend-go/internal/app/runtimeinfo/schedulers.go`
- Modify: `backend-go/internal/jobs/handler.go`

**Step 1: 添加运行时接口**

在 `runtimeinfo/schedulers.go` 中添加：
```go
var TagHierarchyCleanupSchedulerInterface interface{}
```

**Step 2: 在 runtime.go 中启动调度器**

在 `StartRuntime()` 中添加（参照 TagQualityScore）：
```go
runtime.TagHierarchyCleanup = jobs.NewTagHierarchyCleanupScheduler(86400)
if err := runtime.TagHierarchyCleanup.Start(); err != nil {
    logging.Warnf("Failed to start tag hierarchy cleanup scheduler: %v", err)
} else {
    logging.Infoln("Tag hierarchy cleanup scheduler started successfully")
}
```

在 `SetupGracefulShutdown()` 中添加停止逻辑。

在 Runtime 结构体中添加字段：
```go
TagHierarchyCleanup *jobs.TagHierarchyCleanupScheduler
```

**Step 3: 注册到 handler**

在 `handler.go` 的 `schedulerDescriptors()` 中添加：
```go
{
    Name:        "tag_hierarchy_cleanup",
    DisplayName: "Tag Hierarchy Cleanup",
    Description: "Auto-merge duplicate tags in deep hierarchy trees",
    Get: func() interface{} {
        return runtimeinfo.TagHierarchyCleanupSchedulerInterface
    },
},
```

**Commit:**
```bash
git add backend-go/internal/app/runtime.go
git add backend-go/internal/app/runtimeinfo/schedulers.go
git add backend-go/internal/jobs/handler.go
git commit -m "feat: register tag hierarchy cleanup scheduler"
```

---

## Task 4: 测试和验证

**Step 1: 编译检查**
```bash
cd backend-go
go build ./...
```

**Step 2: 运行单元测试**
```bash
go test ./internal/domain/topicanalysis -v
go test ./internal/jobs -v
```

**Step 3: 启动服务验证**
```bash
go run cmd/server/main.go
```

然后手动触发调度器测试：
```bash
curl -X POST http://localhost:5000/api/schedulers/tag_hierarchy_cleanup/trigger
```

**Step 4: 检查日志输出**
确认日志中有类似输出：
```
Tag-hierarchy-cleanup scheduler started with interval: 24h0m0s
Found X trees for category event
Tree ...: processed Y tags, Z merges found, W applied
```

---

## 设计决策记录

### 1. 为什么递归分割而不是整树截断？

**整树截断**（取 Top 50 标签）会丢失深层节点，导致跨层比较不完整。

**递归分割**保证每棵子树内部完整性，虽然根节点不会和曾孙节点直接比较，但通过逐层向上归并，最终能清理所有层级。

### 2. 为什么串行处理？

用户明确要求"不要并行处理，资源不够的"。串行处理虽然慢，但：
- 不会同时占用多个 LLM quota
- 不会导致数据库并发问题
- 日志更清晰，便于调试

### 3. 为什么 depth >= 5？

用户指定。深层级更容易出现语义漂移和重复。

### 4. 为什么使用 CapabilityTopicTagging？

用户指定"用打 tag 的模型"。这个模型已经熟悉标签语义，无需额外训练。

### 5. 合并后如何处理 abstract 关系？

`MergeTags` 函数内部已经处理了 `topic_tag_relations` 的迁移（见 `migrateTagRelations`），合并后源标签的所有关系会自动转移到目标标签。

---

## 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| LLM 误判导致错误合并 | 要求 LLM 返回 confidence，只执行 confidence >= 0.8 的建议 |
| 大树的递归深度过大 | 树的深度 >= 5，但标签总数可能很大；通过递归分割每批 <= 50 控制 |
| 合并操作失败导致数据不一致 | MergeTags 使用数据库事务，失败自动回滚 |
| 调度器卡死 | 使用 executionMutex 防止并发执行，30 分钟超时机制 |

---

## 后续优化方向

1. **增量处理**：记录每棵树最后清理时间，只处理有变化的树
2. **置信度阈值可调**：将 confidence threshold 做成配置项
3. **批量大小可调**：将每批 50 个做成配置项
4. **前端展示**：在管理后台显示清理历史和统计
