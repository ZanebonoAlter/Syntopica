# 标签提取时生成 Description 优化 - 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在文章打标签的 LLM 提取阶段（extractCandidates）直接输出 event/keyword 标签的 description，创建新标签时直接使用，省去后续 generateTagDescription 的异步调用。

**Architecture:** 修改标签提取 prompt 要求 LLM 同时输出 description；在解析时提取 description；创建新标签时直接写入 description 字段；person 标签保持现有路径不变；已有标签复用时若已有 description 则不覆盖。

**Tech Stack:** Go, PostgreSQL, LLM prompt engineering

---

## 背景与收益

当前流程：
```
extractCandidates(LLM) → 创建标签 → go generateTagDescription(再次LLM)
```

**数据**：7天内 `tag_description` + `tag_description_person` = **2,132 次 LLM 调用**

本计划将 **event/keyword 标签**的 description 生成合并到提取阶段，预期减少 **~1,600 次** LLM 调用。

---

## 核心原则

1. **person 标签不走此优化** — 需要结构化属性（country/organization/role/domains），prompt 太重
2. **已有 description 不覆盖** — 复用已有标签时，若 description 已存在，保持原值
3. **兜底机制保留** — `BackfillMissingDescriptions` 继续作为兜底
4. **向后兼容** — 如果 LLM 未返回 description，行为与现在一致

---

## 变更文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `backend-go/internal/domain/topictypes/types.go` | 修改 | ExtractedTag 添加 Description 字段 |
| `backend-go/internal/domain/topicextraction/extractor_enhanced.go` | 修改 | prompt + schema + parse 支持 description |
| `backend-go/internal/domain/topicextraction/tagger.go` | 修改 | 创建标签时直接写入 description，跳过异步生成 |

---

## Task 1: ExtractedTag 添加 Description 字段

**文件：** `backend-go/internal/domain/topictypes/types.go:45-52`

**Step 1: 修改 ExtractedTag 结构体**

在 `ExtractedTag` 中添加 `Description` 字段：

```go
// ExtractedTag is the raw output from AI extraction
type ExtractedTag struct {
	Label       string   `json:"label"`
	Category    string   `json:"category"`   // event, person, keyword
	Confidence  float64  `json:"confidence"` // 0-1 confidence score
	Aliases     []string `json:"aliases,omitempty"`
	Evidence    string   `json:"evidence,omitempty"` // Why this tag was extracted
	Description string   `json:"description,omitempty"` // 标签描述（新增）
}
```

**Step 2: 验证编译**

Run: `cd backend-go && go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
cd backend-go
git add internal/domain/topictypes/types.go
git commit -m "feat(tag-extraction): add Description field to ExtractedTag"
```

---

## Task 2: 修改标签提取 Prompt 和 Schema

**文件：** `backend-go/internal/domain/topicextraction/extractor_enhanced.go`

### Step 1: 修改 System Prompt

在 `buildExtractionSystemPrompt()` 的标签输出格式部分（约第 259 行），将示例改为：

```go
每个标签输出格式：
{"label": "标签名称", "category": "event|person|keyword", "confidence": 0.0-1.0, "aliases": ["别名1"], "evidence": "提取依据", "description": "标签的简短描述（中文，1-2句，客观事实，仅event和keyword需要，person可不填）"}

描述要求（仅 event 和 keyword）：
- 中文，1-2句话，客观事实
- 解释标签指代什么，不重复标签名
- 例如 "ChatGPT" → "OpenAI开发的大型语言模型聊天机器人"
- 例如 "苹果WWDC 2024" → "苹果公司于2024年6月举办的全球开发者大会"
- person 标签的 description 可留空，系统会后续单独生成
```

### Step 2: 修改 JSON Schema

在 `tagExtractionSchema()` 的 Items Properties 中添加：

```go
"description": {Type: "string", Description: "标签的简短描述（中文，1-2句，客观事实。仅event和keyword需要，person可留空）"},
```

### Step 3: 修改解析函数

在 `parseExtractedTags()` 中解析 description：

```go
result = append(result, topictypes.ExtractedTag{
	Label:       strings.TrimSpace(t.Label),
	Category:    cat,
	Confidence:  conf,
	Aliases:     t.Aliases,
	Evidence:    t.Evidence,
	Description: strings.TrimSpace(t.Description), // 新增
})
```

### Step 4: 修改 resolveCandidate 传递 description

在 `resolveCandidate()` 中，将 ExtractedTag 的 Description 复制到 TopicTag：

```go
return &topictypes.TopicTag{
	Label:       strings.TrimSpace(candidate.Label),
	Slug:        slug,
	Category:    category,
	Aliases:     candidate.Aliases,
	Score:       candidate.Confidence,
	Description: strings.TrimSpace(candidate.Description), // 新增
}, false, nil
```

### Step 5: 验证编译

Run: `cd backend-go && go build ./...`
Expected: PASS

### Step 6: Commit

```bash
cd backend-go
git add internal/domain/topicextraction/extractor_enhanced.go
git commit -m "feat(tag-extraction): include description in extraction prompt and parsing"
```

---

## Task 3: 创建标签时直接使用 description

**文件：** `backend-go/internal/domain/topicextraction/tagger.go`

### Step 1: 修改 findOrCreateTag - 创建新标签路径

在创建新标签的位置（约第 495-504 行），添加 description：

```go
newTag := models.TopicTag{
	Slug:        slug,
	Label:       tag.Label,
	Category:    category,
	Kind:        kind,
	Icon:        tag.Icon,
	Aliases:     string(aliasesJSON),
	IsCanonical: true,
	Source:      source,
	// 新增：非 person 标签直接使用提取时的 description
	Description: getTagDescription(tag, category),
}
```

### Step 2: 添加辅助函数

在 tagger.go 中添加辅助函数（放在文件末尾或合适位置）：

```go
// getTagDescription 决定创建新标签时是否使用提取阶段生成的 description
// 规则：
// - person 标签返回空（需要结构化属性，走单独生成路径）
// - 其他标签如果有 description 则使用
// - 否则返回空（后续由 BackfillMissingDescriptions 兜底）
func getTagDescription(tag topictypes.TopicTag, category string) string {
	if category == "person" {
		return ""
	}
	return tag.Description
}
```

### Step 3: 修改 findOrCreateTag - 跳过非 person 标签的异步生成

在创建新标签后的逻辑（约第 508-513 行），改为：

```go
if articleContext != "" {
	// person 标签仍然需要异步生成结构化属性
	if category == "person" {
		go generateTagDescription(newTag.ID, tag.Label, category, articleContext)
	}
	// event/keyword 标签的 description 已在创建时写入，无需再次生成
} else if es != nil {
	go generateAndSaveEmbedding(es, &newTag)
}
```

### Step 4: 修改 createChildOfAbstract - 创建子标签路径

在 `createChildOfAbstract()` 中（约第 521-535 行），同样添加 description：

```go
newTag := models.TopicTag{
	Slug:        slug,
	Label:       tag.Label,
	Category:    category,
	Kind:        kind,
	Icon:        tag.Icon,
	Aliases:     aliasesJSON,
	IsCanonical: true,
	Source:      source,
	// 新增
	Description: getTagDescription(tag, category),
}
```

并修改后续异步生成逻辑（约第 560-564 行）：

```go
if articleContext != "" {
	if category == "person" {
		go generateTagDescription(newTag.ID, tag.Label, category, articleContext)
	}
} else if es != nil {
	go generateAndSaveEmbedding(es, &newTag)
}
```

### Step 5: 修改复用已有标签路径 - 不覆盖已有 description

在复用已有标签的几个位置，确保不覆盖已有 description：

**位置 1：exact match（约第 236-257 行）**
在更新 existing 时，description 不要覆盖：

```go
existing.Label = tag.Label
// ... 其他字段更新 ...
// 不更新 existing.Description（已有 description 保持原值）
```

**位置 2：event fallback（约第 301-321 行）**
同样不更新 description。

**位置 3：merge 路径（约第 358-396 行）**
在更新 existing 后，不更新 description。

**位置 4：slug fallback（约第 471-490 行）**
不更新 description。

> 注意：当前代码在这些位置都没有更新 description，所以理论上不需要改。但需要确认没有意外覆盖。

### Step 6: 验证编译

Run: `cd backend-go && go build ./...`
Expected: PASS

### Step 7: Commit

```bash
cd backend-go
git add internal/domain/topicextraction/tagger.go
git commit -m "feat(tag-creation): use extracted description directly for event/keyword tags"
```

---

## Task 4: 运行测试

### Step 1: 运行相关单元测试

Run:
```bash
cd backend-go
go test ./internal/domain/topicextraction/... -v
```
Expected: PASS（或现有失败项未增加）

### Step 2: 运行类型检查

Run:
```bash
cd backend-go
go build ./...
```
Expected: PASS

### Step 3: Commit

```bash
cd backend-go
git add .
git commit -m "test: verify tag extraction with description changes"
```

---

## Task 5: 验证端到端行为

### Step 1: 启动后端服务

Run:
```bash
cd backend-go
go run cmd/server/main.go
```

### Step 2: 触发一篇文章打标签

可以通过以下方式触发：
1. 在 Web UI 中刷新一个 feed
2. 或调用 API: `POST /api/articles/{id}/retag`
3. 或等待自动调度器运行

### Step 3: 验证数据库

Run:
```bash
docker exec zanebono-rssreader-pgvector psql -U postgres -d rss_reader -c "
SELECT label, category, description, source 
FROM topic_tags 
WHERE created_at > NOW() - INTERVAL '10 minutes' 
ORDER BY created_at DESC 
LIMIT 10;
"
```

Expected:
- event/keyword 标签应该有 description（非空）
- person 标签的 description 应该为空（后续由单独路径生成）

### Step 4: 验证 ai_call_logs

Run:
```bash
docker exec zanebono-rssreader-pgvector psql -U postgres -d rss_reader -c "
SELECT capability, (request_meta::jsonb)->>'operation' as operation, COUNT(*) 
FROM ai_call_logs 
WHERE created_at > NOW() - INTERVAL '10 minutes' 
GROUP BY capability, (request_meta::jsonb)->>'operation';
"
```

Expected:
- `tag_description` 调用次数应明显减少（非 person 标签不再触发）
- `tag_description_person` 仍然会有（person 标签保持原路径）

---

## Task 6: 文档更新

### Step 1: 更新 tagging-flow.md

在 `docs/guides/tagging-flow.md` 的相关章节添加说明：

在 "1. 统一入口：findOrCreateTag" 章节，在 CREATE 节点后添加：
```markdown
> **注意**：对于 event/keyword 标签，description 在提取阶段（ExtractTagsFromArticle）已由 LLM 一并生成，
> 创建时直接写入，无需额外调用 generateTagDescription。person 标签仍需要单独生成结构化属性。
```

### Step 2: Commit

```bash
git add docs/guides/tagging-flow.md
git commit -m "docs(tagging): document description generation optimization"
```

---

## 回滚方案

如果需要回滚：

1. 恢复 `topictypes/types.go` 中 `ExtractedTag` 的 `Description` 字段
2. 恢复 `extractor_enhanced.go` 中的 prompt/schema/parse 修改
3. 恢复 `tagger.go` 中的 description 写入和异步生成逻辑
4. 重新编译部署

---

## 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| LLM 提取质量下降 | 高 | prompt 中 description 要求用可选字段标注；充分测试后再上线 |
| 输出 token 超限 | 中 | 监控 maxTokens 使用情况；必要时从 2048 增加到 4096 |
| person 标签描述缺失 | 低 | person 保持原路径不变；不影响 |
| 已有 description 被覆盖 | 中 | 复用标签时不更新 description；已确认现有代码不覆盖 |

---

## 后续优化方向

1. **batch_tag_judgment 的 description**：当前 batch 判断时也可以收集 description，但目前影响不大
2. **description 质量评分**：根据 description 长度和内容质量，决定是否保留或重新生成
3. **prompt A/B 测试**：对比有无 description 要求的提取效果，选择更优 prompt
