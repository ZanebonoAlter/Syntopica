# Design — tolerate-schema-envelope-extraction-output

## D1. 模型输出形态诊断（先确认，再改）

`ai_call_logs` 的 `response_snippet` 是模型原始响应。抽取 11-12 的样本，形态一致：

```
{"type":"object","properties":{"event_person_tags":[{...}],"keyword_tags":[{...}]},"required":["event_person_tags","keyword_tags"]}
```

即 **schema 骨架被当成了输出模板**，值被塞进 `properties`。数组内容本身是合法的（label/category/description/auxiliary_labels 齐全），只是多了一层 schema 包装。

**与 prompt 的关系**：user prompt 末句 `请返回JSON对象格式: {"tags": [标签列表]}。`（`extractor_enhanced.go:313`）指向的是 merge-tag-extraction-branches 之前的**旧单数组**形态，与 system prompt 正例、JSON Schema 的**双数组**矛盾。4B 模型面对两条矛盾指令，产出「schema 骨架 + 混合值」。

## D2. 两处修复（根因 + 兜底）

### D2.1 prompt 指令对齐（根因）

`extractor_enhanced.go` 的 `buildExtractionUserPrompt` 末句：

- 改前：`请返回JSON对象格式: {"tags": [标签列表]}。`
- 改后：`请返回 JSON 对象，包含 event_person_tags 与 keyword_tags 两个数组（格式见系统提示【输出格式】正例）。`

**不动 `extractor_section.go:162`**：section/aggregate 路径的 schema 是单数组 `{"tags":[...]}`（`sectionExtractionSchema()`），它那句指令**与 schema 一致**，改了反而制造新的矛盾。

### D2.2 解析器外壳容错（兜底）

新增共用 helper（放 `extractor_enhanced.go`，section 路径同包可用）：

```go
// unwrapSchemaEnvelope strips the JSON-Schema envelope some models emit
// ({"type":"object","properties":{...},"required":[...]}) and returns the
// inner object. Non-envelope content is returned unchanged.
func unwrapSchemaEnvelope(content string) string
```

**判定条件**（三者同时满足才算外壳，避免误伤正常响应）：
1. 顶层是 JSON 对象；
2. 含 `properties` 键，且其值是 JSON 对象；
3. 含 `type` 或 `required` 键（schema 特征；正常标签响应不会有）。

**行为**：把 `properties` 的原始 JSON 文本作为新 content 返回；其余原样返回。**幂等**：解开一次后顶层不再有 `type`/`required`，第二次调用原样返回。

**接入点**：
- `unmarshalMergedExtraction`（merged 路径）：入口处 `content = unwrapSchemaEnvelope(content)` → 外壳解开后是规范双数组，走既有 canonical 分支。
- `parseRawTagObjects`（section 路径 + merged 的 legacy 裸数组回退）：入口处同样解一次 → section 外壳解开后是 `{"tags":[...]}`，走既有 wrapper 分支。

**不放松畸形判定**：外壳解开后若 `properties` 里既无 `event_person_tags` 也无 `keyword_tags`（merged）或既无 `tags` 也非数组（section），仍 SHALL 报解析错误触发重试——**不会把坏数据当合法结果**。既有「非空对象缺两个目标键 → parse error」的守卫保持有效。

## D3. 影响面

| 位置 | 变化 |
| --- | --- |
| `extractor_enhanced.go` `buildExtractionUserPrompt` | 末句指令改为双数组（根因） |
| `extractor_enhanced.go` `unmarshalMergedExtraction` | 入口解外壳（兜底） |
| `extractor_enhanced.go` `parseRawTagObjects` | 入口解外壳（兜底，section 路径同享） |
| `extractor_section.go` | **不改**（prompt 指令与其 schema 一致；解析经 `parseRawTagObjects` 自动获得兜底） |

**标签语义零变化**：外壳解开后走的是同一条校验链（aux 校验 → description 校验 → 类别优先级去重 → keyword 上限 3 / 总上限 5 → 泛词黑名单 → 示例词泄漏护栏）。

## D4. 明确不做

- 不改 schema 本身（双数组是既定契约，`tagging-domain` 主 spec 依赖它）；
- 不改 `retire-heuristic-tag-fallback` 的失败语义（失败仍失败、仍进队列重试）；
- 不动 section 路径的 prompt 指令；
- 不为「模型不听话」放宽 aux/description 校验规则。
