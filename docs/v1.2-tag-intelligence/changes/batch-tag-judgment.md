# Batch Tag Judgment 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将标签匹配从"只看最高相似度候选"改为"所有候选整体送 LLM 判断"，支持每个候选独立判定（merge/abstract/none），候选多时分批处理。

**Architecture:**
- `TagMatch()` 不再按阈值分支（high_similarity / ai_judgment），改为只要有 >= LowSimilarity 的候选就统一返回 `"candidates"` 类型
- `callLLMForTagJudgment()` 支持分批：每批最多 `judgmentBatchSize`（默认 8）个候选，后续批次携带前批结果作为上下文
- LLM 返回 JSON 数组，每个元素是对单个候选的独立判断：merge / abstract / none
- `ExtractAbstractTag()` 汇总多轮结果，处理混合操作（部分合并、部分抽象、部分独立）
- `findOrCreateTag()` 简化为：exact → 复用，candidates → LLM 批量判断，其余 → 新建

**Tech Stack:** Go, Gin, GORM, LLM JSON mode

---

## 核心改动概览

### 改动前的流程

```
TagMatch() → best = candidates[0]
  ├── exact → 复用
  ├── high_similarity (>= 0.97)
  │   ├── 普通标签 → 自动复用
  │   └── 抽象标签 → 创建子标签
  ├── ai_judgment (0.78~0.97) → LLM 判断（最多 3 个候选）
  │   ├── merge → 合并到 best
  │   ├── abstract → 创建抽象标签
  │   └── none → 跳出，创建新标签 ← 问题：跳过了其他候选
  ├── low_similarity (< 0.78) → 创建新标签
  └── no_match → 创建新标签
```

### 改动后的流程

```
TagMatch() → 所有 candidates >= LowSimilarity（top 20）
  ├── exact → 复用（不变）
  ├── candidates（有 >= LowSimilarity 的候选）
  │   → ExtractAbstractTag(ctx, candidates, newLabel, category)
  │   → 内部分批调用 callLLMForTagJudgment:
  │     Round 1: candidates[0:8] → LLM 返回 JSON 数组
  │       [merge A, none B, abstract C, ...]
  │     Round 2: candidates[8:16] + 前批结果摘要 → LLM 返回更多判断
  │     ...
  │   → 汇总所有轮次结果:
  │     ├── 有 merge → 合并新标签到 LLM 指定的最佳候选
  │     ├── 有 abstract → 为相关候选创建抽象标签
  │     └── none → 候选各自独立
  │   → 返回 PrimaryAction（新标签的最终决策）
  └── no_match → 创建新标签
```

### 关键行为变化

| 场景 | 改动前 | 改动后 |
|------|--------|--------|
| 新标签 A 相似于 B(0.96) 和 C(0.93)，LLM 对 B 返回 none | 跳过 C，创建新标签 | LLM 看到所有候选，per-candidate 判断：B→none, C→merge |
| 新标签 A 相似于抽象标签 B(0.98) 和普通标签 C(0.97) | 自动创建 A 为 B 的子标签，跳过 C | LLM 分别判断：B→abstract, C→merge，A 合并到 C |
| 候选数量 > 3 | 最多判断 3 个 | 分批处理，每批 8 个，前批结果作为上下文 |
| 多个候选应不同处理 | 全部同一个 action | 每个候选独立 action，混合 merge/abstract/none |

---

## Task 1: 修改 `TagMatchResult` 和 `TagMatch()`

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go:42-291`

**Step 1: 修改 `TagMatchResult` 结构**

把 `MatchType` 的可选值从 `"exact" | "high_similarity" | "ai_judgment" | "low_similarity" | "no_match"` 改为 `"exact" | "candidates" | "no_match"`。

删除 `ShouldCreate` 字段（由调用方根据 `MatchType` 决定）。

```go
type TagMatchResult struct {
	MatchType   string          // "exact", "candidates", "no_match"
	ExistingTag *models.TopicTag // "exact" 时使用
	Similarity  float64         // 最高相似度（用于日志）
	Candidates  []TagCandidate  // "candidates" 时使用：所有 >= LowSimilarity 的候选
}
```

**Step 2: 修改 `TagMatch()` 方法**

去掉 high_similarity / ai_judgment 分支。当有候选 >= LowSimilarity 时，统一返回 `"candidates"`，把所有满足条件的候选都放进去。

```go
func (s *EmbeddingService) TagMatch(ctx context.Context, label, category string, aliases string) (*TagMatchResult, error) {
	// Step 1: Check for exact match by slug in the same category (active tags only)
	slug := topictypes.Slugify(label)
	var existingTag models.TopicTag
	err := database.DB.Scopes(activeTagFilter).Where("slug = ? AND category = ?", slug, category).First(&existingTag).Error
	if err == nil {
		return &TagMatchResult{
			MatchType:   "exact",
			ExistingTag: &existingTag,
			Similarity:  1.0,
		}, nil
	}

	// Step 2: Check for alias match (active tags only)
	if aliases != "" {
		var aliasTags []models.TopicTag
		if err := database.DB.Scopes(activeTagFilter).Where("category = ?", category).Find(&aliasTags).Error; err == nil {
			for _, t := range aliasTags {
				if containsAlias(t.Aliases, label) {
					return &TagMatchResult{
						MatchType:   "exact",
						ExistingTag: &t,
						Similarity:  1.0,
					}, nil
				}
			}
		}
	}

	// Step 3: Vector similarity matching — return ALL candidates above LowSimilarity
	candidate := &models.TopicTag{
		Label:    label,
		Category: category,
		Aliases:  aliases,
	}

	candidates, err := s.FindSimilarTags(ctx, candidate, category, 20)
	if err != nil {
		return &TagMatchResult{
			MatchType: "no_match",
		}, nil
	}

	var validCandidates []TagCandidate
	for _, c := range candidates {
		if c.Similarity >= s.thresholds.LowSimilarity {
			validCandidates = append(validCandidates, c)
		}
	}

	if len(validCandidates) == 0 {
		return &TagMatchResult{
			MatchType:  "no_match",
			Similarity: bestSimilarity(candidates),
		}, nil
	}

	return &TagMatchResult{
		MatchType:   "candidates",
		Similarity:  validCandidates[0].Similarity,
		Candidates:  validCandidates,
	}, nil
}

func bestSimilarity(candidates []TagCandidate) float64 {
	if len(candidates) == 0 {
		return 0
	}
	return candidates[0].Similarity
}
```

注意 `FindSimilarTags` 的 `limit` 从 5 提到 20，确保覆盖足够多的候选。

**Step 3: 运行现有测试确认无破坏**

Run: `cd backend-go && go test ./internal/domain/topicanalysis/... -v -run "TestBuildTagEmbeddingText|TestContainsAlias"`
Expected: PASS（这些测试不涉及 TagMatch）

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/embedding.go
git commit -m "refactor: unify TagMatch to return all candidates without threshold branching"
```

---

## Task 2: 修改 LLM 判断返回结构和 prompt

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go:599-812`

**Step 1: 修改 `tagJudgment` 结构 — 增加候选标识字段**

```go
type tagJudgment struct {
	CandidateLabel string // 判断针对的候选标签名称
	Action         string // "merge", "abstract", "none"
	MergeTarget    string // action=merge: 指定合并目标候选标签名
	MergeLabel     string // action=merge: 合并后统一名称
	AbstractName   string // action=abstract
	Description    string // action=abstract
	Reason         string
}
```

**Step 2: 修改 `callLLMForTagJudgment` 支持分批 + 数组返回**

新增常量和辅助函数：

```go
const judgmentBatchSize = 8

type previousRoundResult struct {
	CandidateLabel string
	Action         string
	TargetLabel    string // merge target or abstract name
}
```

修改 `callLLMForTagJudgment` 改为分批调用，返回 `[]tagJudgment`：

```go
func callLLMForTagJudgment(ctx context.Context, candidates []TagCandidate, newLabel string, category string, narrativeContext string) ([]tagJudgment, error) {
	var allJudgments []tagJudgment
	var previousResults []previousRoundResult

	for batchStart := 0; batchStart < len(candidates); batchStart += judgmentBatchSize {
		batchEnd := batchStart + judgmentBatchSize
		if batchEnd > len(candidates) {
			batchEnd = len(candidates)
		}
		batch := candidates[batchStart:batchEnd]

		judgments, err := callLLMForTagJudgmentBatch(ctx, batch, newLabel, category, narrativeContext, previousResults)
		if err != nil {
			logging.Warnf("Tag judgment batch %d-%d failed: %v", batchStart, batchEnd, err)
			// 前批失败不阻塞后续批次，继续处理
			continue
		}

		for _, j := range judgments {
			allJudgments = append(allJudgments, j)
			previousResults = append(previousResults, previousRoundResult{
				CandidateLabel: j.CandidateLabel,
				Action:         j.Action,
				TargetLabel:    j.MergeTarget,
			})
		}
	}

	if len(allJudgments) == 0 {
		return nil, fmt.Errorf("all judgment batches failed")
	}

	return allJudgments, nil
}
```

**Step 3: 实现 `callLLMForTagJudgmentBatch` — 单批次 LLM 调用**

```go
func callLLMForTagJudgmentBatch(ctx context.Context, batch []TagCandidate, newLabel string, category string, narrativeContext string, previousResults []previousRoundResult) ([]tagJudgment, error) {
	router := airouter.NewRouter()
	prompt := buildBatchTagJudgmentPrompt(batch, newLabel, category, previousResults)

	if narrativeContext != "" {
		prompt += fmt.Sprintf("\n\nAdditional context from narrative analysis:\n%s\nUse this context to help determine if these tags belong to the same thematic thread.", narrativeContext)
	}

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON array."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "array",
			Items: &airouter.JSONSchema{
				Type: "object",
				Properties: map[string]airouter.SchemaProperty{
					"candidate_label": {Type: "string", Description: "判断针对的候选标签名称"},
					"action":          {Type: "string", Description: "判断结果：merge 表示新标签与该候选是同一概念应合并，abstract 表示需要创建抽象概括标签，none 表示无关联"},
					"merge_target":    {Type: "string", Description: "action=merge 时必填：指定新标签应合并到哪个候选（填候选标签名称）"},
					"merge_label":     {Type: "string", Description: "action=merge 时必填：合并后的统一名称"},
					"abstract_name":   {Type: "string", Description: "action=abstract 时必填：抽象标签名称（1-160字）"},
					"description":     {Type: "string", Description: "action=abstract 时必填：抽象标签中文客观描述（500字以内）"},
					"reason":          {Type: "string", Description: "判断理由"},
				},
				Required: []string{"candidate_label", "action", "reason"},
			},
		},
		Temperature: func() *float64 { f := 0.3; return &f }(),
		Metadata: map[string]any{
			"operation":       "tag_judgment_batch",
			"candidate_count": len(batch),
			"new_label":       newLabel,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return parseBatchTagJudgmentResponse(result.Content)
}
```

**Step 4: 实现 `parseBatchTagJudgmentResponse`**

```go
func parseBatchTagJudgmentResponse(content string) ([]tagJudgment, error) {
	content = jsonutil.SanitizeLLMJSON(content)

	var parsed []struct {
		CandidateLabel string `json:"candidate_label"`
		Action         string `json:"action"`
		MergeTarget    string `json:"merge_target"`
		MergeLabel     string `json:"merge_label"`
		AbstractName   string `json:"abstract_name"`
		Description    string `json:"description"`
		Reason         string `json:"reason"`
	}

	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		// 如果数组解析失败，尝试单对象 fallback
		return parseSingleJudgmentFallback(content)
	}

	var judgments []tagJudgment
	for _, p := range parsed {
		action := strings.ToLower(strings.TrimSpace(p.Action))
		if action != ActionMerge && action != ActionAbstract && action != ActionNone {
			continue
		}

		j := tagJudgment{
			CandidateLabel: strings.TrimSpace(p.CandidateLabel),
			Action:         action,
			Reason:         p.Reason,
		}

		switch action {
		case ActionMerge:
			j.MergeTarget = strings.TrimSpace(p.MergeTarget)
			j.MergeLabel = strings.TrimSpace(p.MergeLabel)
			if j.MergeLabel == "" {
				j.MergeLabel = j.CandidateLabel
			}
		case ActionAbstract:
			j.AbstractName = strings.TrimSpace(p.AbstractName)
			if j.AbstractName == "" {
				continue
			}
			if len(j.AbstractName) > maxAbstractNameLen {
				j.AbstractName = j.AbstractName[:maxAbstractNameLen]
			}
			j.Description = strings.TrimSpace(p.Description)
			if len(j.Description) > 500 {
				j.Description = j.Description[:500]
			}
		}

		judgments = append(judgments, j)
	}

	if len(judgments) == 0 {
		return nil, fmt.Errorf("no valid judgments parsed from LLM response")
	}

	return judgments, nil
}

func parseSingleJudgmentFallback(content string) ([]tagJudgment, error) {
	// fallback: 解析单对象响应为数组
	var parsed struct {
		Action       string `json:"action"`
		MergeLabel   string `json:"merge_label"`
		AbstractName string `json:"abstract_name"`
		Description  string `json:"description"`
		Reason       string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as array or object: %w", err)
	}

	action := strings.ToLower(strings.TrimSpace(parsed.Action))
	if action != ActionMerge && action != ActionAbstract && action != ActionNone {
		return nil, fmt.Errorf("invalid action %q", parsed.Action)
	}

	return []tagJudgment{{
		Action:       action,
		MergeLabel:   strings.TrimSpace(parsed.MergeLabel),
		AbstractName: strings.TrimSpace(parsed.AbstractName),
		Description:  strings.TrimSpace(parsed.Description),
		Reason:       parsed.Reason,
	}}, nil
}
```

**Step 5: 修改 `buildBatchTagJudgmentPrompt` — per-candidate prompt**

替换原来的 `buildTagJudgmentPrompt`。三个 category（person / event / keyword default）的 prompt 都需要改为 per-candidate 格式。

```go
func buildCandidateList(candidates []TagCandidate, newLabel string) string {
	var parts []string
	for _, c := range candidates {
		if c.Tag != nil {
			tagType := "normal"
			if c.Tag.Source == "abstract" {
				tagType = "abstract"
			}
			desc := ""
			if c.Tag.Description != "" {
				runes := []rune(c.Tag.Description)
				if len(runes) > 200 {
					desc = fmt.Sprintf(" (description: %s...)", string(runes[:200]))
				} else {
					desc = fmt.Sprintf(" (description: %s)", c.Tag.Description)
				}
			}
			parts = append(parts, fmt.Sprintf("- %q (similarity: %.2f, type: %s)%s", c.Tag.Label, c.Similarity, tagType, desc))
		}
	}
	parts = append(parts, fmt.Sprintf("- %q (new tag)", newLabel))
	return strings.Join(parts, "\n")
}

func buildPreviousResultsSummary(results []previousRoundResult) string {
	if len(results) == 0 {
		return ""
	}
	var parts []string
	for _, r := range results {
		switch r.Action {
		case ActionMerge:
			parts = append(parts, fmt.Sprintf("- %q → merge (into %q)", r.CandidateLabel, r.TargetLabel))
		case ActionAbstract:
			parts = append(parts, fmt.Sprintf("- %q → abstract (%q)", r.CandidateLabel, r.TargetLabel))
		case ActionNone:
			parts = append(parts, fmt.Sprintf("- %q → none (independent)", r.CandidateLabel))
		}
	}
	return fmt.Sprintf("Previous round decisions:\n%s\n", strings.Join(parts, "\n"))
}

func buildBatchTagJudgmentPrompt(candidates []TagCandidate, newLabel string, category string, previousResults []previousRoundResult) string {
	tagList := buildCandidateList(candidates, newLabel)
	prevSummary := buildPreviousResultsSummary(previousResults)

	perCandidateRule := `
For each candidate tag, return an independent judgment:
- merge: the new tag and this candidate are the SAME concept — they should be unified
  - merge_target: fill with the candidate label that the new tag should merge into
  - merge_label: the unified name after merge
- abstract: the new tag and this candidate are DISTINCT but RELATED concepts — they need an abstract parent tag
  - abstract_name: name for the abstract parent tag (1-160 chars)
  - description: objective Chinese description (≤500 chars)
- none: the new tag has no meaningful relationship with this candidate

Important:
- If similarity >= 0.97 with a normal candidate, usually merge is correct
- If a candidate is an abstract tag, merging into one of its concrete children is often better than creating another child
- You can return different actions for different candidates
- merge_target should reference one of the candidate labels from the list above

Return a JSON array:
[
  {"candidate_label": "候选标签名", "action": "merge/abstract/none", "merge_target": "目标候选", "merge_label": "统一名称", "abstract_name": "抽象名", "description": "描述", "reason": "理由"},
  ...
]`

	switch category {
	case "person":
		return fmt.Sprintf(`以下是语义相似的人物标签:
%s
%s
请为每个候选标签独立判断与新标签 %q 的关系:
%s

判断标准:
- 只有同一人物的不同叫法才用 merge
- 不同人物只有在紧密的组织/角色关系（同团队、同家族、师徒关系）时才用 abstract
- 仅仅"同领域"、"同事件参与者"不构成 abstract 的充分理由，用 none
- 绝不要创建形如"人物A与人物B"的抽象标签——如果 abstract_name 只是列举人名，用 none`, tagList, prevSummary, newLabel, perCandidateRule)

	case "event":
		return fmt.Sprintf(`以下是语义相似的事件标签:
%s
%s
请为每个候选标签独立判断与新标签 %q 的关系:
%s

判断标准:
- 只有明确是同一事件的不同表述才用 merge
- 相似但独立的事件（如同一系列的不同事件）用 abstract
- 没有实质关联（只是语义相似度碰巧高）时用 none`, tagList, prevSummary, newLabel, perCandidateRule)

	default:
		return fmt.Sprintf(`Given these semantically similar tags:
%s
%s
Judge the relationship between the new tag %q and each candidate independently:
%s

Criteria:
- merge: same concept with different names/spellings
- abstract: distinct but related concepts sharing a common theme
- none: no meaningful relationship beyond semantic similarity
- abstract_name/merge_label should be in the original language of the tags
- description must be objective, factual — no subjective opinions`, tagList, prevSummary, newLabel, perCandidateRule)
	}
}
```

**Step 6: 编写单元测试**

在 `abstract_tag_service_test.go` 中新增测试：

```go
func TestBuildCandidateList(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{Label: "React", Source: "abstract"}, Similarity: 0.96},
		{Tag: &models.TopicTag{Label: "Vue", Source: "heuristic"}, Similarity: 0.93},
	}
	result := buildCandidateList(candidates, "Svelte")
	if !strings.Contains(result, `type: abstract`) {
		t.Error("should mark abstract candidates")
	}
	if !strings.Contains(result, `type: normal`) {
		t.Error("should mark non-abstract candidates as normal")
	}
	if !strings.Contains(result, "Svelte (new tag)") {
		t.Error("should include new tag")
	}
}

func TestParseBatchTagJudgmentResponse(t *testing.T) {
	input := `[{"candidate_label":"GPT-4","action":"merge","merge_target":"GPT-4","merge_label":"GPT-4","reason":"same"},{"candidate_label":"Vue","action":"none","reason":"different"}]`
	results, err := parseBatchTagJudgmentResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 judgments, got %d", len(results))
	}
	if results[0].Action != ActionMerge {
		t.Errorf("expected merge, got %s", results[0].Action)
	}
	if results[0].MergeTarget != "GPT-4" {
		t.Errorf("expected merge target 'GPT-4', got %q", results[0].MergeTarget)
	}
	if results[1].Action != ActionNone {
		t.Errorf("expected none, got %s", results[1].Action)
	}
}

func TestParseBatchTagJudgmentResponseFallback(t *testing.T) {
	input := `{"action":"merge","merge_label":"GPT-4","reason":"same"}`
	results, err := parseSingleJudgmentFallback(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 judgment, got %d", len(results))
	}
	if results[0].Action != ActionMerge {
		t.Errorf("expected merge, got %s", results[0].Action)
	}
}

func TestBuildPreviousResultsSummary(t *testing.T) {
	results := []previousRoundResult{
		{CandidateLabel: "React", Action: ActionMerge, TargetLabel: "React"},
		{CandidateLabel: "Vue", Action: ActionNone},
	}
	summary := buildPreviousResultsSummary(results)
	if !strings.Contains(summary, "React → merge") {
		t.Error("should show merge result")
	}
	if !strings.Contains(summary, "Vue → none") {
		t.Error("should show none result")
	}
}
```

Run: `cd backend-go && go test ./internal/domain/topicanalysis/... -v -run "TestBuildCandidateList|TestParseBatch|TestBuildPreviousResultsSummary"`
Expected: PASS

**Step 7: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git add backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
git commit -m "feat: batch tag judgment — per-candidate JSON array response with multi-round processing"
```

---

## Task 3: 修改 `selectMergeTarget` 支持按 label 查找

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go:608-623`

**Step 1: 扩展 `selectMergeTarget`**

新增按 label 匹配的逻辑，优先级：slug 精确匹配 → label 匹配 → 非抽象优先 → fallback 首个候选。

```go
func selectMergeTarget(candidates []TagCandidate, mergeTarget string, mergeLabel string) *models.TopicTag {
	// Priority 1: match by slug against mergeTarget
	mergeTargetSlug := topictypes.Slugify(mergeTarget)
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Slug == mergeTargetSlug {
			return c.Tag
		}
	}

	// Priority 2: match by slug against mergeLabel
	mergeLabelSlug := topictypes.Slugify(mergeLabel)
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Slug == mergeLabelSlug {
			return c.Tag
		}
	}

	// Priority 3: match by label string
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Label == mergeTarget {
			return c.Tag
		}
	}

	// Priority 4: first non-abstract candidate
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Source != "abstract" {
			return c.Tag
		}
	}

	// Fallback: first candidate with a tag
	for _, c := range candidates {
		if c.Tag != nil {
			return c.Tag
		}
	}
	return nil
}
```

**Step 2: 更新现有测试**

修改 `TestSelectMergeTarget`，适配新签名（3 个参数）：

```go
func TestSelectMergeTarget(t *testing.T) {
	t.Run("matches by merge target slug", func(t *testing.T) {
		candidates := []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "GPT-4", Slug: "gpt-4"}},
			{Tag: &models.TopicTag{ID: 2, Label: "ChatGPT", Slug: "chatgpt"}},
		}
		target := selectMergeTarget(candidates, "GPT-4", "GPT-4o")
		if target == nil || target.ID != 1 {
			t.Errorf("expected tag ID 1, got %v", target)
		}
	})

	t.Run("matches by merge label slug when target not found", func(t *testing.T) {
		candidates := []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "React", Slug: "react"}},
		}
		target := selectMergeTarget(candidates, "React.js", "React")
		if target == nil || target.ID != 1 {
			t.Errorf("expected tag ID 1 via merge label, got %v", target)
		}
	})

	t.Run("prefers non-abstract candidate", func(t *testing.T) {
		candidates := []TagCandidate{
			{Tag: &models.TopicTag{ID: 1, Label: "编程语言", Slug: "bian-cheng-yu-yan", Source: "abstract"}},
			{Tag: &models.TopicTag{ID: 2, Label: "Python", Slug: "python", Source: "heuristic"}},
		}
		target := selectMergeTarget(candidates, "未知", "Python")
		if target == nil {
			t.Fatal("expected non-nil target")
		}
		if target.ID != 2 {
			t.Errorf("expected non-abstract tag ID 2, got %d", target.ID)
		}
	})

	t.Run("returns nil for empty candidates", func(t *testing.T) {
		target := selectMergeTarget(nil, "anything", "anything")
		if target != nil {
			t.Error("expected nil for empty candidates")
		}
	})
}
```

Run: `cd backend-go && go test ./internal/domain/topicanalysis/... -v -run TestSelectMergeTarget`
Expected: PASS

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git add backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
git commit -m "feat: selectMergeTarget supports label-based lookup and prefers non-abstract"
```

---

## Task 4: 修改 `ExtractAbstractTag` 使用批量 judgment

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go:66-246`

**Step 1: 修改 `ExtractAbstractTag` 处理 `[]tagJudgment`**

核心逻辑：收集所有 judgments → 处理 merge 判断 → 处理 abstract 判断 → 返回 PrimaryAction。

```go
func ExtractAbstractTag(ctx context.Context, candidates []TagCandidate, newLabel string, category string, opts ...ExtractAbstractTagOption) (*TagExtractionResult, error) {
	if len(candidates) < 1 {
		return nil, fmt.Errorf("need at least 1 candidate for abstract tag extraction, got %d", len(candidates))
	}

	cfg := &extractAbstractTagConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if category == "" && len(candidates) > 0 && candidates[0].Tag != nil {
		category = candidates[0].Tag.Category
	}
	if category == "" {
		category = "keyword"
	}

	judgments, err := callLLMForTagJudgment(ctx, candidates, newLabel, category, cfg.narrativeContext)
	if err != nil {
		logging.Warnf("Tag judgment LLM call failed: %v", err)
		return nil, err
	}

	return processJudgments(ctx, judgments, candidates, newLabel, category)
}
```

**Step 2: 实现 `processJudgments` — 汇总多轮结果**

```go
func processJudgments(ctx context.Context, judgments []tagJudgment, candidates []TagCandidate, newLabel string, category string) (*TagExtractionResult, error) {
	// 分类 judgments
	var mergeJudgments, abstractJudgments []tagJudgment
	for _, j := range judgments {
		switch j.Action {
		case ActionMerge:
			mergeJudgments = append(mergeJudgments, j)
		case ActionAbstract:
			abstractJudgments = append(abstractJudgments, j)
		}
	}

	// 优先处理 merge: 新标签合并到最佳候选
	if len(mergeJudgments) > 0 {
		bestMerge := mergeJudgments[0]
		// 选择 merge_target 最匹配的候选
		mergeTarget := selectMergeTarget(candidates, bestMerge.MergeTarget, bestMerge.MergeLabel)
		if mergeTarget == nil {
			return nil, fmt.Errorf("no suitable merge target found for label %q (target=%q)", bestMerge.MergeLabel, bestMerge.MergeTarget)
		}
		logging.Infof("Tag judgment: merge into existing tag %q (id=%d), label=%q", mergeTarget.Label, mergeTarget.ID, bestMerge.MergeLabel)
		return &TagExtractionResult{
			Action:      ActionMerge,
			MergeTarget: mergeTarget,
			MergeLabel:  bestMerge.MergeLabel,
		}, nil
	}

	// 处理 abstract: 为相关候选创建抽象标签
	if len(abstractJudgments) > 0 {
		bestAbstract := abstractJudgments[0]
		return processAbstractJudgment(ctx, candidates, bestAbstract, newLabel, category)
	}

	// 全部 none
	logging.Infof("Tag judgment: all candidates independent for %q", newLabel)
	return &TagExtractionResult{
		Action: ActionNone,
	}, nil
}
```

**Step 3: 实现 `processAbstractJudgment`**

从原 `ExtractAbstractTag` 的 abstract 分支提取出来，逻辑基本不变：

```go
func processAbstractJudgment(ctx context.Context, candidates []TagCandidate, judgment tagJudgment, newLabel string, category string) (*TagExtractionResult, error) {
	abstractName := judgment.AbstractName
	abstractDesc := judgment.Description

	slug := topictypes.Slugify(abstractName)
	if slug == "" {
		return nil, fmt.Errorf("generated empty slug for abstract name %q", abstractName)
	}

	candidateSlugs := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c.Tag != nil {
			candidateSlugs[c.Tag.Slug] = true
		}
	}

	if candidateSlugs[slug] {
		logging.Infof("Abstract name %q (slug=%s) collides with a candidate tag, falling back to merge", abstractName, slug)
		mergeTarget := selectMergeTarget(candidates, abstractName, judgment.MergeLabel)
		if mergeTarget == nil {
			return nil, fmt.Errorf("abstract name %q collides with candidate but no merge target found", abstractName)
		}
		return &TagExtractionResult{
			Action:      ActionMerge,
			MergeTarget: mergeTarget,
			MergeLabel:  abstractName,
		}, nil
	}

	// (后面和原 abstract 分支相同：创建抽象标签、建立父子关系)
	// 保留原有 abstract_tag_service.go:140-246 的逻辑，封装在此函数中
	// ...
}
```

> **注意：** `processAbstractJudgment` 的核心逻辑（创建抽象标签、建立父子关系、环检测等）直接从原 `ExtractAbstractTag` 的 abstract 分支（`abstract_tag_service.go:140-246`）搬过来，不需要改动。此处省略重复代码，实施时直接复制。

**Step 4: 运行编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译通过

**Step 5: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "refactor: ExtractAbstractTag uses per-candidate batch judgments with multi-round processing"
```

---

## Task 5: 修改 `findOrCreateTag` 简化匹配分支

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go:142-339`

**Step 1: 简化 switch 逻辑**

把 `high_similarity`、`ai_judgment` 两个 case 合并为 `"candidates"`：

```go
func findOrCreateTag(ctx context.Context, tag topictypes.TopicTag, source string, articleContext string) (*models.TopicTag, error) {
	slug := topictypes.Slugify(tag.Label)
	category := NormalizeDisplayCategory(tag.Kind, tag.Category)
	kind := NormalizeTopicKind(tag.Kind, category)

	aliases := tag.Aliases
	if len(aliases) == 0 {
		aliases = []string{}
	}
	aliasesJSON, _ := json.Marshal(aliases)

	es := getEmbeddingService()
	if es != nil {
		matchResult, err := es.TagMatch(ctx, tag.Label, category, string(aliasesJSON))
		if err != nil {
			logging.Warnf("TagMatch failed, falling back to exact match: %v", err)
		} else {
			switch matchResult.MatchType {

			case "exact":
				if matchResult.ExistingTag != nil {
					existing := matchResult.ExistingTag
					existing.Label = tag.Label
					existing.Category = category
					existing.Source = source
					if tag.Icon != "" {
						existing.Icon = tag.Icon
					}
					if len(tag.Aliases) > 0 {
						aJSON, _ := json.Marshal(tag.Aliases)
						existing.Aliases = string(aJSON)
					}
					existing.Kind = kind
					if err := database.DB.Save(existing).Error; err != nil {
						return nil, err
					}
					go ensureTagEmbedding(es, existing.ID)
					go backfillTagDescription(existing.ID, existing.Label, existing.Category, existing.Description, articleContext)
					return existing, nil
				}

			case "candidates":
				candidates := matchResult.Candidates
				logging.Infof("Batch tag judgment for %q: %d candidates (top similarity %.2f)", tag.Label, len(candidates), matchResult.Similarity)
				result, judgmentErr := topicanalysis.ExtractAbstractTag(ctx, candidates, tag.Label, category)
				if judgmentErr != nil || result == nil {
					logging.Warnf("Tag judgment failed for %q, falling back to new tag creation: %v", tag.Label, judgmentErr)
					break
				}

				if result.Action == topicanalysis.ActionMerge {
					existing := result.MergeTarget
					if result.MergeLabel != "" {
						existing.Label = result.MergeLabel
					} else {
						existing.Label = tag.Label
					}
					existing.Category = category
					existing.Source = source
					if len(tag.Aliases) > 0 {
						aJSON, _ := json.Marshal(tag.Aliases)
						existing.Aliases = string(aJSON)
					}
					if tag.Icon != "" {
						existing.Icon = tag.Icon
					}
					existing.Kind = kind
					if err := database.DB.Save(existing).Error; err != nil {
						logging.Warnf("Failed to save merged tag %d: %v", existing.ID, err)
						break
					}
					go ensureTagEmbedding(es, existing.ID)
					go backfillTagDescription(existing.ID, existing.Label, existing.Category, existing.Description, articleContext)
					return existing, nil
				}

				if result.Action == topicanalysis.ActionNone {
					logging.Infof("Tag judgment: none — tag %q is independent from %d candidates, creating new tag", tag.Label, len(candidates))
					break
				}

				// ActionAbstract
				for _, c := range candidates {
					if c.Tag != nil {
						if delErr := topicanalysis.DeleteTagEmbedding(c.Tag.ID); delErr != nil {
							logging.Warnf("Failed to delete embedding for child tag %d: %v", c.Tag.ID, delErr)
						}
					}
				}
				newTag, childErr := createChildOfAbstract(ctx, es, tag, category, kind, source, articleContext, string(aliasesJSON), result.AbstractTag)
				if childErr != nil {
					logging.Warnf("Failed to create child of abstract %d: %v", result.AbstractTag.ID, childErr)
					break
				}
				return newTag, nil

			case "no_match":
			}
		}
	}

	// Fallback: exact slug+category match (when embedding unavailable)
	// or creation path for no_match/candidates that fell through
	var dbTag models.TopicTag
	err := database.DB.Where("slug = ? AND category = ?", slug, category).First(&dbTag).Error
	if err == nil {
		dbTag.Label = tag.Label
		dbTag.Category = category
		dbTag.Source = source
		if tag.Icon != "" {
			dbTag.Icon = tag.Icon
		}
		if len(tag.Aliases) > 0 {
			aJSON, _ := json.Marshal(tag.Aliases)
			dbTag.Aliases = string(aJSON)
		}
		dbTag.Kind = kind
		if err := database.DB.Save(&dbTag).Error; err != nil {
			return nil, err
		}
		if es != nil {
			go ensureTagEmbedding(es, dbTag.ID)
		}
		go backfillTagDescription(dbTag.ID, dbTag.Label, dbTag.Category, dbTag.Description, articleContext)
		return &dbTag, nil
	}

	// Create new tag
	newTag := models.TopicTag{
		Slug:        slug,
		Label:       tag.Label,
		Category:    category,
		Kind:        kind,
		Icon:        tag.Icon,
		Aliases:     string(aliasesJSON),
		IsCanonical: true,
		Source:      source,
	}
	if err := database.DB.Create(&newTag).Error; err != nil {
		return nil, err
	}

	if articleContext != "" {
		go generateTagDescription(newTag.ID, tag.Label, category, articleContext)
	} else if es != nil {
		go generateAndSaveEmbedding(es, &newTag)
	}

	return &newTag, nil
}
```

关键变化：
- 删除了 `high_similarity` 中对 `isAbstract` 的特殊处理（创建子标签的逻辑改由 LLM 判断）
- 删除了 `ai_judgment` 中 `validCandidates` 过滤和 `ExtractAbstractTag` 的手动候选构造（现在 `TagMatch` 已返回完整候选列表）
- 删除了 `low_similarity` case（已统一为 `no_match`）
- `"candidates"` case 把所有候选直接传给 `ExtractAbstractTag`

**Step 2: 编译验证**

Run: `cd backend-go && go build ./...`
Expected: 编译通过

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicextraction/tagger.go
git commit -m "refactor: simplify findOrCreateTag to use unified candidates path"
```

---

## Task 6: 修复叙事流程 + 清理废弃引用

**Files:**
- Modify: `backend-go/internal/domain/narrative/tag_feedback.go:127-130`
- Search for any external callers of `TagMatch` or references to old match types

**Step 1: 修复叙事流程候选缺少 Similarity**

`tag_feedback.go:91` 已经计算了 `sim`，需要在构造候选时填充：

```go
// 修改前（tag_feedback.go:127-130）
candidates := []topicanalysis.TagCandidate{
    {Tag: &tagA},
    {Tag: &tagB},
}

// 修改后
candidates := []topicanalysis.TagCandidate{
    {Tag: &tagA, Similarity: sim},
    {Tag: &tagB, Similarity: sim},
}
```

**Step 2: 搜索旧 match type 引用**

搜索以下字符串确认无其他引用：
- `"high_similarity"`
- `"ai_judgment"`
- `"low_similarity"`
- `ShouldCreate`

Run: `rg "high_similarity|ai_judgment|low_similarity|ShouldCreate" backend-go/ --type go`
Expected: 仅在注释或测试中出现，无业务代码引用

如果有测试文件引用旧 match type，更新测试。

**Step 3: Commit**

```bash
git add backend-go/internal/domain/narrative/tag_feedback.go
git add -A backend-go/
git commit -m "fix: fill Similarity in narrative tag_feedback candidates + clean up old match types"
```

---

## Task 7: 更新现有测试适配新签名

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go`

**Step 1: 更新 `TestBuildTagJudgmentPrompt`**

改为测试新的 `buildBatchTagJudgmentPrompt`：

```go
func TestBuildBatchTagJudgmentPrompt(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{Label: "大语言模型", Source: "abstract"}, Similarity: 0.92},
		{Tag: &models.TopicTag{Label: "GPT-4", Source: "heuristic"}, Similarity: 0.88},
	}
	newLabel := "Gemini Pro"

	for _, category := range []string{"person", "event", "keyword"} {
		t.Run(category, func(t *testing.T) {
			prompt := buildBatchTagJudgmentPrompt(candidates, newLabel, category, nil)
			if !strings.Contains(prompt, "大语言模型") {
				t.Error("prompt should contain candidate label")
			}
			if !strings.Contains(prompt, "Gemini Pro") {
				t.Error("prompt should contain new label")
			}
			if !strings.Contains(prompt, "merge_target") {
				t.Error("prompt should instruct merge_target field")
			}
			if !strings.Contains(prompt, "candidate_label") {
				t.Error("prompt should instruct per-candidate judgment")
			}
			if !strings.Contains(prompt, "type: abstract") {
				t.Error("prompt should mark abstract candidates")
			}
		})
	}
}

func TestBuildBatchTagJudgmentPromptWithPreviousResults(t *testing.T) {
	candidates := []TagCandidate{
		{Tag: &models.TopicTag{Label: "Svelte", Slug: "svelte"}, Similarity: 0.82},
	}
	previousResults := []previousRoundResult{
		{CandidateLabel: "React", Action: ActionMerge, TargetLabel: "React"},
		{CandidateLabel: "Vue", Action: ActionNone},
	}
	prompt := buildBatchTagJudgmentPrompt(candidates, "SolidJS", "keyword", previousResults)
	if !strings.Contains(prompt, "React → merge") {
		t.Error("prompt should include previous round merge result")
	}
	if !strings.Contains(prompt, "Vue → none") {
		t.Error("prompt should include previous round none result")
	}
}
```

**Step 2: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
git commit -m "test: update tests for batch tag judgment with per-candidate array response"
```

---

## Task 8: 全量编译和测试

**Step 1: 全量编译**

Run: `cd backend-go && go build ./...`
Expected: 编译通过

**Step 2: 运行所有 topicanalysis 测试**

Run: `cd backend-go && go test ./internal/domain/topicanalysis/... -v`
Expected: PASS

**Step 3: 运行所有 topicextraction 测试**

Run: `cd backend-go && go test ./internal/domain/topicextraction/... -v`
Expected: PASS

**Step 4: 运行全量测试**

Run: `cd backend-go && go test ./... -count=1`
Expected: PASS（可能有依赖外部服务的测试 Skip，无 FAIL）

---

## Task 9: 更新文档

**Files:**
- Modify: `docs/guides/topic-graph.md:404-508`（标签匹配与抽象层级章节）

**Step 1: 更新三阈值匹配流程图**

将旧的阈值分支图替换为新流程：

```
新标签 → TagMatch() 生成 embedding → FindSimilarTags 搜索
    ↓
┌──────────────────────────────────────────────────────────────────┐
│ exact（slug/别名完全匹配）        → 复用现有标签，更新元信息        │
│ 有候选 >= 0.78                   → 分批送 LLM per-candidate 判断:  │
│                                    merge: 合并到 LLM 指定的候选    │
│                                    abstract: 创建抽象标签+子标签   │
│                                    none: 候选独立，创建新标签      │
│                                    多批处理，前批结果作为下批上下文 │
│ 无候选或全部 < 0.78              → 全新标签，生成 embedding        │
└──────────────────────────────────────────────────────────────────┘
```

**Step 2: 更新相关代码表**

在相关代码表中增加提示："`findOrCreateTag` 不再区分高/中/低阈值，统一由 LLM per-candidate 判断"。

**Step 3: Commit**

```bash
git add docs/guides/topic-graph.md
git commit -m "docs: update tag matching flow for batch per-candidate judgment"
```

---

## 风险评估

| 风险 | 缓解措施 |
|------|----------|
| LLM 调用成本增加（高阈值也走 LLM） | 高阈值通常只有 1-2 个候选，token 开销小；per-candidate 判断避免了整体误判 |
| LLM 返回非 JSON 数组 | `parseSingleJudgmentFallback` 兼容单对象响应 |
| 候选过多导致 prompt 过长 | `judgmentBatchSize=8` 分批处理，每批候选有限 |
| 分批后 LLM 判断不一致（前后矛盾） | 后续批次携带前批结果摘要作为上下文，减少矛盾 |
| 破坏现有标签层级关系 | `ExtractAbstractTag` 的 abstract 分支逻辑不变，仍正确处理父子关系和环检测 |
| 叙事流程兼容性 | `tag_feedback.go` 已补充 Similarity 字段，2 个候选无需分批 |
| 并发安全 | `findOrCreateTag` 每次调用独立，无共享可变状态 |
