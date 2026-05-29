# Phase 4 整树审查替换 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 用整树 LLM 审查替换现有 Phase 4 的深度压缩逻辑，解决标签归属错配问题。

**Architecture:** 在 `hierarchy_cleanup.go` 中新增 `ReviewHierarchyTrees`，构建完整抽象树后序列化为文本树送 LLM 审查，返回 move/new_abstract 指令，校验后执行。迁移失败时跳过并保留旧关系，避免产生孤儿标签。大树先做 root + 直接子节点的根层审查，再按子树拆分。新抽象走 `findSimilarExistingAbstract` 匹配已有抽象，而非无脑放根目录。

**Tech Stack:** Go, GORM, airouter (LLM)

**Design doc:** `docs/plans/2026-04-25-tree-review-phase4-redesign.md`

---

### Task 1: 序列化与 LLM 数据结构

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`

**Step 1: 新增 LLM 输出数据结构**

在 `hierarchy_cleanup.go` 的类型定义区域（约第 50 行 `treeCleanupAbstract` 之后）添加：

```go
type treeReviewMove struct {
	TagID     uint   `json:"tag_id"`
	ToParent  uint   `json:"to_parent"`
	Reason    string `json:"reason"`
}

type treeReviewAbstract struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ChildrenIDs []uint `json:"children_ids"`
	Reason      string `json:"reason"`
}

type treeReviewJudgment struct {
	Moves         []treeReviewMove    `json:"moves"`
	NewAbstracts  []treeReviewAbstract `json:"new_abstracts"`
	Notes         string              `json:"notes"`
}
```

**Step 2: 新增 `serializeTreeForReview` 函数**

递归序列化 TreeNode 为缩进文本树，附带 ID/label/description。放在 `callCleanupLLM` 之后。

```go
func serializeTreeForReview(node *TreeNode) string {
	var sb strings.Builder
	serializeNode(&sb, node, "", true)
	return sb.String()
}

func serializeNode(sb *strings.Builder, node *TreeNode, prefix string, isLast bool) {
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		fmt.Fprintf(sb, "[id:%d] %s", node.Tag.ID, node.Tag.Label)
		if node.Tag.Description != "" {
			desc := truncateStr(node.Tag.Description, 80)
			fmt.Fprintf(sb, " (描述: %s)", desc)
		}
		sb.WriteString("\n")
	} else {
		fmt.Fprintf(sb, "%s%s[id:%d] %s", prefix, connector, node.Tag.ID, node.Tag.Label)
		if node.Tag.Description != "" {
			desc := truncateStr(node.Tag.Description, 80)
			fmt.Fprintf(sb, " (描述: %s)", desc)
		}
		sb.WriteString("\n")
	}
	for i, child := range node.Children {
		newPrefix := prefix
		if prefix == "" {
			newPrefix = "  "
		} else if isLast {
			newPrefix = prefix + "    "
		} else {
			newPrefix = prefix + "│   "
		}
		serializeNode(sb, child, newPrefix, i == len(node.Children)-1)
	}
}
```

**Step 3: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go
rtk git commit -m "feat(tag-cleanup): add tree review data structures and serialization"
```

---

### Task 2: LLM 审查调用

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`

**Step 1: 新增函数指针（可测试）**

在 `abstract_tag_service.go` 的函数指针区域（约第 30 行 `mergeTagsFn` 之后）添加：

```go
callTreeReviewLLMFn = callTreeReviewLLM
```

在 `abstract_tag_service.go` 文件的函数指针声明区域添加：

```go
callTreeReviewLLMFn func(prompt string) (*treeReviewJudgment, error)
```

**Step 2: 实现 `callTreeReviewLLM`**

放在 `hierarchy_cleanup.go` 的 `callCleanupLLM` 之后：

```go
func callTreeReviewLLM(prompt string) (*treeReviewJudgment, error) {
	router := airouter.NewRouter()
	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy review assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"moves": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"tag_id":    {Type: "integer"},
							"to_parent": {Type: "integer"},
							"reason":    {Type: "string"},
						},
						Required: []string{"tag_id", "to_parent", "reason"},
					},
				},
				"new_abstracts": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"name":         {Type: "string"},
							"description":  {Type: "string"},
							"children_ids": {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"reason":       {Type: "string"},
						},
						Required: []string{"name", "description", "children_ids", "reason"},
					},
				},
				"notes": {Type: "string"},
			},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation": "tree_review",
		},
	}

	result, err := router.Chat(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("tree review LLM call failed: %w", err)
	}

	content := jsonutil.SanitizeLLMJSON(result.Content)
	var judgment treeReviewJudgment
	if err := json.Unmarshal([]byte(content), &judgment); err != nil {
		return nil, fmt.Errorf("parse tree review response: %w", err)
	}

	logging.Infof("Tree review LLM judgment: %d moves, %d new abstracts",
		len(judgment.Moves), len(judgment.NewAbstracts))

	return &judgment, nil
}
```

**Step 3: 新增 `buildTreeReviewPrompt`**

```go
func buildTreeReviewPrompt(treeStr string, category string) string {
	return fmt.Sprintf(`请审查以下 %s 类别的标签树，检查子标签的归属是否合理，并给出调整建议。

%s

规则:
- 检查每个子标签是否真正属于其父标签
- 地理/区域不同且无直接关联的标签，不应在同一抽象父下
- 概念领域明显不同的标签，不应在同一父下
- to_parent=0 表示脱离成为独立根节点
- 非零 to_parent 表示迁移到树中已有标签下
- new_abstracts 用于建议创建新分组，children_ids 至少 2 个
- 如果树结构合理无需调整，返回空的 moves 和 new_abstracts

返回 JSON:
{
  "moves": [
    {"tag_id": 123, "to_parent": 0, "reason": "..."}
  ],
  "new_abstracts": [
    {"name": "新抽象名", "description": "描述", "children_ids": [123, 456], "reason": "..."}
  ]
}`, category, treeStr)
}
```

**Step 4: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: PASS

**Step 5: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/domain/topicanalysis/abstract_tag_service.go
rtk git commit -m "feat(tag-cleanup): add tree review LLM call and prompt builder"
```

---

### Task 3: Move 校验与执行

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`

**Step 1: 写测试**

在 `hierarchy_cleanup_test.go` 末尾添加：

```go
func TestValidateTreeReviewMove_TagNotActive(t *testing.T) {
	node := &TreeNode{Tag: makeTagWithStatus(1, "a", "inactive"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 0}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for inactive tag")
	}
}

func TestValidateTreeReviewMove_TagNotInTree(t *testing.T) {
	tagMap := map[uint]*TreeNode{}
	move := treeReviewMove{TagID: 999, ToParent: 0}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for tag not in tree")
	}
}

func TestValidateTreeReviewMove_SelfParent(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 1}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for self-parent")
	}
}

func TestValidateTreeReviewMove_ValidDetach(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 0}

	err := validateTreeReviewMove(move, tagMap)
	if err != nil {
		t.Errorf("expected no error for valid detach, got: %v", err)
	}
}

func TestValidateTreeReviewMove_InvalidTarget(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	move := treeReviewMove{TagID: 1, ToParent: 999}

	err := validateTreeReviewMove(move, tagMap)
	if err == nil {
		t.Error("expected error for non-existent target")
	}
}

// 另加 DB 集成测试：构造 parent ancestry + child subtree，使新关系深度超过 4。
// 期望 validateTreeReviewMove 返回错误，且 executeTreeReviewMove 不应被调用。
```

**Step 2: 运行测试确认失败**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestValidateTreeReviewMove -v`
Expected: FAIL（函数未定义）

**Step 3: 实现 `validateTreeReviewMove`**

```go
func validateTreeReviewMove(move treeReviewMove, tagMap map[uint]*TreeNode) error {
	node, ok := tagMap[move.TagID]
	if !ok {
		return fmt.Errorf("tag %d not found in tree", move.TagID)
	}
	if node.Tag.Status != "active" {
		return fmt.Errorf("tag %d is not active", move.TagID)
	}
	if move.ToParent != 0 {
		if move.ToParent == move.TagID {
			return fmt.Errorf("tag %d cannot be its own parent", move.TagID)
		}
		target, ok := tagMap[move.ToParent]
		if !ok {
			return fmt.Errorf("target parent %d not found in tree", move.ToParent)
		}
		if target.Tag.Status != "active" {
			return fmt.Errorf("target parent %d is not active", move.ToParent)
		}
		if database.DB != nil {
			wouldCycle, err := wouldCreateCycle(database.DB, move.ToParent, move.TagID)
			if err != nil {
				return fmt.Errorf("check cycle for move %d -> %d: %w", move.TagID, move.ToParent, err)
			}
			if wouldCycle {
				return fmt.Errorf("move %d -> %d would create cycle", move.TagID, move.ToParent)
			}
			childSubtreeDepth := getAbstractSubtreeDepth(database.DB, move.TagID)
			parentAncestryDepth := getTagDepthFromRoot(move.ToParent)
			if childSubtreeDepth+parentAncestryDepth+1 > 4 {
				return fmt.Errorf("move %d -> %d would exceed max depth 4", move.TagID, move.ToParent)
			}
		}
	}
	return nil
}
```

**Step 4: 运行测试确认通过**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestValidateTreeReviewMove -v`
Expected: PASS

**Step 5: 实现 `executeTreeReviewMove`**

该函数处理实际的关系删除和创建。关键约束：非 detach 迁移必须先成功创建新父关系，再清理旧父关系；如果新父关系创建失败，保留旧关系并返回错误，不能把标签留成孤儿。放在 `validateTreeReviewMove` 之后：

```go
func executeTreeReviewMove(move treeReviewMove) error {
	if move.ToParent == 0 {
		var oldParents []models.TopicTagRelation
		if err := database.DB.Where(
			"child_id = ? AND relation_type = ?", move.TagID, "abstract",
		).Find(&oldParents).Error; err != nil {
			return fmt.Errorf("load old parents for detach tag %d: %w", move.TagID, err)
		}
		result := database.DB.Where(
			"child_id = ? AND relation_type = ?", move.TagID, "abstract",
		).Delete(&models.TopicTagRelation{})
		if result.Error != nil {
			return fmt.Errorf("detach tag %d: %w", move.TagID, result.Error)
		}
		for _, old := range oldParents {
			go EnqueueAbstractTagUpdate(old.ParentID, "child_moved")
		}
		logging.Infof("Tree review: detached tag %d (reason: %s)", move.TagID, move.Reason)
		return nil
	}

	var oldParents []models.TopicTagRelation
	if err := database.DB.Where("child_id = ? AND relation_type = ?", move.TagID, "abstract").Find(&oldParents).Error; err != nil {
		return fmt.Errorf("load old parents for tag %d: %w", move.TagID, err)
	}

	if err := linkAbstractParentChild(move.TagID, move.ToParent); err != nil {
		logging.Warnf("Tree review: move %d -> %d failed, keeping old parents: %v", move.TagID, move.ToParent, err)
		return fmt.Errorf("link failed: %w", err)
	}

	for _, old := range oldParents {
		if old.ParentID == move.ToParent {
			continue
		}
		if err := database.DB.Delete(&models.TopicTagRelation{}, old.ID).Error; err != nil {
			return fmt.Errorf("delete old parent relation %d for tag %d: %w", old.ID, move.TagID, err)
		}
		go EnqueueAbstractTagUpdate(old.ParentID, "child_moved")
	}
	_, _ = resolveMultiParentConflict(move.TagID)
	go EnqueueAbstractTagUpdate(move.ToParent, "child_adopted")

	logging.Infof("Tree review: moved tag %d under %d (reason: %s)", move.TagID, move.ToParent, move.Reason)
	return nil
}
```

**Step 6: 验证编译 + 测试**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestValidateTreeReviewMove -v`
Expected: PASS

**Step 7: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go
rtk git commit -m "feat(tag-cleanup): add move validation and execution for tree review"
```

---

### Task 4: 修改 `BuildTagForest` 支持可配置 minDepth

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go`

**Step 1: 写测试**

用内存 DB 构造两棵 event 抽象树：一棵深度 2，一棵深度 3。

断言：
- `BuildTagForest("event")` 只返回深度 3 的树
- `BuildTagForest("event", 2)` 返回深度 2 和深度 3 的树
- `BuildTagForest("event", 4)` 返回空

这个测试能覆盖默认值兼容性和新参数行为，不要使用 placeholder 测试。

**Step 2: 修改 `BuildTagForest` 签名**

将当前函数签名从：
```go
func BuildTagForest(category string) ([]*TreeNode, error) {
```
改为：
```go
func BuildTagForest(category string, minDepth ...int) ([]*TreeNode, error) {
```

在函数体中，将：
```go
if depth >= MinTreeDepthForCleanup {
```
改为：
```go
md := MinTreeDepthForCleanup
if len(minDepth) > 0 {
    md = minDepth[0]
}
if depth >= md {
```

**Step 3: 更新现有调用点**

`ExecuteHierarchyCleanupPhase4` 调用 `BuildTagForest(category)` — 保持不变（用默认值）。

**Step 4: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: PASS

**Step 5: 运行现有测试**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestBuildTagForest -v`
Expected: PASS（如果没有现有测试则跳过）

Run: `cd backend-go && go test ./internal/domain/topicanalysis -v`
Expected: PASS

**Step 6: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go
rtk git commit -m "refactor(tag-cleanup): make BuildTagForest minDepth configurable"
```

---

### Task 5: 核心入口 `ReviewHierarchyTrees`

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`

**Step 1: 新增时间窗口过滤函数**

```go
func filterTreesWithRecentRelations(forest []*TreeNode, windowDays int) []*TreeNode {
	if windowDays <= 0 {
		return forest
	}
	cutoff := time.Now().AddDate(0, 0, -windowDays)

	var recentTagIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Select("DISTINCT CASE WHEN relation_type = 'abstract' THEN parent_id ELSE parent_id END as tag_id").
		Where("relation_type = 'abstract' AND created_at >= ?", cutoff).
		Pluck("parent_id", &recentTagIDs)

	var recentChildIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = 'abstract' AND created_at >= ?", cutoff).
		Pluck("child_id", &recentChildIDs)

	recentSet := make(map[uint]bool)
	for _, id := range recentTagIDs {
		recentSet[id] = true
	}
	for _, id := range recentChildIDs {
		recentSet[id] = true
	}

	var filtered []*TreeNode
	for _, root := range forest {
		if treeContainsTag(root, recentSet) {
			filtered = append(filtered, root)
		}
	}
	return filtered
}

func treeContainsTag(node *TreeNode, tagSet map[uint]bool) bool {
	if tagSet[node.Tag.ID] {
		return true
	}
	for _, child := range node.Children {
		if treeContainsTag(child, tagSet) {
			return true
		}
	}
	return false
}
```

**Step 2: 新增大树拆分函数和根层审查切片**

复用现有 `collectAllTags` + 按≤50 节点拆分子树。大树必须额外生成 root + 直接子节点的根层审查切片，避免拆分后漏掉第一层错挂：

```go
func splitLargeTree(root *TreeNode, maxNodes int) []*TreeNode {
	if countNodes(root) <= maxNodes {
		return []*TreeNode{root}
	}
	var subtrees []*TreeNode
	for _, child := range root.Children {
		subtrees = append(subtrees, splitLargeTree(child, maxNodes)...)
	}
	return subtrees
}

func rootLevelReviewTree(root *TreeNode) *TreeNode {
	clone := &TreeNode{Tag: root.Tag, Depth: root.Depth, ArticleCount: root.ArticleCount}
	for _, child := range root.Children {
		childClone := &TreeNode{Tag: child.Tag, Depth: child.Depth, ArticleCount: child.ArticleCount, Parent: clone}
		clone.Children = append(clone.Children, childClone)
	}
	return clone
}
```

**Step 3: 实现 `ReviewHierarchyTrees`**

```go
type TreeReviewResult struct {
	TreesReviewed int
	MovesApplied  int
	GroupsCreated int
	GroupsReused  int
	Errors        []string
}

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
	for _, root := range forest {
		if countNodes(root) > 50 {
			reviewOneTree(rootLevelReviewTree(root), category, result)
		}
		subtrees := splitLargeTree(root, 50)
		for _, tree := range subtrees {
			reviewOneTree(tree, category, result)
		}
	}
	return result, nil
}

func reviewOneTree(tree *TreeNode, category string, result *TreeReviewResult) {
	treeStr := serializeTreeForReview(tree)
	prompt := buildTreeReviewPrompt(treeStr, category)

	judgment, err := callTreeReviewLLMFn(prompt)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("tree root %d: %v", tree.Tag.ID, err))
		return
	}
	result.TreesReviewed++

	tagMap := make(map[uint]*TreeNode)
	for _, node := range collectAllTags(tree) {
		tagMap[node.Tag.ID] = node
	}

	for _, move := range judgment.Moves {
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

**Step 4: 实现 `validateAndCreateReviewAbstract`**

```go
func validateAndCreateReviewAbstract(abs treeReviewAbstract, tagMap map[uint]*TreeNode, category string) (bool, error) {
	if len(abs.ChildrenIDs) < 2 {
		return false, fmt.Errorf("need at least 2 children, got %d", len(abs.ChildrenIDs))
	}
	for _, id := range abs.ChildrenIDs {
		node, ok := tagMap[id]
		if !ok {
			return false, fmt.Errorf("child %d not in tree", id)
		}
		if node.Tag.Status != "active" {
			return false, fmt.Errorf("child %d not active", id)
		}
	}

	var candidates []TagCandidate
	for _, id := range abs.ChildrenIDs {
		candidates = append(candidates, TagCandidate{Tag: tagMap[id].Tag, Similarity: 0.9})
	}

	if existing := findSimilarExistingAbstractFn(context.Background(), abs.Name, abs.Description, category, candidates); existing != nil {
		logging.Infof("Tree review: reusing existing abstract %d (%q) instead of creating %q", existing.ID, existing.Label, abs.Name)
		for _, id := range abs.ChildrenIDs {
			if uint(id) == existing.ID {
				continue
			}
			if err := linkAbstractParentChild(id, existing.ID); err != nil {
				return false, fmt.Errorf("link child %d to existing abstract %d: %w", id, existing.ID, err)
			}
		}
		return false, nil
	}

	slug := topictypes.Slugify(abs.Name)
	if slug == "" {
		return false, fmt.Errorf("generated empty slug for abstract name %q", abs.Name)
	}
	var existingBySlug models.TopicTag
	if err := database.DB.Where("slug = ? AND category = ? AND status = ?", slug, category, "active").First(&existingBySlug).Error; err == nil {
		for _, id := range abs.ChildrenIDs {
			if id == existingBySlug.ID {
				continue
			}
			if err := linkAbstractParentChild(id, existingBySlug.ID); err != nil {
				return false, fmt.Errorf("link child %d to slug-matched abstract %d: %w", id, existingBySlug.ID, err)
			}
		}
		return false, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("check existing abstract slug %q: %w", slug, err)
	}

	treeCleanupAbs := treeCleanupAbstract{
		Name:        abs.Name,
		Description: abs.Description,
		ChildrenIDs: abs.ChildrenIDs,
		Reason:      abs.Reason,
	}
	return true, createAbstractTagDirectly(treeCleanupAbs, tagMap, category)
}
```

**Step 5: 注册函数指针**

在 `abstract_tag_service.go` 的函数指针区域添加：

```go
callTreeReviewLLMFn = callTreeReviewLLM
```

确保 `callTreeReviewLLMFn` 的类型声明在 `abstract_tag_service.go` 中：

```go
callTreeReviewLLMFn func(prompt string) (*treeReviewJudgment, error)
```

**Step 6: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: PASS

**Step 7: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/domain/topicanalysis/abstract_tag_service.go
rtk git commit -m "feat(tag-cleanup): implement ReviewHierarchyTrees core logic"
```

---

### Task 6: 替换调度器 Phase 4 调用

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go`
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup_test.go`

**Step 1: 更新 summary 结构**

在 `TagHierarchyCleanupRunSummary` 中，将：

```go
Phase4Trees     int `json:"phase4_trees"`
Phase4Merges    int `json:"phase4_merges"`
Phase4Reparents int `json:"phase4_reparents"`
```

替换为：

```go
TreesReviewed int `json:"trees_reviewed"`
MovesApplied  int `json:"moves_applied"`
GroupsCreated int `json:"tree_groups_created"`
GroupsReused  int `json:"tree_groups_reused"`
```

**Step 2: 替换 `runCleanupCycle` 中 Phase 4 调用**

将 Phase 4 块（约第 287-303 行）：

```go
// Phase 4: Cross-layer dedup + depth compression
for _, category := range []string{"event", "keyword"} {
    phase4Result, phase4Err := topicanalysis.ExecuteHierarchyCleanupPhase4(category)
    ...
}
```

替换为：

```go
// Phase 4: Tree review
for _, category := range []string{"event", "keyword", "person"} {
    reviewResult, reviewErr := topicanalysis.ReviewHierarchyTrees(category, 14)
    if reviewErr != nil {
        logging.Errorf("Phase 4 tree review failed for %s: %v", category, reviewErr)
        summary.Errors++
        continue
    }
    summary.TreesReviewed += reviewResult.TreesReviewed
    summary.MovesApplied += reviewResult.MovesApplied
    summary.GroupsCreated += reviewResult.GroupsCreated
    summary.GroupsReused += reviewResult.GroupsReused
    summary.Errors += len(reviewResult.Errors)
    for _, errMsg := range reviewResult.Errors {
        logging.Warnf("Phase 4 %s: %s", category, errMsg)
    }
    logging.Infof("Phase 4 (%s): reviewed %d trees, %d moves, %d groups created, %d groups reused", category, reviewResult.TreesReviewed, reviewResult.MovesApplied, reviewResult.GroupsCreated, reviewResult.GroupsReused)
}
```

**Step 3: 更新 Reason 字符串**

将：
```go
summary.Reason = fmt.Sprintf("zombie=%d, flat_merges=%d, orphaned_rels=%d, multi_parent=%d, empty_abstracts=%d, phase4_trees=%d, phase4_merges=%d, phase4_reparents=%d",
    summary.ZombieDeactivated, summary.FlatMergesApplied, summary.OrphanedRelations, summary.MultiParentFixed, summary.EmptyAbstracts, summary.Phase4Trees, summary.Phase4Merges, summary.Phase4Reparents)
```

替换为：
```go
summary.Reason = fmt.Sprintf("zombie=%d, flat_merges=%d, orphaned_rels=%d, multi_parent=%d, empty_abstracts=%d, trees_reviewed=%d, moves=%d, groups_created=%d, groups_reused=%d",
    summary.ZombieDeactivated, summary.FlatMergesApplied, summary.OrphanedRelations, summary.MultiParentFixed, summary.EmptyAbstracts, summary.TreesReviewed, summary.MovesApplied, summary.GroupsCreated, summary.GroupsReused)
```

**Step 4: 更新日志行**

将 `logging.Infoln("Starting tag cleanup cycle (4-phase)")` 保持不变（仍是4阶段）。

**Step 5: 更新调度器测试**

`tag_hierarchy_cleanup_test.go` 中 `TestRunCleanupCycleSummaryOmitsLegacyTreeFields` 已经检查了 `trees_processed`、`tags_processed`、`merges_applied`、`abstracts_created` 不存在。新字段必须避开 `abstracts_created`，使用 `tree_groups_created` / `tree_groups_reused`，避免和旧字段语义冲突。

新增检查：
```go
for _, key := range []string{"trees_reviewed", "moves_applied", "tree_groups_created", "tree_groups_reused"} {
    if _, exists := payload[key]; !exists {
        t.Fatalf("summary missing expected key %q: %#v", key, payload)
    }
}
```

**Step 6: 验证编译 + 测试**

Run: `cd backend-go && go build ./... && go test ./internal/jobs -run TestRunCleanupCycle -v`
Expected: PASS

**Step 7: Commit**

```bash
rtk git add backend-go/internal/jobs/tag_hierarchy_cleanup.go backend-go/internal/jobs/tag_hierarchy_cleanup_test.go
rtk git commit -m "feat(tag-cleanup): wire ReviewHierarchyTrees into Phase 4 scheduler"
```

---

### Task 7: 清理旧 Phase 4 代码

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go`

**Step 1: 删除旧函数**

删除以下函数（确认无其他调用方）：
- `cleanupDeepHierarchyTree`
- `ProcessTree`
- `processBatch`
- `processRootCrossLayer`
- `collectDeepNodes`
- `buildCleanupPrompt`（旧 prompt）
- `callCleanupLLM`（旧 LLM 调用）
- `validateAndExecuteMerge`（旧的深度差≥2 merge 逻辑）
- `validateAndExecuteAbstract`（已由 `validateAndCreateReviewAbstract` 替代）

注意：保留 `BuildTagForest`、`buildTreeNode`、`calculateTreeDepth`、`countNodes`、`collectAllTags`、`findCycleRoots`、`createAbstractTagDirectly`、`isDirectParentChild`、`abs` — 这些仍被新代码使用。

**Step 2: 更新/删除旧测试**

删除引用旧函数的测试：
- `TestExecuteHierarchyCleanupPhase4SkipsShallowTrees`
- `TestValidateAndExecuteMerge_*` 系列
- `TestCollectDeepNodes_*`
- `TestMinTreeDepthConstant`（如果不再使用常量）

保留仍在使用的测试：
- `TestCalculateTreeDepth_*`
- `TestCountNodes`
- `TestCollectAllTags`
- `TestIsDirectParentChild`
- `TestAbs`

**Step 3: 验证编译 + 全量测试**

Run: `cd backend-go && go build ./... && go test ./internal/domain/topicanalysis -v`
Expected: PASS

**Step 4: Commit**

```bash
rtk git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go
rtk git commit -m "refactor(tag-cleanup): remove old Phase 4 depth-compression code"
```

---

### Task 8: 更新文档

**Files:**
- Modify: `docs/guides/tagging-flow.md`

**Step 1: 更新第 11 节标签清理机制**

更新 Phase 4 的描述，反映新的整树审查逻辑。更新速查表。

**Step 2: Commit**

```bash
rtk git add docs/guides/tagging-flow.md
rtk git commit -m "docs: update tagging-flow with new Phase 4 tree review"
```

---

### Task 9: 全量验证

**Step 1: 运行全量后端测试**

Run: `cd backend-go && go test ./... -v`
Expected: PASS

**Step 2: 运行全量构建**

Run: `cd backend-go && go build ./...`
Expected: PASS
