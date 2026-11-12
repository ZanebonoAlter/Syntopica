<!-- complexity: moderate -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->
<!-- 域声明理由：编辑路径 backend-go/internal/tagmanagement/service/core/（→ semantic-board 打标契约）命中 doc-impact-applies 标签 -->

# 修复模型「schema 外壳」输出导致的打标解析失败

## Why

`retire-heuristic-tag-fallback` 删掉规则词兜底后，打标失败**变得可见**（任务 failed → 退避重试 → 停在 failed 态）。但 20 篇试验批暴露了一个**独立的、被兜底长期掩盖的根因**：模型 82% 的响应解析失败，文章拿不到 event 标签 → 日报仍缺。

**现象**（`ai_call_logs` 实测）：本地 4B 模型（`Qwen3.5-4B-UD-Q8_K_XL.gguf`）在 JSON-schema 引导下，有时把**schema 骨架本身**当输出：

```json
{"type":"object","properties":{"event_person_tags":[...],"keyword_tags":[...]},"required":["event_person_tags","keyword_tags"]}
```

数组被塞进 `properties` 里，而解析器只认顶层键 → `merged extraction object has neither event_person_tags nor keyword_tags keys` → 解析失败 → 任务失败。

**按天的形态分布**（`ai_call_logs` 汇总）：

| 日期 | 调用数 | schema 外壳 | 顶层双数组（正常） |
| --- | --- | --- | --- |
| 11-05 | 2168 | 0 | 1011 |
| 11-06 | 17519 | 0 | 1164 |
| 11-09 | 5388 | 0 | 1918 |
| 11-10 | 5248 | 1774 | 547 |
| 11-11 | 3659 | 2902 | 229 |
| 11-12 | 1423 | 1168 | 112 |

**根因（prompt 自相矛盾）**：merged 提取路径的 **user prompt 末句**仍是

```
请返回JSON对象格式: {"tags": [标签列表]}。
```

而 system prompt 的【输出格式】正例与下发给模型的 JSON Schema 都是**双数组**（`event_person_tags` / `keyword_tags`）——`{"tags": [...]}` 是 merge-tag-extraction-branches 之前的旧单数组形态。**两条指令互相矛盾**：system prompt 说双数组、user prompt 末句说单数组。4B 模型被撕扯后既没给纯双数组，也没给纯单数组，而是给出「schema 骨架 + 值塞在 properties」的混合体。

（对照：**section/aggregate 路径的 schema 本来就是单数组 `{"tags":[...]}`，它那句指令是对的，不动**。）

**为什么现在才炸**：11-10 之前模型返回的是「旧 per-branch 形态 + 截断」（`finish_reason=length`），不是外壳形态；11-10 起 schema 形态切换，矛盾指令的后果从「另一种畸形」变成了「schema 外壳」。

## What Changes

- **prompt 指令对齐（根因）**：`extractor_enhanced.go` 的 `buildExtractionUserPrompt` 末句改为与真实 schema 一致的双数组说明（`event_person_tags` + `keyword_tags`），不再指向已废弃的 `{"tags": [...]}` 单数组。`extractor_section.go` 的同类指令**不动**（其 schema 确为单数组）。
- **解析器容错（兜底）**：新增 `unwrapSchemaEnvelope`——识别 `{"type":"object","properties":{...},"required":[...]}` 外壳并取 `properties` 内的对象继续解析。接入点两处：`unmarshalMergedExtraction`（merged 路径）与 `parseRawTagObjects`（section 路径 + legacy 裸数组回退）。**不改变契约**：外壳里的数据与规范形态语义完全相同，只是多了一层包装——与既有「`{"tags":...}` 包装」「legacy 裸数组」两种容忍形态同性质。
- **测试**：新增外壳形态的解析用例（merged 双数组 / section 单数组 / 空数组 / 非外壳不误伤）+ prompt 指令与 schema 一致性的守护用例。

## Capabilities

### Modified Capabilities

- `tagging-domain`（`openspec/specs/tagging-domain/spec.md`）：新增 Requirement「提取响应形态容错与 prompt 指令一致性」——模型返回 schema 外壳形态 SHALL 被解析器识别；提取 prompt 的输出格式指令 SHALL 与下发 schema 一致。

## Impact

- **代码**：后端 2 个文件（`extractor_enhanced.go` / `extractor_section.go` 若共用 helper 则仅前者）+ 测试；前端零改动。
- **行为**：打标解析成功率提升（目标：schema 外壳形态 100% 被吃掉）；**不改变任何标签语义**——外壳里的 `event_person_tags` / `keyword_tags` 按既有校验规则逐条处理（aux 校验、description 校验、上限截断、泛词黑名单全部照旧）。
- **风险**：外壳容错是「宽松解析」，若模型返回**真正畸形**的对象（如 `properties` 里既无两个键），仍 SHALL 报解析错误触发重试——不会把坏数据当合法结果。
- **后续**：本 change 落地后即可重打 `retire-heuristic-tag-fallback` 遗留的 2488 篇无标签文章（tasks 16/17）。
