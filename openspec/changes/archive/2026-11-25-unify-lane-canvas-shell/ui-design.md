<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约 · unify-lane-canvas-shell（minor）

> 本 change 不新增页面、面板、弹窗或交互模式。全部改动都发生在**已存在的全屏画布内部**：把两支画布各自维护的外壳层收敛成共用单元，并把新建侧已有、编辑侧缺失的两条吸顶轴补齐。视觉变化面 = 编辑泳道左侧多一条常驻列、顶部日期轴常驻。

## 入口与入口变更

**入口零变更**。两个模式各自的入口原样保留：

| 模式 | 入口 | 文件 |
|---|---|---|
| 新建泳道（`compose`） | 版块态势页顶栏「＋ 新建泳道」 | `console/BoardConsoleHeader.vue` → `useConsoleShellOverlays.ts:handleNewLane()` |
| 编辑泳道（`correct`） | 矩阵行入口 / 右轨话题入口 | `console/BoardConsoleShell.vue` → `BoardComposeOverlay.vue`（`mode=correct`） |

两个模式仍由同一个 `console/BoardComposeOverlay.vue` 承载；本 change **不新增**任何按钮、菜单项、快捷键或跳转路径。

## 受影响状态

只列**受影响**的状态，其余（loading 骨架 / error 条 / 空态说明 / 保存确认）行为不变：

| 状态 | 新建泳道画布 | 编辑泳道画布 | 本 change 的影响 |
|---|---|---|---|
| 纵滚可读性（日期轴带） | 不变 | **变** | 纵滚溢出时日期刻度层带上不透明底 + 提档底 + 下缘 1px 分隔线，保证刻度不被滑上来的内容盖住；不溢出时不画（与改动前一致） |
| `success`（正常有内容） | 不变 | **变** | 编辑画布横滚时左侧分区标签列常驻；纵滚时日期轴常驻。两者都叠在内容之上（不透明底 + 6 级渐隐阴影），节点与标题照旧绘制 |
| 无横向溢出（内容宽 ≤ 视口） | 不变 | **不变** | 固定列仅在有溢出时出现（`overflowingX` 门控），不溢出时与改动前逐像素一致 |
| 无纵向溢出 | 不变 | **不变** | 同上门控 |
| `loading`（骨架） | 不变 | 不变 | 骨架不参与吸顶（沿用既有骨架几何） |
| `empty`（窗口内无节点） | 不变 | 不变 | 空态说明仍居中；固定列无内容可不渲染底 |
| `error`（读失败 + 重试） | 不变 | 不变 | 错误条位置不变 |
| 拖拽中 | 不变 | **变** | 滚轮在拖拽期间不再触发缩放（与新建侧对齐）；拖拽手势、落点高亮、失败回滚全不变 |

窄屏（<768px）不改：两个模式本就只出「需要宽屏或横屏」提示，不画残缺工作台。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
|---|---|---|---|---|
| D1 | 泳道内容的增删改入口 | `canvas` | `dialog`, `drawer` | `pinned` |
| D2 | 编辑泳道的分区名在横向滚动时保持可见 | `canvas`（P1：依赖分区与节点的空间关系才能判归属） | `panel`, `drawer`, `dialog`（不得为「看分区名」另开浮层） | `open` |
| D3 | 编辑泳道的日期轴在纵向滚动时保持可见 | `canvas`（P1：刻度与日期列必须空间对齐） | 同上 | `open` |
| D4 | 滚动/缩放/吸顶外壳的实现归属 | `canvas` | —（实现层决策，不是交互模式；记此以免被误读为新增模式） | `pinned` |

> D1 来自 `docs/reference/standard/frontend/interaction.md` 既有台账（`pinned`，不得替换）。D2/D3 是同一 `canvas` 模式内的**可见性补强**，不引入新模式，故标 `open`——它们描述的是本 change 新增的契约，待实现与验收完成后才具备 pinned 资格。

## Data Provenance

本 change **不引入任何新的用户可见数据项**。编辑画布新增的固定列显示的是既有字段：

| 数据项 | source | 说明 |
|---|---|---|
| 分区名（话题带 / 候选区 / 只读参照） | `派生 静态文案` | 硬编码的分区标题，非数据 |
| 日期轴刻度 | `派生 props.dates` | 由服务端回显窗口展开的日期数组（既有 `serverWindow.from/to`） |
| 泳道名（话题带副标题） | `派生 props.laneLabel` | 既有 prop，来自话题 label |
| 节点标题与接近度 | 不变 | 与改动前同源（`compose-candidates` / 节点数据） |

无 `none` 项，无需批准。

## 复用组件与布局模式

**布局模式：自由宽度工作台（既有，本 change 不改变）**。两个画布都处在 `BoardComposeOverlay.vue` 的全屏三栏里，沿用既有声明——左栏 300px / 右栏 320px（新建）、左 380 / 右 300（修正），中央画布 `minmax(0, 1fr)` 填满剩余宽度。这是 `layout.md` 允许的「少数特殊工作台脱离四模式」既有例外，不是本 change 新开的。

**复用**（MUST 复用，不新建同义组件）：

| 复用对象 | 用途 | 本 change 动作 |
|---|---|---|
| `composables/laneComposeLogic.ts` | 缩放/平移/接近度/标签宽度等纯函数 | **只读复用**，不改语义 |
| `console/ObservationRangeControls.vue` | 时间视窗（档位 + 刷选 + 曲线） | 不动（三处已共用） |
| `assets/css/main.css` 语义色 token（`--color-bg-base` / `--color-border-*` / `--color-accent`） | 吸顶列底色、分隔线、阴影 | 复用；**不新增语义色**（吸顶列是中性面，不承载新语义） |
| `console/LaneCorrectionCanvas.vue` 既有 `.lcc-*` 类名体系 | 编辑画布样式 | 复用同一前缀，不另起命名空间 |

**本 change 新增的两个单元**（不是用户可见组件，是内部实现单元，因此不计入「新组件」审批）：

- `composables/useLaneCanvasShell.ts` —— 缩放 / 平移 / 容器测量 / 滚动同步 / `Esc` 外壳
- 吸顶标签列单元（组件或渲染函数，归位以 `design.md` 为准）—— 反向 `translate` + 不透明底 + 提档底色 + 6 级渐隐阴影 + 溢出门控

**明确不新增**：不新增弹窗、抽屉、面板、工具栏按钮、快捷键、图例、教学文案。

## State Matrix（受影响子集）

| 态 | 触发条件 | 表现 | 恢复路径 |
|---|---|---|---|
| 横滚溢出 | `vbWidth × zoom > 容器 clientWidth` | 左侧固定列出现（分区名 / 行标签），带 6 级渐隐阴影 | 缩回或内容收窄即消失，回到无固定列的原样 |
| 纵滚溢出 | `canvasHeight × zoom > 容器 clientHeight` | 日期轴钉在画布顶部 | 同上 |
| 无溢出 | 上述两个条件都不成立 | 与改动前逐像素一致（不下发 transform、不画固定列底） | — |
| 折叠带出现/消失（仅编辑画布） | `foldNodes.length` 0 ↔ 非 0 | 固定列宽度在 150 ↔ 226px 之间变化一次 | 属可容忍的一次布局变化，不重排内容 |

## Acceptance

**验收映射（指名道姓）**

| 断言 | 落点 |
|---|---|
| 编辑画布横滚后分区名 / 纵滚后日期轴仍在视口内 | 临时探针验证（`standard/shared/test-design.md`「白盒是临时验证」），证据入 `ui-verification/report.md` §2–§4；**探针用完清理，不提交为永久业务测试**（纯 UI 走现行 UI 验收证据） |
| 拖拽期间滚轮不缩放、`Esc` 同时取消拖拽与平移 | **真实 Bug 回归**（改前确实漂移）：`front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` 的「手势外壳与编排画布同源」组，保留 |
| 新建画布吸顶行为**不回归** | 既有存量用例 `front/app/features/tags/components/compose/LaneComposeCanvas.test.ts` 全绿（本 change 未新增/未删除其用例） |
| 缩放/平移语义不变 | 既有存量用例 `front/app/features/tags/composables/laneComposeLogic.test.ts` + 上述两组全绿 |

**视觉检查**：本 change 为 `minor`，不强制双档 agent-browser 截图；但需在 `ui-verification/report.md` 建账（已建，§6 台账 A1–A8），并记录一次**编辑画布横向滚动前/后**的对照（固定列出现、阴影、无内容遮挡错位）+ 一次折叠带出现时的固定列宽度变化。无实拍的部分按「未覆盖 + 原因」记账，**构建部署后补拍并回填**。

**与批准原型的差异说明**：`ui-prototype: none`，无原型可比对；基线 = `ui-design.md` 本节 + `specs/lane-correction-surface` 新增 Requirement。
