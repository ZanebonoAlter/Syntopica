# Design: 页边注问答 LLM 自由选词搜索（翻案 D7 搜索策略）

## Context

见 proposal.md Why：D7 的搜索 query 由锚文本机械截断生成（`margin_notes_websearch.go` `MarginNoteSearchQuery`），`quoted_text` not null 使问题文本回退分支成为死代码——同一页边注下问任何问题，搜索词与结果一字不差且与问题语义脱钩。本 change 将搜索词的决策权交给 LLM（用户探索拍板方案 A「LLM 两阶段」）。

现状链路（`margin_notes_qa.go` `AskMarginNote`）：锚文本 query → `searchWebForMarginNote` 单次 SearXNG（≤5 条）→ prompt 组装 → 单次 airouter `open_notebook` 调用 → 白名单校验 → 落库。回答契约、白名单机制、失败静默降级、审计落点全部保留，只动「query 怎么来 + 搜几次 + 结果怎么合」。

## Goals / Non-Goals

**Goals:**

- 搜索词由 LLM 依据问题语义自由生成（1~3 条），替代锚文本机械截断
- 多 query 并行搜索、按 URL 合并去重进白名单池，防编造机制不放松
- 完整降级链：任何一步失败都能回到「现状行为」或「无联网」，问答永不因联网链路阻断
- query 生成调用全量审计（operation 区分），效果可回放

**Non-Goals:**

- 不做独立的 query 生成路由配置面（capability 级新路由/设置页），复用 `open_notebook`；operation 级轻量路由留未来观察审计数据后再议
- 不改 dataenrichment agent 循环的 web_search 后端选择（Bocha 照旧，D7 原文照抄）
- 不做搜索结果 rerank / 质量过滤 / 抓网页正文（仍是原始结果直拼 prompt）
- 前端零改动（web_sources 结构与展示不变）

## Decisions

### D1 两阶段调用：前置轻量 query 生成调用，回答调用不变

搜索结果必须先于回答调用存在（回答 prompt 需要搜索参考块），一次调用无法「中途出去搜一次」——两阶段是结构必然。前置调用只输出 `{"queries": string[]}`（1~3 条），`maxTokens` 压缩至 128、temperature 0.1、JSONMode+JSONSchema（对齐回答调用的结构化输出模式），输入为锚文本（截 ≤200 runes）+ 问题 + 线索标题/摘要（如有）。

- 备选否决「塞进回答调用让 LLM 先吐 queries 再答」：一次调用无法在输出 queries 后暂停去搜索。
- 备选否决「agent 循环化（web_search 工具）」：推翻「页边注 QA 是单次调用」的架构前提，改动面与延迟不可控（探索阶段方案 D）。

### D2 query 生成复用 `open_notebook` 路由，operation 区分审计

`daily_report.margin_note_search_query` 落 ai_call_logs（airouter 强制 Operation 必填 + 每次尝试落审计），与回答调用 `daily_report.margin_note_qa` 区分，效果可单独回放。复用理由：零配置面新增，用户已配好 `open_notebook` 即可用；query 输出极短（≤128 tokens），即使用大模型成本也远低于回答调用。代价记入 Risks（并发占槽）。

### D3 端点预检前置：未配置/禁用时连 query 生成也不发起

现状 endpoint 检查藏在 `marginNoteWebSearchFn` 内部（搜索时才查）。改为 `AskMarginNote` 在发起 query 生成**之前**先调 `marginNoteSearxngEndpoint()`：未配置/禁用 → 直接跳过整个联网链路（省一次 LLM 调用），行为与现状一致。避免「SearXNG 明明没配、还白花一次 query 生成调用」。

### D4 多 query 并行搜索与合并

- **并行**：1~3 个 query 各自 `searxng.Search`（客户端单次超时 8s 不变），service 层 goroutine 并发、各自独立超时——最坏墙钟 8s 而非串行 24s。
- **合并**：按 URL（trim 后精确匹配）去重保序（query 生成顺序 × 各 query 内结果顺序），池子截前 5（`marginNoteSearxngResultLimit` 不变）进 prompt。
- **部分失败**：单个 query 搜索失败/空不拖垮其他 query，成功的照用；全部失败 → 无联网参考块（静默，Warn 留审计）。此时**不**再回退锚文本——SearXNG 同一轮挂掉，锚文本 query 大概率同样失败，二次尝试只加延迟。

### D5 降级决策树（完整）

```
marginNoteSearxngEndpoint() 未配置/禁用
  → 跳过联网链路（现状行为，省 query 生成调用）
已配置
  → query 生成（LLM 调用，D1）
      ├─ 成功出 1~3 条 → 并行搜索（D4）
      │     ├─ ≥1 条有结果 → 合并去重 top5 进 prompt
      │     └─ 全部失败/空 → 无联网参考块（静默降级）
      └─ 失败/超时/输出空/解析失败
            → 回退锚文本 query（截前 80 runes，现状规则）
                  ├─ 有结果 → 进 prompt
                  └─ 失败/空 → 无联网参考块（静默降级）
```

回退用锚文本的理由：query 生成失败 ≠ SearXNG 挂了，锚文本 query 是与现状完全相同的保底搜索；而搜索阶段全失败说明 SearXNG 本身不可用（D4），不再二跳。

### D6 query 生成 prompt 要点

system 定位「搜索查询生成器」：依据读者问题与划选原文生成 1~3 条适合网页搜索的中文查询词——问题与划选原文主题一致时搜原文核心概念；问题延伸（如问选举而原文讲民调）时以问题为准；每条是精炼搜索词组而非完整句子；不输出解释。白名单/防编造措辞与回答 prompt 同口径。具体措辞实现时定，测试断言不锁 prompt 文案（stub 钩子层断言行为）。

### D7（翻案原 D7 决策）搜索策略从「每次必搜固定规则」改为「每次必搜 LLM 选词」

原 D7 三选一选了「每次必搜+失败静默降级」否决「LLM 两阶段」，当时理由是简单、零额外调用。实际使用证伪：固定规则使搜索与问题脱钩。「每次必搜」「失败静默降级」「联网是增益不是依赖」三条精神全部保留，只换 query 来源与调用次数（1 → 最多 2）。

### D8 测试钩子

`marginNoteQueryGenFn` 包级可替换钩子（对齐 `marginNoteChatFn` / `marginNoteWebSearchFn` 先例）：测试 stub query 生成与搜索两个环节，白盒断言降级树各分支（三钩子组合覆盖：生成成功/失败/空 × 搜索全成/部分/全败 × endpoint 未配置）。

## Risks / Trade-offs

- [端到端延迟增加：+1 次快 LLM 调用 + 一轮并行搜索（最坏 8s）] → query 生成短输出快返回；多 query 并行使搜索墙钟不随 query 数线性增长；回退链保底
- [open_notebook 并发信号量 2，两阶段使单次提问占槽时长变长、并发追问可能排队] → 排队不拒绝（airouter 信号量等待语义）；单用户场景连续追问为主、影响可接受；用户可调 `ai_routes.max_concurrency`；留待审计数据观察
- [复用 open_notebook 路由，query 生成可能用贵模型] → 输出 ≤128 tokens，单次成本远低于回答调用；审计数据可量化；若显著再做 operation 级轻量路由（Non-goal）
- [LLM 选词质量不稳定（搜偏/搜宽）] → 白名单防编造不受影响；审计可回放全部 query；D5 回退链保证最坏回到现状行为
- [伪引用残留：参考块与问题不相关时 LLM 仍可能硬挑来源] → query 生成对齐问题语义后参考块相关性整体提升，该风险随之缓解；后续可观察审计再做相关性过滤（Non-goal）

## Migration Plan

纯 service 层代码改动，无 DB migration、无配置新增、无前端改动。部署即生效；回滚 = 还原代码（行为回到锚文本固定 query，已落库的 QA 数据无 schema 变化不受影响）。

## Open Questions

（无——探索阶段的方向决策已由用户拍板，实现细节均有 D1~D8 锚定；query 生成 prompt 具体措辞实现时定，不锁测试。）
