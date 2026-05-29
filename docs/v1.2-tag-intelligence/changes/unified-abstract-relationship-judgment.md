# 统一抽象标签关系判断 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将中间相似度区间（0.78-0.97）的 `aiJudgeAbstractHierarchy` 替换为支持 merge/parent/skip 的统一判断函数，让 LLM 成为最终裁判，同时保留现有层级深度和循环保护。

**Architecture:** 新增 `judgeAbstractRelationship` 函数替换 `aiJudgeAbstractHierarchy`，返回 4 种动作（merge/parent_A/parent_B/skip），并在解析层校验 LLM 输出。高相似度区间（≥0.97）保持 `mergeOrLinkSimilarAbstract` 不变。调用方 `MatchAbstractTagHierarchy` 根据返回值分发执行，parent 动作仍通过 `linkAbstractParentChild` 统一执行深度、循环、类型校验。

**Tech Stack:** Go, Gin, airouter, GORM

**Plan Review Notes:** 原计划 Task 2 的替换代码会移除显式深度判断，并且没有校验 LLM 返回的 action/target。执行时必须按本修订版实现：无效 LLM 输出返回错误并跳过；parent 链接不绕过 `linkAbstractParentChild`；merge target 必须是 A 或 B。

---

## Task 1: 新增并测试 `judgeAbstractRelationship` 解析校验

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_judgment.go`（在 `aiJudgeAbstractHierarchy` 之后插入新类型、校验函数和判断函数）
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go`（新增解析校验单元测试）

**Step 1: 定义返回类型和校验函数**

在 `aiJudgeAbstractHierarchy` 函数之后插入：

```go
type abstractRelationJudgment struct {
	Action string `json:"action"` // "merge" | "parent_A" | "parent_B" | "skip"
	Target string `json:"target"` // "A" | "B"，merge 时保留的标签
	Reason string `json:"reason"`
}

func normalizeAbstractRelationJudgment(judgment *abstractRelationJudgment) error {
	if judgment == nil {
		return fmt.Errorf("empty abstract relation judgment")
	}
	judgment.Action = strings.TrimSpace(judgment.Action)
	judgment.Target = strings.ToUpper(strings.TrimSpace(judgment.Target))
	judgment.Reason = strings.TrimSpace(judgment.Reason)

	switch judgment.Action {
	case "merge":
		if judgment.Target != "A" && judgment.Target != "B" {
			return fmt.Errorf("invalid merge target %q", judgment.Target)
		}
	case "parent_A", "parent_B", "skip":
		// target is ignored for non-merge actions, but normalize a missing value for stable logs/tests.
		if judgment.Target != "A" && judgment.Target != "B" {
			judgment.Target = ""
		}
	default:
		return fmt.Errorf("invalid abstract relation action %q", judgment.Action)
	}
	return nil
}
```

**Step 2: 新增判断函数**

继续插入：

```go

func judgeAbstractRelationship(ctx context.Context, tag1ID, tag2ID uint) (*abstractRelationJudgment, error) {
	var tag1, tag2 models.TopicTag
	if err := database.DB.First(&tag1, tag1ID).Error; err != nil {
		return nil, fmt.Errorf("load tag %d: %w", tag1ID, err)
	}
	if err := database.DB.First(&tag2, tag2ID).Error; err != nil {
		return nil, fmt.Errorf("load tag %d: %w", tag2ID, err)
	}

	children1 := loadAbstractChildLabels(tag1ID, 8)
	children2 := loadAbstractChildLabels(tag2ID, 8)

	router := airouter.NewRouter()
	prompt := fmt.Sprintf(`Given two abstract topic tags, determine their relationship.

Tag A: %q (%s)
Tag A's children: %s

Tag B: %q (%s)
Tag B's children: %s

Choose one action:
- "merge": They describe the exact same concept (synonyms, translations, different wording for the same idea). Specify which to keep in "target".
- "parent_A": Tag A is the broader/more general concept, Tag B is a specific sub-concept of A.
- "parent_B": Tag B is the broader/more general concept, Tag A is a specific sub-concept of B.
- "skip": They look similar but should NOT be related (different domains, different regions, unrelated topics).

Respond with JSON: {"action": "merge"|"parent_A"|"parent_B"|"skip", "target": "A"|"B", "reason": "brief explanation"}

Rules:
- Use the children tags to understand what each abstract tag actually covers
- "merge" ONLY when they are truly the same concept — not just related or overlapping
- "skip" when children tags show they cover completely different domains or regions
- For parent/child, the parent should be the more general/broader concept
- If they are equally broad but related, prefer "skip" over forcing a relationship`,
		tag1.Label, formatTagPromptContext(&tag1), formatChildLabels(children1),
		tag2.Label, formatTagPromptContext(&tag2), formatChildLabels(children2))

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
				"action": {Type: "string", Description: "merge, parent_A, parent_B, 或 skip"},
				"target": {Type: "string", Description: "A 或 B，merge 时保留的标签；其他动作时也需要填写"},
				"reason": {Type: "string", Description: "判断理由"},
			},
			Required: []string{"action", "target", "reason"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation": "judge_abstract_relationship",
			"tag_a":     tag1ID,
			"tag_b":     tag2ID,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed abstractRelationJudgment
	if err := json.Unmarshal([]byte(jsonutil.SanitizeLLMJSON(result.Content)), &parsed); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}
	if err := normalizeAbstractRelationJudgment(&parsed); err != nil {
		return nil, err
	}

	return &parsed, nil
}
```

**Step 3: 新增校验测试**

在 `abstract_tag_service_test.go` 中新增：

```go
func TestNormalizeAbstractRelationJudgment(t *testing.T) {
	t.Run("accepts merge target", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "merge", Target: "b", Reason: " same "}
		if err := normalizeAbstractRelationJudgment(judgment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if judgment.Target != "B" || judgment.Reason != "same" {
			t.Fatalf("normalized judgment = %+v", judgment)
		}
	})

	t.Run("rejects invalid merge target", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "merge", Target: "C"}
		if err := normalizeAbstractRelationJudgment(judgment); err == nil {
			t.Fatal("expected invalid target error")
		}
	})

	t.Run("rejects unknown action", func(t *testing.T) {
		judgment := &abstractRelationJudgment{Action: "link", Target: "A"}
		if err := normalizeAbstractRelationJudgment(judgment); err == nil {
			t.Fatal("expected invalid action error")
		}
	})
}
```

**Step 4: 测试验证**

```bash
cd backend-go && go test ./internal/domain/topicanalysis -run TestNormalizeAbstractRelationJudgment -v -count=1
```

Expected: 新测试通过

**Step 5: 编译验证**

```bash
cd backend-go && go build ./...
```

Expected: 编译通过

**Step 6: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_judgment.go backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
git commit -m "feat(topicanalysis): add abstract relation judgment"
```

---

## Task 2: 修改 `MatchAbstractTagHierarchy` 分发新动作

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_hierarchy.go:76-109`

**Step 1: 替换循环逻辑**

将 `MatchAbstractTagHierarchy` 中的候选处理循环（第 76-109 行）替换为：

```go
	for _, candidate := range candidates {
		if candidate.Tag == nil {
			continue
		}

		if candidate.Similarity >= thresholds.HighSimilarity {
			if err := mergeOrLinkSimilarAbstract(ctx, abstractTagID, candidate.Tag.ID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: merge/link failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, err)
			}
			continue
		}

		if candidate.Similarity < thresholds.LowSimilarity {
			continue
		}

		judgment, err := judgeAbstractRelationship(ctx, abstractTagID, candidate.Tag.ID)
		if err != nil {
			logging.Warnf("MatchAbstractTagHierarchy: AI judgment failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, err)
			continue
		}

		switch judgment.Action {
		case "merge":
			sourceID, targetID := abstractTagID, candidate.Tag.ID
			if strings.EqualFold(judgment.Target, "A") {
				sourceID, targetID = candidate.Tag.ID, abstractTagID
			}
			if mergeErr := MergeTags(sourceID, targetID); mergeErr != nil {
				logging.Warnf("MatchAbstractTagHierarchy: merge failed for %d into %d: %v", sourceID, targetID, mergeErr)
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: merged %d into %d (AI judged, reason=%s)", sourceID, targetID, judgment.Reason)
			return
		case "parent_A":
			// linkAbstractParentChild enforces cycle, type, and max-depth constraints.
			if err := linkAbstractParentChild(candidate.Tag.ID, abstractTagID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: failed to link %d under %d: %v", candidate.Tag.ID, abstractTagID, err)
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: %d is child of %d (AI judged, reason=%s)", candidate.Tag.ID, abstractTagID, judgment.Reason)
		case "parent_B":
			// linkAbstractParentChild enforces cycle, type, and max-depth constraints.
			if err := linkAbstractParentChild(abstractTagID, candidate.Tag.ID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: failed to link %d under %d: %v", abstractTagID, candidate.Tag.ID, err)
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: %d is child of %d (AI judged, reason=%s)", abstractTagID, candidate.Tag.ID, judgment.Reason)
		case "skip":
			logging.Infof("MatchAbstractTagHierarchy: skipped %d vs %d (AI judged, reason=%s)", abstractTagID, candidate.Tag.ID, judgment.Reason)
		default:
			logging.Warnf("MatchAbstractTagHierarchy: unknown action %q for %d vs %d", judgment.Action, abstractTagID, candidate.Tag.ID)
		}
	}
```

**Step 2: 确认 `strings` 已导入**

检查 `abstract_tag_hierarchy.go` 的 import，如果缺少 `"strings"` 则添加。

**Step 3: 编译验证**

```bash
cd backend-go && go build ./...
```

Expected: 编译通过

**Step 4: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_hierarchy.go
git commit -m "refactor(topicanalysis): use judgeAbstractRelationship for medium-similarity candidates"
```

---

## Task 3: 删除旧函数 `aiJudgeAbstractHierarchy`

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_judgment.go:833-906`

**Step 1: 删除 `aiJudgeAbstractHierarchy` 函数**

删除第 833-906 行的整个函数。

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

Expected: 编译通过（无其他调用方）

**Step 3: Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_judgment.go
git commit -m "cleanup(topicanalysis): remove aiJudgeAbstractHierarchy (replaced by judgeAbstractRelationship)"
```

---

## Task 4: 运行测试验证

**Step 1: 运行 topicanalysis 包测试**

```bash
cd backend-go && go test ./internal/domain/topicanalysis -v -count=1
```

Expected: 所有测试通过

**Step 2: 运行全量测试**

```bash
cd backend-go && go test ./...
```

Expected: 所有测试通过

**Step 3: 最终 Commit（如有修复）**

如有测试失败修复，commit 修复代码。

---

## 改动摘要

| 文件 | 改动 |
|------|------|
| `abstract_tag_judgment.go` | 新增 `abstractRelationJudgment` 类型 + `judgeAbstractRelationship` 函数；删除 `aiJudgeAbstractHierarchy` |
| `abstract_tag_hierarchy.go` | `MatchAbstractTagHierarchy` 中间相似度区间改用 `judgeAbstractRelationship`，支持 merge/parent_A/parent_B/skip |

**不变的部分：**
- 高相似度（≥0.97）仍走 `mergeOrLinkSimilarAbstract`
- 低相似度（<0.78）仍跳过
- `adoptNarrowerAbstractChildren` 不变
- 跨层去重逻辑不变
