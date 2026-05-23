## Why

叙事（narrative）系统的后端生成逻辑存在高/低量分治，导致抽象标签树没有被单独处理为一个板块（Board），前端两套模式（版块/时间线）并存，交互逻辑混乱。需要统一为"全部/分类 → 具体板块 → 相关叙事"的三级导航模型，以抽象标签树为板块划分的核心依据。

## What Changes

- **确定性 Board 创建**：每棵活跃的抽象标签树自动成为一个 Board，不再依赖 LLM 分组
- **杂项 Board**：未归入任何抽象树的事件标签，数量 ≤3 直接归入一个杂项 Board，>3 则由 LLM 分组为多个杂项 Board
- **删除低量直出模式**：移除 `generateDirectForLowVolume`，所有叙事都必须属于一个 Board
- **删除 abstract 卡片叙事**：Board 本身就是抽象树的代表，不再需要 `source="abstract"` 的叙事
- **简化 LLM 调用链**：Board 创建不再调用 LLM，LLM 只用于 Board 内叙事生成（`GenerateNarrativesForBoard`）和杂项事件分组（`PartitionMiscEvents`）
- **prev_board_ids 改用抽象标签匹配**：通过抽象标签 ID 确定性匹配昨日 Board，替代 LLM 推测
- **前端移除时间线模式**：删除 `NarrativeCanvas.client.vue`，只保留版块模式
- **前端统一作用域系统**：删除两套 scope/mode 状态，只保留一套 `scopeMode: global | category`

## Capabilities

### New Capabilities
- `narrative-board-generation`: 从抽象标签树确定性创建 Board，杂项事件分组，每个 Board 内 LLM 生成叙事
- `narrative-board-frontend`: 统一的三级导航前端交互（全局/分类 → Board 列表 → Board 内叙事）

### Modified Capabilities
<!-- No existing specs to modify -->

## Impact

- **后端**：`board_generator.go`, `service.go`, `generator.go`, `collector.go` 中生成流程重写
- **前端**：`NarrativePanel.vue` 大幅度简化，删除 `NarrativeCanvas.client.vue`
- **数据模型**：`NarrativeBoard` 新增 `abstract_tag_id` 字段用于确定性匹配
- **LLM 调用**：Board 分组调用被移除，杂项分组调 LLM，总数变化不大但每次 prompt 更精准
