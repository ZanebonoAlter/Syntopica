# Fix Event Keyword Embedding Pipeline

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 修复 event 标签关键词提取和 embedding 管线的两个断裂点，使 event_keyword embedding 正常生成并用于 concept 匹配。

**Architecture:** 两个断裂点需要修复：(1) `findOrCreateTag` 创建 event 标签后不触发 `generateTagDescription`，导致 description 和 keywords 永远为空；(2) `batchGenerateTagDescriptions` 批量路径只返回 description，不提取 keywords。修复后 event 标签在创建时即触发 description+keywords 生成，backfill 路径也正确提取 keywords。

**Tech Stack:** Go, GORM, SQLite (测试), PostgreSQL (生产)

**Root Cause Analysis:**

```
断裂点 A: findOrCreateTag (tagger.go:162-191)
  创建新标签 → 跳过 embedding (event) → PlaceTagInHierarchy → 缓存
  ❌ 缺少: go generateTagDescription(newTag.ID, tag.Label, category, articleContext)

断裂点 B: batchGenerateTagDescriptions (tagger.go:358-484)
  批量 LLM → 只返回 {id, description} → 不写 metadata.event_keywords
  ❌ 缺少: event 标签的 keywords 提取和保存

断裂点 C: BackfillMissingDescriptions (description_backfill.go:14-64)
  调用 batch → 批量路径不处理 keywords → 结果写回时只 update description
  ❌ 缺少: event 标签单独走 generateTagDescription 路径
```

---

## Issue #1: findOrCreateTag 创建 event 标签后触发 description 生成

**Type:** AFK
**Blocked by:** None

### What to build

当 `findOrCreateTag` 创建新的 event 标签后，启动 goroutine 调用 `generateTagDescription`，使 event 标签在创建时即生成 description 和 keywords。非 event 标签行为不变。

### Task 1: 添加 event 标签 description 触发

**Files:**
- Modify: `backend-go/internal/domain/tagging/tagger.go:180-188`

**Step 1: 在 `findOrCreateTag` 创建新标签代码块中，event 跳过 embedding 之后、PlaceTagInHierarchy 之前，添加 description 生成触发**

在 `tagger.go` 第 182 行之后（`go ensureTagEmbedding` 的 if 块之后），添加：

```go
if category == "event" {
	go generateTagDescription(newTag.ID, tag.Label, category, articleContext)
}
```

注意：`articleContext` 参数已经在 `findOrCreateTag` 的函数签名中（第 64 行），直接可用。

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tagger.go
git commit -m "fix: trigger generateTagDescription for new event tags in findOrCreateTag"
```

### Acceptance Criteria

- [ ] 新建 event 标签后自动触发 `generateTagDescription` goroutine
- [ ] 新建 event 标签的 `metadata.event_keywords` 在 LLM 返回后被写入
- [ ] 非 event 标签（keyword, person）行为不变
- [ ] `go build ./...` 编译通过

### Blocked by

None - can start immediately

---

## Issue #2: batchGenerateTagDescriptions 支持 event keywords 提取

**Type:** AFK
**Blocked by:** None

### What to build

修改 `batchGenerateTagDescriptions` 的批量 LLM 路径，对 event 标签增加 keywords 提取。批量返回结构新增 `keywords` 字段，event 标签的 keywords 写入 `topic_tags.metadata`。同时在 `BackfillMissingDescriptions` 中，将结果写回时对 event 标签也写 keywords 并触发 re-embedding。

### Task 2: 修改批量 LLM prompt 和 schema 支持 keywords

**Files:**
- Modify: `backend-go/internal/domain/tagging/tagger.go:396-483`

**Step 1: 修改批量 prompt，增加 keywords 提取指令**

将 `batchGenerateTagDescriptions` 中批量 prompt（tagger.go:396-408）改为：

```go
prompt := fmt.Sprintf(`为以下标签批量生成 description（中文，每个 1-2 句话，客观事实，500 字以内）。

标签列表：
%s

规则：
- 每个标签的 description 必须解释该标签是什么，不能只重复标签名
- person 类标签说明人物身份
- event 类标签说明事件经过
- keyword 类标签说明概念领域
- 对 event 类标签，额外提取 3-5 个关键词（实体名、地名、动作词），避免泛泛的词如"事件""情况"

返回 JSON: {"descriptions": [{"id": 标签ID, "description": "描述内容", "keywords": ["关键词1", ...]}, ...]}
非 event 类标签的 keywords 字段留空数组 []`, string(itemsJSON))
```

**Step 2: 修改 JSON schema 支持 keywords 字段**

将 `batchGenerateTagDescriptions` 中的 JSONSchema（tagger.go:418-434）改为：

```go
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
					"keywords": {
						Type:        "array",
						Items:       &airouter.SchemaProperty{Type: "string"},
						Description: "event 标签的关键词列表，非 event 标签为空数组",
					},
				},
				Required: []string{"id", "description"},
			},
		},
	},
	Required: []string{"descriptions"},
},
```

**Step 3: 修改解析结构体和结果返回**

将解析部分（tagger.go:458-483）改为：

```go
content := jsonutil.SanitizeLLMJSON(result.Content)
var parsed struct {
	Descriptions []struct {
		ID          uint     `json:"id"`
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
	} `json:"descriptions"`
}
if err := json.Unmarshal([]byte(content), &parsed); err != nil {
	logging.Warnf("batchGenerateTagDescriptions: parse failed: %v", err)
	return nil
}

type descResult struct {
	Description string
	Keywords    []string
}
results := make(map[uint]*descResult)
validIDs := make(map[uint]bool, len(items))
for _, item := range items {
	validIDs[item.ID] = true
}
for _, d := range parsed.Descriptions {
	if d.Description != "" && validIDs[d.ID] {
		desc := d.Description
		if len([]rune(desc)) > 500 {
			desc = string([]rune(desc)[:500])
		}
		results[d.ID] = &descResult{
			Description: desc,
			Keywords:    d.Keywords,
		}
	}
}
return results
```

注意：`batchGenerateTagDescriptions` 的返回类型需要从 `map[uint]string` 改为 `map[uint]*descResult`。这会影响所有调用方。

**Step 4: 更新函数签名**

将 `batchGenerateTagDescriptions` 签名（tagger.go:358）改为：

```go
type batchDescResult struct {
	Description string
	Keywords    []string
}

func batchGenerateTagDescriptions(tags []models.TopicTag) map[uint]*batchDescResult {
```

**Step 5: 更新内部单标签 fallback 路径**

tagger.go:367 单标签路径，原返回 `map[uint]string{tags[0].ID: ""}`，改为 `map[uint]*batchDescResult{tags[0].ID: {}}`（空 result 表示已由 `generateTagDescription` 处理）。

**Step 6: 编译验证**

```bash
cd backend-go && go build ./...
```

### Task 3: 更新 BackfillMissingDescriptions 处理 keywords

**Files:**
- Modify: `backend-go/internal/domain/tagging/description_backfill.go:38-59`

**Step 1: 更新 `BackfillMissingDescriptions` 中对 batch 结果的处理**

将 `description_backfill.go` 第 39-59 行改为：

```go
results := batchGenerateTagDescriptions(batch)
for _, tag := range batch {
	result, ok := results[tag.ID]
	if !ok {
		continue
	}
	if result == nil {
		processed++
		continue
	}
	if result.Description == "" {
		continue
	}

	if tag.Category == "event" && len(result.Keywords) > 0 {
		metadataMap := models.MetadataMap{
			"event_keywords": result.Keywords,
		}
		if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).Updates(map[string]any{
			"description": result.Description,
			"metadata":    metadataMap,
		}).Error; err != nil {
			logging.Warnf("description backfill: failed to update event tag %d: %v", tag.ID, err)
		} else {
			processed++
		}
	} else {
		if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).
			Update("description", result.Description).Error; err != nil {
			logging.Warnf("description backfill: failed to update tag %d: %v", tag.ID, err)
		} else {
			processed++
		}
	}
	if qs := getEmbeddingQueueService(); qs != nil {
		if err := qs.Enqueue(tag.ID); err != nil {
			logging.Warnf("description backfill: failed to enqueue re-embedding for tag %d: %v", tag.ID, err)
		}
	}
}
```

注意：将 re-embedding enqueue 移到循环末尾，对所有成功更新的标签统一触发（包括 event 和非 event）。

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/tagger.go backend-go/internal/domain/tagging/description_backfill.go
git commit -m "fix: batch description path supports event keywords extraction and re-embedding"
```

### Acceptance Criteria

- [ ] `batchGenerateTagDescriptions` 对 event 标签返回 keywords
- [ ] `BackfillMissingDescriptions` 正确写 keywords 到 event 标签 metadata
- [ ] `BackfillMissingDescriptions` 对所有成功更新的标签触发 re-embedding
- [ ] 非 event 标签行为不变（keywords 为空数组）
- [ ] `go build ./...` 编译通过

### Blocked by

None - can start immediately

---

## Issue #3: 单元测试验证 event keyword 提取管线

**Type:** AFK
**Blocked by:** Issue #1, Issue #2

### What to build

为修复后的代码路径编写单元测试，验证：(1) `generateTagDescription` 正确解析 LLM 返回的 description+keywords 并写入 metadata；(2) `batchGenerateTagDescriptions` 批量路径正确提取 event keywords；(3) `BackfillMissingDescriptions` 正确处理 event 标签的 keywords 和 re-embedding 触发。

### Task 4: 测试 generateTagDescription keywords 解析

**Files:**
- Modify: `backend-go/internal/domain/tagging/embedding_test.go`

**Step 1: 添加 `getEventKeywords` 解析测试**

在 `embedding_test.go` 中添加：

```go
func TestGetEventKeywords(t *testing.T) {
	tests := []struct {
		name     string
		metadata models.MetadataMap
		expected []string
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			expected: nil,
		},
		{
			name:     "empty metadata",
			metadata: models.MetadataMap{},
			expected: nil,
		},
		{
			name:     "valid keywords",
			metadata: models.MetadataMap{"event_keywords": []interface{}{"美国", "伊朗", "制裁"}},
			expected: []string{"美国", "伊朗", "制裁"},
		},
		{
			name:     "string array",
			metadata: models.MetadataMap{"event_keywords": []string{"美国", "伊朗"}},
			expected: []string{"美国", "伊朗"},
		},
		{
			name:     "mixed types filtered",
			metadata: models.MetadataMap{"event_keywords": []interface{}{"美国", 123, "伊朗"}},
			expected: []string{"美国", "伊朗"},
		},
		{
			name:     "wrong type",
			metadata: models.MetadataMap{"event_keywords": "not an array"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := &models.TopicTag{Metadata: tt.metadata}
			result := getEventKeywords(tag)
			if len(result) != len(tt.expected) {
				t.Fatalf("getEventKeywords() = %v (len %d), want %v (len %d)", result, len(result), tt.expected, len(tt.expected))
			}
			for i, kw := range result {
				if kw != tt.expected[i] {
					t.Errorf("keyword[%d] = %q, want %q", i, kw, tt.expected[i])
				}
			}
		})
	}
}
```

**Step 2: 运行测试**

```bash
cd backend-go && go test ./internal/domain/tagging/ -run TestGetEventKeywords -v
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/embedding_test.go
git commit -m "test: add unit tests for getEventKeywords parsing"
```

### Task 5: 测试 event keyword embedding 生成路径

**Files:**
- Modify: `backend-go/internal/domain/tagging/embedding_test.go`

**Step 1: 添加 embedding queue processNext 对 event keyword 处理的集成测试**

在 `embedding_test.go` 中添加：

```go
func TestProcessNextEventKeywordEmbeddings(t *testing.T) {
	db := setupEmbeddingTestDB(t)

	tag := models.TopicTag{
		Label:       "特朗普访华",
		Category:    "event",
		Kind:        "topic",
		Slug:        "trump-visits-china",
		Status:      "active",
		Description: "美国总统特朗普对中国的国事访问",
		Metadata: models.MetadataMap{
			"event_keywords": []interface{}{"特朗普", "中国", "访华", "中美关系"},
		},
	}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	task := models.EmbeddingQueue{
		TagID:  tag.ID,
		Status: "pending",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	keywords := getEventKeywords(&tag)
	if len(keywords) != 4 {
		t.Fatalf("expected 4 keywords, got %d: %v", len(keywords), keywords)
	}

	expectedKeywords := []string{"特朗普", "中国", "访华", "中美关系"}
	for i, kw := range keywords {
		if kw != expectedKeywords[i] {
			t.Errorf("keyword[%d] = %q, want %q", i, kw, expectedKeywords[i])
		}
	}

	for _, kw := range keywords {
		kwHash := hashText(EmbeddingTypeEventKeyword + "\n" + kw)
		if kwHash == "" {
			t.Errorf("empty hash for keyword %q", kw)
		}
		var count int64
		db.Model(&models.TopicTagEmbedding{}).
			Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", tag.ID, EmbeddingTypeEventKeyword, kwHash).
			Count(&count)
		if count != 0 {
			t.Errorf("embedding should not exist yet for keyword %q", kw)
		}
	}
}
```

**Step 2: 运行测试**

```bash
cd backend-go && go test ./internal/domain/tagging/ -run TestProcessNextEventKeywordEmbeddings -v
```

**Step 3: Commit**

```bash
git add backend-go/internal/domain/tagging/embedding_test.go
git commit -m "test: add integration test for event keyword embedding path"
```

### Task 6: 全量验证

**Step 1: 运行全量测试**

```bash
cd backend-go && go test ./internal/domain/tagging/ -v
```

**Step 2: 运行质量门禁**

```bash
cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...
```

### Acceptance Criteria

- [ ] `TestGetEventKeywords` 测试通过
- [ ] `TestProcessNextEventKeywordEmbeddings` 测试通过
- [ ] `go test ./internal/domain/tagging/ -v` 全部通过
- [ ] `golangci-lint run ./...` 无新增 warning
- [ ] `go build ./...` 编译通过

### Blocked by

- Issue #1
- Issue #2

---

## Issue #4: 数据库回填现有 event 标签的 keywords

**Type:** HITL (需要确认回填策略)
**Blocked by:** Issue #1, Issue #2, Issue #3

### What to build

对现有 168 个 event 标签回填 keywords：通过 pg-diagnose 确认哪些需要回填，手动触发 `BackfillMissingDescriptions` 或设计一次性回填脚本。

### Task 7: 确认回填范围并执行

**Step 1: 通过 pg-diagnose 确认当前状态**

```sql
SELECT count(*) FILTER (WHERE metadata->>'event_keywords' IS NOT NULL) as has_keywords,
  count(*) FILTER (WHERE metadata->>'event_keywords' IS NULL) as no_keywords,
  count(*) FILTER (WHERE description IS NULL OR description = '') as no_desc
FROM topic_tags WHERE category='event' AND status='active';
```

**Step 2: 清空现有 event 标签的 embedding，强制重新生成**

对于已经有 embedding 但没有 keywords 的 event 标签，需要重新触发 description 生成 + re-embedding：

```sql
-- 先清除 event 标签的旧 embedding（让 re-embedding 生成含 keywords 的新 embedding）
DELETE FROM topic_tag_embeddings WHERE topic_tag_id IN (
  SELECT tt.id FROM topic_tags tt WHERE tt.category = 'event' AND tt.status = 'active'
);

-- 清除 event 标签的 description（让 backfill 重新生成含 keywords 的 description）
UPDATE topic_tags SET description = '' WHERE category = 'event' AND status = 'active';
```

**Step 3: 等待 cleanup scheduler Phase 7 自动回填**

或者手动调用 API 触发。

**Step 4: 验证回填结果**

```sql
SELECT count(*) FILTER (WHERE metadata->>'event_keywords' IS NOT NULL) as has_keywords,
  count(*) as total
FROM topic_tags WHERE category='event' AND status='active';
```

**Step 5: 验证 event_keyword embedding 已生成**

```sql
SELECT embedding_type, count(*) FROM topic_tag_embeddings
WHERE topic_tag_id IN (SELECT id FROM topic_tags WHERE category='event' AND status='active')
GROUP BY embedding_type;
```

### Acceptance Criteria

- [ ] 所有有 description 的 event 标签都有 `metadata.event_keywords`
- [ ] `topic_tag_embeddings` 中有 `event_keyword` 类型的行
- [ ] 每个 event 标签的 keyword embedding 行数 = keywords 数量
- [ ] embedding_queue 中无残留 pending event 任务

### Blocked by

- Issue #1
- Issue #2
- Issue #3

---

## Issue #5: 端到端验证 concept 匹配和 bootstrap

**Type:** HITL (需要人工观察效果)
**Blocked by:** Issue #4

### What to build

Keywords embedding 生成后，验证 concept 匹配加权、bootstrap 聚类、hierarchy placement 完整流程。

### Task 8: 验证 concept 匹配和 bootstrap

**Step 1: 触发 bootstrap**

手动触发 concept bootstrap，验证 event 标签能聚类生成 concept。

**Step 2: pg-diagnose 验证**

```sql
-- board_concepts 应有 event 类型的 concept
SELECT * FROM board_concepts WHERE category = 'event';

-- event 标签应在 hierarchy 中
SELECT count(DISTINCT r.child_id) FROM topic_tag_relations r
JOIN topic_tags c ON r.child_id = c.id
WHERE c.category = 'event' AND c.status = 'active';

-- concept embedding 应已生成
SELECT count(*) FROM topic_tag_embeddings
WHERE embedding_type = 'concept' AND topic_tag_id IN (
  SELECT tag_id FROM board_concepts_tags
);
```

**Step 3: 验证加权匹配逻辑**

检查 `MatchTagToConcept` 对 event 标签使用了多行 embedding 加权平均。

### Acceptance Criteria

- [ ] `board_concepts` 中有 event 类型的 concept
- [ ] Event 标签在 hierarchy 中有 parent
- [ ] Concept embedding 已生成
- [ ] 无孤立 event 标签（或归入 default concept）

### Blocked by

- Issue #4

---

## 验证清单

- [ ] `go build ./...` 编译通过
- [ ] `go test ./...` 全部通过
- [ ] `golangci-lint run ./...` 无新增 warning
- [ ] 新建 event 标签自动触发 description + keywords 生成
- [ ] Backfill 批量路径正确提取 event keywords
- [ ] `topic_tag_embeddings` 中有 `event_keyword` 类型行
- [ ] Event 标签 concept 匹配和 bootstrap 正常工作
- [ ] 非 event 标签行为无回归
