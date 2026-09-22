<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

- 入口不变：`/tags` 板块页 →「日报」tab → 打开任意一期日报进入阅读视图 → 展开话题泳道（首个泳道保持现状自动展开）。
- 变更点仅在泳道展开体**内部**：顶部新增「泳道趋势」区块（active zone 泳道、有 topic id 时渲染，与现有 `DailyReportMiniLifeline` 的挂载条件一致）；下方 7 天节点图与今天的 section 明细保持现状与顺序。
- 板块内容 tab 的泳道动态卡片不改（短版态势句照旧）。

## 受影响状态

- **loading**：14 天长版随 lane 数据请求态展示（板块级 lane-dynamics 响应未返回时区块骨架/占位）；月/年档切换时按 contexts 请求态展示加载提示（复用现有 `RequestCacheEntry` 风格的加载行）。
- **empty**：月/年无归档行 →「该周期暂无归档摘要」占位（与「态势待结算」同风格文案条）；14 天长版缺失 → 回退显示短版态势句并标注「长版随下次日报结算生成」。
- **error**：lane-dynamics 或 contexts 请求失败 → 内联错误条 + 重试按钮（复用 `LaneDynamicsPanel` 的 stale/error 样式模式），不影响下方现状内容渲染。
- **success**：三档切换即时切换概要内容；14 天档含「展开逐日事件」折叠开关（默认收起），展开后按「日期 → 当日事件列表」渲染并如实标注折叠数。

## 复用组件与布局模式

- 布局：不新增页面/弹窗/导航；趋势区是泳道展开体内的行内区块，宽度跟随日报阅读视图既有内容列（reader 风格正文列），无自由宽度。
- 分段切换：复用既有分段按钮样式模式（与 `BoardThreadBrowser` 的 7天/14天/30天/全部 窗口切换同族，原生 button + active 态，不引新组件库）。
- 折叠展开：复用「还有 N 条」就地展开模式（`LaneDynamicsCard` 既有交互）。
- 状态条/占位：复用 `--bg-sunken` 占位条、内联错误条既有视觉 token。
- 新组件仅一个：`LaneTrendOverview`（`features/tags/components/daily-report/` 下），无基础组件缺口。

## 验收映射

- 组件测试（vitest）：`LaneTrendOverview.test.ts` 覆盖三档切换、长版缺失回退短版、月/年空归档占位、逐日事件折叠与折叠计数；`DailyReportTopicSection` 挂载冒烟（趋势区出现在泳道展开体顶部、现状内容不缺失）。
- 人工验证：本地起服后打开日报展开泳道，核对趋势区三档与板块内容卡片短版互不影响；1440×900 目测无横向溢出。
