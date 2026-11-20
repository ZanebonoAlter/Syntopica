# 技术设计：右轨「当日其他话题」子块

> 配套制品：`proposal.md`（为什么 / 改什么）、`ui-design.md`（交互与数据契约）、`specs/workshop-board-console/spec.md`（条款 delta）。
> 后端零改动：全部数据在前端既有派生层（`buildOtherTopics` + 壳层单 store）。

## 背景事实（已核对）

- 右轨容器 `BoardContextRail.vue`：五个顶层区块，`rail-block-now`（「当日日报」）带 `bcr-block--grow`。
- ≥1440px 时 `.bcr.is-rail` 满高可滚；`.bcr-block--grow { flex: 1 1 auto }` 吃掉剩余高度，而块内 `RailNowBrief` 顶部对齐不撑开 → **差值即右轨底部可见留白**（本次要填的空）。
- 页面壳 `BoardConsoleBody.vue` 的 `.console { height: 100dvh; overflow: hidden }` → 右轨加内容**不会撑高页面**。
- 「当日其他话题」矩阵组与右轨泳道层是**两套量纲**：泳道层「N 条」= lane-dynamics 泳道当日合计（与矩阵日格同源同值）；其他话题组「N 条线索」= 分区非综合稿 thread 数。

## Decisions

### D1 — 子块挂「当日日报」块内，不新增顶层区块

- **决策**：新子块作为 `rail-block-now` 的第二个子块（泳道层之下），右轨顶层五块顺序不变。
- **落点**：`front/app/features/tags/components/console/BoardContextRail.vue:113`
- **取舍**：顶层顺序契约（delta spec「右轨首屏顺序」Scenario）与既有 `BoardContextRail.test.ts` 顺序断言都不破；`--grow` 剩余高度自动被填满，无需算高度。代价：该块的 testid 语义从「当日日报块」扩为「当日日报 + 其他话题」，实现时必须让子块自带标题行与分隔，视觉上仍读作两块。

### D2 — 消费既有 `OtherTopicsView`，零新增请求

- **决策**：右轨直接消费壳层已算好的 `OtherTopicsView`（成员 / 行序 / 计数 / 状态词），不自己算排除口径。
- **落点**：`front/app/features/tags/components/console/dayOtherTopics.ts:170`（`buildOtherTopics`）
- **取舍**：单一实现点（`dayOtherTopics.ts` 头注「组件/壳层不得再算第二套排除口径」），右轨零请求。代价：右轨显示的是「按当日报告分区列出」的成员，部分泳道当日内容未取到时仍照列（与矩阵一致，走既有附注说明）。

### D3 — 接线沿用「壳层取一次、右轨与工作面共用一份」

- **决策**：`ConsoleRailDeps` 加 `otherTopics` → `RailContext` → `RailBinding` → `BoardContextRail` prop；壳层把 `wiring.otherTopics` 接进 `useConsoleShellRail` 的 deps。
- **落点**：`front/app/features/tags/components/console/useConsoleRail.ts:22`（`ConsoleRailDeps`）
- **取舍**：与 `now`（当日日报）同款装配路径，零新增 store、零新增请求。代价：`RailBinding` 多一个字段。**工作面（`BoardWideWorkface` 的 `variant="full"`）也会出现该子块**——工作面并非只在 <1200px 出现（≥1200px 时右轨的体检详情/候选/来源入口同样打开它，且页签可切到 context），而 `.bcr-other` 的媒体查询按**视口**而非 variant 判定。故工作面内 `@select-other` / `@retry-now` 必须与 `@locate-lane` 一样接到壳层（review R-1），否则该渲染面的点行与重试是死键。

### D4 — 右轨另做受控展示组件，不复用矩阵折叠组

- **决策**：新建 `RailOtherTopicsBrief.vue`（纯受控：只渲染 `OtherTopicsView`），不复用 `DayOtherTopicsGroup.vue`。
- **落点**：`front/app/features/tags/components/console/RailOtherTopicsBrief.vue`（新建）
- **取舍**：`DayOtherTopicsGroup` 是 `display: contents` 的矩阵网格行，依赖 `observationMatrixRows.css` 的列对齐语义，搬进右轨会破网格；右轨另做一份单列纵向排版。代价：两份渲染实现共享同一份数据模型（字段同源，样式不同）。

### D5 — 显示门槛 ≥1440px（CSS 媒体查询）

- **决策**：子块默认 `display: none`，仅 `@media (min-width: 1440px)` 显示。
- **落点**：`front/app/features/tags/components/console/BoardContextRail.vue`（样式段 `@media (min-width: 1440px)`）
- **取舍**：只有该宽度段存在 `--grow` 留白可填；1200–1439.98px 右轨随页面流动，出该子块会改变该视口页面高度，守「≥768px 视口布局与交互零变化」红线。代价：1200–1439px 用户看不到该子块（该段本就没有留白问题）。

### D6 — 两套量纲显式分家（标题 + 单位后缀）

- **决策**：新子块标题「当日其他话题」+ 计数徽标「N 条线索」；行内线索数写「N 条线索」；泳道层保持「N 条」。两者不并排成同一列。
- **落点**：delta spec「其他话题层不引入第三套口径」Scenario
- **取舍**：用单位后缀隐式区分，不写长口径说明（界面文字纪律禁教学语/口径科普）。代价：区分依赖阅读者注意单位后缀，实现时必须保证泳道层是「条」、其他话题层是「条线索」。

### D7 — 点行定位复用既有 `selectOther` 选行链

- **决策**：子块每行可点 → 右轨发事件 → 壳层调既有 `selectOther({ sectionId, date, topicId })`，由主区既有阅读槽承接；右轨不自己改选择状态。
- **落点**：`front/app/features/tags/components/console/otherTopicSelection.ts:142`（`selectOther`）
- **取舍**：不新造选中态、不新开浮层（`selectOther` 自带成员合法性与选中日校验）。代价：事件要经 `BoardContextRail` → `BoardConsoleBody` → `BoardConsoleShell` 三层透传。

### D8 — 后端零改动

- **决策**：不新增端点、不改字段、不改数据模型。
- **落点**：`front/app/api/dailyReports.ts:428`（既有 `GET /daily-reports/:id`）
- **取舍**：全部在前端派生层完成；代价：无。

### D9 — 文档同步（flow 红线句 + 已登记例外增补）

- **决策**：① `docs/reference/flow/semantic-board.md` 的右轨顺序描述与「candidate 分区 MUST NOT 进右轨」约束句按 delta 改写；② `docs/reference/standard/frontend/layout.md` 的「已登记例外」表按增补口径在同一单元格内标注本 change 名 + 表下追加增补说明，不另起一行。
- **落点**：`docs/reference/flow/semantic-board.md:249`
- **取舍**：flow 文档是活文档，不改就与实现矛盾（归档门禁 F 段会拦）。代价：两处文档改动需与代码同批提交。

### D10 — spec 改写走 REMOVED + ADDED 整条重写

- **决策**：不写 MODIFIED，改为 `## REMOVED Requirements`（旧条「治理右轨与治理抽屉」+ Reason/Migration）+ `## ADDED Requirements`（新条「治理右轨、治理抽屉与当日其他话题子块」，含原 11 条 Scenario 全文 + 4 条新 Scenario）。
- **落点**：`openspec/changes/add-other-topics-to-context-rail/specs/workshop-board-console/spec.md`
- **取舍**：openspec 2.1.1 的 MODIFIED 硬校验要求「不得丢任何既有 Scenario」（`diffScenarioNames`），而本 change 要推翻的正是其中一条 Scenario（候选分区不进右轨）——保留它就得让标题与新规则互相打脸，改标题就过不了校验。按仓库先例（`2026-11-09-slim-lane-snapshot-trend` 等）整条重写：语义干净，代价是新条在 spec 文件里的位置排到末尾（新增条目按 ADDED 追加）。

## 风险与既有约束核对

| 约束 | 落点 | 本 change 的处理 |
| --- | --- | --- |
| 右轨五块顺序契约 | delta spec「右轨首屏顺序」 | 不动（D1） |
| 右轨不得提供日期选择器 | delta spec「治理右轨、治理抽屉与当日其他话题子块」 | 不加（D2 复用矩阵选中日） |
| 泳道层不并排两套量纲 | delta spec「泳道层不并排两套量纲」 | 新层分块分单位（D6） |
| 排除口径单一实现点 | `dayOtherTopics.ts` 头注 | 消费现成模型（D2） |
| ≥768px 视口零变化 | `standard/frontend/layout.md` | 门控 ≥1440px（D5） |
| 已登记例外增补口径 | `standard/frontend/layout.md` §已登记例外 | 同单元格标注 + 表下说明（D9） |
| 界面文字纪律 | `standard/frontend/interaction.md` | 只用标题/徽标/计数/状态词，不写口径教学语（D6） |
