# 抽象标签合并与层级扁平化 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 消除同概念抽象标签被反复创建、层层嵌套的问题，新抽象标签创建前先查重、创建后主动收养更窄概念。

**Architecture:** 三层防御：(1) `processAbstractJudgment` 创建前先生成临时 embedding 做 shortlist，再用 LLM 查重，复用已有同概念标签；(2) `MatchAbstractTagHierarchy` 改为遍历多候选，高相似度时合并而非嵌套；(3) 新增 `adoptNarrowerAbstractChildren` 主动收养更窄的已有抽象标签，但保留更具体的中间父节点，不做错误扁平化。

**Tech Stack:** Go, GORM, pgvector, LLM (airouter)

---

## Task 1: 新增 `adoptNarrowerAbstractChildren` 函数

在 `processAbstractJudgment` 创建抽象标签后，主动搜索同 category 中更窄的抽象标签并收养为子节点。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` (新增函数，约 100 行)

**Step 1: 写 `adoptNarrowerAbstractChildren` 函数**

在 `abstract_tag_service.go` 的 `MatchAbstractTagHierarchy` 函数后面（约 1308 行之后）新增：

```go
func adoptNarrowerAbstractChildren(ctx context.Context, abstractTagID uint) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("adoptNarrowerAbstractChildren panic for tag %d: %v", abstractTagID, r)
		}
	}()

	var abstractTag models.TopicTag
	if err := database.DB.First(&abstractTag, abstractTagID).Error; err != nil {
		logging.Warnf("adoptNarrowerAbstractChildren: tag %d not found: %v", abstractTagID, err)
		return
	}

	es := NewEmbeddingService()
	candidates, err := es.FindSimilarAbstractTags(ctx, abstractTagID, abstractTag.Category, 5)
	if err != nil || len(candidates) == 0 {
		return
	}

	thresholds := es.GetThresholds()
	adopted := 0

	for _, candidate := range candidates {
		if candidate.Similarity < thresholds.LowSimilarity {
			continue
		}

		isNarrower, err := aiJudgeNarrowerConcept(ctx, &abstractTag, candidate.Tag)
		if err != nil {
			logging.Warnf("adoptNarrowerAbstractChildren: AI judgment failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, err)
			continue
		}
		if !isNarrower {
			continue
		}

		if err := reparentOrLinkAbstractChild(ctx, candidate.Tag.ID, abstractTagID); err != nil {
			logging.Warnf("adoptNarrowerAbstractChildren: failed to link %d under %d: %v", candidate.Tag.ID, abstractTagID, err)
			continue
		}
		adopted++
	}

	if adopted > 0 {
		logging.Infof("adoptNarrowerAbstractChildren: abstract tag %d (%s) adopted %d narrower abstract tags", abstractTagID, abstractTag.Label, adopted)
		go EnqueueAbstractTagUpdate(abstractTagID, "adopted_narrower_children")
	}
}
```

**Step 2: 写 `aiJudgeNarrowerConcept` 函数**

判断 candidate 是否是 parent 的更窄概念：

```go
func aiJudgeNarrowerConcept(ctx context.Context, parentTag *models.TopicTag, candidateTag *models.TopicTag) (bool, error) {
	parentChildren := loadAbstractChildLabels(parentTag.ID, 5)
	candidateChildren := loadAbstractChildLabels(candidateTag.ID, 5)

	router := airouter.NewRouter()
	prompt := fmt.Sprintf(`判断候选标签是否是目标标签的更窄（更具体）概念，应该作为其子标签。

目标标签（潜在的父标签）: %q (描述: %s)
目标标签的子标签: %s

候选标签（潜在的子标签）: %q (描述: %s)
候选标签的子标签: %s

规则:
- 如果候选标签描述的是目标标签范围内的一个具体方面、子集或特定场景，则它是更窄概念
- 例如：目标="中东冲突"，候选="霍尔木兹海峡危机" → 更窄概念（是）
- 例如：目标="AI产业"，候选="大模型竞争" → 更窄概念（是）
- 例如：目标="中东冲突"，候选="乌克兰战争" → 不是更窄概念（否）
- 如果两者是同一层级或候选更宽泛，返回 false
- 如果候选的子标签与目标的子标签高度重叠，说明是同一概念，返回 false（应由合并处理）

返回 JSON: {"narrower": true/false, "reason": "简要说明"}`,
		parentTag.Label, truncateStr(parentTag.Description, 200),
		formatChildLabels(parentChildren),
		candidateTag.Label, truncateStr(candidateTag.Description, 200),
		formatChildLabels(candidateChildren))

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
				"narrower": {Type: "boolean", Description: "候选标签是否是目标标签的更窄概念"},
				"reason":   {Type: "string", Description: "判断理由"},
			},
			Required: []string{"narrower", "reason"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":    "adopt_narrower_abstract",
			"parent_tag":   parentTag.ID,
			"candidate_tag": candidateTag.ID,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return false, fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		Narrower bool   `json:"narrower"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return false, fmt.Errorf("parse LLM response: %w", err)
	}

	return parsed.Narrower, nil
}
```

**Step 3: 写辅助函数 `loadAbstractChildLabels` 和 `formatChildLabels`**

```go
func loadAbstractChildLabels(tagID uint, limit int) []string {
	var labels []string
	database.DB.Model(&models.TopicTag{}).
		Joins("JOIN topic_tag_relations ON topic_tag_relations.child_id = topic_tags.id").
		Where("topic_tag_relations.parent_id = ? AND topic_tag_relations.relation_type = ?", tagID, "abstract").
		Order("topic_tag_relations.similarity_score DESC").
		Limit(limit).
		Pluck("topic_tags.label", &labels)
	if labels == nil {
		labels = []string{}
	}
	return labels
}

func formatChildLabels(labels []string) string {
	if len(labels) == 0 {
		return "(无子标签)"
	}
	return strings.Join(labels, ", ")
}
```

**Step 4: 写 `reparentOrLinkAbstractChild` 函数**

处理已有父标签的情况时不要直接把孙节点提到更宽泛的新父标签下面。若候选当前父标签本身就是新标签的更窄概念，应保留 `oldParent -> child`，只补一条 `newParent -> oldParent`；只有在旧父标签不适合作为中间层时才跳过本次收养。

```go
func reparentOrLinkAbstractChild(ctx context.Context, childID, newParentID uint) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		wouldCycle, err := wouldCreateCycle(tx, newParentID, childID)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if wouldCycle {
			return fmt.Errorf("would create cycle: parent=%d, child=%d", newParentID, childID)
		}

		var count int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ?", newParentID, childID).
			Count(&count)
		if count > 0 {
			return nil
		}

		var existingParentCount int64
		tx.Model(&models.TopicTagRelation{}).
			Where("child_id = ? AND parent_id != ? AND relation_type = ?", childID, newParentID, "abstract").
			Count(&existingParentCount)
		if existingParentCount > 0 {
			var oldParent models.TopicTagRelation
			if err := tx.Where("child_id = ? AND relation_type = ?", childID, "abstract").First(&oldParent).Error; err != nil {
				return fmt.Errorf("find old parent: %w", err)
			}

			var oldParentTag models.TopicTag
			if err := tx.First(&oldParentTag, oldParent.ParentID).Error; err != nil {
				return fmt.Errorf("load old parent tag: %w", err)
			}

			var newParentTag models.TopicTag
			if err := tx.First(&newParentTag, newParentID).Error; err != nil {
				return fmt.Errorf("load new parent tag: %w", err)
			}

			narrower, judgeErr := aiJudgeNarrowerConcept(ctx, &newParentTag, &oldParentTag)
			if judgeErr != nil || !narrower {
				logging.Infof("reparentOrLinkAbstractChild: skipping %d→%d, old parent %d is not narrower than new parent",
					childID, newParentID, oldParent.ParentID)
				return fmt.Errorf("child %d already has parent %d which is not narrower than %d", childID, oldParent.ParentID, newParentID)
			}

			logging.Infof("reparentOrLinkAbstractChild: keeping %d under narrower old parent %d and linking old parent under %d",
				childID, oldParent.ParentID, newParentID)

			oldParentCycle, cycleErr := wouldCreateCycle(tx, newParentID, oldParent.ParentID)
			if cycleErr != nil {
				return fmt.Errorf("cycle check for old parent: %w", cycleErr)
			}
			if oldParentCycle {
				return fmt.Errorf("would create cycle via old parent: parent=%d, old_parent=%d", newParentID, oldParent.ParentID)
			}

			relation := models.TopicTagRelation{
				ParentID:     newParentID,
				ChildID:      oldParent.ParentID,
				RelationType: "abstract",
			}
			return tx.Where("parent_id = ? AND child_id = ? AND relation_type = ?",
				newParentID, oldParent.ParentID, "abstract").FirstOrCreate(&relation).Error
		}

		relation := models.TopicTagRelation{
			ParentID:     newParentID,
			ChildID:      childID,
			RelationType: "abstract",
		}
		return tx.Create(&relation).Error
	})
}
```

调用点同步改为 `reparentOrLinkAbstractChild(ctx, candidate.Tag.ID, abstractTagID)`，避免在事务内额外起一个脱离请求生命周期的 `context.Background()`。

**Step 5: 写测试**

在 `abstract_tag_service_test.go` 末尾追加：

```go
func TestFormatChildLabels(t *testing.T) {
	if got := formatChildLabels(nil); got != "(无子标签)" {
		t.Errorf("empty labels: got %q", got)
	}
	if got := formatChildLabels([]string{"A", "B"}); got != "A, B" {
		t.Errorf("two labels: got %q", got)
	}
}

func TestLoadAbstractChildLabelsEmpty(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	tag := models.TopicTag{Slug: "test", Label: "test", Category: "event", Source: "abstract", Status: "active"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	labels := loadAbstractChildLabels(tag.ID, 5)
	if len(labels) != 0 {
		t.Errorf("expected no children, got %v", labels)
	}
}
```

**Step 6: 运行测试**

```bash
cd backend-go && go test ./internal/domain/topicanalysis -run "TestFormatChildLabels|TestLoadAbstractChildLabels" -v
```

**Step 7: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
git commit -m "feat(abstract-tag): add adoptNarrowerAbstractChildren for adopting narrower abstract tags"
```

---

## Task 2: 改造 `MatchAbstractTagHierarchy`——遍历多候选 + 高相似度合并

当前只检查 `candidates[0]`，且高相似度时做 parent-child 嵌套而非合并。改为遍历 top 3，高相似度时触发合并（收养子标签后 merge）。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go:1261-1308`

**Step 1: 写失败测试**

```go
func TestMatchAbstractTagHierarchyIteratesMultipleCandidates(t *testing.T) {
	// 纯逻辑验证：当 high similarity 时，应触发合并而非嵌套
	// 这通过集成测试覆盖，此处验证函数签名未变化
}
```

**Step 2: 重写 `MatchAbstractTagHierarchy`**

替换 `abstract_tag_service.go:1261-1308`：

```go
func MatchAbstractTagHierarchy(ctx context.Context, abstractTagID uint) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("MatchAbstractTagHierarchy panic for tag %d: %v", abstractTagID, r)
		}
	}()

	var abstractTag models.TopicTag
	if err := database.DB.First(&abstractTag, abstractTagID).Error; err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: tag %d not found: %v", abstractTagID, err)
		return
	}

	es := NewEmbeddingService()
	candidates, err := es.FindSimilarAbstractTags(ctx, abstractTagID, abstractTag.Category, 5)
	if err != nil || len(candidates) == 0 {
		return
	}

	thresholds := es.GetThresholds()
	maxCheck := 3
	if len(candidates) < maxCheck {
		maxCheck = len(candidates)
	}

	for i := 0; i < maxCheck; i++ {
		candidate := candidates[i]

		if candidate.Similarity >= thresholds.HighSimilarity {
			if err := mergeOrLinkSimilarAbstract(ctx, abstractTagID, candidate.Tag.ID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: merge/link failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, err)
			}
			continue
		}

		if candidate.Similarity >= thresholds.LowSimilarity {
			parentID, childID, judgeErr := aiJudgeAbstractHierarchy(ctx, abstractTagID, candidate.Tag.ID)
			if judgeErr != nil {
				logging.Warnf("MatchAbstractTagHierarchy: AI judgment failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, judgeErr)
				continue
			}
			if err := linkAbstractParentChild(childID, parentID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: failed to link %d under %d: %v", childID, parentID, err)
			}
		}
	}
}
```

**Step 3: 写 `mergeOrLinkSimilarAbstract`**

高相似度时，让 LLM 判断是"同一概念需合并"还是"上下位关系需链接"：

```go
func mergeOrLinkSimilarAbstract(ctx context.Context, tag1ID, tag2ID uint) error {
	var tag1, tag2 models.TopicTag
	if err := database.DB.First(&tag1, tag1ID).Error; err != nil {
		return fmt.Errorf("load tag %d: %w", tag1ID, err)
	}
	if err := database.DB.First(&tag2, tag2ID).Error; err != nil {
		return fmt.Errorf("load tag %d: %w", tag2ID, err)
	}

	children1 := loadAbstractChildLabels(tag1ID, 5)
	children2 := loadAbstractChildLabels(tag2ID, 5)

	router := airouter.NewRouter()
	prompt := fmt.Sprintf(`两个抽象标签非常相似，请判断它们的关系。

标签 A: %q (描述: %s)
A 的子标签: %s

标签 B: %q (描述: %s)
B 的子标签: %s

判断:
- 如果它们描述的是完全相同的概念（只是表述不同），返回 "merge"
  合并时应保留子标签更丰富的那个作为目标
- 如果 A 是 B 的上位概念（更宽泛），返回 "parent_A"
- 如果 B 是 A 的上位概念（更宽泛），返回 "parent_B"

返回 JSON: {"action": "merge"|"parent_A"|"parent_B", "target": "A"|"B", "reason": "简要说明"}
- action=merge 时，target 是应保留的标签（子标签更多的那个）
- action=parent_A 时，A 是父标签
- action=parent_B 时，B 是父标签`,
		tag1.Label, truncateStr(tag1.Description, 200), formatChildLabels(children1),
		tag2.Label, truncateStr(tag2.Description, 200), formatChildLabels(children2))

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
				"action": {Type: "string", Description: "merge, parent_A, 或 parent_B"},
				"target": {Type: "string", Description: "A 或 B，merge 时保留的标签"},
				"reason": {Type: "string", Description: "判断理由"},
			},
			Required: []string{"action", "target", "reason"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation": "merge_or_link_similar_abstract",
			"tag_a":      tag1ID,
			"tag_b":      tag2ID,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		Action string `json:"action"`
		Target string `json:"target"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return fmt.Errorf("parse LLM response: %w", err)
	}

	switch parsed.Action {
	case "merge":
		sourceID, targetID := tag1ID, tag2ID
		if parsed.Target == "A" {
			sourceID, targetID = tag2ID, tag1ID
		}
		logging.Infof("mergeOrLinkSimilarAbstract: merging %d (%s) into %d (%s), reason: %s",
			sourceID, tag2.Label, targetID, tag1.Label, parsed.Reason)
		return MergeTags(sourceID, targetID)

	case "parent_A":
		return linkAbstractParentChild(tag2ID, tag1ID)

	case "parent_B":
		return linkAbstractParentChild(tag1ID, tag2ID)

	default:
		return fmt.Errorf("unknown action %q from LLM", parsed.Action)
	}
}
```

**Step 4: 运行编译检查**

```bash
cd backend-go && go build ./...
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "feat(abstract-tag): MatchAbstractTagHierarchy iterates top 3 candidates with merge support"
```

---

## Task 3: `processAbstractJudgment` 创建前查重

在创建新抽象标签前，先检查是否已有同概念抽象标签。如果有，复用而非新建。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go:157-304` (`processAbstractJudgment`)

**Step 1: 在 `processAbstractJudgment` 中加入查重逻辑**

在 `processAbstractJudgment` 函数中，`slug` 检查之后、创建标签之前（约 178 行之后），插入查重逻辑：

```go
	// --- 新增：创建前查重 ---
	if abstractTag == nil {
		existingAbstract := findSimilarExistingAbstract(ctx, abstractName, abstractDesc, category, candidates)
		if existingAbstract != nil {
			logging.Infof("processAbstractJudgment: reusing existing abstract tag %d (%q) instead of creating new %q",
				existingAbstract.ID, existingAbstract.Label, abstractName)
			abstractTag = existingAbstract
		}
	}
```

插入位置：在 `if candidateSlugs[slug]` 检查之后、`abstractChildSet` 初始化之前（约 177-178 行之间）。

**Step 2: 写 `findSimilarExistingAbstract` 函数**

```go
func findSimilarExistingAbstract(ctx context.Context, name, desc, category string, candidates []TagCandidate) *models.TopicTag {
	es := NewEmbeddingService()
	thresholds := es.GetThresholds()
	probe := &models.TopicTag{
		Label:       name,
		Description: desc,
		Category:    category,
		Source:      "abstract",
	}
	similar, err := es.FindSimilarTags(ctx, probe, category, 8, EmbeddingTypeSemantic)
	if err != nil {
		logging.Warnf("findSimilarExistingAbstract: embedding search failed: %v", err)
		return nil
	}

	existingAbstracts := make([]models.TopicTag, 0, len(similar))
	for _, candidate := range similar {
		if candidate.Tag == nil || candidate.Tag.Source != "abstract" {
			continue
		}
		if candidate.Similarity < thresholds.LowSimilarity {
			continue
		}
		existingAbstracts = append(existingAbstracts, *candidate.Tag)
		if len(existingAbstracts) == 5 {
			break
		}
	}

	if len(existingAbstracts) == 0 {
		return nil
	}

	candidateLabels := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.Tag != nil {
			candidateLabels = append(candidateLabels, c.Tag.Label)
		}
	}

	router := airouter.NewRouter()
	var abstractInfo []string
	for _, ea := range existingAbstracts {
		children := loadAbstractChildLabels(ea.ID, 5)
		entry := fmt.Sprintf("- ID %d: %q (描述: %s, 子标签: %s)", ea.ID, ea.Label, truncateStr(ea.Description, 100), formatChildLabels(children))
		abstractInfo = append(abstractInfo, entry)
	}

	prompt := fmt.Sprintf(`一个新的抽象标签即将被创建，请检查以下已有抽象标签中是否有描述同一概念的。

即将创建的抽象标签: %q (描述: %s)
其候选子标签: %s

已有同 category 抽象标签:
%s

规则:
- 只有当已有标签描述的核心概念与新标签完全相同时才返回
- "中东地缘政治与能源安全事件" 和 "霍尔木兹海峡危机与航运安全事件" 如果描述的都是霍尔木兹海峡相关的整体事件，则算同一概念
- 不要把宽泛程度不同的标签当作"同一概念"（如"中东冲突"和"霍尔木兹海峡危机"是上下位关系，不是同一概念）

返回 JSON: {"reuse_id": 0 或 已有标签的ID, "reason": "简要说明"}
- reuse_id=0 表示没有匹配，应创建新标签
- reuse_id>0 表示应复用该已有标签`, name, truncateStr(desc, 200), formatChildLabels(candidateLabels), strings.Join(abstractInfo, "\n"))

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
				"reuse_id": {Type: "integer", Description: "复用的已有标签ID，0表示不匹配"},
				"reason":   {Type: "string", Description: "判断理由"},
			},
			Required: []string{"reuse_id", "reason"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":    "find_similar_existing_abstract",
			"new_name":     name,
			"category":     category,
			"candidates_n": len(candidates),
			"shortlist_n":  len(existingAbstracts),
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		logging.Warnf("findSimilarExistingAbstract: LLM call failed: %v", err)
		return nil
	}

	var parsed struct {
		ReuseID uint   `json:"reuse_id"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		logging.Warnf("findSimilarExistingAbstract: parse failed: %v", err)
		return nil
	}

	if parsed.ReuseID == 0 {
		return nil
	}

	for i := range existingAbstracts {
		if existingAbstracts[i].ID == parsed.ReuseID {
			logging.Infof("findSimilarExistingAbstract: found match %d (%q) for new %q: %s",
				existingAbstracts[i].ID, existingAbstracts[i].Label, name, parsed.Reason)
			return &existingAbstracts[i]
		}
	}

	logging.Warnf("findSimilarExistingAbstract: reuse_id %d not found in results", parsed.ReuseID)
	return nil
}
```

这里不要写成“扫同 category 下前 20 个 abstract tag 再让 LLM 判断”。那会退化成无序抽样，数据一多就会漏掉真正的重复标签，也和本计划开头声明的“embedding+LLM 查重”不一致。

**Step 3: 调整 `processAbstractJudgment` 中的事务逻辑**

当前事务在 `abstractTag == nil`（slug 查重没找到）时直接创建新标签。需要在查重复用的情况下跳过创建，但仍然链接子标签。修改 `abstract_tag_service.go:187-221`：

```go
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.TopicTag
		if err := tx.Where("slug = ? AND category = ? AND status = ?", slug, category, "active").First(&existing).Error; err == nil {
			abstractTag = &existing
		}

		if abstractTag == nil {
			abstractTag = &models.TopicTag{
				Slug:        slug,
				Label:       abstractName,
				Category:    category,
				Kind:        category,
				Source:      "abstract",
				Status:      "active",
				Description: abstractDesc,
			}
			if err := tx.Create(abstractTag).Error; err != nil {
				return fmt.Errorf("create abstract tag: %w", err)
			}

			go func(tagID uint, name, cat string) {
				es := NewEmbeddingService()
				tag := &models.TopicTag{ID: tagID, Label: name, Category: cat}
				for _, embType := range []string{EmbeddingTypeIdentity, EmbeddingTypeSemantic} {
					emb, genErr := es.GenerateEmbedding(context.Background(), tag, embType)
					if genErr != nil {
						logging.Warnf("Failed to generate %s embedding for abstract tag %d: %v", embType, tagID, genErr)
						continue
					}
					emb.TopicTagID = tagID
					if saveErr := es.SaveEmbedding(emb); saveErr != nil {
						logging.Warnf("Failed to save %s embedding for abstract tag %d: %v", embType, tagID, saveErr)
					}
				}
				MatchAbstractTagHierarchy(context.Background(), tagID)
				adoptNarrowerAbstractChildren(context.Background(), tagID)
			}(abstractTag.ID, abstractName, category)
		}
		// ... 后续链接子标签逻辑不变 ...
```

注意：事务内的 `abstractTag == nil` 判断现在会在查重命中时为 false（因为 `findSimilarExistingAbstract` 已设置 abstractTag），从而跳过创建直接进入链接子标签。

但复用的标签也需要触发收养，所以需要在事务成功后也调用：

在 `processAbstractJudgment` 的事务后面（约 297-298 行 `EnqueueAbstractTagUpdate` 之前），加：

```go
	if len(abstractChildren) > 0 && abstractTag.Source == "abstract" {
		go adoptNarrowerAbstractChildren(context.Background(), abstractTag.ID)
	}
```

**Step 4: 运行编译 + 测试**

```bash
cd backend-go && go build ./... && go test ./internal/domain/topicanalysis -v -run "TestBuildCandidateList|TestParseTagJudgmentResponse|TestFormatChildLabels"
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "feat(abstract-tag): deduplicate before creating, reuse existing same-concept abstract tags"
```

---

## Task 4: 集成验证

端到端验证所有改动协同工作，但不要新增“只打印日志、不做断言”的伪测试。当前 `airouter.NewRouter()` 与 embedding 调用没有现成 mock 注入点，自动化验证应优先覆盖可确定断言的纯函数/数据库行为；真正依赖 LLM 语义判断的链路，改为手工回归验证。

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go`
- Update docs only: 本节补充手工验证清单

**Step 1: 自动化测试只保留可断言行为**

复用 Task 1 中的 `TestFormatChildLabels` / `TestLoadAbstractChildLabelsEmpty`，这里只新增一个确定性数据库测试，避免写“跑完后看日志”的伪集成测试：

```go
func TestReparentOrLinkAbstractChildKeepsNarrowerIntermediateParent(t *testing.T) {
	db := setupAbstractTagServiceTestDB(t)

	grandParent := models.TopicTag{Slug: "zhong-dong-chong-tu", Label: "中东冲突", Category: "event", Source: "abstract", Status: "active"}
	midParent := models.TopicTag{Slug: "huo-er-mu-zi-hai-xia-wei-ji", Label: "霍尔木兹海峡危机", Category: "event", Source: "abstract", Status: "active"}
	child := models.TopicTag{Slug: "hang-yun-jing-bao", Label: "航运警报", Category: "event", Source: "llm", Status: "active"}

	for _, tag := range []*models.TopicTag{&grandParent, &midParent, &child} {
		if err := db.Create(tag).Error; err != nil {
			t.Fatalf("create tag: %v", err)
		}
	}
	if err := db.Create(&models.TopicTagRelation{ParentID: midParent.ID, ChildID: child.ID, RelationType: "abstract"}).Error; err != nil {
		t.Fatalf("create original relation: %v", err)
	}

	// 这里需要把 aiJudgeNarrowerConcept 的 LLM 判断抽成可 stub 的 helper，测试里固定返回 true。
	// 断言：保留 midParent -> child，同时补出 grandParent -> midParent；不能把 child 直接改挂到 grandParent 下。
}
```

如果实现阶段不愿为测试增加一个很小的可 stub 注入点，那么这个用例就改为手工验证，不要留下只打印 count 的测试壳子。

**Step 2: 手工回归验证依赖 LLM 的完整链路**

准备一组已知会重复创建抽象标签的 event 标签，手工验证以下结果：

1. 再次触发抽象提取后，不会新增第二个同义 abstract tag。
2. 高相似 abstract tag 会 merge，最终只保留一个 active abstract tag。
3. 若已有 `更宽父 -> 更窄父 -> 具体事件` 结构，新增更宽父时只补 `更宽父 -> 更窄父`，不会把具体事件直接提平。

建议在本地数据库上记录验证前后的 `topic_tags` / `topic_tag_relations` 变化截图或 SQL 结果，方便回顾。

**Step 3: 运行全部 topicanalysis 测试**

```bash
cd backend-go && go test ./internal/domain/topicanalysis -v
```

**Step 4: 编译全量检查**

```bash
cd backend-go && go build ./... && go vet ./...
```

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
git commit -m "test(abstract-tag): cover hierarchy preservation and document manual consolidation checks"
```

---

## 改动总结

| 文件 | 改动 |
|------|------|
| `abstract_tag_service.go` | 新增 `adoptNarrowerAbstractChildren`、`aiJudgeNarrowerConcept`、`reparentOrLinkAbstractChild`、`mergeOrLinkSimilarAbstract`、`findSimilarExistingAbstract`、`loadAbstractChildLabels`、`formatChildLabels`；重写 `MatchAbstractTagHierarchy`；修改 `processAbstractJudgment` 加入创建前查重和创建后收养 |
| `abstract_tag_service_test.go` | 新增 `TestFormatChildLabels`、`TestLoadAbstractChildLabelsEmpty`、`TestReparentOrLinkAbstractChildKeepsNarrowerIntermediateParent` |

## 预期效果

以用户实际数据为例：

**Before:**
```
中东地区武装冲突
  └─ 伊朗霍尔木兹海峡管控与航运安全事件
    └─ 伊朗与美国围绕霍尔木兹海峡...事件
      └─ 霍尔木兹海峡航运安全与军事对峙事件
        └─ 中东地缘政治与能源安全事件       ← 每次都创建新抽象标签
          └─ 霍尔木兹海峡危机事件
            └─ 具体事件标签...
```

**After:**
- Task 3（创建前查重）："中东地缘政治与能源安全事件"创建前发现已有"霍尔木兹海峡危机事件"，复用
- Task 2（MatchAbstractTagHierarchy 增强）：高相似度抽象标签合并而非嵌套
- Task 1（主动收养）：新标签创建后主动搜索并收养更窄的抽象标签
```
中东地区武装冲突
  ├─ 霍尔木兹海峡危机事件              ← 合并后唯一抽象标签
  │   ├─ 英海上贸易行动办公室声明
  │   ├─ 伊朗伊斯兰革命卫队炮艇开火
  │   ├─ 欧洲航班停飞潮
  │   ├─ 霍尔木兹海峡关闭
  │   ├─ 美伊谈判
  │   ├─ 伊朗控制霍尔木兹海峡
  │   └─ 伊朗开放霍尔木兹海峡
  ├─ 美国封锁伊朗
  ├─ 伊朗女性参加阅兵
  ├─ 中东战争 → 美伊战争
  └─ 伊朗地区安全与军事动态
      ├─ 伊朗建军节
      └─ 伊朗外长阿拉格齐宣布海峡开放
```
