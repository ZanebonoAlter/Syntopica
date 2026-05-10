# 标签提取判断优化：merge vs abstract

**Goal:** 中间带（0.78~0.97 相似度）标签匹配时，让 AI 判断是 merge（同一概念合并）还是 abstract（创建抽象父标签），通过 JSON schema enum 强制约束。

**Architecture:** 在 `ExtractAbstractTag` 入口处新增 AI 判断步骤。LLM 返回 `action: "merge"` 或 `action: "abstract"`，调用方根据结果走不同分支。merge 复用现有标签，abstract 走原流程。三个 category（person/event/keyword）各有独立 prompt。

**Tech Stack:** Go, Gin, GORM, LLM (JSON mode + JSON schema enum)

---

## Task 1: SchemaProperty 添加 Enum 字段

**Files:**
- Modify: `backend-go/internal/platform/airouter/openai_compatible.go:28-34`

**改动：** `SchemaProperty` 结构体新增 `Enum []string` 字段（`json:"enum,omitempty"`），支持 JSON schema 的 enum 约束，让 LLM 输出限定在指定值范围内。

```go
type SchemaProperty struct {
    Type        string                    `json:"type,omitempty"`
    Enum        []string                  `json:"enum,omitempty"`      // 新增
    Items       *SchemaProperty           `json:"items,omitempty"`
    Properties  map[string]SchemaProperty `json:"properties,omitempty"`
    Required    []string                  `json:"required,omitempty"`
    Description string                    `json:"description,omitempty"`
}
```

**验证：** `go build ./...`

---

## Task 2: 新增 TagExtractionResult 类型和常量

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go:19-37`

**改动：** 在常量区新增 `ActionMerge`、`ActionAbstract`，新增 `TagExtractionResult` 结构体作为 `ExtractAbstractTag` 的新返回类型。

```go
const (
    maxAbstractNameLen = 160
    ActionMerge    = "merge"
    ActionAbstract = "abstract"
)

type TagExtractionResult struct {
    Action      string           // "merge" or "abstract"
    MergeTarget *models.TopicTag // for "merge": the existing tag to reuse
    MergeLabel  string           // for "merge": LLM-recommended unified label
    AbstractTag *models.TopicTag // for "abstract": the new abstract parent tag
}
```

**设计说明：**
- `ActionMerge`：标签指向同一概念，直接复用现有标签，不创建抽象标签
- `ActionAbstract`：标签相关但不同，走原流程创建抽象父标签

---

## Task 3: 替换 LLM 调用链（核心改动）

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go`

### 3a: 新增 tagJudgment 内部类型和 selectMergeTarget

```go
type tagJudgment struct {
    Action       string // "merge" or "abstract"
    AbstractName string // for "abstract"
    Description  string // for "abstract"
    MergeLabel   string // for "merge"
    Reason       string
}

func selectMergeTarget(candidates []TagCandidate, mergeLabel string) *models.TopicTag
```

`selectMergeTarget` 按 slug 匹配合并目标，fallback 到第一个 candidate。

### 3b: callLLMForAbstractName → callLLMForTagJudgment

**签名变更：**
- 旧：`callLLMForAbstractName(ctx, candidates, newLabel) (string, string, error)`
- 新：`callLLMForTagJudgment(ctx, candidates, newLabel, category, narrativeContext) (*tagJudgment, error)`

**JSON Schema 变更：**

旧 schema（只返回抽象名称）：
```json
{"properties": {"abstract_name": ..., "description": ..., "reason": ...}, "required": ["abstract_name", "reason"]}
```

新 schema（带 action 枚举约束）：
```json
{
  "properties": {
    "action":        {"type": "string", "enum": ["merge", "abstract"]},
    "merge_label":   {"type": "string", "description": "合并后的统一名称（action=merge 时必填）"},
    "abstract_name": {"type": "string", "description": "抽象标签名称（action=abstract 时必填）"},
    "description":   {"type": "string", "description": "描述（action=abstract 时必填）"},
    "reason":        {"type": "string", "description": "判断理由"}
  },
  "required": ["action", "reason"]
}
```

**关键设计：** `required` 只包含 `["action", "reason"]`，但 `parseTagJudgmentResponse` 在代码层根据 action 校验对应字段非空。这比 JSON schema `oneOf` 兼容性更好。

### 3c: buildAbstractTagPrompt → buildTagJudgmentPrompt

三个 category 的 prompt 全面重写，核心变化：

| 旧 prompt | 新 prompt |
|-----------|-----------|
| "提取一个概括所有人的抽象标签" | "判断这些标签的关系：merge（同一人物）还是 abstract（不同人物）" |
| "提取一个概括所有事件的抽象标签" | "判断这些标签的关系：merge（同一事件）还是 abstract（不同事件）" |
| "Extract a common abstract concept" | "Judge the relationship: merge (same concept) or abstract (related concepts)" |

每个 prompt 都包含：
1. merge 和 abstract 的定义和判断标准
2. "不确定时优先使用 abstract" 的保守策略
3. 各 action 对应字段的填写规则
4. JSON 输出格式示例

### 3d: parseAbstractTagResponse → parseTagJudgmentResponse

**验证逻辑：**
1. 解析 JSON
2. 校验 action 值必须是 "merge" 或 "abstract"
3. merge 时校验 `merge_label` 非空
4. abstract 时校验 `abstract_name` 非空且 ≤ 160 字，`description` ≤ 500 字

---

## Task 4: ExtractAbstractTag 函数重写

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go`

**签名变更：**
- 旧：`ExtractAbstractTag(...) (*models.TopicTag, error)`
- 新：`ExtractAbstractTag(...) (*TagExtractionResult, error)`

**新增 merge 处理分支（在 category 确定后、abstract 创建前）：**

```go
if judgment.Action == ActionMerge {
    mergeTarget := selectMergeTarget(candidates, judgment.MergeLabel)
    if mergeTarget == nil {
        return nil, fmt.Errorf("no suitable merge target found")
    }
    logging.Infof("Tag judgment: merge into existing tag %q (id=%d)", mergeTarget.Label, mergeTarget.ID)
    return &TagExtractionResult{Action: ActionMerge, MergeTarget: mergeTarget}, nil
}
```

**abstract 分支保持不变**，但返回值改为包装在 `TagExtractionResult` 中。

---

## Task 5: tagger.go 调用方更新

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` — `findOrCreateTag` 函数的 `ai_judgment` 分支

**旧流程：**
```
ExtractAbstractTag → abstractTag → DeleteTagEmbedding(candidates) → createChildOfAbstract(newTag, abstractTag)
```

**新流程：**
```
ExtractAbstractTag → result
├── result.Action == "merge"  → 复用 result.MergeTarget，更新 aliases/label/description，返回
└── result.Action == "abstract" → DeleteTagEmbedding(candidates) → createChildOfAbstract(newTag, result.AbstractTag)
```

**merge 分支具体处理：**
1. 使用 `result.MergeTarget`（现有标签）
2. 优先使用 `result.MergeLabel`（LLM 推荐的统一标签名），fallback 到 `tag.Label`
3. 更新其 category/source/aliases/icon/kind
3. `database.DB.Save` 保存
4. 异步补 embedding 和 description
5. 直接返回现有标签

**注意：** merge 分支跳过了 `DeleteTagEmbedding` 和 `createChildOfAbstract`，不会创建新标签或抽象关系。

---

## Task 6: tag_feedback.go 叙事反馈更新

**Files:**
- Modify: `backend-go/internal/domain/narrative/tag_feedback.go` — `triggerAbstractExtractionWithContext` 函数

**新增 merge 分支：**

当 `result.Action == ActionMerge` 时：
1. 从两个现有标签中确定 source（被合并）和 target（保留）
2. target 是 `result.MergeTarget`，source 是另一个
3. 调用 `MergeTags(sourceID, targetID)` 执行合并（迁移文章关联、摘要关联、标记 merged 状态）
4. 日志记录

当 `result.Action == ActionAbstract` 时：走原逻辑，日志记录。

---

## Task 7: 测试更新

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service_test.go`

### 新增测试

| 测试 | 覆盖 |
|------|------|
| `TestParseTagJudgmentResponse` | merge/abstract action、空 merge_label、空 abstract_name、无效 action、无效 JSON、description 截断、abstract_name 超长 |
| `TestSelectMergeTarget` | slug 匹配、fallback 到第一个 candidate、空 candidates 返回 nil |

### 重命名测试

| 旧名 | 新名 |
|------|------|
| `TestBuildAbstractTagPrompt` | `TestBuildTagJudgmentPrompt` |
| `TestBuildAbstractTagPromptWithDescription` | `TestBuildTagJudgmentPromptWithDescription` |
| `TestBuildAbstractTagPromptPerson` | `TestBuildTagJudgmentPromptPerson` |
| `TestBuildAbstractTagPromptEvent` | `TestBuildTagJudgmentPromptEvent` |

prompt 测试断言从检查 `abstract_name` 改为检查 `merge` 和 `abstract` 关键字。

---

## Task 8: 文档更新

**Files:**
- Modify: `docs/guides/topic-graph.md`

**三阈值匹配流程图更新：**

旧：`0.78~0.97 + 普通标签 → LLM 抽象提取，两个子标签`

新：`0.78~0.97 + 普通标签 → LLM 判断 merge 或 abstract：merge: 合并为同一标签 / abstract: 创建抽象标签，建立子标签`

**新增"标签关系判断"章节**，描述 merge/abstract 判断逻辑。

---

## 验证清单

```bash
cd backend-go
go build ./...                          # 编译通过
go test ./internal/domain/topicanalysis/... -v    # 70 tests pass
go test ./internal/domain/topicextraction/... -v  # 10 tests pass
go test ./internal/domain/narrative/... -v         # 30 tests pass
```

---

## 附：tag_feedback.go 中 source/target 选择逻辑说明

```go
// tag_feedback merge 时需要确定谁是 source（被合并）谁是 target（保留）
targetID := result.MergeTarget.ID
sourceID := tagAID
if targetID == tagAID {
    sourceID = tagBID
}
```

逻辑：`result.MergeTarget` 是 AI 推荐保留的标签（target），另一个是被合并的标签（source）。`MergeTags(sourceID, targetID)` 会把 source 的文章/摘要关联迁移到 target。

---

## 风险评估

| 风险 | 等级 | 缓解 |
|------|------|------|
| `ExtractAbstractTag` 返回类型变更导致编译错误 | 已解决 | 所有调用方已更新并通过编译 |
| LLM 不遵守 enum 约束返回其他值 | 低 | `parseTagJudgmentResponse` 代码层二次校验 action 值 |
| merge 后 embedding 未更新 | 低 | merge 分支会 `ensureTagEmbedding` 补 embedding |
| 保守策略导致过多 abstract | 可接受 | prompt 明确"不确定时优先 abstract" |
