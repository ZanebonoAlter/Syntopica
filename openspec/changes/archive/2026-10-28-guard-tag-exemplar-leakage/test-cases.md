# Test Cases: guard-tag-exemplar-leakage

故事：本地小模型照抄打标 prompt 的 few-shot 示例词（PostgreSQL 等），把无关文章误挂标签；本 change 在两条 LLM 解析出口加「示例词 + 可见输入无依据 → 丢弃」护栏，并一次性清理存量误挂。

## ⓪ 继承与调整（MODIFIED tagging-domain，新增 Requirement 无旧 Scenario 继承；旧测试资产处置——已全部落地）

| 旧测试 | 旧断言 | 处置 | 动作 |
| --- | --- | --- | --- |
| extractor_test.go ExtractTags（PostgreSQL update） | Title/Summary 均含 PostgreSQL，断言挂载 | 不动——恰为护栏放行分支正向用例（Scenario 2） | 保留 |
| extractor_test.go:88/137/521、extractor_section_test.go TestParseSectionTags* | parseMergedExtractionTags / parseSectionTags 纯解析 | 不动——护栏在解析出口，不在纯解析函数 | 保留 |
| extractor_test.go TestExtractTagsMakesExactlyOneMergedCall | Summary "OpenAI is shipping GPT-5." 无 postgres 却断言 PostgreSQL 挂载 | **断言的是泄漏行为**——Summary 补 "with a PostgreSQL memory backend" | 修改 fixture |
| extractor_section_test.go SingleCall / SucceedsAfterRetry / CarriesSectionMetadata | 栏目正文「AI缓存技术取得新突破。」无 Sam Altman/Kubernetes 却断言挂载 | 栏目正文补两词提及（保留原文子串，CarriesSectionMetadata 的 userPrompt Contains 断言不变） | 修改 fixture |
| article_tagger_aggregate_test.go aggregateMarkdownSummary() | 栏目正文无 PostgreSQL/Sam Altman/Kubernetes 却断言挂载（影响 MonoContentForms/AllSectionsFailed/DedupesAndTiers/SkipsFailedSection 五个测试） | 「科技动态」栏目正文补 PostgreSQL + Sam Altman + Kubernetes 提及（DedupesAndTiers 的跨片去重素材 Kubernetes 保留） | 修改 fixture |
| article_tagger_aggregate_test.go WithoutSectionsFallsBackToMono | bodyOf("本期没有栏目结构的纯文本摘要。") 无 postgres | bodyOf 文本补「含 PostgreSQL 动态」 | 修改 fixture |
| CapsAtFifteenTags / ScoreTiers 等动态标签测试 | 标签为「栏目0标签A」等生成词，非示例词 | 不动——不受护栏影响 | 保留 |

## 主链路（步/动作/来源 Scenario/期望/层/落点）

| # | 动作 | Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| 1 | guard 收到 label=PostgreSQL、可见文本无 postgres 的候选 | S1 | 丢弃 + errors 尾注 | 函数单测 | guard 单测 |
| 2 | guard 收到 label=PostgreSQL、Title="PostgreSQL update" | S2 | 保留 | 函数单测 | guard 单测 |
| 3 | guard 收到 label=LangChain（非示例词）、无提及 | S3 | 不拦 | 函数单测 | guard 单测 |
| 4 | event 标签 aux 含无提及 PostgreSQL 项 | S4 | event 保留、aux 项剔除 | 函数单测 | guard 单测 |
| 5 | section 片无 Kubernetes，LLM 返回 Kubernetes | S5 | 丢弃 + warn 日志 | 行为（fake router） | aggregate 测试 |
| 6 | LLM 返回小写 postgres | S6 | Slugify 归一后丢弃 | 函数单测 | guard 单测 |
| 7 | prompt 文本改示例词、集合未同步 | S7 | 防漂移测试红 | 函数单测 | 防漂移测试 |
| 8 | 清理工具 dry-run | S8 | 打印 ~25 行清单、零删除 | 命令+真库 | cmd 工具 |
| 9 | 清理工具 --apply | S9 | 关联 36→11、标签保留、重跑 dry-run 空 | 命令+真库 | cmd 工具 |

## 变体走查

- 输入：空串/纯空白 label → 上游 Slugify 空已跳过，guard 不处理（既有行为）；单 token 非示例词 → 不拦；大小写变体 → S6 拦；超长 label → 不命中集合，不拦
- 前置：空 tags 切片 → 原样返回空；全部候选命中示例词且无提及 → 全丢，零候选走既有「空结果合法」条款；重复 label → merge 层去重在 guard 之后，guard 先行逐个判定，行为一致
- 时间窗口：不适用（无时间窗口逻辑）✂️
- 幂等：guard 纯函数幂等；清理工具重复 --apply 第二次无删行（S9）
- 可用性：不适用（纯后端，无 UI）✂️

## 层选择说明

护栏判定 = 纯逻辑 → 函数单测；出口行为 = 既有 fake router 模式（不起新层）；存量清理 = 真库命令验证（量小、幂等、留 dry-run 清单）；无 HTTP/UI 变更，不需要 handler/Vitest/agent-browser。

## 效果核对（真库量化——已完成）

- 触发原因：416 期（article 153877）正文 0 提及 postgres 却挂 PostgreSQL 标签；摸底 36 挂载中 25 篇 0 提及（69%）
- 方法：cmd/admin tagguard-cleanup dry-run 清单 + apply 后 BEGIN/ROLLBACK 只读复核
- 量化结果：清理 28 行（PostgreSQL 26 + 某国央行上调基准利率占位词 2 + kubernetes 1，比摸底多 3 行系后台新打标 + event 占位词标签）；PostgreSQL 挂载 36→11（**=目标值**）；article 153877 不再挂 1556；占位词标签「某国央行上调基准利率」(228458) 无关联后孤儿清理删除；PostgreSQL/kubernetes 标签本体保留；重跑 dry-run leaked=0（幂等）；Transformer 对照组 6 条全保留（不误伤）
- 结论判据：护栏上线后新文章不再出现「示例词 + 0 提及」新挂载（观察窗口一周，ai_call_logs 响应抽查）
