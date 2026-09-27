# Delta: board-topic-landscape（整体移除）

## REMOVED Requirements

### Requirement: 话题态势版图入口
**Reason**: 话题态势版图（活力条/节奏气泡图/姿态卡片墙）是计数统计型概览，用户无法从中获知泳道最近发生了什么；「板块内容」tab 的首屏概览职责由 board-lane-dynamics（泳道动态视图）接管。
**Migration**: 前端 `TopicLandscapePanel` 及 `topic-landscape/` 组件目录删除；原承载的两个用户能力在新视图保留等价入口——①空态「生成日报」引导（新视图空态复用同一端点与 WS 进度模式）；②「点态势卡片→话题总览 focus 联动」（新泳道动态卡片保留跳转）。

### Requirement: 态势派生基于 identity 轨
**Reason**: stance 五态派生（active/stalled/emerging/pending/archived）服务于态势卡片墙的分组展示，随视图一并退役。
**Migration**: 无下游依赖（stance 仅 TopicLandscapePanel 消费；数据增强 situation_cards 有独立派生逻辑不受影响）。

### Requirement: 态势分区卡片墙
**Reason**: 分组卡片墙视图整体退役。
**Migration**: 用户可从「话题总览」tab（BoardThreadBrowser 话题管理列表）查看全部话题状态；候选态的引导由 board-lane-dynamics 候选栏承接。

### Requirement: 话题卡片 mini-lifeline
**Reason**: mini-lifeline 逐日 section 计数条属态势卡片内嵌展示，随卡片墙退役；board-lane-dynamics 的「发展时间线」以日期→事件粒度提供更强的叙事等价物。
**Migration**: 无（叙事级时间线为超集替代）。

### Requirement: 待激活话题引导
**Reason**: 态势卡片墙内嵌的待激活引导交互退役。
**Migration**: 候选泳道的可见性提示由 board-lane-dynamics 候选栏承接（只读列表）；转正操作保持既有「话题总览」管理入口不变。

### Requirement: 活力顶栏
**Reason**: 窗口计数统计条（文章数/section 数/话题数）退役，计数信息不是用户决策所需。
**Migration**: 无（用户明确表示计数展示无价值）。

### Requirement: 空态处理
**Reason**: 态势版图的空态引导随视图退役。
**Migration**: 等价能力（无日报时引导生成日报）迁入 board-lane-dynamics 空态 Requirement。

### Requirement: 后端聚合接口
**Reason**: `GET /semantic-boards/:id/topic-landscape` 仅服务态势版图，无其他消费方（已验证）。
**Migration**: API 移除；替代读取路径为 board-lane-dynamics 的 `GET /semantic-boards/:id/lane-dynamics`（泳道卡数据聚合端点）。

### Requirement: 话题节奏总览气泡图
**Reason**: echarts 气泡图为统计可视化，随态势版图退役。
**Migration**: 无（发展时间线提供事件级动态叙事替代）。
