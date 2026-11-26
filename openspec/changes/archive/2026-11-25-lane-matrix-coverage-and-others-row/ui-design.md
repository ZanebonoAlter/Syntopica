<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# lane-matrix-coverage-and-others-row — UI 设计契约

（minor 档：全部复用既有矩阵行/格/组 summary/行头菜单与就地编辑模式，不新增页面、不改信息架构、不改固定视口栅格例外。）

## 入口与入口变更

| 入口 | 位置 | 变更 |
| --- | --- | --- |
| 矩阵行头 ⋯ 菜单 | `ObservationMatrixLaneRow`（活跃 / 休眠 / 覆盖组同款） | **新增**第一项「重命名」（其余「编辑泳道 / 归档泳道」不变）；点后行内名字变输入框 |
| 矩阵底部「窗口内未进日报」组 | `ObservationMatrixGrid`，位于休眠组之后 | **新增**：组 summary（`N` + 泳道名 + 最近一次日期）+ 默认展开的行；行头 ⋯ 与活跃行一致（「未进日报」身份由所在组表达，行内不再重复标签，避免挤占窄标签列的名字） |
| 矩阵「其他话题（未入泳道）」整行 | `ObservationMatrixGrid`，位于活跃行之后、当日其他话题组之前 | **新增**一行：逐日格 = 未入泳道线索数 |
| 「当日其他话题」组 summary | `DayOtherTopicsGroup` | 不变（本次只保证休眠 summary 补名字，其他组文案零改动） |
| 休眠泳道 summary | `ObservationMatrixGrid` | **补**泳道名（既有 spec 要求「折叠条显示休眠数量与泳道名」，当前实现只显示数量） |

## 受影响状态

| 状态 | 影响 |
| --- | --- |
| loading | 覆盖清单读取中：不渲染覆盖组（不占位、不冒「无覆盖」）；矩阵其余行不受影响 |
| empty | 覆盖清单成功且为空（所有活跃泳道都在窗口内）：整组隐藏，MUST NOT 显示「0 条」空组 |
| error | 覆盖清单失败：在矩阵底部显示一行弱化提示「窗口外泳道清单未取到 · 重试」，MUST NOT 影响矩阵行/格 |
| success | 覆盖组默认展开：组标题列出泳道名 + 「最近一次 MM-DD」；行内格全为「—」不可点；行头 ⋯ 可改名/编辑/归档 |
| 其他话题行 | 后端无 `other_topics`（旧后端）→ 整行隐藏；无任何 completed 日报（空数组）→ 整行隐藏；有日报 0 线索 → 显示 `0` 可点；可见轴到 365 池区段时档窗外格显示「—」且 title 写「档窗外，其他话题读数未取到」 |
| 重命名 | 输入中：行头只显示输入框（不触发行选中）；保存失败：输入框保留 + toast 报错；成功：退出编辑并刷新名单 |

## 复用组件与布局模式

- 布局模式：**不变**——L1 版块态势仍为固定视口栅格例外（三栏常驻 + 页面 `overflow: hidden`，矩阵舞台内部横滚）。
- 复用组件/样式：`RowActionMenu`（`⋯` 与菜单）、`observationMatrixRows.css`（行高/格/sticky 标签列/选中深浅）、组 summary 的 `omx-group-*` 样式族、`useNotify`（成功/失败 toast）。**不新增**基础组件、不新增色板、不新增弹窗。
- 「其他话题」行视觉：与泳道行同网格同列宽，行头用既有弱化字重 + 「其他话题」标签；格子沿用线索色深（行内归一），但格边框使用虚线（`border-style: dashed`）以区分「不是泳道」；MUST NOT 新增第二种色阶或语义色。
- 覆盖组行视觉：与休眠行同款弱化（`is-dormant`），身份由组标题「窗口内未进日报」表达（行内不重复小标，避免窄标签列把泳道名挤成 5 个字）；名字为静态标签（不可选中，不产生新选中态），⋯ 菜单保留；MUST NOT 用危险色（它们是正常活跃泳道）。
- 窄屏：L1 窄屏本就不渲染矩阵（既有降级），本次不新增窄屏形态。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 新泳道可见性：矩阵底部独立组、默认展开、标题列出名字与最近一次日期 | `panel` | `dialog`, `drawer`, `page` | `pinned`（用户 2026-11-25 选定「底部单独一组，默认展开」） |
| D2 | 重命名：行头 ⋯ →「重命名」→ 行内输入框（Enter 保存 / Esc·失焦取消） | `inline-edit` | `dialog`, `page` | `pinned`（用户 2026-11-25 选定「⋯ 菜单加重命名」） |
| D3 | 其他话题整行格值：线索数（与泳道格同口径），title 带分区数 | `panel` | `dialog` | `pinned`（用户 2026-11-25 选定「线索数同口径」） |
| D4 | 点其他话题行的有日报格：选中该日并展开下方「当日其他话题」组 | `command` | `dialog`, `page`, `drawer` | `pinned` |

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 覆盖组泳道清单 / 名字 / 最近日期 | `api GET /semantic-boards/:id/topics`（既有） | 只取 `status === 'active'` 且不在 A 档行里的；`last_seen_date` 原样展示，缺失显示「无记录」 |
| 其他话题逐日线索数 / 分区数 | `api GET /semantic-boards/:id/lane-dynamics` 新增 `other_topics[]` | 服务端按 completed 日报、排除 active/watch 泳道话题、综合稿不计线索数；前端不自行推算 |
| 泳道改名结果 | `api PATCH /daily-reports/topics/:id`（既有） | 与修正工作面同一写入路径与同一响应；成功后重取 A 档与覆盖清单 |
| 休眠组泳道名 | A 档 `lanes[].label`（既有） | 纯展示，不新增数据 |

## Prototype

无需原型（minor 档：矩阵行/格/组/就地编辑全部为既有模式的新组合，无新界面结构）。

## Acceptance

- 组件测试：
  - `front/app/features/tags/components/console/ObservationMatrix.test.ts`：覆盖组默认展开/空态隐藏/名字与最近日期、点「重命名」进输入框、Enter 保存调用注入函数、Esc 取消；
  - `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`：其他话题整行逐日格值/0 可点/无日报「—」、旧响应隐藏；休眠 summary 含泳道名；
  - `front/app/features/tags/components/console/ObservationMatrixLaneRow.test.ts`：菜单含「重命名」且不触发行选中；输入框 Enter/Esc/空值。
- 纯函数测试：覆盖清单派生（active 过滤、去重、排序、last_seen 缺失）；normalizer 的 `other_topics` 三态（缺字段/空数组/有值）。
- 命令：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrixLaneRow.test.ts --maxWorkers=2` → 全绿。
- 视觉自检（minor 档不做归档级双视口门禁，但本次触及矩阵行密度）：1440×900 与 1920×1080 下覆盖组与其他话题行不产生页面级横向溢出、sticky 标签列不遮挡、行高与既有行一致；dark 主题下虚线格与弱化行可辨。
