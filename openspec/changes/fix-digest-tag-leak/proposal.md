<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

2026-10-10 俄乌冲突版块日报出现「OpenAI营收口径反转」线索，点开源文章却是华尔街见闻多新闻 digest（标签列表含「特朗普与普京达成柴油供应协议压回落价」）。全链路排查（存档：`docs/research/daily-report-digest-contamination/explore-findings.md`）确认线索生成本身无截断，根因是 digest 文章的实体污染被两个环境性缺陷放大成用户可见错误：

1. **方向校验无区分度**：`DirectionSimThreshold` 绝对阈值 0.5 在新闻域形同虚设——实测 OpenAI 标签 identity 向量 vs 俄乌冲突版块 = 0.6023（放行），vs 其正确归属「生成式AI」版块 = 0.8109。新闻语料向量普遍挤在 0.5~0.7 区间，绝对阈值切不开。
2. **去重 tiebreak 内容盲**：digest 一篇文章拆出的多个单文章事件标签在日报去重时二选一，`shouldReplace` 只比文章数和 ID——同篇 1 文章平手时 ID 小者赢。本例 OpenAI 标签（ID 204328）比柴油标签（204331）早创建 1.4 秒而存活，把与版块相关的柴油标签杀掉，纯抽签与内容无关。

## What Changes

- **方向校验加相对门**（`board-direction-check`）：非 direct_hit 匹配除现有绝对阈值外，增加相对条件——tag identity 向量与目标版块的 cosine 不得比「该标签与所有版块的最佳 cosine」低出 margin 以上，否则标记 `direction_mismatch=true`。margin 进 `ai_settings` 可配置。实测案例中正确归属 0.81 vs 串味 0.60 差距 0.21，相对门可稳定拦截。
- **去重 tiebreak 加版块相关性**（`daily-report-system`）：同文章标签合并时，tiebreak 优先级从「文章数 > ID」扩为「文章数 > 与当前版块向量的 cosine 近距 > ID」。日报管线在去重前加载参战标签的 identity embedding 与版块 embedding，比较失败时回退 ID 顺序（保持现有行为兜底）。
- **明确不做**（评估为不稳定，留观察不动）：事件标签辅助标签改事件句级作用域（抽取链路深层改动）；L3 聚类允许拆散异质组（LLM 行为难约束）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `board-direction-check`: 方向校验从单绝对阈值改为「绝对阈值 + 相对最佳版块差距门」双条件；新增 margin 配置项与对应场景
- `daily-report-system`: 「事件标签去重」Requirement 的 tiebreak 规则扩为「文章数 > 版块相关性 > ID」，新增 embedding 缺失回退场景

## Impact

- `backend-go/internal/tagmanagement/service/board/semantic_board_matching.go`：evaluateSemanticBoardMatches 方向校验分支、LoadConfig 新增 margin key
- `backend-go/internal/topicgraph/service/daily_report_dedup.go`：DeduplicateTags / shouldReplace 接收版块相关性信号
- `backend-go/internal/topicgraph/service/daily_report_orchestrator.go`：collect 与去重之间接线（加载 identity/board embedding）
- `ai_settings` 新增 key：`semantic_board_match_direction_relative_margin`（默认值 design 阶段定）
- 无 DB 迁移（`direction_mismatch` 列已存在）、无前端改动、无 API 契约变化
