<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->
<!-- review: waived -->

# Proposal: 清理 workshop-board-console 里事件线量纲的 spec 残留

## Why

`retire-story-adopt-event-metrics`（2026-11-11 归档）把事件线（story）身份与量纲整体退役，`fix-lane-metric-provenance`（2026-11-16 归档）进一步把矩阵日格收敛为**非综合稿线索数合计**、格色深收敛为**行内相对强度**。但主 spec `workshop-board-console` 的「**矩阵视图（泳道 × 日期）**」这条 requirement 从未同步——它仍写着「格内数字为该泳道当日的事件线数」「图例说明数字为事件线数、颜色为来源数分档」。

结果：同一个 spec 文件里，第 262 行的「泳道热度口径（线索数单一口径）」明确写「矩阵日格 SHALL NOT 再引入事件线数」，而「矩阵视图」要求「格内数字为该泳道当日的事件线数」——**两条 requirement 直接冲突**。实现是对的（`clue_count` + `composite_count` 已在代码里），纯粹是 spec 文本债；但留着会让后续读者/实现者按错误口径理解。

## What Changes

把「矩阵视图（泳道 × 日期）」这条 requirement 的 4 处事件线口径改为现行口径（线索数 + 行内相对强度），其余条款（断档标记、零线索可开、日期列头联动、只读、横向滚动）**原样保留**：

| 位置 | 现文 | 改为 |
| --- | --- | --- |
| 正文 1 | 格内数字为该泳道当日的事件线数 | 格内数字为该泳道当日的**非综合稿线索数合计** |
| 正文 1 | 有分区但零事件（全部事件为综合稿或无归组且无事件线） | 有分区但零线索（全部线索均为综合稿） |
| 正文 2 | 图例说明数字为事件线数、颜色为来源数分档 | 图例说明数字为**线索数**、颜色为**行内相对强度** |
| Scenario「图例可见」 | 数字 = 当日事件线数；颜色 = 当天被报道最多的那条事件线的来源数 | 数字 = 当日线索数；颜色 = 行内相对强度（不是来源数分档） |
| Scenario「零线索可开」 | 无事件线且无非综合稿事件 | 无非综合稿线索 |

## Capabilities

### New Capabilities

（无。）

### Modified Capabilities

- `workshop-board-console`: 「矩阵视图（泳道 × 日期）」的事件线量纲残留改为现行线索数口径，与同文件的「泳道热度口径（线索数单一口径）」对齐。

## Impact

**纯 spec 文本修订**，不改任何代码、接口、数据模型、UI。

- 不涉及 `docs/reference/`（这条 requirement 是 openspec spec 层契约，flow 文档里的矩阵口径已在 `fix-lane-metric-provenance` 中同步）。
- 不涉及前端/后端文件。
- 不改行为语义：现行实现已经是线索数口径，本 change 只是让 spec 追上实现。

**并发提示**：与 `unify-lane-workbench`（改 `lane-correction-surface` / `workshop-lane-compose`）不撞文件、不撞能力。
