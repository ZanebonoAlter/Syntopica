<!-- ui-impact: none -->
<!-- constraint-domains: topic-graph, daily-report -->

# relax-lane-snapshot-length-caps

## Why

泳道「近 14 天」趋势快照（短版态势句 + 长版叙述）的 100 字/500 字上限是 board-lane-dynamics 设计时定的卡片布局保护值，实测 LLM 正常发挥经常超出：超出部分被 `truncateRunes` 硬切成半句烂尾；更糟的是单次调用 `MaxTokens=768` 偏紧，模型啰嗦时 JSON 被中途掐断 → 解析失败走降级 → 整段输出只留前 100 字当短版、长版整段丢失。用户看到的就是"趋势文字被截断"。

## What Changes

- 短版态势句机械上限 100 → 200 字（`laneSnapshotMaxRunes`）
- 长版叙述机械上限 500 → 1000 字（`laneSnapshotDetailMaxRunes`）
- 单次结算调用 `MaxTokens` 768 → 1536，消除 JSON 被 max_tokens 掐断导致的"长版丢失、短版被砍"降级路径
- system prompt 中的字数要求文案同步放宽（≤100字/≤500字 → ≤200字/≤1000字）
- 相关注释（模型字段注释、前端组件头注释）与 pin 常量值的测试同步更新
- 数据库字段为 `text`、前端为全文展示，均无需改动；存量数据天然兼容（旧快照 shorter than new caps，下次结算覆盖时自然享受新上限）

## Capabilities

### Modified Capabilities

- `board-lane-dynamics`：「日报后滚动态势结算」Requirement 的字数规定（100/500 → 200/1000）与「机械截断保护」Scenario 的上限数值同步放宽。

## Impact

- 代码：`backend-go/internal/topicgraph/service/lane_snapshot.go`（3 个常量 + system prompt）、`lane_snapshot_test.go`（pin 常量的断言）、`repository/daily_report_models.go` 与 `front/.../LaneTrendOverview.vue` 注释。
- spec：`openspec/specs/board-lane-dynamics/spec.md` 两处字数。
- 无 API/DB schema 变更，无迁移，旧数据无需处理。
