<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, ai-summary -->

## Why

D7 落地的联网搜索策略存在实测缺陷：搜索 query 由锚文本机械截断生成（`MarginNoteSearchQuery` 优先 quoted_text 前 80 runes），而 `report_annotations.quoted_text` 是 not null 列——问题文本的回退分支是永远走不到的死代码。结果：同一页边注下问任何问题，搜索词一字不差，返回的参考文章每次固定且与问题语义脱钩（用户实测：在台湾局势相关段落上问选举问题，搜回的全是锚文本同事件报道，与问题无关）。LLM 在整个流程中无搜索决策权——搜索发生在它之前、由固定规则决定。用户探索后拍板方案 A「LLM 两阶段」：让 LLM 看着问题和上下文自由选择搜索词汇。

## What Changes

- **翻案 D7 搜索策略**：提问时前置一次轻量 LLM 调用（输入锚文本+问题+线索上下文），自由生成 1~3 个搜索 query，替代「锚文本截前 80 runes」的固定规则。
- **多 query 并行搜索合并**：多个 query 并行调本地 SearXNG，结果按 URL 合并去重进白名单池；回答引用的网络来源白名单校验机制不变（池子从单 query 结果集扩展为多 query 合并结果）。
- **回答调用契约不变**：回答仍是单次 airouter 调用，schema `{answer, cited_article_ids[], web_sources[], terms[]}` 与落库契约不变；前端零改动。
- **降级链路**：query 生成失败/超时/输出为空 → 静默回退现状行为（锚文本 query 搜索），保持 D7「联网是增益不是依赖」精神；搜索失败静默降级照旧。
- **审计**：query 生成调用走 airouter 落 ai_call_logs，operation `daily_report.margin_note_search_query`（与回答调用的 `daily_report.margin_note_qa` 区分）。
- **spec 修订**：`daily-report-margin-notes` 联网补强 requirement——搜索 query 来源改为「LLM 自由生成优先、锚文本为降级回退」，删除「不增加 LLM 调用次数」措辞（每次提问 LLM 调用从 1 次变为最多 2 次）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-margin-notes`: 联网补强 requirement 的搜索策略变化——搜索 query 由固定规则（锚文本优先）改为 LLM 自由生成（1~3 个）+ 失败回退锚文本；每次提问 LLM 调用从 1 次变为最多 2 次（回答仍单次）；搜索结果白名单从单 query 结果集扩展为多 query 合并去重结果池；「每次必搜+失败静默降级」与 web_sources 白名单防编造机制维持不变。

## Impact

- **后端**：`backend-go/internal/topicgraph/service/margin_notes_qa.go`（`AskMarginNote` 前置 query 生成调用）、`margin_notes_websearch.go`（query 生成 prompt/schema、多 query 并行搜索与合并去重、降级回退）；airouter 审计新增 operation。query 生成复用 `open_notebook` capability 路由（不新增路由配置面，细化见 design）。
- **前端**：零改动（web_sources 结构、卡片 chips、管理页展示全部不变）。
- **数据库**：无 migration（annotation_qas 表结构不变）。
- **AI 成本**：每次提问 +1 次短输出 LLM 调用（query 生成，输出仅 1~3 个查询词，maxTokens 压缩）；SearXNG 本机零 API 成本，多 query 并行调用；端到端延迟增加约一次快 LLM 调用 + 一轮并行搜索。
