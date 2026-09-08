<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

无新入口。变更发生在日报详情/杂志视图的既有物化板块（`lane_tier=watch_keyword` / `watch_sentence`）内部：

1. **板块标题**：`cluster_label` 数据源由「watch 名照抄」变为「LLM 当日贴合标题（兜底=watch 名）」——前端展示字段与渲染路径不变，纯数据层变化。
2. **watch 名小装饰**：既有 `SectionWatchBadge`（挂在 `DailyReportTopicSection` 标题旁的类型徽标，现仅显示「关键字物化板块/一句话物化话题」）扩展为同时显示 watch 名（如「关键字物化板块 · harness」），让用户在 LLM 标题之外始终看得到追踪源。需要后端在 section 序列化上透出 watch 名装饰字段（transient，`gorm:"-"` 模式，参照 `TopicWatchHit.WatchLabel` 先例；具体载运方式 design 阶段定）。

## 受影响状态

- **success**：物化板块标题=LLM 当日标题，badge 带 watch 名——正常渲染路径。
- **error（裁决降级）**：后端 AI 失败回退召回全量 + 标题兜底 watch 名——前端表现与现状完全一致，无新错误态。
- **loading / empty**：不受影响（日报生成完成后整体返回；无命中日无板块，现状不变）。

## 复用组件与布局模式

- 复用 `DailyReportTopicSection` 板块结构（不改布局模式、不改杂志/时间线编排）。
- 复用并小幅扩展 `SectionWatchBadge`（加一个可选 prop / 装饰文案拼接，不新建组件）；样式继续用 `data-lane-tier` 既有 token。
- 不涉及 dialog / 新页面 / layout mode 变更。

## 验收映射

- 组件测试：`SectionWatchBadge` 渲染 watch 名装饰（有/无 watch 名两态）。
- opencli / 人工验证：生成一期含物化板块的日报，检查 LLM 标题与 badge 装饰展示（归档前 UI 验收证据）。
