<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

用户在 2026-10-09 日报中发现线索"伊朗对以色列发动直接军事打击"描述为 2024 年 4 月，与当日文章（讲美伊谈判/胡塞袭击沙特，无伊以冲突）完全脱节。调查（docs/research/event-tag-time-anchor/explore-findings.md）确认根因是 prompt 示例污染 + description 一次生成永不更新：`generateTagDescription` 的 few-shot 示例自带"伊朗袭击以色列 → 2024年4月"时间锚，LLM 照抄示例而非文章上下文中的 `[日期: …]`；三个 extractor prompt 的正例也用同一标签名，诱导在伊朗军事话题文章上输出该标签。面上普查：近 60 天 4714 个含年份的 event 标签中 176 个时间锚比创建时间早 ≥1 年，其中确有与文章内容脱节的病态样本（伊朗袭击以色列、伊朗停止向船只装载原油等）。

## What Changes

- **换毒示例**：`tagger.go generateTagDescription` event 分支的 few-shot 示例、`extractor_section.go buildSectionExtractionPrompt` 与 `extractor_enhanced.go buildMergedExtractionPrompt` 的输出正例，全部去掉"伊朗袭击以色列"这个自带历史时间锚的真实事件例子，换成中性示例（keyword/event 各留时间中性或虚构的例子）。
- **加时间锚纪律**：`generateTagDescription` 的 event 分支 prompt 增加硬约束——事件发生时间必须以 articleContext 中的 `[日期: …]`（文章发布日期）为参照；禁止引入文章上下文中没有的时间信息（先验知识）；描述事件时间时优先使用文章日期所在的时间语境。
- **存量数据修复（一次性）**：扫描存量 event 标签，筛选"事件主体时间锚早于创建时间 ≥1 年"的候选，对确认为病态（事件主体发生时间锚在过去且与标签创建时首挂文章语境脱节）的标签，用首挂文章的 articleContext 重新生成 description。以管理脚本/后台任务形式提供，可指定标签 ID 精准重生成。
- **不做的事**：不改日报 threads 生成逻辑（修复标签源头后，后续日报自然正确）；不引入"复用时自动刷新 description"机制（涉及并发与成本权衡，另行评估）；不重建历史日报。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `tagging-domain`: 新增 event 标签 description 生成的时间锚纪律要求——description 的时间信息 SHALL 锚定文章发布日期语境，prompt 示例 SHALL 使用时间中性示例；以及存量病态标签 description 的重生成行为。

## Impact

- **代码**：`backend-go/internal/tagmanagement/service/core/` 下 `tagger.go`（generateTagDescription prompt）、`extractor_section.go`、`extractor_enhanced.go`（正例替换）。
- **数据**：`topic_tags` 表中病态 event 标签的 description 字段被重写（受影响约几十个，修复前先列清单确认）。`daily_report_threads` 历史行不动。
- **AI 调用**：prompt 文本变更，无接口/路由变化；重生成走现有 airouter 通道。
- **用户可见行为**：修复后次日起新生成的日报线索不再复读陈旧时间锚；标签详情页中病态标签的描述更新为与文章语境一致的版本。
- **需要的操作**：apply 后跑一次存量修复（脚本/任务），对清单确认后执行；无迁移、无配置变更。
