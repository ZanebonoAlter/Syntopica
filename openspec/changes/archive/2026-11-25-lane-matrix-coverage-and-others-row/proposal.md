<!-- complexity: medium -->
<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- constraint-domains: semantic-board, daily-report -->

## Why

版块态势矩阵（`ObservationMatrix`）现在会丢掉两类真实数据，用户看版时直接变成「看不见」：

1. **新建/窗口外泳道整体消失**。矩阵行来自 `lane-dynamics` 的 `lanes`，而后端只为「窗口内有锚定 section」的话题生成行（`computeBoardLaneDynamics` 里 `aggs` 只来自窗口 sections；spec 的「沉寂不展示」）。用户实测：新建「以色列内部选举」（话题 2645，active，manual，5 个分区在 2026-10-27~11-09），当前 14 天窗是 11-11~11-24 → 后端不返回 → 矩阵、右轨都没有它，连行头 ⋯（编辑/归档）也无处可点。用户第一反应是「新建的泳道丢了」。
2. **改名只有一条深路径**。矩阵行 ⋯ 只有「编辑泳道 / 归档泳道」；改名要进全屏修正工作面、再点标题文字才变输入框。用户找不到，于是判定「没法改名」。
3. **「当日其他话题」只能一天一天点**。该组只画所选日一列（`buildOtherTopics` 只用所选日 `reportDay`），跨日没有任何读数；用户想知道哪几天「没进泳道的话题」多，只能逐日点过去。

## What Changes

- **窗口内未进日报的活跃泳道可见**（前端，零新增后端接口）：控制台用既有 `GET /semantic-boards/:id/topics` 取本板块全部 persistent topic，`status === 'active'` 且不在 A 档矩阵行里的泳道，进矩阵**底部独立组「窗口内未进日报」**——组标题直接列出泳道名与最近一次日期，**默认展开**；行内逐日格全部为「—」（诚实的「该窗口无分区」），行头 ⋯ 保持可用（重命名 / 编辑泳道 / 归档泳道）。该组不参与 A 档排序/休眠判定，也不进右轨名单。
- **矩阵行头就地改名**：行头 ⋯ 新增「重命名」——点后泳道名变输入框（Enter 保存 / Esc 或失焦取消），复用既有 `PATCH /daily-reports/topics/:id`（`updateTopic`），成功后刷新 A 档与泳道覆盖清单；失败保留输入并提示。不新开弹窗、不新增接口。
- **矩阵新增「其他话题（未入泳道）」整行**（后端补每日聚合 + 前端一行）：`lane-dynamics` 响应新增 `other_topics` 逐日数组（completed 日报日一条，含 0），格值 = 当日未入泳道分区里**非综合稿线索数**之和（与泳道格、当日其他话题组同口径），title/aria 带分区数；点有日报的格 → 选中该日并展开下方「当日其他话题」组。无日报日显示「—」不可点；旧后端无该字段时整行隐藏（向后兼容）。

**不做什么**：

- 不改 `lane-dynamics` 的 `lanes` 语义（「沉寂不展示」仍成立，雷达总览/候选栏不受影响）；新泳道可见纯前端补齐。
- 不改话题状态机、命中计数、归属计算与日报生成逻辑；改名沿用既有 PATCH，不重算质心。
- 不为「其他话题」新增第二套排除口径：后端定义与前端 `dayOtherTopics` 的 A 名单排除保持一致（persistent_topic_id 为空或不属于 active/watch 话题）。
- 界面入口不变：页面 / 弹层 / 路由均不引入；不引入新依赖。

## Capabilities

### New Capabilities

- 无（本 change 只修改既有矩阵/聚合契约）。

### Modified Capabilities

- `workshop-board-console`：矩阵新增「窗口内未进日报」组与「其他话题」整行；休眠组折叠条补泳道名；矩阵选中日联动扩展一条入口。
- `board-lane-dynamics`：聚合端点新增 `other_topics` 逐日读数（未入泳道分区的线索数与分区数，含真实 0）。
- `lane-correction-surface`：改名入口从「只在修正工作面标题」扩展到「矩阵行头 ⋯ 就地改名」，复用同一条写入与校验。

## Impact

- **前端**：
  - 数据：`useConsoleShellData` 新增 `listBoardTopics` 一次性读取（含 loading/error/陈旧，切板块复位）；新增纯函数把「A 档行 + 活跃话题清单」折成窗口外行。
  - 组件：`ObservationMatrix` / `ObservationMatrixGrid` 新增覆盖组与「其他话题」行；`ObservationMatrixLaneRow` 新增重命名就地编辑 + 菜单项；`DayOtherTopicsGroup` 的 summary 补泳道名（既有 spec 要求）。
  - 接线：`BoardConsoleShell` 提供 `renameLane` 函数 prop、刷新覆盖清单；`consoleStageBinding` 透传覆盖行与重命名动作；`useConsoleReadingWiring` 新增「点其他话题整行 → 选中日 + 展开其他组」入口。
  - 样式：复用 `observationMatrixRows.css` 行/格与组 summary 样式，新增一条「其他话题」行的弱化/虚线视觉（仅区分身份，不新增色板）。
- **后端**：`LaneDynamicsResponse` 新增 `other_topics`；`computeBoardLaneDynamics` 用一条窗口聚合 SQL 得到每日「未入泳道」分区数与非综合稿线索数（`daily_report_sections` + `board_daily_reports` + `daily_report_threads`，completed 日报、PG/SQLite 兼容写法）。无迁移、无写路径。
- **数据**：无迁移、无写路径、无新表。
- **用户可见变化**：新建但窗口内没进日报的泳道不再消失，会在矩阵底部「窗口内未进日报」组里显示；行头 ⋯ 可以直接改名；矩阵多一行「其他话题（未入泳道）」，可一眼看出每天有多少未入泳道的线索，点格可跳当日其他话题。
- **部署后需要用户做的操作**：无。旧数据无需回填；后端未升级时前端「其他话题」行自动隐藏，其余两项照常工作。
- **测试**：后端新增聚合口径单测（含 0 值日、非 completed 日报排除、综合稿排除、空泳道集）；前端新增覆盖清单纯函数、矩阵组渲染/改名/其他话题行的组件测试与既有测试更新。
