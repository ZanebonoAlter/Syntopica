# Tag Hierarchy Cleanup LLM 批量优化 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 减少 `TagHierarchyCleanupScheduler` 7 阶段定时任务中的 LLM 调用次数，通过批量处理将 N 次调用压缩为 1 次。

**Architecture:** 从最高 ROI 的 Phase 7（description 回填）入手，逐阶段优化。Phase 7 将逐标签 LLM 改为批量 prompt；Phase 6 将多棵小树合并为一次审查；Phase 4/5 去除不必要的 sleep；Phase 3 将逐冲突 LLM 改为批量判断。

**Tech Stack:** Go, Gin, GORM, PostgreSQL, airouter (LLM router)

---

## LLM 调用链路现状

| 阶段 | 文件 | 当前调用模式 | 调用次数 |
|------|------|------------|---------|
| Phase 3 | `tag_cleanup.go:312-343` | `resolveMultiParentConflict` → `aiJudgeBestParent` 逐个 | N |
| Phase 4 | `queue_batch_processor.go:17-57` | `adoptNarrowerAbstractChildren` 逐任务 + 500ms sleep | N×(1+M) |
| Phase 5 | `queue_batch_processor.go:59-103` | `refreshAbstractTag` 逐任务 + 500ms sleep | N×(1+M) |
| Phase 6 | `hierarchy_cleanup.go:211-232` | 每棵树 1 次 `callTreeReviewLLM` | T |
| Phase 7 | `description_backfill.go:16-45` | `generateTagDescription` 逐标签 + 500ms sleep | N |

M = 异步 `MatchAbstractTagHierarchy` 的级联 LLM 调用  
T = 需审查的树数量（3 类别 × 每类别树数）

---

### Task 1: Phase 7 — 批量 description 回填

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/description_backfill.go`
- Modify: `backend-go/internal/domain/topicextraction/tagger.go`（新增批量函数）

**Step 1: 在 tagger.go 新增 `batchGenerateTagDescriptions` 函数**

当前 `generateTagDescription`（`tagger.go:598`）每个标签 1 次 LLM。新增批量版本，将多个标签打包成 1 次 prompt。

```go
// batchGenerateTagDescriptions generates descriptions for multiple tags in a single LLM call.
// Returns a map of tagID -> description.
func batchGenerateTagDescriptions(tags []models.TopicTag) map[uint]string {
    if len(tags) == 0 {
        return nil
    }
    if len(tags) == 1 {
        // 单标签走原有逻辑
        articleContext := buildArticleContextForTag(tags[0].ID)
        if articleContext == "" {
            return nil
        }
        generateTagDescription(tags[0].ID, tags[0].Label, tags[0].Category, articleContext)
        return nil // 原函数已直接写 DB
    }

    // 构建批量上下文
    type tagContext struct {
        ID       uint   `json:"id"`
        Label    string `json:"label"`
        Category string `json:"category"`
        Context  string `json:"context"`
    }
    var items []tagContext
    for _, tag := range tags {
        ctx := buildArticleContextForTag(tag.ID)
        if ctx == "" {
            continue
        }
        items = append(items, tagContext{
            ID:       tag.ID,
            Label:    tag.Label,
            Category: tag.Category,
            Context:  ctx,
        })
    }
    if len(items) == 0 {
        return nil
    }

    itemsJSON, _ := json.MarshalIndent(items, "", "  ")
    prompt := fmt.Sprintf(`为以下标签批量生成 description（中文，每个 1-2 句话，客观事实，500 字以内）。

标签列表：
%s

规则：
- 每个标签的 description 必须解释该标签是什么，不能只重复标签名
- person 类标签说明人物身份
- event 类标签说明事件经过
- keyword 类标签说明概念领域

返回 JSON: {"descriptions": [{"id": 标签ID, "description": "描述内容"}, ...]}`,
        string(itemsJSON))

    router := airouter.NewRouter()
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
                "descriptions": {
                    Type: "array",
                    Items: &airouter.SchemaProperty{
                        Type: "object",
                        Properties: map[string]airouter.SchemaProperty{
                            "id":          {Type: "integer"},
                            "description": {Type: "string"},
                        },
                        Required: []string{"id", "description"},
                    },
                },
            },
            Required: []string{"descriptions"},
        },
        Temperature: func() *float64 { f := 0.3; return &f }(),
        Metadata: map[string]any{
            "operation": "tag_description_batch",
            "count":     len(items),
        },
    }

    result, err := router.Chat(context.Background(), req)
    if err != nil {
        logging.Warnf("batchGenerateTagDescriptions: LLM call failed: %v", err)
        return nil
    }

    content := jsonutil.SanitizeLLMJSON(result.Content)
    var parsed struct {
        Descriptions []struct {
            ID          uint   `json:"id"`
            Description string `json:"description"`
        } `json:"descriptions"`
    }
    if err := json.Unmarshal([]byte(content), &parsed); err != nil {
        logging.Warnf("batchGenerateTagDescriptions: parse failed: %v", err)
        return nil
    }

    results := make(map[uint]string)
    for _, d := range parsed.Descriptions {
        if d.Description != "" && len([]rune(d.Description)) <= 500 {
            results[d.ID] = d.Description
        }
    }
    return results
}
```

**Step 2: 修改 `BackfillMissingDescriptions` 使用批量函数**

将 `description_backfill.go:16-45` 的逐标签循环改为批量调用。

```go
func BackfillMissingDescriptions() (int, error) {
    var tags []models.TopicTag
    if err := database.DB.
        Where("status = ? AND (description IS NULL OR description = '')", "active").
        Limit(50).
        Find(&tags).Error; err != nil {
        return 0, err
    }

    if len(tags) == 0 {
        return 0, nil
    }

    logging.Infof("description backfill: found %d tags without description", len(tags))

    // 批量生成（每批最多 10 个，避免 prompt 过长）
    batchSize := 10
    processed := 0
    for i := 0; i < len(tags); i += batchSize {
        end := i + batchSize
        if end > len(tags) {
            end = len(tags)
        }
        batch := tags[i:end]

        results := batchGenerateTagDescriptions(batch)
        for _, tag := range batch {
            if desc, ok := results[tag.ID]; ok {
                if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).
                    Update("description", desc).Error; err != nil {
                    logging.Warnf("description backfill: failed to update tag %d: %v", tag.ID, err)
                } else {
                    processed++
                }
            }
        }
        time.Sleep(500 * time.Millisecond) // 批次间保持间隔
    }

    logging.Infof("description backfill: updated %d/%d tags", processed, len(tags))
    return processed, nil
}
```

**Step 3: 运行测试验证**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicextraction/description_backfill.go backend-go/internal/domain/topicextraction/tagger.go
git commit -m "perf(tag-cleanup): batch description backfill — 50 LLM calls → 5 batches"
```

---

### Task 2: Phase 6 — 合并小树审查

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`

**Step 1: 新增 `reviewForestBatched` 函数**

当前 `ReviewHierarchyTrees`（`hierarchy_cleanup.go:211`）对每棵树独立调 LLM。将节点数 ≤ 阈值的多棵小树合并为一次审查。

在 `hierarchy_cleanup.go` 中新增：

```go
const smallTreeThreshold = 20 // 节点数 ≤ 此值的小树可合并审查

// reviewForestBatched merges small trees into batched LLM reviews.
// Trees with nodeCount > smallTreeThreshold are reviewed individually.
func reviewForestBatched(forest []*TreeNode, category string, result *TreeReviewResult) {
    var smallTrees []*TreeNode
    for _, root := range forest {
        if countNodes(root) <= smallTreeThreshold {
            smallTrees = append(smallTrees, root)
        } else {
            // 大树仍然逐棵审查（可能拆分）
            for _, tree := range splitReviewTrees(root, 50) {
                reviewOneTree(tree, category, result)
            }
        }
    }

    if len(smallTrees) == 0 {
        return
    }

    // 合并小树为一次审查（最多 5 棵树或 100 个节点）
    batch := mergeSmallTreesForReview(smallTrees, 5, 100)
    for _, group := range batch {
        reviewOneTree(group, category, result)
    }
}

// mergeSmallTreesForReview merges multiple small trees under a virtual root for batched review.
// maxTrees: max trees per batch. maxNodes: max total nodes per batch.
func mergeSmallTreesForReview(trees []*TreeNode, maxTrees, maxNodes int) []*TreeNode {
    var batches []*TreeNode
    var currentBatch []*TreeNode
    currentNodes := 0

    for _, tree := range trees {
        treeNodes := countNodes(tree)
        if len(currentBatch) >= maxTrees || currentNodes+treeNodes > maxNodes {
            if len(currentBatch) > 0 {
                batches = append(batches, createVirtualRoot(currentBatch))
            }
            currentBatch = nil
            currentNodes = 0
        }
        currentBatch = append(currentBatch, tree)
        currentNodes += treeNodes
    }
    if len(currentBatch) > 0 {
        batches = append(batches, createVirtualRoot(currentBatch))
    }
    return batches
}

// createVirtualRoot creates a virtual root node wrapping multiple trees for review.
func createVirtualRoot(trees []*TreeNode) *TreeNode {
    virtualRoot := &TreeNode{
        Tag: &models.TopicTag{
            ID:       0, // 虚拟根节点
            Label:    "[合并审查]",
            Category: trees[0].Tag.Category,
            Source:   "virtual",
        },
        Depth: 0,
    }
    for _, tree := range trees {
        tree.Parent = virtualRoot
        virtualRoot.Children = append(virtualRoot.Children, tree)
    }
    return virtualRoot
}
```

**Step 2: 修改 `ReviewHierarchyTrees` 使用批量审查**

```go
func ReviewHierarchyTrees(category string, windowDays int) (*TreeReviewResult, error) {
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
    reviewForestBatched(forest, category, result)
    return result, nil
}
```

**Step 3: 调整 `reviewOneTree` 支持虚拟根节点**

在 `reviewOneTree` 中处理虚拟根节点（ID=0）的特殊情况：

```go
func reviewOneTree(tree *TreeNode, category string, result *TreeReviewResult) {
    treeStr := serializeTreeForReview(tree)
    prompt := buildTreeReviewPrompt(treeStr, category)

    judgment, err := callTreeReviewLLMFn(prompt)
    if err != nil {
        rootID := uint(0)
        if tree.Tag != nil {
            rootID = tree.Tag.ID
        }
        result.Errors = append(result.Errors, fmt.Sprintf("tree root %d: %v", rootID, err))
        return
    }
    result.TreesReviewed++

    tagMap := make(map[uint]*TreeNode)
    for _, node := range collectAllTags(tree) {
        if node.Tag != nil && node.Tag.ID != 0 { // 跳过虚拟根节点
            tagMap[node.Tag.ID] = node
        }
    }

    // 虚拟根节点时，所有 merge/move 都允许（无 root 保护）
    isVirtual := tree.Tag == nil || tree.Tag.ID == 0

    for _, merge := range judgment.Merges {
        if !isVirtual && merge.SourceID == tree.Tag.ID {
            result.Errors = append(result.Errors, fmt.Sprintf("merge %d->%d: root node cannot be used as merge source", merge.SourceID, merge.TargetID))
            continue
        }
        if err := validateTreeReviewMerge(merge, tagMap); err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("merge %d->%d: %v", merge.SourceID, merge.TargetID, err))
            continue
        }
        if err := MergeTags(merge.SourceID, merge.TargetID); err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("merge %d->%d: %v", merge.SourceID, merge.TargetID, err))
            continue
        }
        delete(tagMap, merge.SourceID)
        result.MergesApplied++
    }

    for _, move := range judgment.Moves {
        if !isVirtual && move.TagID == tree.Tag.ID {
            result.Errors = append(result.Errors, fmt.Sprintf("move %d: review root node cannot be moved", move.TagID))
            continue
        }
        if _, ok := tagMap[move.TagID]; !ok {
            continue
        }
        if err := validateTreeReviewMove(move, tagMap); err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("move %d: %v", move.TagID, err))
            continue
        }
        if err := executeTreeReviewMove(move); err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("move %d: %v", move.TagID, err))
            continue
        }
        result.MovesApplied++
    }

    for _, abs := range judgment.NewAbstracts {
        created, err := validateAndCreateReviewAbstract(abs, tagMap, category)
        if err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("abstract %s: %v", abs.Name, err))
            continue
        }
        if created {
            result.GroupsCreated++
        } else {
            result.GroupsReused++
        }
    }
}
```

**Step 4: 运行测试验证**

```bash
cd backend-go && go build ./...
cd backend-go && go test ./internal/domain/topicanalysis/ -v -run "TestReview|TestBuild"
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go
git commit -m "perf(tag-cleanup): batch small tree reviews in Phase 6 — T LLM calls → fewer batches"
```

---

### Task 3: Phase 4/5 — 去除 sleep，优化处理节奏

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/queue_batch_processor.go`

**Step 1: 移除 `ProcessPendingAdoptNarrowerTasks` 中的 sleep**

当前 `queue_batch_processor.go:52` 有 `time.Sleep(500ms)`。LLM 调用本身已有网络延迟，无需额外 sleep。

```go
// queue_batch_processor.go:34-53
for _, task := range tasks {
    if err := adoptNarrowerAbstractChildren(context.Background(), task.AbstractTagID); err != nil {
        logging.Warnf("adopt narrower batch: failed for tag %d: %v", task.AbstractTagID, err)
        markAdoptNarrowerFailed(task.ID, err.Error())
        continue
    }

    now := time.Now()
    if err := database.DB.Model(&models.AdoptNarrowerQueue{}).
        Where("id = ?", task.ID).
        Updates(map[string]interface{}{
            "status":       models.AdoptNarrowerQueueStatusCompleted,
            "completed_at": now,
        }).Error; err != nil {
        logging.Warnf("adopt narrower batch: failed to mark task %d completed: %v", task.ID, err)
    }

    processed++
    // removed: time.Sleep(500 * time.Millisecond)
}
```

**Step 2: 移除 `ProcessPendingAbstractTagUpdateTasks` 中的 sleep**

```go
// queue_batch_processor.go:80-99
for _, task := range tasks {
    if err := svc.refreshAbstractTag(task.AbstractTagID); err != nil {
        logging.Warnf("abstract tag update batch: failed for tag %d: %v", task.AbstractTagID, err)
        markAbstractTagUpdateFailed(task.ID, err.Error())
        continue
    }

    now := time.Now()
    if err := database.DB.Model(&models.AbstractTagUpdateQueue{}).
        Where("id = ?", task.ID).
        Updates(map[string]interface{}{
            "status":       models.AbstractTagUpdateQueueStatusCompleted,
            "completed_at": now,
        }).Error; err != nil {
        logging.Warnf("abstract tag update batch: failed to mark task %d completed: %v", task.ID, err)
    }

    processed++
    // removed: time.Sleep(500 * time.Millisecond)
}
```

**Step 3: 运行测试验证**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/queue_batch_processor.go
git commit -m "perf(tag-cleanup): remove unnecessary 500ms sleep between Phase 4/5 tasks"
```

---

### Task 4: Phase 3 — 批量多父冲突解决

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/tag_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_hierarchy.go`

**Step 1: 新增 `batchResolveMultiParentConflicts` 函数**

当前 `CleanupMultiParentConflicts`（`tag_cleanup.go:312`）逐个调用 `resolveMultiParentConflict`，后者会逐个调 `aiJudgeBestParent`。将多个冲突打包给 LLM 一次判断。

在 `abstract_tag_hierarchy.go` 中新增：

```go
type multiParentConflict struct {
    ChildID uint
    Parents []parentWithInfo
    Child   *models.TopicTag
}

type batchParentJudgment struct {
    Decisions []parentDecision `json:"decisions"`
}

type parentDecision struct {
    ChildID   uint `json:"child_id"`
    BestIndex int  `json:"best_index"` // 0-based index in parents list
}

// batchResolveMultiParentConflicts resolves multiple multi-parent conflicts in a single LLM call.
func batchResolveMultiParentConflicts(conflicts []multiParentConflict) (int, []string) {
    if len(conflicts) == 0 {
        return 0, nil
    }

    // 构建批量 prompt
    type conflictEntry struct {
        ChildID uint     `json:"child_id"`
        Child   string   `json:"child_label"`
        Parents []string `json:"parent_labels"`
    }
    var entries []conflictEntry
    for _, c := range conflicts {
        var parentLabels []string
        for _, p := range c.Parents {
            parentLabels = append(parentLabels, fmt.Sprintf("%d:%s", p.Parent.ID, p.Parent.Label))
        }
        entries = append(entries, conflictEntry{
            ChildID: c.ChildID,
            Child:   c.Child.Label,
            Parents: parentLabels,
        })
    }

    entriesJSON, _ := json.MarshalIndent(entries, "", "  ")
    prompt := fmt.Sprintf(`以下标签有多个父标签（多父冲突），请为每个子标签选择最合适的父标签。

冲突列表：
%s

规则：
- 选择最具体、最相关的父标签
- 如果子标签与某个父标签有直接从属关系，选该父标签
- 如果子标签是某父标签领域的具体实例，选该父标签

返回 JSON: {"decisions": [{"child_id": ID, "best_index": 父标签在列表中的序号(从0开始)}, ...]}`,
        string(entriesJSON))

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
                "decisions": {
                    Type: "array",
                    Items: &airouter.SchemaProperty{
                        Type: "object",
                        Properties: map[string]airouter.SchemaProperty{
                            "child_id":   {Type: "integer"},
                            "best_index": {Type: "integer"},
                        },
                        Required: []string{"child_id", "best_index"},
                    },
                },
            },
            Required: []string{"decisions"},
        },
        Temperature: func() *float64 { f := 0.2; return &f }(),
        Metadata: map[string]any{
            "operation":     "batch_resolve_multi_parent",
            "conflict_count": len(conflicts),
        },
    }

    result, err := router.Chat(context.Background(), req)
    if err != nil {
        logging.Warnf("batchResolveMultiParentConflicts: LLM call failed: %v", err)
        return 0, nil
    }

    content := jsonutil.SanitizeLLMJSON(result.Content)
    var judgment batchParentJudgment
    if err := json.Unmarshal([]byte(content), &judgment); err != nil {
        logging.Warnf("batchResolveMultiParentConflicts: parse failed: %v", err)
        return 0, nil
    }

    // 构建 childID -> conflict 映射
    conflictMap := make(map[uint]*multiParentConflict)
    for i := range conflicts {
        conflictMap[conflicts[i].ChildID] = &conflicts[i]
    }

    resolved := 0
    var errors []string
    for _, decision := range judgment.Decisions {
        conflict, ok := conflictMap[decision.ChildID]
        if !ok {
            continue
        }
        if decision.BestIndex < 0 || decision.BestIndex >= len(conflict.Parents) {
            errors = append(errors, fmt.Sprintf("child %d: invalid best_index %d", decision.ChildID, decision.BestIndex))
            continue
        }

        // 保留 best_index 对应的父，删除其他
        for i, p := range conflict.Parents {
            if i == decision.BestIndex {
                continue
            }
            if err := database.DB.Delete(&models.TopicTagRelation{}, p.RelationID).Error; err != nil {
                errors = append(errors, fmt.Sprintf("child %d: remove parent %d: %v", decision.ChildID, p.Parent.ID, err))
                continue
            }
            logging.Infof("batchResolveMultiParentConflicts: removed parent %d from child %d, keeping parent %d",
                p.Parent.ID, decision.ChildID, conflict.Parents[decision.BestIndex].Parent.ID)
        }
        resolved++
    }

    return resolved, errors
}
```

**Step 2: 修改 `CleanupMultiParentConflicts` 使用批量函数**

```go
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

    // 收集所有冲突详情
    var multiConflicts []multiParentConflict
    for _, c := range conflicts {
        var relations []models.TopicTagRelation
        if err := database.DB.Where("child_id = ? AND relation_type = ?", c.ChildID, "abstract").
            Preload("Parent").Find(&relations).Error; err != nil {
            continue
        }
        var parents []parentWithInfo
        var childTag models.TopicTag
        for _, r := range relations {
            if r.Parent != nil {
                parents = append(parents, parentWithInfo{RelationID: r.ID, Parent: r.Parent})
            }
        }
        if len(parents) <= 1 {
            continue
        }
        if err := database.DB.First(&childTag, c.ChildID).Error; err != nil {
            continue
        }
        multiConflicts = append(multiConflicts, multiParentConflict{
            ChildID: c.ChildID,
            Parents: parents,
            Child:   &childTag,
        })
    }

    // 批量解决（每批最多 10 个冲突）
    batchSize := 10
    totalResolved := 0
    var allErrors []string
    for i := 0; i < len(multiConflicts); i += batchSize {
        end := i + batchSize
        if end > len(multiConflicts) {
            end = len(multiConflicts)
        }
        batch := multiConflicts[i:end]
        resolved, errors := batchResolveMultiParentConflicts(batch)
        totalResolved += resolved
        allErrors = append(allErrors, errors...)
    }

    logging.Infof("CleanupMultiParentConflicts: resolved %d conflicts", totalResolved)
    return totalResolved, allErrors, nil
}
```

**Step 3: 运行测试验证**

```bash
cd backend-go && go build ./...
cd backend-go && go test ./internal/domain/topicanalysis/ -v -run "TestCleanup"
```

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/tag_cleanup.go backend-go/internal/domain/topicanalysis/abstract_tag_hierarchy.go
git commit -m "perf(tag-cleanup): batch multi-parent conflict resolution in Phase 3"
```

---

### Task 5: 验证端到端优化效果

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go`（添加耗时日志）

**Step 1: 在 `runCleanupCycle` 中添加阶段耗时日志**

在每个 Phase 前后添加 `time.Since` 日志，便于监控优化效果：

```go
// Phase 2
phaseStart := time.Now()
for _, category := range []string{"event", "keyword"} {
    // ...existing code...
}
logging.Infof("Phase 2 completed in %v", time.Since(phaseStart))

// Phase 4
phaseStart = time.Now()
adopted, err := topicanalysis.ProcessPendingAdoptNarrowerTasks()
logging.Infof("Phase 4 completed in %v (processed %d)", time.Since(phaseStart), adopted)

// Phase 5
phaseStart = time.Now()
updated, err := topicanalysis.ProcessPendingAbstractTagUpdateTasks()
logging.Infof("Phase 5 completed in %v (processed %d)", time.Since(phaseStart), updated)

// Phase 6
phaseStart = time.Now()
for _, category := range []string{"event", "keyword", "person"} {
    // ...existing code...
}
logging.Infof("Phase 6 completed in %v", time.Since(phaseStart))

// Phase 7
phaseStart = time.Now()
backfilled, err := topicextraction.BackfillMissingDescriptions()
logging.Infof("Phase 7 completed in %v (processed %d)", time.Since(phaseStart), backfilled)
```

**Step 2: 手动触发一次清理任务验证**

```bash
# 启动后端
cd backend-go && go run cmd/server/main.go

# 手动触发（通过 API）
curl -X POST http://localhost:5000/api/scheduler/tag_hierarchy_cleanup/trigger
```

**Step 3: 检查日志确认调用次数减少**

关注日志中的 `operation` 字段：
- `tag_description_batch` 应出现 5 次（50/10），而非 50 次 `tag_description`
- `tree_review` 调用次数应小于树总数
- `batch_resolve_multi_parent` 应出现而非多次 `aiJudgeBestParent`

**Step 4: Commit**

```bash
git add backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "perf(tag-cleanup): add per-phase timing logs for monitoring"
```

---

## 优化效果预估

| 阶段 | 优化前 LLM 调用 | 优化后 LLM 调用 | 节省 |
|------|----------------|----------------|------|
| Phase 3 | N（逐冲突） | ⌈N/10⌉（批量） | ~90% |
| Phase 4 | N + M（逐任务 + sleep） | N + M（无 sleep） | 时间成本 |
| Phase 5 | N + M（逐任务 + sleep） | N + M（无 sleep） | 时间成本 |
| Phase 6 | T（每棵树） | ⌈T/k⌉（小树合并） | ~50-70% |
| Phase 7 | 50（逐标签 + sleep） | 5（10个/批） | ~90% |

**总体估算**：LLM 调用次数减少 50-70%，总运行时间减少 30-50%。
