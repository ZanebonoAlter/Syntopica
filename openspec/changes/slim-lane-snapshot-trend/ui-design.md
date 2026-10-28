<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与变更

- 入口不变：TagsPage 日报阅读层 → 泳道展开体顶部「泳道趋势」区（`LaneTrendOverview.vue`），三档 tab（近 14 天 / 月 / 年）保持。
- 变更仅限 14 天档正文内容源：从「长版 detail 优先（缺失回退短版 + 提示）」收敛为「单版演进叙事态势句直接渲染」；正文视觉样式复用现有 `lto__text`（pre-line 纯文本、全文展示不截断），无新增布局结构。
- 「长版随下次日报结算生成」提示条删除；逐日事件展开（默认收起）不变；月/年档不变。

## 受影响状态

- **success（14 天档快照就绪）**：渲染单版态势句全文（rolling_summary），不再读 rolling_detail。
- **empty（快照缺失）**：维持现有「态势待结算」占位（不变）。
- **degraded（存量长版在场）**：降级分支删除——前端不再消费 detail；summary 空串不可达（结算空守卫保证新写非空），无占位分支。loading/error 为宿主 lane-dynamics 请求态，不因本 change 改变。

## 复用组件与布局模式

- 全部复用：`LaneTrendOverview` 现有容器、tab 组、`lto__text` 文本样式、逐日事件折叠；无新组件、无 layout mode 变更（组件嵌在泳道展开体内，随宿主布局）。

## 验收映射

- 组件测试：`front/app/features/tags/components/daily-report/LaneTrendOverview.test.ts`——改造现有用例：快照仅含 summary（单版）渲染全文；detail 存在的存量快照不再渲染长版；删「长版缺失回退+提示」用例；快照缺失占位用例保留。
- 人工验证：本地 :5100 打开日报阅读层展开任一泳道，14 天档显示单段态势句、无「长版待生成」提示（ui-verification/report.md 建账）。
