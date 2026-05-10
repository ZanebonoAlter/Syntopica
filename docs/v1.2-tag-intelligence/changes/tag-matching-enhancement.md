# Tag 匹配增强：人物属性、叙事联动、Prompt 分化、关注标签叙事

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 解决人物标签 embedding 匹配分散、抽象标签 re-embedding 跑偏、事件标签缺少叙事上下文、关注标签缺少专属叙事总结四个问题，提升标签聚合质量。

**Architecture:** 四阶段递进：先给 person 标签加结构化属性增强 embedding 语义锚定，再按 category 分化 re-embedding prompt 防跑偏，让叙事结果反馈到事件标签合并，最后为关注标签生成专属叙事维度总结。

**Tech Stack:** Go, GORM, PostgreSQL + pgvector, LLM (airouter)

---

## 背景

### 问题 1：人物标签 embedding 匹配太分散

当前 `buildTagEmbeddingText` 只拼接 `label + description + aliases + category`。同一人物的不同称谓（"贝森特" vs "美国财长贝森特"）在 embedding 空间距离很远，导致：

- 无法正确合并同一人物
- 中间带触发 LLM 抽象提取，产出"美国财长贝森特经济观点"这种超出范围的结果
- 子标签 "贝森特" 和 "美国财长贝森特" 本应是同一人却成了两个独立标签

### 问题 2：抽象标签 re-embedding prompt 跑偏

`regenerateAbstractLabelAndDescription` 的 prompt 对所有 category 使用同一模板，LLM 对人物标签容易：

- 发散到观点/立场等超出实际覆盖的描述
- 多此一举地总结，比如"经济观点"实际上子标签只是一个人名的不同写法

### 问题 3：事件标签缺少叙事上下文

"美伊海上封锁"、"巴基斯坦促进和谈"、"美伊停火"在叙事维度是同一条线索，但在事件标签中因缺少上下文而分散。叙事系统有这个上下文（`related_tag_ids`），但没有反馈给标签匹配。

### 问题 4：关注标签缺少专属叙事总结

用户标记关注（watched）的标签代表持续追踪的主题，但当前叙事系统只做全局层面的叙事线索发现，不会针对某个关注标签生成"这个标签最近的发展脉络"这类维度总结。用户需要看到：我关注的美伊局势，最近几天有什么进展。

---

## Phase 1：Person 标签结构化属性 + 增强 Embedding

### Task 1.1：TopicTag 模型新增 `metadata` 字段

**Files:**
- Modify: `backend-go/internal/domain/models/topic_graph.go:57` (TopicTag struct, QualityScore 后)
- Modify: `backend-go/internal/platform/database/postgres_migrations.go` (追加迁移)

**Step 1：添加迁移**

在 `postgres_migrations.go` 的 `migrations` 切片末尾追加：

```go
{
    Version:     "20260416_0002",
    Description: "Add metadata JSONB column to topic_tags for structured tag attributes.",
    Up: func(db *gorm.DB) error {
        if err := db.Exec("ALTER TABLE topic_tags ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}'::jsonb").Error; err != nil {
            return fmt.Errorf("add metadata column to topic_tags: %w", err)
        }
        return nil
    },
},
```

**Step 2：添加 TopicTag struct 字段**

在 `topic_graph.go` 的 `TopicTag` struct 中 `QualityScore` 后面添加：

```go
Metadata MetadataMap `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
```

> **注意：** 使用 `serializer:json` 确保 GORM 自动将 map 序列化为 JSONB。`MetadataMap` 类型定义在同一文件中。

在 `topic_graph.go` 文件中（struct 外、`TableName` 前）定义类型：

```go
type MetadataMap map[string]any
```

> **为什么不用 `map[string]interface{}`：** 命名类型方便后续添加方法（如 `GetCountry()`），且 `any` 是 Go 1.18+ 惯用写法。项目未使用 `gorm/datatypes`（已确认 go.mod 中无此依赖），自行定义最简洁。

**Step 3：验证迁移运行**

Run: `cd backend-go && go run cmd/server/main.go` (观察日志确认迁移执行)
Expected: 日志中出现 `20260416_0002` migration applied

**Step 4：Commit**

```bash
git add backend-go/internal/domain/models/topic_graph.go backend-go/internal/platform/database/postgres_migrations.go
git commit -m "feat: add metadata JSONB column to topic_tags for structured tag attributes"
```

---

### Task 1.2：人物标签描述生成时提取结构化属性

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go:369-440` (`generateTagDescription`)

**修改策略：增量修改，不重写函数。** 只在现有函数中插入 person 分支。

**Step 1：在 `generateTagDescription` 函数中插入 person 分支**

具体修改点（在现有 `router := airouter.NewRouter()` 之前）：

```go
func generateTagDescription(tagID uint, label, category, articleContext string) {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("[WARN] generateTagDescription panic for tag %d: %v", tagID, r)
        }
    }()

    // ===== 新增：person 类标签使用增强 prompt =====
    if category == "person" {
        generatePersonTagDescription(tagID, label, articleContext)
        return
    }
    // ===== 新增结束 =====

    router := airouter.NewRouter()
    // ... 现有非 person 代码完全不变 ...
}
```

**Step 2：在同一文件中添加 `generatePersonTagDescription` 函数**

放在 `generateTagDescription` 之后：

```go
func generatePersonTagDescription(tagID uint, label, articleContext string) {
    router := airouter.NewRouter()

    prompt := fmt.Sprintf(`Given this person tag and article context, generate a description and extract structured attributes.

Tag: %q
Category: person
Context from article: %s

Description requirements:
- Must be in Chinese (中文)
- Objective, factual statement about WHO this person IS, not what they said or did in this specific article
- Keep under 500 characters
- Focus on: identity, position, affiliation

Structured attributes to extract:
- country: nationality or primary country of activity (中文, e.g. "美国", "中国")
- organization: primary organization or institution (中文)
- role: primary position or title (中文, e.g. "财政部长", "CEO")
- domains: areas of expertise or influence, as array of strings (中文, e.g. ["经济政策", "金融监管"])

Respond with JSON: {"description": "your answer", "person_attrs": {"country": "...", "organization": "...", "role": "...", "domains": [...]}}`, label, articleContext)

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
                "description": {Type: "string", Description: "人物标签的中文客观描述"},
                "person_attrs": {
                    Type: "object",
                    Properties: map[string]airouter.SchemaProperty{
                        "country":      {Type: "string", Description: "国籍或主要活动国家"},
                        "organization": {Type: "string", Description: "主要组织或机构"},
                        "role":         {Type: "string", Description: "主要职务或头衔"},
                        "domains":      {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}, Description: "专业领域"},
                    },
                },
            },
            Required: []string{"description", "person_attrs"},
        },
        Temperature: func() *float64 { f := 0.3; return &f }(),
    }

    result, err := router.Chat(context.Background(), req)
    if err != nil {
        log.Printf("[WARN] Person description LLM call failed for tag %d: %v", tagID, err)
        return
    }

    var parsed struct {
        Description string `json:"description"`
        PersonAttrs struct {
            Country      string   `json:"country"`
            Organization string   `json:"organization"`
            Role         string   `json:"role"`
            Domains      []string `json:"domains"`
        } `json:"person_attrs"`
    }
    if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil || parsed.Description == "" {
        log.Printf("[WARN] Failed to parse person description for tag %d", tagID)
        return
    }

    desc := parsed.Description
    if len([]rune(desc)) > 500 {
        desc = string([]rune(desc)[:500])
    }

    metadataMap := map[string]any{
        "country":      parsed.PersonAttrs.Country,
        "organization": parsed.PersonAttrs.Organization,
        "role":         parsed.PersonAttrs.Role,
        "domains":      parsed.PersonAttrs.Domains,
    }

    if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tagID).Updates(map[string]any{
        "description": desc,
        "metadata":    models.MetadataMap(metadataMap),
    }).Error; err != nil {
        log.Printf("[WARN] Failed to save description+metadata for person tag %d: %v", tagID, err)
        return
    }

    qs := getEmbeddingQueueService()
    if err := qs.Enqueue(tagID); err != nil {
        log.Printf("[WARN] Failed to enqueue re-embedding after person description update for tag %d: %v", tagID, err)
    }
}
```

> **日志风格：** 新函数使用 `log.Printf` 与 `generateTagDescription` 保持一致（现有代码用标准库 `log`，非 `logging` 包），避免同一函数内混用两种日志。

**Step 3：验证编译通过**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 4：Commit**

```bash
git add backend-go/internal/domain/topicextraction/tagger.go
git commit -m "feat: extract structured person attributes during tag description generation"
```

---

### Task 1.3：增强 `buildTagEmbeddingText` 使用人物属性

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/embedding.go:643-664` (`buildTagEmbeddingText`)

**Step 1：修改 embedding 文本构建**

> **关键设计：** person 标签的结构化身份属性（role、country、org）拼接在 label 之后、description 之前，而非末尾。embedding 模型对文本开头的权重更高，"贝森特 + 财政部长 + 美国" 靠前能最大化同一人物不同称谓的向量接近度。

```go
func buildTagEmbeddingText(tag *models.TopicTag) string {
    text := tag.Label

    // person 标签：紧跟身份属性，提高语义锚定效果
    if tag.Category == "person" && tag.Metadata != nil {
        if role, ok := tag.Metadata["role"].(string); ok && role != "" {
            text += " " + role
        }
        if country, ok := tag.Metadata["country"].(string); ok && country != "" {
            text += " " + country
        }
        if org, ok := tag.Metadata["organization"].(string); ok && org != "" {
            text += " " + org
        }
    }

    if tag.Description != "" {
        text += ". " + tag.Description
    }

    if tag.Aliases != "" {
        var aliases []string
        if err := json.Unmarshal([]byte(tag.Aliases), &aliases); err == nil {
            for _, alias := range aliases {
                text += " " + alias
            }
        } else {
            text += " " + tag.Aliases
        }
    }

    text += " " + tag.Category

    // person 标签：追加领域信息（权重较低，放末尾）
    if tag.Category == "person" && tag.Metadata != nil {
        if domains, ok := tag.Metadata["domains"].([]any); ok {
            for _, d := range domains {
                if ds, ok := d.(string); ok && ds != "" {
                    text += " " + ds
                }
            }
        }
    }

    return text
}
```

**Step 2：更新已有测试**

`embedding_test.go` 中有 `TestBuildTagEmbeddingText`，需要添加 person metadata 的测试用例。在现有测试中追加：

```go
t.Run("person with metadata", func(t *testing.T) {
    tag := &models.TopicTag{
        Label:    "贝森特",
        Category: "person",
        Metadata: models.MetadataMap{
            "country":      "美国",
            "role":         "财政部长",
            "organization": "美国财政部",
            "domains":      []any{"经济政策", "金融监管"},
        },
    }
    result := buildTagEmbeddingText(tag)
    if !strings.Contains(result, "财政部长") {
        t.Errorf("expected role in embedding text, got: %s", result)
    }
    if !strings.Contains(result, "美国") {
        t.Errorf("expected country in embedding text, got: %s", result)
    }
    // 验证顺序：role 和 country 在 description 之前
    roleIdx := strings.Index(result, "财政部长")
    descMarker := ". " // description 前缀
    descIdx := strings.Index(result, descMarker)
    if descIdx >= 0 && roleIdx >= descIdx {
        t.Errorf("role should appear before description in embedding text, got: %s", result)
    }
})
```

**Step 3：验证编译和测试通过**

Run: `cd backend-go && go build ./... && go test ./internal/domain/topicanalysis/... -run TestBuildTagEmbeddingText -v`

**Step 4：Commit**

```bash
git add backend-go/internal/domain/topicanalysis/embedding.go backend-go/internal/domain/topicanalysis/embedding_test.go
git commit -m "feat: enrich person tag embedding text with structured attributes (identity-first order)"
```

---

### Task 1.4：已有 person 标签的 metadata 补填

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/person_metadata_backfill.go`

**Step 1：创建补填函数**

为已有的 person 标签（无 metadata 或 metadata 为空）异步补填结构化属性。

> **注意：** 此文件在 `topicanalysis` 包中，该包有 `NewEmbeddingQueueService(logger *zap.Logger)` 构造函数（传 `nil` 会 fallback 到 `zap.NewNop()`），不使用 `topicextraction` 包中的 `getEmbeddingQueueService()`（跨包不可访问）。

```go
package topicanalysis

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/platform/airouter"
    "my-robot-backend/internal/platform/database"
    "my-robot-backend/internal/platform/logging"
)

func BackfillPersonMetadata() (int, error) {
    var tags []models.TopicTag
    if err := database.DB.Where("category = ? AND status = ? AND (metadata IS NULL OR metadata = '{}'::jsonb OR metadata = '')", "person", "active").
        Limit(100).
        Find(&tags).Error; err != nil {
        return 0, fmt.Errorf("query person tags without metadata: %w", err)
    }

    logging.Infof("person metadata backfill: found %d tags to process", len(tags))

    processed := 0
    for _, tag := range tags {
        if err := backfillSinglePersonMetadata(tag); err != nil {
            logging.Warnf("person metadata backfill failed for tag %d (%s): %v", tag.ID, tag.Label, err)
            continue
        }
        processed++
        // 限速：每批次间休眠，避免 LLM API 过载
        time.Sleep(500 * time.Millisecond)
    }

    logging.Infof("person metadata backfill: processed %d/%d", processed, len(tags))
    return processed, nil
}

func backfillSinglePersonMetadata(tag models.TopicTag) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    router := airouter.NewRouter()

    prompt := fmt.Sprintf(`Extract structured attributes for this person tag.

Tag: %q
Description: %s

Extract:
- country: nationality or primary country (中文)
- organization: primary organization (中文)
- role: primary position or title (中文)
- domains: areas of expertise as array (中文)

Respond with JSON: {"country": "...", "organization": "...", "role": "...", "domains": [...]}`, tag.Label, tag.Description)

    result, err := router.Chat(ctx, airouter.ChatRequest{
        Capability: airouter.CapabilityTopicTagging,
        Messages: []airouter.Message{
            {Role: "system", Content: "你是一个人物属性提取助手，只输出合法JSON。"},
            {Role: "user", Content: prompt},
        },
        JSONMode: true,
        JSONSchema: &airouter.JSONSchema{
            Type: "object",
            Properties: map[string]airouter.SchemaProperty{
                "country":      {Type: "string"},
                "organization": {Type: "string"},
                "role":         {Type: "string"},
                "domains":      {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}},
            },
        },
        Temperature: func() *float64 { f := 0.2; return &f }(),
    })
    if err != nil {
        return fmt.Errorf("LLM call failed: %w", err)
    }

    var attrs struct {
        Country      string   `json:"country"`
        Organization string   `json:"organization"`
        Role         string   `json:"role"`
        Domains      []string `json:"domains"`
    }
    if err := json.Unmarshal([]byte(result.Content), &attrs); err != nil {
        return fmt.Errorf("parse response: %w", err)
    }

    metadataMap := models.MetadataMap{
        "country":      attrs.Country,
        "organization": attrs.Organization,
        "role":         attrs.Role,
        "domains":      attrs.Domains,
    }

    if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).
        Update("metadata", metadataMap).Error; err != nil {
        return fmt.Errorf("update metadata: %w", err)
    }

    qs := NewEmbeddingQueueService(nil)
    if err := qs.Enqueue(tag.ID); err != nil {
        logging.Warnf("Failed to enqueue re-embedding after metadata backfill for tag %d: %v", tag.ID, err)
    }

    logging.Infof("person metadata backfilled for tag %d (%s): country=%s, org=%s, role=%s",
        tag.ID, tag.Label, attrs.Country, attrs.Organization, attrs.Role)
    return nil
}
```

**Step 2：注册 API 触发端点**

在 `embedding_queue_handler.go` 或新增 handler 中添加手动触发路由：

```
POST /api/embedding/person-metadata/backfill
```

调用 `BackfillPersonMetadata()` 并返回 `{"success": true, "data": {"processed": N}}`。

**Step 3：验证编译通过**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 4：Commit**

```bash
git add backend-go/internal/domain/topicanalysis/person_metadata_backfill.go
git commit -m "feat: add person metadata backfill for existing person tags with rate limiting"
```

---

## Phase 2：按 Category 分化 Re-embedding Prompt

### Task 2.1：分化 `regenerateAbstractLabelAndDescription` prompt

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go:277-353` (`regenerateAbstractLabelAndDescription`)

**Step 1：按 category 分化 prompt**

> **修改策略：** 只替换 `prompt := fmt.Sprintf(...)` 那一段（当前在 L287-303），将其改为 `switch abstractTag.Category` 分支。后续的 `router := airouter.NewRouter()` 及 `req := airouter.ChatRequest{...}` 逻辑完全不变。

将现有的：

```go
prompt := fmt.Sprintf(`Given this abstract topic tag and its child tags...`, ...)
```

替换为：

```go
var prompt string
switch abstractTag.Category {
case "person":
    prompt = fmt.Sprintf(`你是一个标签分类助手。给定一个人物类型的抽象标签及其子标签，重新生成抽象标签的 label 和 description。

抽象标签: %q
当前描述: %s

子标签:
%s

要求:
- label: 概括所有子标签指向的人物身份（1-160字）。保持当前 label 如果仍然准确。
- description: 中文，1-2 句话，客观说明这是谁、什么身份。不要延伸到观点、立场、事件。不要评价。500 字以内。
- 重点：这个抽象标签代表的是"人"，不是"人的观点"。label 围绕人物身份，description 说明身份背景。
- 如果子标签都是同一个人的不同称谓，label 应该是最标准的称谓，description 说明其身份。

示例:
- 子标签 "贝森特", "美国财长贝森特" → label: "贝森特", description: "美国财政部长斯科特·贝森特（Scott Bessent），曾任对冲基金经理"
- 子标签 "马斯克", "Elon Musk", "特斯拉CEO" → label: "马斯克", description: "特斯拉和 SpaceX 首席执行官，X（原 Twitter）所有者"

返回 JSON: {"label": "your answer", "description": "your answer"}`,
        abstractTag.Label,
        abstractTag.Description,
        strings.Join(childParts, "\n"))

case "event":
    prompt = fmt.Sprintf(`你是一个标签分类助手。给定一个事件类型的抽象标签及其子标签，重新生成抽象标签的 label 和 description。

抽象标签: %q
当前描述: %s

子标签:
%s

要求:
- label: 概括所有子标签涉及的事件主线（1-160字）。保持当前 label 如果仍然准确。
- description: 中文，1-2 句话，客观说明事件是什么、涉及哪些方面。不要延伸到影响分析、价值判断。500 字以内。
- 重点：这个抽象标签代表的是"事件/事态"，聚焦于事实经过和涉及方。

返回 JSON: {"label": "your answer", "description": "your answer"}`,
        abstractTag.Label,
        abstractTag.Description,
        strings.Join(childParts, "\n"))

default: // keyword 及其他
    prompt = fmt.Sprintf(`Given this abstract topic tag and its child tags, regenerate the abstract tag's label and description.

Abstract tag: %q
Current description: %s

Child tags:
%s

Label and description requirements:
- label: A concise name (1-160 chars) that encompasses ALL child tags. Keep the current label if it still accurately represents the child tags. Only change it if the child tag scope has clearly shifted. Must be in the original language of the tags.
- description: Must be in Chinese (中文). Objective, factual summary that encompasses ALL child tags. 1-2 sentences, under 500 characters. Must explain the concept, not just restate the name. Should be broader than any single child tag's description.

Respond with JSON: {"label": "your answer", "description": "your answer"}`,
        abstractTag.Label,
        abstractTag.Description,
        strings.Join(childParts, "\n"))
}
```

**Step 2：验证编译通过**

Run: `cd backend-go && go build ./...`
Expected: 编译成功

**Step 3：Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_update_queue.go
git commit -m "feat: differentiate re-embedding prompt by tag category (person/event/keyword)"
```

---

### Task 2.2：同步分化 `ExtractAbstractTag` 的 LLM prompt

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` (`callLLMForAbstractName` + `buildAbstractTagPrompt`)

**Step 1：修改 `buildAbstractTagPrompt` 接受 category 参数**

当前签名（L562）：

```go
func buildAbstractTagPrompt(candidates []TagCandidate, newLabel string) string
```

改为：

```go
func buildAbstractTagPrompt(candidates []TagCandidate, newLabel string, category string) string
```

在函数体中，`return fmt.Sprintf(...)` 那段替换为按 category 分支（与 Task 2.1 类似的分化策略）。`default` 分支保留当前英文 prompt。

**Step 2：修改 `callLLMForAbstractName` 签名**

当前签名（L525）：

```go
func callLLMForAbstractName(ctx context.Context, candidates []TagCandidate, newLabel string) (string, string, error)
```

改为：

```go
func callLLMForAbstractName(ctx context.Context, candidates []TagCandidate, newLabel string, category string) (string, string, error)
```

函数体内 `buildAbstractTagPrompt` 调用追加 `category` 参数。

**Step 3：更新 `ExtractAbstractTag` 调用处**

`ExtractAbstractTag`（L42）已有 `category` 参数，只需在 L54 的 `callLLMForAbstractName` 调用中追加 `category`：

```go
abstractName, abstractDesc, err := callLLMForAbstractName(ctx, candidates, newLabel, category)
```

**Step 4：更新测试**

`abstract_tag_service_test.go` 中有 `TestBuildAbstractTagPrompt` 和 `TestBuildAbstractTagPromptWithDescription`，需要：
- 更新函数调用签名（加 `category` 参数）
- 添加 `person` 和 `event` category 的测试用例

**Step 5：验证编译和测试通过**

Run: `cd backend-go && go build ./... && go test ./internal/domain/topicanalysis/... -run TestBuildAbstractTagPrompt -v`

**Step 6：Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go
git commit -m "feat: differentiate abstract tag extraction prompt by category"
```

---

## Phase 3：叙事反馈到事件标签合并

### Task 3.1：叙事保存后触发事件标签聚合检查

**Files:**
- Modify: `backend-go/internal/domain/narrative/service.go:51` (GenerateAndSave)
- Create: `backend-go/internal/domain/narrative/tag_feedback.go`

**Step 1：创建 `tag_feedback.go`**

> **速率控制：** 限制每条叙事最多处理 5 对标签对，避免大量叙事时产生 goroutine 和 LLM 调用风暴。

```go
package narrative

import (
    "context"
    "fmt"

    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/domain/topicanalysis"
    "my-robot-backend/internal/platform/database"
    "my-robot-backend/internal/platform/logging"
)

const maxPairsPerNarrative = 5

func feedbackNarrativesToTags(outputs []NarrativeOutput) {
    for _, out := range outputs {
        if len(out.RelatedTagIDs) < 2 {
            continue
        }
        go checkNarrativeEventTagClustering(out)
    }
}

func checkNarrativeEventTagClustering(out NarrativeOutput) {
    defer func() {
        if r := recover(); r != nil {
            logging.Warnf("checkNarrativeEventTagClustering panic: %v", r)
        }
    }()

    var tags []models.TopicTag
    database.DB.Where("id IN ? AND category = ? AND status = ?", out.RelatedTagIDs, "event", "active").Find(&tags)
    if len(tags) < 2 {
        return
    }

    var eventTagIDs []uint
    for _, t := range tags {
        eventTagIDs = append(eventTagIDs, t.ID)
    }

    // 排除已有父子关系的标签
    var relatedIDs []uint
    database.DB.Model(&models.TopicTagRelation{}).
        Where("(parent_id IN ? OR child_id IN ?) AND relation_type = ?", eventTagIDs, eventTagIDs, "abstract").
        Pluck("parent_id", &relatedIDs)
    var childIDs []uint
    database.DB.Model(&models.TopicTagRelation{}).
        Where("(parent_id IN ? OR child_id IN ?) AND relation_type = ?", eventTagIDs, eventTagIDs, "abstract").
        Pluck("child_id", &childIDs)
    relatedIDs = append(relatedIDs, childIDs...)
    relatedSet := make(map[uint]bool, len(relatedIDs))
    for _, id := range relatedIDs {
        relatedSet[id] = true
    }

    var unclusteredIDs []uint
    for _, id := range eventTagIDs {
        if !relatedSet[id] {
            unclusteredIDs = append(unclusteredIDs, id)
        }
    }
    if len(unclusteredIDs) < 2 {
        return
    }

    es := topicanalysis.NewEmbeddingService()
    ctx := context.Background()

    pairsChecked := 0
    for i := 0; i < len(unclusteredIDs) && pairsChecked < maxPairsPerNarrative; i++ {
        for j := i + 1; j < len(unclusteredIDs) && pairsChecked < maxPairsPerNarrative; j++ {
            pairsChecked++
            idA, idB := unclusteredIDs[i], unclusteredIDs[j]

            var embA, embB models.TopicTagEmbedding
            if err := database.DB.Where("topic_tag_id = ?", idA).First(&embA).Error; err != nil {
                continue
            }
            if err := database.DB.Where("topic_tag_id = ?", idB).First(&embB).Error; err != nil {
                continue
            }

            // pgvector <=> 是余弦距离（cosine distance），similarity = 1 - distance
            sim, err := computeEmbeddingSimilarity(embA.EmbeddingVec, embB.EmbeddingVec)
            if err != nil {
                continue
            }

            thresholds := es.GetThresholds()

            // 只处理 middle band：有语义关联但不够自动合并
            if sim >= thresholds.LowSimilarity && sim < thresholds.HighSimilarity {
                logging.Infof("narrative-tag-feedback: event tags %d and %d have similarity %.4f (in middle band), triggering abstract extraction with narrative context",
                    idA, idB, sim)

                narrativeContext := fmt.Sprintf("Narrative: %s\nSummary: %s", out.Title, out.Summary)
                triggerAbstractExtractionWithContext(ctx, idA, idB, narrativeContext)
            }
        }
    }
}

func computeEmbeddingSimilarity(vecAStr, vecBStr string) (float64, error) {
    // pgvector cosine distance (<=>): distance = 1 - cosine_similarity
    // 所以 similarity = 1 - distance
    query := "SELECT ($1::vector <=> $2::vector) AS distance"
    var distance float64
    if err := database.DB.Raw(query, vecAStr, vecBStr).Scan(&distance).Error; err != nil {
        return 0, err
    }
    return 1.0 - distance, nil
}

func triggerAbstractExtractionWithContext(ctx context.Context, tagAID, tagBID uint, narrativeContext string) {
    var tagA, tagB models.TopicTag
    if err := database.DB.First(&tagA, tagAID).Error; err != nil {
        return
    }
    if err := database.DB.First(&tagB, tagBID).Error; err != nil {
        return
    }

    candidates := []topicanalysis.TagCandidate{
        {Tag: &tagA},
        {Tag: &tagB},
    }

    abstractTag, err := topicanalysis.ExtractAbstractTag(ctx, candidates, tagA.Label, tagA.Category,
        topicanalysis.WithNarrativeContext(narrativeContext))
    if err != nil || abstractTag == nil {
        logging.Warnf("narrative-tag-feedback: abstract extraction with context failed for %d+%d: %v", tagAID, tagBID, err)
        return
    }

    logging.Infof("narrative-tag-feedback: created abstract tag %d (%s) from narrative-driven clustering of %d+%d",
        abstractTag.ID, abstractTag.Label, tagAID, tagBID)
}
```

**Step 2：在 `GenerateAndSave` 中调用反馈**

在 `service.go:51` 的 `markEndedNarratives(date, outputs, prevNarratives)` 之后添加：

```go
go feedbackNarrativesToTags(outputs)
```

**Step 3：验证编译通过**

Run: `cd backend-go && go build ./...`

**Step 4：Commit**

```bash
git add backend-go/internal/domain/narrative/tag_feedback.go backend-go/internal/domain/narrative/service.go
git commit -m "feat: feedback narrative context to event tag clustering with rate limiting"
```

---

### Task 3.2：为 `ExtractAbstractTag` 添加 narrative context 支持（functional options 模式）

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go`

**设计：** 不创建平行函数（避免事务逻辑、关系创建、cycle 检测等代码重复），而是用 functional options 模式给 `ExtractAbstractTag` 添加可选的叙事上下文。

**Step 1：定义 option 类型**

在 `abstract_tag_service.go` 中添加：

```go
type ExtractAbstractTagOption func(*extractAbstractTagConfig)

type extractAbstractTagConfig struct {
    narrativeContext string
}

func WithNarrativeContext(ctx string) ExtractAbstractTagOption {
    return func(c *extractAbstractTagConfig) {
        c.narrativeContext = ctx
    }
}
```

**Step 2：修改 `ExtractAbstractTag` 签名**

```go
func ExtractAbstractTag(ctx context.Context, candidates []TagCandidate, newLabel string, category string, opts ...ExtractAbstractTagOption) (*models.TopicTag, error) {
```

在函数开头解析 options：

```go
cfg := &extractAbstractTagConfig{}
for _, opt := range opts {
    opt(cfg)
}
```

修改 `callLLMForAbstractName` 调用，传入叙事上下文：

```go
abstractName, abstractDesc, err := callLLMForAbstractName(ctx, candidates, newLabel, category, cfg.narrativeContext)
```

**Step 3：修改 `callLLMForAbstractName` 签名和实现**

```go
func callLLMForAbstractName(ctx context.Context, candidates []TagCandidate, newLabel string, category string, narrativeContext string) (string, string, error) {
```

在 `buildAbstractTagPrompt` 调用之后、构造 `req` 之前，注入叙事上下文：

```go
prompt := buildAbstractTagPrompt(candidates, newLabel, category)

if narrativeContext != "" {
    prompt += fmt.Sprintf("\n\nAdditional context from narrative analysis:\n%s\nUse this context to help determine if these tags belong to the same thematic thread.", narrativeContext)
}
```

**Step 4：更新调用方**

除了 Phase 3 的新调用方（使用 `WithNarrativeContext`），还需要更新 `findOrCreateTag` 中的现有调用（`abstract_tag_service.go` 中搜 `ExtractAbstractTag(`），添加空 options 或保持默认行为（`opts` 为空时 `cfg.narrativeContext` 为 `""`，行为不变）。

**Step 5：验证编译通过**

Run: `cd backend-go && go build ./...`

**Step 6：Commit**

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "feat: add functional options to ExtractAbstractTag for narrative context injection"
```

---

## Phase 4：关注标签叙事维度总结

### Task 4.1：关注标签叙事维度总结生成

**Files:**
- Create: `backend-go/internal/domain/narrative/watched_narrative.go`

**背景：** 用户标记关注的标签（`is_watched = true`）代表持续追踪的主题。当前叙事系统只做全局叙事线索发现，不会针对某个关注标签生成专属的发展脉络总结。需要在每次叙事生成后，为每个有活跃文章的关注标签生成一个独立的"标签叙事"——汇总这个标签最近几天的文章内容，输出进展摘要。

**Step 1：创建 `watched_narrative.go`**

```go
package narrative

import (
    "context"
    "fmt"
    "strings"
    "time"

    "my-robot-backend/internal/domain/models"
    "my-robot-backend/internal/domain/topicanalysis"
    "my-robot-backend/internal/platform/airouter"
    "my-robot-backend/internal/platform/database"
    "my-robot-backend/internal/platform/logging"
)

type WatchedTagNarrativeOutput struct {
    TagID     uint   `json:"tag_id"`
    TagLabel  string `json:"tag_label"`
    Summary   string `json:"summary"`
    DateRange string `json:"date_range"`
}

// GenerateWatchedTagNarratives 为所有关注标签生成叙事维度总结。
// 在 GenerateAndSave 之后异步调用。
func GenerateWatchedTagNarratives(date time.Time) {
    watchedIDs, childIDs, err := topicanalysis.GetWatchedTagIDsExpanded(database.DB)
    if err != nil {
        logging.Warnf("watched-narrative: failed to get watched tags: %v", err)
        return
    }
    if len(watchedIDs) == 0 {
        return
    }

    // 加上子标签 ID，扩大文章覆盖面
    allTagIDs := append(watchedIDs, childIDs...)
    watchedSet := make(map[uint]bool, len(watchedIDs))
    for _, id := range watchedIDs {
        watchedSet[id] = true
    }

    // 查询最近 3 天有活跃文章的标签
    since := date.AddDate(0, 0, -2)
    endOfDay := date.Add(24 * time.Hour)

    type tagActivity struct {
        TopicTagID uint
        Cnt        int
    }
    var activities []tagActivity
    database.DB.Model(&models.ArticleTopicTag{}).
        Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
        Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
        Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", allTagIDs, since, endOfDay).
        Group("article_topic_tags.topic_tag_id").
        Having("COUNT(DISTINCT article_topic_tags.article_id) >= 2").
        Scan(&activities)

    if len(activities) == 0 {
        return
    }

    // 找出活跃的 watched 标签（包含子标签活跃的）
    activeWatchedMap := make(map[uint]int) // watched tag id -> article count
    for _, act := range activities {
        if watchedSet[act.TopicTagID] {
            activeWatchedMap[act.TopicTagID] = act.Cnt
        }
    }
    // 子标签的活跃算到其 watched 父标签上
    if len(childIDs) > 0 {
        var relations []models.TopicTagRelation
        database.DB.Where("child_id IN ? AND parent_id IN ?", childIDs, watchedIDs).Find(&relations)
        for _, rel := range relations {
            for _, act := range activities {
                if act.TopicTagID == rel.ChildID {
                    activeWatchedMap[rel.ParentID] += act.Cnt
                }
            }
        }
    }

    if len(activeWatchedMap) == 0 {
        return
    }

    // 逐个生成（限速）
    for tagID, articleCount := range activeWatchedMap {
        if articleCount < 2 {
            continue
        }
        go generateSingleWatchedNarrative(tagID, since, endOfDay)
        time.Sleep(200 * time.Millisecond)
    }
}

func generateSingleWatchedNarrative(watchedTagID uint, since, until time.Time) {
    defer func() {
        if r := recover(); r != nil {
            logging.Warnf("generateSingleWatchedNarrative panic for tag %d: %v", watchedTagID, r)
        }
    }()

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    // 加载标签信息
    var tag models.TopicTag
    if err := database.DB.First(&tag, watchedTagID).Error; err != nil {
        logging.Warnf("watched-narrative: tag %d not found: %v", watchedTagID, err)
        return
    }

    // 获取关联的文章标题和摘要
    var articles []models.Article
    database.DB.Joins("JOIN article_topic_tags ON article_topic_tags.article_id = articles.id").
        Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?", watchedTagID, since, until).
        Order("articles.pub_date DESC").
        Limit(20).
        Find(&articles)

    // 也获取子标签关联的文章（如果 watched 标签是抽象标签）
    var childIDs []uint
    database.DB.Model(&models.TopicTagRelation{}).
        Where("parent_id = ? AND relation_type = ?", watchedTagID, "abstract").
        Pluck("child_id", &childIDs)
    if len(childIDs) > 0 {
        var childArticles []models.Article
        database.DB.Joins("JOIN article_topic_tags ON article_topic_tags.article_id = articles.id").
            Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", childIDs, since, until).
            Order("articles.pub_date DESC").
            Limit(20).
            Find(&childArticles)
        articles = append(articles, childArticles...)
    }

    if len(articles) == 0 {
        return
    }

    // 去重
    seen := make(map[uint]bool)
    var uniqueArticles []models.Article
    for _, a := range articles {
        if !seen[a.ID] {
            seen[a.ID] = true
            uniqueArticles = append(uniqueArticles, a)
        }
    }
    articles = uniqueArticles

    // 构建文章摘要列表
    var articleParts []string
    for _, a := range articles {
        part := fmt.Sprintf("- [%s] %s", a.PubDate.Format("01-02"), a.Title)
        if a.AIContentSummary != "" {
            summary := a.AIContentSummary
            if len([]rune(summary)) > 200 {
                summary = string([]rune(summary)[:200]) + "..."
            }
            part += fmt.Sprintf(": %s", summary)
        }
        articleParts = append(articleParts, part)
    }

    prompt := fmt.Sprintf(`你是一个新闻分析助手。以下是与标签 "%s" 相关的近期文章列表。
请基于这些文章，生成一段该标签的近期发展脉络总结。

时间范围: %s ~ %s
相关文章:
%s

要求:
- 中文，300-800 字
- 按时间线梳理关键进展
- 客观总结事实，不要评价
- 如果文章间有因果或递进关系，明确指出
- 如果信息不足以生成有意义的发展脉络，直接说明

返回 JSON: {"summary": "你的总结"}`, tag.Label, since.Format("2006-01-02"), until.Format("2006-01-02"), strings.Join(articleParts, "\n"))

    router := airouter.NewRouter()
    result, err := router.Chat(ctx, airouter.ChatRequest{
        Capability: airouter.CapabilityNarrative,
        Messages: []airouter.Message{
            {Role: "system", Content: "你是一个新闻分析助手，只输出合法JSON。"},
            {Role: "user", Content: prompt},
        },
        JSONMode: true,
        JSONSchema: &airouter.JSONSchema{
            Type: "object",
            Properties: map[string]airouter.SchemaProperty{
                "summary": {Type: "string", Description: "该关注标签的近期发展脉络总结"},
            },
            Required: []string{"summary"},
        },
        Temperature: func() *float64 { f := 0.4; return &f }(),
    })
    if err != nil {
        logging.Warnf("watched-narrative: LLM call failed for tag %d (%s): %v", tag.ID, tag.Label, err)
        return
    }

    var parsed struct {
        Summary string `json:"summary"`
    }
    if err := parseJSONContent(result.Content, &parsed); err != nil || parsed.Summary == "" {
        logging.Warnf("watched-narrative: failed to parse response for tag %d: %v", tag.ID, err)
        return
    }

    // 保存到 narrative_summaries 表
    startOfDay := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location())
    record := models.NarrativeSummary{
        Title:         fmt.Sprintf("关注标签：%s 近期动态", tag.Label),
        Summary:       parsed.Summary,
        Status:        models.NarrativeStatusContinuing,
        Period:        "watched_tag",
        PeriodDate:    startOfDay,
        Generation:    1,
        RelatedTagIDs: fmt.Sprintf("[%d]", tag.ID),
        Source:        "ai",
    }

    if err := database.DB.Create(&record).Error; err != nil {
        logging.Warnf("watched-narrative: failed to save for tag %d (%s): %v", tag.ID, tag.Label, err)
        return
    }

    logging.Infof("watched-narrative: saved narrative for watched tag %d (%s), articles=%d", tag.ID, tag.Label, len(articles))
}

func parseJSONContent(content string, target any) error {
    content = strings.TrimSpace(content)
    content = strings.TrimPrefix(content, "```json")
    content = strings.TrimPrefix(content, "```")
    content = strings.TrimSuffix(content, "```")
    content = strings.TrimSpace(content)

    import "encoding/json"
    return json.Unmarshal([]byte(content), target)
}
```

> **注意：** `parseJSONContent` 中使用了 strip markdown fence 的逻辑，与 generator.go 中的 `stripMarkdownFence` 功能相同。实现时应复用 `stripMarkdownFence`，而非新建函数。上面的代码仅为示例——实际实现时删除 `parseJSONContent`，直接调用 `stripMarkdownFence` + `json.Unmarshal`。

**Step 2：在 `GenerateAndSave` 中触发**

在 `service.go` 的 `go feedbackNarrativesToTags(outputs)` 之后添加：

```go
go GenerateWatchedTagNarratives(date)
```

**Step 3：验证编译通过**

Run: `cd backend-go && go build ./...`

**Step 4：Commit**

```bash
git add backend-go/internal/domain/narrative/watched_narrative.go backend-go/internal/domain/narrative/service.go
git commit -m "feat: generate per-tag narrative summaries for watched tags"
```

---

## Phase 5：文档更新

### Task 5.1：更新 topic-graph 文档

**Files:**
- Modify: `docs/guides/topic-graph.md`

**Step 1：在文档中补充以下内容**

在"标签匹配与抽象层级"章节后追加：

- 人物标签的结构化属性机制（metadata 字段、属性提取、embedding 增强）
- 按 category 分化的 prompt 策略
- 叙事反馈到事件标签合并的流程
- 关注标签叙事维度总结

**Step 2：Commit**

```bash
git add docs/guides/topic-graph.md
git commit -m "docs: update topic-graph guide with person metadata, category prompts, narrative feedback, watched tag narratives"
```

---

## 验证清单

完成所有 Task 后，运行以下验证：

```bash
cd backend-go
go build ./...
go test ./internal/domain/topicanalysis/... -v
go test ./internal/domain/topicextraction/... -v
go test ./internal/domain/narrative/... -v
```

手动验证：
1. 启动服务，确认 migration 执行
2. 调用 `POST /api/embedding/person-metadata/backfill` 补填已有 person 标签
3. 观察日志中 person 标签的 re-embedding 是否包含结构化属性
4. 触发一次叙事生成，检查日志中是否有 `narrative-tag-feedback` 相关输出
5. 检查新生成的抽象标签 description 是否按 category 约束在合理范围内
6. 确认关注标签的叙事总结写入 `narrative_summaries`（period = "watched_tag"）

---

## 实现顺序和依赖关系

```
Phase 1 (Task 1.1 → 1.2 → 1.3 → 1.4)  ← 最优先，解决"贝森特"问题
    ↓
Phase 2 (Task 2.1 → 2.2)                ← 与 Phase 1 独立，可并行
    ↓
Phase 3 (Task 3.1 → 3.2)                ← 依赖 Phase 2 的 prompt 分化
    ↓
Phase 4 (Task 4.1)                       ← 独立功能，可与 Phase 2/3 并行
    ↓
Phase 5 (Task 5.1)                       ← 最后更新文档
```

**并行策略：**
- Phase 1 和 Phase 2 无代码依赖，可并行
- Phase 3 依赖 Phase 2（`ExtractAbstractTag` 需要 functional options + prompt 分化）
- Phase 4（关注标签叙事）独立于 Phase 1/2/3，可随时开始
- Phase 5 最后统一更新文档
