<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 不适用记录

## N/A Reason

本 change **不改任何界面**：`ui-impact: none`，与 proposal 头部声明一致。

- **改动全在数据判定层**：事件指纹多一个字段（`object_type`）、事件线多一列（`board_stories.object_type`）、归属判定拆两层、矩阵日格颜色换一个计数口径。**无新页面、无新面板、无新弹层、无新路由、无新交互、无文案改动、无前端代码改动**。
- **用户可见的变化是「数据变了」，不是「界面变了」**：跨天延续生效后，雷达右栏「今日该读 / 今天可能升温」里出现的条目，其事件线**可能跨多天**，「今天可能升温」档不再结构性为空，14 日矩阵里跨天线在有事件的每一天各占一格 —— 这些都发生在**上一条 change（`surface-story-events-in-radar`）已建好的既有界面**上，界面本身与交互模式一字不动。
- **关于「跨天延续的线要可人工复核」**：本 design 的误合并防护**不依赖新界面**（判据层收紧 + `is_development` 标记 + `first_seen_date < last_seen_date` 的可识别口径 + CLI 快照/还原 + 人工抽样验收门）。仓库里 `event-story` spec 早有一条「人工纠正优先」约束（`daily_report_threads.story_manual`），但**至今没有对应端点与界面**（只有数据列）——补齐「人工拆分/指派事件线」的复核入口属于**独立缺口**，本 change 不假装它存在、也不把它顺手塞进来；**另开 change**。若届时复核入口要进产品界面，按 ui-impact 规则走 minor/major 流程，与本 change 无关。
- 因此：**不做原型**（`ui-prototype: none`），**不建 `ui-verification/` 证据账本**，**不写前端验收映射**。本 change 的验收全部落在后端单测 / 隔离 PostgreSQL 集成测试 / CLI 行为 / 真库量化核对（见 `test-cases.md` §4 效果核对）。
