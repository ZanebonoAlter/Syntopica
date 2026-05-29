# Phase 4 树审查增强：merges 操作 + 根节点保护

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 增强 Phase 4 整树审查，支持抽象标签 merge 操作，防止同一标签在树中重复出现；同时增加根节点保护机制防止根节点标签名被随意修改或被降级为子节点。

**Architecture:** 扩展现有 `reviewOneTree` 流程，在 LLM 的 JSON Schema 和 prompt 中新增 `merges` 操作类型。执行顺序改为 merges → moves → new_abstracts，确保先消除重复再调整结构。根节点保护分两层：`refreshAbstractTag` 中锁定根节点 label，`buildTreeReviewPrompt` 中禁止根节点被 move 降级。

**Tech Stack:** Go, Gin, GORM, airouter (LLM)

---

## 变更范围

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go` | 修改 | 核心变更：新增 merges 结构体、prompt、schema、执行逻辑、根节点保护 |
| `backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go` | 修改 | 根节点 label 锁定 |
| `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go` | 修改 | 新增 merges 相关测试、根节点保护测试 |
| `backend-go/internal/jobs/tag_hierarchy_cleanup.go` | 修改 | 统计增加 MergesApplied |
| `docs/guides/tagging-flow.md` | 修改 | 更新 Phase 4 文档 |

---

## Task 1: 新增 `treeReviewMerge` 结构体和更新 `treeReviewJudgment`

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go:33-57`

**Step 1: 在 `treeReviewAbstract` 后面新增 `treeReviewMerge` 结构体**

在 `hierarchy_cleanup.go` 的 `treeReviewMove` 结构体（40-44行）后面新增：

```go
type treeReviewMerge struct {
	SourceID uint   `json:"source_id"`
	TargetID uint   `json:"target_id"`
	Reason   string `json:"reason"`
}
```

**Step 2: 更新 `treeReviewJudgment` 增加 `Merges` 字段**

```go
type treeReviewJudgment struct {
	Moves        []treeReviewMove     `json:"moves"`
	Merges       []treeReviewMerge    `json:"merges"`
	NewAbstracts []treeReviewAbstract `json:"new_abstracts"`
	Notes        string               `json:"notes"`
}
```

**Step 3: Run test to verify compilation**

Run: `cd backend-go && go build ./...`
Expected: BUILD SUCCESS

**Step 4: Commit**

```
feat(tag-cleanup): add treeReviewMerge struct and Merges field to treeReviewJudgment
```

---

## Task 2: 更新 LLM JSON Schema 增加 `merges`

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go:421-449`

**Step 1: 在 `callTreeReviewLLM` 的 JSONSchema Properties 中增加 `merges`**

在 `callTreeReviewLLM` 函数的 `Properties` map 中（"moves" 和 "new_abstracts" 之间）加入：

```go
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
```

**Step 2: 更新 `callTreeReviewLLM` 中的日志行**

在 `callTreeReviewLLM` 函数中（约469行），将日志从：
```go
logging.Infof("Tree review LLM judgment: %d moves, %d new abstracts",
    len(judgment.Moves), len(judgment.NewAbstracts))
```
改为：
```go
logging.Infof("Tree review LLM judgment: %d merges, %d moves, %d new abstracts",
    len(judgment.Merges), len(judgment.Moves), len(judgment.NewAbstracts))
```

**Step 3: Run test to verify compilation**

Run: `cd backend-go && go build ./...`
Expected: BUILD SUCCESS

**Step 4: Commit**

```
feat(tag-cleanup): add merges to LLM JSON schema for tree review
```

---

## Task 3: 更新 Prompt 增加 merges 规则

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go:475-498`

**Step 1: 更新 `buildTreeReviewPrompt` 的规则和返回 JSON 示例**

将整个 `buildTreeReviewPrompt` 函数替换为：

```go
func buildTreeReviewPrompt(treeStr string, category string) string {
	return fmt.Sprintf(`请审查以下 %s 类别的标签树，检查子标签的归属是否合理，并给出调整建议。

%s

规则:
- 检查每个子标签是否真正属于其父标签
- 地理/区域不同且无直接关联的标签，不应在同一抽象父下
- 概念领域明显不同的标签，不应在同一父下
- 树的顶级根节点（第一个 [id:...] ）不允许被 move 为其他节点的子节点，也不允许作为 merge 的 source
- 非 root 的子节点可以 merge（source 合并进 target），合并后 source 的子节点会自动迁移到 target 下
- to_parent=0 表示脱离成为独立根节点
- 非零 to_parent 表示迁移到树中已有标签下
- merges 用于合并树中语义重复的抽象标签，source 合并进 target（target 保留）
- new_abstracts 用于建议创建新分组，children_ids 至少 2 个
- 可以同时返回 moves、merges、new_abstracts，不必只选一种
- 如果树结构合理无需调整，返回空的 moves、merges 和 new_abstracts

返回 JSON:
{
  "moves": [
    {"tag_id": 123, "to_parent": 0, "reason": "..."}
  ],
  "merges": [
    {"source_id": 123, "target_id": 456, "reason": "..."}
  ],
  "new_abstracts": [
    {"name": "新抽象名", "description": "描述", "children_ids": [123, 456], "reason": "..."}
  ]
}`, category, treeStr)
}
```

**Step 2: 更新现有测试 `TestBuildTreeReviewPromptIncludesRulesAndTree`**

在 `hierarchy_cleanup_test.go` 的 `TestBuildTreeReviewPromptIncludesRulesAndTree` 中增加对 "merges" 关键字的检查：

```go
func TestBuildTreeReviewPromptIncludesRulesAndTree(t *testing.T) {
	tree := "[id:1] 政治人物\n  └── [id:2] 伊朗政治人物\n"

	got := buildTreeReviewPrompt(tree, "person")

	for _, want := range []string{"person 类别", tree, "to_parent=0", "new_abstracts", "merges", "source_id", "target_id", "返回 JSON"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q in:\n%s", want, got)
		}
	}
}
```

**Step 3: Run test**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestBuildTreeReviewPromptIncludesRulesAndTree -v`
Expected: PASS

**Step 4: Commit**

```
feat(tag-cleanup): update tree review prompt with merges rules and root protection
```

---

## Task 4: 实现 `validateTreeReviewMerge` 校验函数

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go`
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go`

**Step 1: 在 `validateTreeReviewMove` 函数后面新增 `validateTreeReviewMerge`**

在 `hierarchy_cleanup.go` 中 `validateTreeReviewMove` 函数（500-537行）之后插入：

```go
func validateTreeReviewMerge(merge treeReviewMerge, tagMap map[uint]*TreeNode) error {
	sourceNode, ok := tagMap[merge.SourceID]
	if !ok {
		return fmt.Errorf("source tag %d not found in tree", merge.SourceID)
	}
	if sourceNode.Tag.Status != "active" {
		return fmt.Errorf("source tag %d is not active (status=%s)", merge.SourceID, sourceNode.Tag.Status)
	}
	targetNode, ok := tagMap[merge.TargetID]
	if !ok {
		return fmt.Errorf("target tag %d not found in tree", merge.TargetID)
	}
	if targetNode.Tag.Status != "active" {
		return fmt.Errorf("target tag %d is not active", merge.TargetID)
	}
	if merge.SourceID == merge.TargetID {
		return fmt.Errorf("cannot merge tag %d into itself", merge.SourceID)
	}
	if database.DB == nil {
		return nil
	}
	wouldCycle, err := wouldCreateCycle(database.DB, merge.TargetID, merge.SourceID)
	if err != nil {
		return fmt.Errorf("check cycle for merge %d -> %d: %w", merge.SourceID, merge.TargetID, err)
	}
	if wouldCycle {
		return fmt.Errorf("merge %d -> %d would create cycle (source is ancestor of target)", merge.SourceID, merge.TargetID)
	}
	childSubtreeDepth := getAbstractSubtreeDepth(database.DB, merge.SourceID)
	parentAncestryDepth := getTagDepthFromRoot(merge.TargetID)
	if childSubtreeDepth+parentAncestryDepth+1 > 4 {
		return fmt.Errorf("merge %d -> %d would exceed max depth 4 after migration", merge.SourceID, merge.TargetID)
	}
	return nil
}
```

**Step 2: 写测试**

在 `hierarchy_cleanup_test.go` 末尾追加：

```go
func TestValidateTreeReviewMerge_SourceNotInTree(t *testing.T) {
	target := &TreeNode{Tag: makeTag(2, "target"), Depth: 1}
	tagMap := map[uint]*TreeNode{2: target}
	merge := treeReviewMerge{SourceID: 999, TargetID: 2}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for source not in tree")
	}
}

func TestValidateTreeReviewMerge_TargetNotInTree(t *testing.T) {
	source := &TreeNode{Tag: makeTag(1, "source"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: source}
	merge := treeReviewMerge{SourceID: 1, TargetID: 999}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for target not in tree")
	}
}

func TestValidateTreeReviewMerge_SelfMerge(t *testing.T) {
	node := &TreeNode{Tag: makeTag(1, "a"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: node}
	merge := treeReviewMerge{SourceID: 1, TargetID: 1}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for self-merge")
	}
}

func TestValidateTreeReviewMerge_SourceInactive(t *testing.T) {
	source := &TreeNode{Tag: makeTagWithStatus(1, "source", "inactive"), Depth: 1}
	target := &TreeNode{Tag: makeTag(2, "target"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: source, 2: target}
	merge := treeReviewMerge{SourceID: 1, TargetID: 2}

	if err := validateTreeReviewMerge(merge, tagMap); err == nil {
		t.Error("expected error for inactive source")
	}
}

func TestValidateTreeReviewMerge_ValidMerge(t *testing.T) {
	source := &TreeNode{Tag: makeTag(1, "source"), Depth: 1}
	target := &TreeNode{Tag: makeTag(2, "target"), Depth: 1}
	tagMap := map[uint]*TreeNode{1: source, 2: target}
	merge := treeReviewMerge{SourceID: 1, TargetID: 2}

	if err := validateTreeReviewMerge(merge, tagMap); err != nil {
		t.Errorf("expected no error for valid merge, got: %v", err)
	}
}

func TestValidateTreeReviewMerge_RejectsDepthOverflow(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	// 构造树: root -> A -> B (B 深度=3)
	//              -> C -> D -> E (C 深度=2, D=3, E=4)
	// merge C -> B: source(C) 子树深度=2(D->E), target(B) 祖先深度=2(A->root)
	// 2 + 2 + 1 = 5 > 4, 应拒绝
	root := models.TopicTag{Label: "root", Slug: "root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagA := models.TopicTag{Label: "A", Slug: "a", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagB := models.TopicTag{Label: "B", Slug: "b", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagC := models.TopicTag{Label: "C", Slug: "c", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagD := models.TopicTag{Label: "D", Slug: "d", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	tagE := models.TopicTag{Label: "E", Slug: "e", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	for _, tag := range []*models.TopicTag{&root, &tagA, &tagB, &tagC, &tagD, &tagE} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag %s: %v", tag.Label, err)
		}
	}
	// root -> A -> B
	db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: tagA.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: tagA.ID, ChildID: tagB.ID, RelationType: "abstract"})
	// root -> C -> D -> E
	db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: tagC.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: tagC.ID, ChildID: tagD.ID, RelationType: "abstract"})
	db.Create(&models.TopicTagRelation{ParentID: tagD.ID, ChildID: tagE.ID, RelationType: "abstract"})

	tagMap := map[uint]*TreeNode{
		root.ID: {Tag: &root, Depth: 1},
		tagA.ID: {Tag: &tagA, Depth: 2},
		tagB.ID: {Tag: &tagB, Depth: 3},
		tagC.ID: {Tag: &tagC, Depth: 2},
		tagD.ID: {Tag: &tagD, Depth: 3},
		tagE.ID: {Tag: &tagE, Depth: 4},
	}

	err := validateTreeReviewMerge(treeReviewMerge{SourceID: tagC.ID, TargetID: tagB.ID}, tagMap)
	if err == nil {
		t.Fatal("expected depth overflow error")
	}
}
```

**Step 3: Run tests**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestValidateTreeReviewMerge -v`
Expected: ALL PASS

**Step 4: Commit**

```
feat(tag-cleanup): add validateTreeReviewMerge with depth and status checks
```

---

## Task 5: 更新 `reviewOneTree` 执行顺序：merges → moves → new_abstracts

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go:293-333`

**Step 1: 重写 `reviewOneTree` 函数**

替换 `reviewOneTree` 为：

```go
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

	for _, merge := range judgment.Merges {
		if merge.SourceID == tree.Tag.ID {
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
		if move.ToParent != 0 && move.TagID == tree.Tag.ID {
			result.Errors = append(result.Errors, fmt.Sprintf("move %d: root node cannot be demoted to child", move.TagID))
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

关键变化：
1. **merges 先执行**，根节点不可作为 merge source（保护整棵树不散架），从 tagMap 中删除已合并的 source
2. **moves 中根节点保护优先判断**：先检查 `move.TagID == tree.Tag.ID && to_parent != 0`，再检查 tagMap 存在性。这避免了根节点被 merge 后又出现在 moves 中时产生误导性 error
3. **moves 中跳过不在 tagMap 的节点**：被 merge 掉的或 LLM 幻觉的 tag 直接静默跳过

**Step 2: 更新 `TreeReviewResult` 增加 `MergesApplied`**

在 `hierarchy_cleanup.go` 的 `TreeReviewResult` 结构体（195-201行）增加字段：

```go
type TreeReviewResult struct {
	TreesReviewed int
	MergesApplied int
	MovesApplied  int
	GroupsCreated int
	GroupsReused  int
	Errors        []string
}
```

**Step 3: 更新现有测试中 mock LLM 返回值适配新结构**

在 `TestReviewHierarchyTreesAppliesLLMMove` 中，mock 返回值改为显式包含空 Merges：

```go
callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
    return &treeReviewJudgment{Moves: []treeReviewMove{{TagID: child.ID, ToParent: newParent.ID, Reason: "test"}}, Merges: nil}, nil
}
```

**Step 4: Run tests**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestReviewHierarchyTreesAppliesLLMMove -v`
Expected: PASS

**Step 5: Commit**

```
feat(tag-cleanup): execute merges before moves in reviewOneTree with root protection
```

---

## Task 6: 更新日志输出增加 merges 统计

**Files:**
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go:296-304`
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go:30-45`
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go:307-309`

**Step 1: 在 `TagHierarchyCleanupRunSummary` 增加 `MergesApplied`**

```go
type TagHierarchyCleanupRunSummary struct {
	TriggerSource     string `json:"trigger_source"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at"`
	ZombieDeactivated int    `json:"zombie_deactivated"`
	FlatMergesApplied int    `json:"flat_merges_applied"`
	OrphanedRelations int    `json:"orphaned_relations"`
	MultiParentFixed  int    `json:"multi_parent_fixed"`
	EmptyAbstracts    int    `json:"empty_abstracts"`
	TreesReviewed     int    `json:"trees_reviewed"`
	MergesApplied     int    `json:"merges_applied"`
	MovesApplied      int    `json:"moves_applied"`
	GroupsCreated     int    `json:"tree_groups_created"`
	GroupsReused      int    `json:"tree_groups_reused"`
	Errors            int    `json:"errors"`
	Reason            string `json:"reason"`
}
```

**Step 2: 在 Phase 4 循环中增加 MergesApplied 累加**

在 `tag_hierarchy_cleanup.go` 的 Phase 4 循环（289-305行）中增加：

```go
summary.MergesApplied += reviewResult.MergesApplied
```

同时更新日志行：

```go
logging.Infof("Phase 4 (%s): reviewed %d trees, %d merges, %d moves, %d groups created, %d groups reused",
    category, reviewResult.TreesReviewed, reviewResult.MergesApplied, reviewResult.MovesApplied, reviewResult.GroupsCreated, reviewResult.GroupsReused)
```

**Step 3: 更新 Reason 字符串增加 merges**

```go
summary.Reason = fmt.Sprintf("zombie=%d, flat_merges=%d, orphaned_rels=%d, multi_parent=%d, empty_abstracts=%d, trees_reviewed=%d, merges=%d, moves=%d, groups_created=%d, groups_reused=%d",
    summary.ZombieDeactivated, summary.FlatMergesApplied, summary.OrphanedRelations, summary.MultiParentFixed, summary.EmptyAbstracts, summary.TreesReviewed, summary.MergesApplied, summary.MovesApplied, summary.GroupsCreated, summary.GroupsReused)
```

**Step 4: Run build**

Run: `cd backend-go && go build ./...`
Expected: BUILD SUCCESS

**Step 5: Commit**

```
feat(tag-cleanup): add MergesApplied to cleanup summary and logs
```

---

## Task 7: 根节点 label 保护 — `refreshAbstractTag` 锁定根节点 label

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go:193-235`

**Step 1: 新增 `isAbstractRoot` 辅助函数**

在 `abstract_tag_update_queue.go` 中新增（接受 `*gorm.DB` 参数以保持测试隔离性）：

```go
func isAbstractRoot(db *gorm.DB, tagID uint) bool {
	var count int64
	db.Model(&models.TopicTagRelation{}).
		Where("child_id = ? AND relation_type = ?", tagID, "abstract").
		Count(&count)
	return count == 0
}
```

**Step 2: 在 `refreshAbstractTag` 中增加根节点 label 保护**

在 `refreshAbstractTag` 函数中，219行 `if newLabel != "" && newLabel != tag.Label {` 的代码块替换为：

```go
if newLabel != "" && newLabel != tag.Label {
	newSlug := topictypes.Slugify(newLabel)
	if newSlug != "" && newSlug != tag.Slug {
		if isAbstractRoot(s.db, abstractTagID) {
			logging.Infof("Skipping label update for root abstract tag %d (%q): root labels are protected", abstractTagID, tag.Label)
		} else {
			var conflictCount int64
			s.db.Model(&models.TopicTag{}).
				Where("slug = ? AND id != ? AND status = ?", newSlug, abstractTagID, "active").
				Count(&conflictCount)
			if conflictCount == 0 {
				updates["label"] = newLabel
				updates["slug"] = newSlug
				tag.Label = newLabel
				tag.Slug = newSlug
			} else {
				logging.Warnf("Skipping label update for abstract tag %d: slug %q already in use", abstractTagID, newSlug)
			}
		}
	}
}
```

即：如果该抽象标签是根节点（没有任何 abstract 类型的 parent），跳过 label 更新，只允许 description 更新。

**Step 3: Run build**

Run: `cd backend-go && go build ./...`
Expected: BUILD SUCCESS

**Step 4: Commit**

```
feat(tag-cleanup): protect root abstract tag labels from regeneration
```

---

## Task 8: 集成测试 — merges 端到端

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup_test.go`

**Step 1: 新增集成测试 `TestReviewHierarchyTreesAppliesLLMMerge`**

在 `hierarchy_cleanup_test.go` 末尾追加：

```go
func TestReviewHierarchyTreesAppliesLLMMerge(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	root := models.TopicTag{Label: "根", Slug: "root", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	dup1 := models.TopicTag{Label: "重复A", Slug: "dup-a", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	dup2 := models.TopicTag{Label: "重复B", Slug: "dup-b", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	other := models.TopicTag{Label: "其他", Slug: "other", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(&dup1).Error; err != nil {
		t.Fatalf("create dup1: %v", err)
	}
	if err := db.Create(&dup2).Error; err != nil {
		t.Fatalf("create dup2: %v", err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("create other: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: dup1.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create root->dup1: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: dup2.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create root->dup2: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: other.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create root->other: %v", err)
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{
			Merges: []treeReviewMerge{{SourceID: dup1.ID, TargetID: dup2.ID, Reason: "test merge"}},
		}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	result, err := ReviewHierarchyTrees("event", 14)
	if err != nil {
		t.Fatalf("ReviewHierarchyTrees: %v", err)
	}
	if result.TreesReviewed != 1 || result.MergesApplied != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	var merged models.TopicTag
	if err := db.First(&merged, dup1.ID).Error; err != nil {
		t.Fatalf("load merged tag: %v", err)
	}
	if merged.Status != "merged" {
		t.Fatalf("source tag status = %q, want merged", merged.Status)
	}
	assertAbstractRelationMissing(t, db, root.ID, dup1.ID)
	assertAbstractRelationExists(t, db, root.ID, dup2.ID)
}
```

**Step 2: 新增根节点 merge source 拒绝测试 `TestReviewHierarchyTreesRejectsRootMergeSource`**

```go
func TestReviewHierarchyTreesRejectsRootMergeSource(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	root := models.TopicTag{Label: "根节点", Slug: "root-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子节点", Slug: "child-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: child.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{
			Merges: []treeReviewMerge{{SourceID: root.ID, TargetID: child.ID, Reason: "should be rejected"}},
		}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	result, err := ReviewHierarchyTrees("event", 14)
	if err != nil {
		t.Fatalf("ReviewHierarchyTrees: %v", err)
	}
	if result.MergesApplied != 0 {
		t.Fatalf("root merge source should be rejected, but merge was applied")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error for root merge source attempt")
	}
	var rootTag models.TopicTag
	if err := db.First(&rootTag, root.ID).Error; err != nil {
		t.Fatalf("load root: %v", err)
	}
	if rootTag.Status != "active" {
		t.Fatalf("root tag should remain active, got %q", rootTag.Status)
	}
}
```

**Step 3: 新增根节点 move 降级拒绝测试 `TestReviewHierarchyTreesRejectsRootDemotion`**

```go
func TestReviewHierarchyTreesRejectsRootDemotion(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)
	root := models.TopicTag{Label: "根节点", Slug: "root-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Label: "子节点", Slug: "child-node", Category: "event", Kind: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root: %v", err)
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: root.ID, ChildID: child.ID, RelationType: "abstract", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}

	originalLLM := callTreeReviewLLMFn
	callTreeReviewLLMFn = func(prompt string) (*treeReviewJudgment, error) {
		return &treeReviewJudgment{
			Moves: []treeReviewMove{{TagID: root.ID, ToParent: child.ID, Reason: "should be rejected"}},
		}, nil
	}
	t.Cleanup(func() { callTreeReviewLLMFn = originalLLM })

	result, err := ReviewHierarchyTrees("event", 14)
	if err != nil {
		t.Fatalf("ReviewHierarchyTrees: %v", err)
	}
	if result.MovesApplied != 0 {
		t.Fatalf("root demotion should be rejected, but move was applied")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected error for root demotion attempt")
	}
	assertAbstractRelationExists(t, db, root.ID, child.ID)
}
```

**Step 4: Run all hierarchy cleanup tests**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run "TestReviewHierarchy|TestBuildTagForest|TestValidateTree" -v`
Expected: ALL PASS

**Step 5: Commit**

```
test(tag-cleanup): add integration tests for merges, root merge source and demotion protection
```

---

## Task 9: 全量测试 + Build 验证

**Files:** 无新改动

**Step 1: Run targeted package tests**

Run: `cd backend-go && go test ./internal/domain/topicanalysis -v`
Expected: ALL PASS

**Step 2: Run full build**

Run: `cd backend-go && go build ./...`
Expected: BUILD SUCCESS

**Step 3: Commit (if any test fixes needed)**

只有测试修复才 commit，否则跳过。

---

## Task 10: 更新文档

**Files:**
- Modify: `docs/guides/tagging-flow.md:251-301`

**Step 1: 更新 Phase 4 章节描述**

在 `tagging-flow.md` 的 Phase 4 部分，更新描述以反映新的 merges 操作和根节点保护：

- Phase 4 整树审查的 LLM 返回增加 `merges` 类型
- 执行顺序：先 merges（消除重复）→ 再 moves（调整位置）→ 最后 new_abstracts（新建分组）
- 根节点保护：根节点不允许被 move 为子节点，也不允许作为 merge 的 source；根节点 label 不被 `refreshAbstractTag` 重生成

更新 Phase 4 表格后的描述段落为：

```markdown
Phase 4 不再只做"深度压缩"。它会把 `event`、`keyword`、`person` 三类抽象树序列化给 LLM 审查子标签归属是否合理。LLM 可同时返回三种操作：

- `merges`：合并树中语义重复的抽象标签（source 合并进 target），消除因多父关系导致的同一标签在树中重复出现。树的根节点不允许作为 merge source。
- `moves`：把标签迁移到树中已有父节点，或 `to_parent=0` 让其脱离为根节点。树的顶级根节点不允许被 move 为其他节点的子节点。
- `new_abstracts`：建议新的分组；系统先复用已有相似抽象或 slug 命中抽象，找不到才创建新抽象。

执行顺序：先 merges（消除重复节点）→ 再 moves（调整归属）→ 最后 new_abstracts（新建分组）。

此外，根抽象标签的 label 受保护：`refreshAbstractTag` 不会修改根节点的 label 和 slug，仅允许更新 description。
```

**Step 2: 更新总结流程图中的对应描述**

在 `tagging-flow.md` 底部总结部分无需改动，但确认 Phase 4 的描述一致。

**Step 3: Commit**

```
docs: update tagging-flow.md with merges and root protection for Phase 4
```

---

## 任务依赖图

```
Task 1 (struct)
  └→ Task 2 (schema) → Task 3 (prompt)
       └→ Task 4 (validate) → Task 5 (reviewOneTree)
                                └→ Task 6 (scheduler stats)
                                └→ Task 7 (root label protection)
                                └→ Task 8 (integration tests)
                                └→ Task 9 (full verification)
                                └→ Task 10 (docs)
```

Task 1-3 是串行的基础设施。Task 4-7 可以并行但建议按序。Task 8 依赖 4-7。Task 9 依赖所有。Task 10 最后。
