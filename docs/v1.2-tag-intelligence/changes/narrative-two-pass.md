# 叙事两遍处理 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将每个分类的叙事生成从单次 LLM 调用改为两遍：Pass 1 处理抽象树（保留层级结构+description），Pass 2 处理未分类 event 标签（带 description），两遍结果合并为同一分类叙事集。跨分类叙事从各分类叙事卡片总结。

**Architecture:** 改动集中在 `narrative` 包的 collector、generator、service 三层。Collector 新增两个采集函数分别收集抽象树和未分类 event；Generator 新增两个 prompt 构建函数和对应 LLM 调用函数；Service 的 `GenerateAndSaveForCategory` 改为两遍调用后合并。API 层和前端不变。

**Tech Stack:** Go, Gin, GORM, PostgreSQL, LLM via airouter

---

### Task 1: 新增抽象树结构化输入类型

**Files:**
- Modify: `backend-go/internal/domain/narrative/collector.go`

**Step 1: 在 collector.go 中添加抽象树节点类型**

在 `TagInput` 结构体后面添加：

```go
type AbstractTreeNode struct {
	ID           uint               `json:"id"`
	Label        string             `json:"label"`
	Category     string             `json:"category"`
	Description  string             `json:"description"`
	ArticleCount int                `json:"article_count"`
	IsAbstract   bool               `json:"is_abstract"`
	Children     []AbstractTreeNode `json:"children,omitempty"`
}
```

**Step 2: 添加抽象树采集函数**

```go
func CollectAbstractTreeInputsByCategory(date time.Time, categoryID uint) ([]AbstractTreeNode, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var feedIDs []uint
	if err := database.DB.Model(&models.Feed{}).
		Where("category_id = ?", categoryID).
		Pluck("id", &feedIDs).Error; err != nil || len(feedIDs) == 0 {
		return nil, nil
	}

	var tagIDs []uint
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.topic_tag_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", feedIDs, startOfDay, endOfDay).
		Pluck("article_topic_tags.topic_tag_id", &tagIDs)

	if len(tagIDs) == 0 {
		return nil, nil
	}

	var relations []models.TopicTagRelation
	database.DB.Where("relation_type = ? AND (parent_id IN ? OR child_id IN ?)",
		"abstract", tagIDs, tagIDs).Find(&relations)

	if len(relations) == 0 {
		return nil, nil
	}

	allIDs := make(map[uint]bool)
	parentOf := make(map[uint]uint)
	childrenOf := make(map[uint][]uint)
	for _, r := range relations {
		allIDs[r.ParentID] = true
		allIDs[r.ChildID] = true
		parentOf[r.ChildID] = r.ParentID
		childrenOf[r.ParentID] = append(childrenOf[r.ParentID], r.ChildID)
	}

	var allTagIDs []uint
	for id := range allIDs {
		allTagIDs = append(allTagIDs, id)
	}

	var tags []models.TopicTag
	database.DB.Where("id IN ? AND status = ?", allTagIDs, "active").Find(&tags)
	tagMap := make(map[uint]models.TopicTag, len(tags))
	for _, t := range tags {
		tagMap[t.ID] = t
	}

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?",
			allTagIDs, feedIDs, startOfDay, endOfDay).
		Group("article_topic_tags.topic_tag_id").
		Scan(&counts)

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	visited := make(map[uint]bool)
	var roots []uint
	for id := range allIDs {
		if _, hasParent := parentOf[id]; !hasParent {
			roots = append(roots, id)
		}
	}

	var result []AbstractTreeNode
	for _, rootID := range roots {
		if visited[rootID] {
			continue
		}
		tree := buildTree(rootID, tagMap, countMap, childrenOf, visited)
		if tree != nil {
			result = append(result, *tree)
		}
	}

	return result, nil
}

func buildTree(id uint, tagMap map[uint]models.TopicTag, countMap map[uint]int, childrenOf map[uint][]uint, visited map[uint]bool) *AbstractTreeNode {
	if visited[id] {
		return nil
	}
	visited[id] = true

	tag, ok := tagMap[id]
	if !ok {
		return nil
	}

	node := &AbstractTreeNode{
		ID:           tag.ID,
		Label:        tag.Label,
		Category:     tag.Category,
		Description:  tag.Description,
		ArticleCount: countMap[tag.ID],
		IsAbstract:   tag.Source == "abstract",
	}

	for _, childID := range childrenOf[id] {
		child := buildTree(childID, tagMap, countMap, childrenOf, visited)
		if child != nil {
			node.Children = append(node.Children, *child)
		}
	}

	return node
}
```

**Step 3: Run build to verify compilation**

Run: `cd backend-go && go build ./internal/domain/narrative/...`
Expected: PASS

**Step 4: Commit**

```bash
git add backend-go/internal/domain/narrative/collector.go
git commit -m "feat(narrative): add AbstractTreeNode type and CollectAbstractTreeInputsByCategory"
```

---

### Task 2: 新增未分类 event 标签采集函数

**Files:**
- Modify: `backend-go/internal/domain/narrative/collector.go`

**Step 1: 添加未分类 event 标签采集函数**

```go
func CollectUnclassifiedEventTagsByCategory(date time.Time, categoryID uint) ([]TagInput, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var feedIDs []uint
	if err := database.DB.Model(&models.Feed{}).
		Where("category_id = ?", categoryID).
		Pluck("id", &feedIDs).Error; err != nil || len(feedIDs) == 0 {
		return nil, nil
	}

	var tagIDs []uint
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.topic_tag_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", feedIDs, startOfDay, endOfDay).
		Pluck("article_topic_tags.topic_tag_id", &tagIDs)

	if len(tagIDs) == 0 {
		return nil, nil
	}

	var relatedIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ? AND (parent_id IN ? OR child_id IN ?)", "abstract", tagIDs, tagIDs).
		Pluck("parent_id", &relatedIDs)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ? AND (parent_id IN ? OR child_id IN ?)", "abstract", tagIDs, tagIDs).
		Pluck("child_id", &childIDs)
	relatedIDs = append(relatedIDs, childIDs...)
	relatedSet := make(map[uint]bool, len(relatedIDs))
	for _, id := range relatedIDs {
		relatedSet[id] = true
	}

	var tags []models.TopicTag
	database.DB.Where("id IN ? AND status = ? AND category = ? AND source != ?",
		tagIDs, "active", "event", "abstract").
		Order("quality_score DESC, feed_count DESC").
		Limit(50).
		Find(&tags)

	if len(tags) == 0 {
		return nil, nil
	}

	var filtered []models.TopicTag
	for _, t := range tags {
		if !relatedSet[t.ID] {
			filtered = append(filtered, t)
		}
	}
	tags = filtered

	if len(tags) == 0 {
		return nil, nil
	}

	tagIDs = make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?",
			tagIDs, feedIDs, startOfDay, endOfDay).
		Group("article_topic_tags.topic_tag_id").
		Scan(&counts)

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	var inputs []TagInput
	for _, tag := range tags {
		inputs = append(inputs, TagInput{
			ID:           tag.ID,
			Label:        tag.Label,
			Category:     tag.Category,
			Description:  tag.Description,
			ArticleCount: countMap[tag.ID],
			Source:       tag.Source,
		})
	}
	return inputs, nil
}
```

**Step 2: Run build**

Run: `cd backend-go && go build ./internal/domain/narrative/...`
Expected: PASS

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/collector.go
git commit -m "feat(narrative): add CollectUnclassifiedEventTagsByCategory"
```

---

### Task 3: 新增 Pass 1 Generator（抽象树叙事）

**Files:**
- Modify: `backend-go/internal/domain/narrative/generator.go`

**Step 1: 添加 Pass 1 system prompt**

```go
const abstractTreeNarrativeSystemPrompt = `你是一名专业的新闻叙事分析师。你收到了已整理的抽象标签树，每棵树代表一个已确认的主题分类，树中的层级关系已经过验证。

你的任务是基于这些已有的结构化主题信息生成叙事卡片。每条叙事应该：
1. 有一个简洁有力的标题（中文，不超过30字，必须是带判断的短句，不能是纯名词）
2. 有一段客观的摘要描述（中文，200-400字，包含关键事实和发展脉络）
3. 有一个状态标签：emerging（新出现）、continuing（持续发展）、splitting（分化）、merging（合并）、ending（趋于结束）
4. 关联到相关的标签ID（从树中选取）
5. 给出置信度分数（0-1）
6. 充分利用树中的层级关系和描述信息，理解事件之间的从属和关联
7. 不要为了凑数而强行合并不相关的树
8. 数量不固定，有几条写几条，没有就返回空数组

输出要求：
1. 顶层必须是 JSON 对象，且只能包含一个字段：narratives
2. narratives 必须是 JSON 数组；没有符合条件的叙事时，返回 {"narratives":[]}
3. narratives 数组中的每个元素都必须包含 title、summary、status、related_tag_ids、parent_ids、confidence_score 字段
4. status 只能是 emerging、continuing、splitting、merging、ending 之一
5. related_tag_ids 和 parent_ids 必须始终输出数组，即使为空也要输出 []
6. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀，禁止输出第二个 JSON 块`
```

**Step 2: 添加 Pass 1 prompt 构建函数**

```go
func buildAbstractTreeNarrativePrompt(trees []AbstractTreeNode, prev []PreviousNarrative) string {
	var sb strings.Builder

	sb.WriteString("## 今日已整理的抽象标签树\n\n")
	for i, tree := range trees {
		writeTreeNode(&sb, fmt.Sprintf("#### 树%d", i+1), tree, 0)
		sb.WriteString("\n")
	}

	if len(prev) > 0 {
		sb.WriteString("\n## 昨日分类叙事（供延续/对比参考）\n\n")
		for _, p := range prev {
			sb.WriteString(fmt.Sprintf("- [ID:%d] %s (状态:%s, 第%d代)\n  摘要: %s\n",
				p.ID, p.Title, p.Status, p.Generation, p.Summary))
		}
	}

	sb.WriteString("\n请基于以上已整理的抽象标签树，生成叙事卡片。充分利用层级关系和标签描述。\n")
	return sb.String()
}

func writeTreeNode(sb *strings.Builder, prefix string, node AbstractTreeNode, depth int) {
	indent := strings.Repeat("  ", depth)
	sb.WriteString(fmt.Sprintf("%s%s: %s (分类:%s, 文章数:%d", prefix, indent, node.Label, node.Category, node.ArticleCount))
	if node.IsAbstract {
		sb.WriteString(", 抽象标签")
	}
	if node.Description != "" {
		sb.WriteString(fmt.Sprintf(", 描述:%s", node.Description))
	}
	sb.WriteString(")\n")

	for _, child := range node.Children {
		writeTreeNode(sb, "-", child, depth+1)
	}
}
```

**Step 3: 添加 Pass 1 LLM 调用函数**

```go
func GenerateNarrativesFromAbstractTrees(ctx context.Context, trees []AbstractTreeNode, prevNarratives []PreviousNarrative) ([]NarrativeOutput, error) {
	if len(trees) == 0 {
		return nil, nil
	}

	prompt := buildAbstractTreeNarrativePrompt(trees, prevNarratives)

	temperature := 0.4
	maxTokens := 8000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: abstractTreeNarrativeSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"narratives": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"title":            {Type: "string", Description: "叙事标题，带判断的短句，不超过30字"},
							"summary":          {Type: "string", Description: "叙事摘要，200-400字"},
							"status":           {Type: "string", Description: "emerging/continuing/splitting/merging/ending"},
							"related_tag_ids":  {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"parent_ids":       {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"confidence_score": {Type: "number", Description: "0-1 置信度"},
						},
						Required: []string{"title", "summary", "status", "related_tag_ids", "parent_ids"},
					},
				},
			},
			Required: []string{"narratives"},
		},
		Metadata: map[string]any{
			"operation":      "abstract_tree_narrative_generation",
			"tree_count":     len(trees),
			"prev_count":     len(prevNarratives),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("abstract tree narrative AI call failed: %w", err)
	}

	logging.Infof("abstract-tree-narrative: raw LLM response length=%d", len(result.Content))

	outputs, err := parseNarrativeResponse(result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse abstract tree narrative response: %w", err)
	}

	allTagIDs := collectAllTagIDsFromTrees(trees)
	outputs = validateNarrativeOutputs(outputs, allTagIDs, prevNarratives)

	logging.Infof("generated %d narratives from %d abstract trees", len(outputs), len(trees))
	return outputs, nil
}

func collectAllTagIDsFromTrees(trees []AbstractTreeNode) []TagInput {
	var inputs []TagInput
	for _, t := range trees {
		collectTagIDsFromNode(t, &inputs)
	}
	return inputs
}

func collectTagIDsFromNode(node AbstractTreeNode, inputs *[]TagInput) {
	*inputs = append(*inputs, TagInput{ID: node.ID})
	for _, child := range node.Children {
		collectTagIDsFromNode(child, inputs)
	}
}
```

**Step 4: Run build**

Run: `cd backend-go && go build ./internal/domain/narrative/...`
Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/narrative/generator.go
git commit -m "feat(narrative): add Pass 1 generator for abstract tree narratives"
```

---

### Task 4: 新增 Pass 2 Generator（未分类 event 叙事）

**Files:**
- Modify: `backend-go/internal/domain/narrative/generator.go`

**Step 1: 添加 Pass 2 system prompt**

```go
const unclassifiedEventNarrativeSystemPrompt = `你是一名专业的新闻叙事分析师。你收到了一批尚未归入任何主题分类的独立事件标签，每个标签都附有描述。

你的任务是从这些未分类事件中识别叙事线索，将相关事件组织成连贯的故事线。每条叙事应该：
1. 有一个简洁有力的标题（中文，不超过30字，必须是带判断的短句，不能是纯名词）
2. 有一段客观的摘要描述（中文，200-400字，包含关键事实和发展脉络）
3. 有一个状态标签：emerging（新出现）、continuing（持续发展）、splitting（分化）、merging（合并）、ending（趋于结束）
4. 关联到相关的标签ID
5. 给出置信度分数（0-1）
6. 充分利用每个标签的描述信息来理解事件内容
7. 按因果、影响、主题关联分组，不要按语义相似度归类
8. 不要为了凑数而强行合并不相关的事件
9. 数量不固定，有几条写几条，没有就返回空数组

输出要求：
1. 顶层必须是 JSON 对象，且只能包含一个字段：narratives
2. narratives 必须是 JSON 数组；没有符合条件的叙事时，返回 {"narratives":[]}
3. narratives 数组中的每个元素都必须包含 title、summary、status、related_tag_ids、parent_ids、confidence_score 字段
4. status 只能是 emerging、continuing、splitting、merging、ending 之一
5. related_tag_ids 和 parent_ids 必须始终输出数组，即使为空也要输出 []
6. 只返回一个合法 JSON 对象，不要输出 Markdown 代码块、解释文字、前后缀，禁止输出第二个 JSON 块`
```

**Step 2: 添加 Pass 2 prompt 构建和 LLM 调用函数**

```go
func buildUnclassifiedEventNarrativePrompt(tags []TagInput, prev []PreviousNarrative) string {
	var sb strings.Builder

	sb.WriteString("## 今日未分类事件标签\n\n")
	for _, t := range tags {
		sb.WriteString(fmt.Sprintf("- [ID:%d] %s (文章数:%d", t.ID, t.Label, t.ArticleCount))
		if t.Description != "" {
			sb.WriteString(fmt.Sprintf(", 描述:%s", t.Description))
		}
		sb.WriteString(")\n")
	}

	if len(prev) > 0 {
		sb.WriteString("\n## 昨日分类叙事（供延续/对比参考）\n\n")
		for _, p := range prev {
			sb.WriteString(fmt.Sprintf("- [ID:%d] %s (状态:%s, 第%d代)\n  摘要: %s\n",
				p.ID, p.Title, p.Status, p.Generation, p.Summary))
		}
	}

	sb.WriteString("\n请从以上未分类事件标签中，识别叙事线索。充分利用描述信息理解每个事件的内容。\n")
	return sb.String()
}

func GenerateNarrativesFromUnclassifiedEvents(ctx context.Context, events []TagInput, prevNarratives []PreviousNarrative) ([]NarrativeOutput, error) {
	if len(events) == 0 {
		return nil, nil
	}

	prompt := buildUnclassifiedEventNarrativePrompt(events, prevNarratives)

	temperature := 0.4
	maxTokens := 8000
	result, err := airouter.NewRouter().Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: unclassifiedEventNarrativeSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"narratives": {
					Type: "array",
					Items: &airouter.SchemaProperty{
						Type: "object",
						Properties: map[string]airouter.SchemaProperty{
							"title":            {Type: "string", Description: "叙事标题，带判断的短句，不超过30字"},
							"summary":          {Type: "string", Description: "叙事摘要，200-400字"},
							"status":           {Type: "string", Description: "emerging/continuing/splitting/merging/ending"},
							"related_tag_ids":  {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"parent_ids":       {Type: "array", Items: &airouter.SchemaProperty{Type: "integer"}},
							"confidence_score": {Type: "number", Description: "0-1 置信度"},
						},
						Required: []string{"title", "summary", "status", "related_tag_ids", "parent_ids"},
					},
				},
			},
			Required: []string{"narratives"},
		},
		Metadata: map[string]any{
			"operation":    "unclassified_event_narrative_generation",
			"event_count":  len(events),
			"prev_count":   len(prevNarratives),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("unclassified event narrative AI call failed: %w", err)
	}

	logging.Infof("unclassified-event-narrative: raw LLM response length=%d", len(result.Content))

	outputs, err := parseNarrativeResponse(result.Content)
	if err != nil {
		return nil, fmt.Errorf("parse unclassified event narrative response: %w", err)
	}

	outputs = validateNarrativeOutputs(outputs, events, prevNarratives)

	logging.Infof("generated %d narratives from %d unclassified events", len(outputs), len(events))
	return outputs, nil
}
```

**Step 3: Run build**

Run: `cd backend-go && go build ./internal/domain/narrative/...`
Expected: PASS

**Step 4: Commit**

```bash
git add backend-go/internal/domain/narrative/generator.go
git commit -m "feat(narrative): add Pass 2 generator for unclassified event narratives"
```

---

### Task 5: 改造 Service 层为两遍调用

**Files:**
- Modify: `backend-go/internal/domain/narrative/service.go`

**Step 1: 重写 `GenerateAndSaveForCategory`**

将现有 `GenerateAndSaveForCategory` 方法体替换为两遍调用：

```go
func (s *NarrativeService) GenerateAndSaveForCategory(date time.Time, categoryID uint, categoryLabel string) (int, error) {
	prevNarratives, err := CollectPreviousNarratives(date, models.NarrativeScopeTypeFeedCategory, &categoryID)
	if err != nil {
		logging.Warnf("narrative: failed to collect previous narratives for category %d: %v", categoryID, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	var allOutputs []NarrativeOutput

	trees, err := CollectAbstractTreeInputsByCategory(date, categoryID)
	if err != nil {
		return 0, fmt.Errorf("collect abstract tree inputs for category %d: %w", categoryID, err)
	}
	if len(trees) > 0 {
		treeOutputs, err := GenerateNarrativesFromAbstractTrees(ctx, trees, prevNarratives)
		if err != nil {
			logging.Warnf("narrative: Pass 1 (abstract trees) failed for category %d: %v", categoryID, err)
		} else {
			allOutputs = append(allOutputs, treeOutputs...)
		}
	}

	events, err := CollectUnclassifiedEventTagsByCategory(date, categoryID)
	if err != nil {
		return 0, fmt.Errorf("collect unclassified event tags for category %d: %w", categoryID, err)
	}
	if len(events) > 0 {
		eventOutputs, err := GenerateNarrativesFromUnclassifiedEvents(ctx, events, prevNarratives)
		if err != nil {
			logging.Warnf("narrative: Pass 2 (unclassified events) failed for category %d: %v", categoryID, err)
		} else {
			allOutputs = append(allOutputs, eventOutputs...)
		}
	}

	if len(allOutputs) == 0 {
		logging.Infof("narrative: no narratives generated for category %d on %s", categoryID, date.Format("2006-01-02"))
		return 0, nil
	}

	if len(allOutputs) > 8 {
		allOutputs = allOutputs[:8]
	}

	catID := categoryID
	opts := &ScopeSaveOpts{
		ScopeType:  models.NarrativeScopeTypeFeedCategory,
		CategoryID: &catID,
		Label:      categoryLabel,
	}

	saved, err := saveNarratives(allOutputs, date, opts)
	if err != nil {
		return 0, fmt.Errorf("save category narratives: %w", err)
	}

	go feedbackNarrativesToTags(allOutputs)

	logging.Infof("narrative: saved %d narratives (pass1_trees=%d, pass2_events=%d) for category %d (%s) on %s",
		saved, len(trees), len(events), categoryID, categoryLabel, date.Format("2006-01-02"))
	return saved, nil
}
```

**Step 2: 删除旧的 `CollectTagInputsByCategory` 函数（在 collector.go 中）**

该函数已被两个新函数取代，从 `collector.go` 中移除 `CollectTagInputsByCategory`。

**Step 3: Run build**

Run: `cd backend-go && go build ./...`
Expected: PASS（确保没有其他地方引用 `CollectTagInputsByCategory`）

**Step 4: Run tests**

Run: `cd backend-go && go test ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add backend-go/internal/domain/narrative/service.go backend-go/internal/domain/narrative/collector.go
git commit -m "feat(narrative): rewrite GenerateAndSaveForCategory with two-pass approach"
```

---

### Task 6: 清理旧的无用代码

**Files:**
- Modify: `backend-go/internal/domain/narrative/collector.go`
- Modify: `backend-go/internal/domain/narrative/collector_test.go`

**Step 1: 保留 `CollectTagInputs` 及其辅助函数**

`CollectTagInputs` 用于全局（非分类）叙事采集，虽然当前 service 层不再直接调用，但 `collector_test.go` 有 5 个测试覆盖它（`TestCollectTagInputs_*`）。保留 `CollectTagInputs`、`collectAbstractTreeTags`、`collectUnclassifiedTags` 不做删除。

仅移除已被 Task 1/2 取代的 `CollectTagInputsByCategory` 函数（已在 Task 5 的 service 改造后无引用）。

**Step 2: Run build + test**

Run: `cd backend-go && go build ./... && go test ./...`
Expected: PASS（`CollectTagInputs` 的测试仍然引用旧函数，编译通过）

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/collector.go
git commit -m "refactor(narrative): remove old CollectTagInputsByCategory replaced by two-pass collectors"
```

---

### Task 7: 更新文档

**Files:**
- Modify: `docs/guides/topic-graph.md`

**Step 1: 更新叙事生成流程文档**

在 `## 叙事脉络（Narrative）` 部分的 `### 生成流程` 中，将原来的 5 步替换为：

```
1. `CollectActiveCategories` 采集当日活跃分类
2. 对每个活跃分类执行两遍生成：
   - Pass 1: `CollectAbstractTreeInputsByCategory` 采集该分类下的抽象标签树（保留层级+description）
     → `GenerateNarrativesFromAbstractTrees` 生成叙事
   - Pass 2: `CollectUnclassifiedEventTagsByCategory` 采集该分类下未归入抽象树的 event 标签（带 description）
     → `GenerateNarrativesFromUnclassifiedEvents` 生成叙事
   - 两遍结果合并保存为 `feed_category` 叙事集
3. `CollectCategoryNarrativeSummaries` 收集各分类叙事卡片
4. `GenerateCrossCategoryNarratives` 从各分类叙事中总结跨分类叙事
5. `markEndedNarratives` / `markEndedGlobalNarratives` 标记终结叙事
```

同时更新 `### 后端架构` 表格，添加新增文件：

| 文件 | 职责 |
|------|------|
| `collector.go` | 数据采集：抽象树、未分类 event 标签、前日叙事、活跃分类 |
| `generator.go` | AI 生成叙事（抽象树叙事 / 未分类 event 叙事 / 跨分类叙事） |

**Step 2: Commit**

```bash
git add docs/guides/topic-graph.md
git commit -m "docs: update narrative generation flow documentation for two-pass approach"
```

---

## 验证步骤

完成所有 Task 后，整体验证：

```bash
cd backend-go && go build ./... && go test ./...
cd front && pnpm exec nuxi typecheck
```

手动验证：启动后端，调用 `POST /api/narratives/regenerate` 触发叙事生成，检查日志中是否出现两遍处理的 log，检查数据库中 `narrative_summaries` 表是否正确生成分类级和全局级叙事。
