<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# clarify-lane-correction-effect-timing — UI 设计契约

（minor 档：复用既有交互模式与布局，不新增界面结构、不改信息架构。）

## 入口与入口变更

| 入口 | 位置 | 变更 |
| --- | --- | --- |
| 编辑泳道（矩阵行「⋯」→「编辑泳道」） | `ObservationMatrixLaneRow.vue` → `BoardComposeOverlay.vue` 修正模式 | **不变**（仍进同一修正工作面） |
| 泳道管理列表行内「编辑」 | `TopicManageDialog.vue` → 同一修正工作面 | **不变** |
| 修正工作面**顶部标题行** | `BoardComposeOverlay.vue` `.bco__top` | **新增**结论型徽标组（立即生效 / 次日日报生效 / 无需保存） |
| 修正工作面**底部页脚** | `BoardComposeOverlay.vue` `.bco__foot` | **撤除**同义小徽标（内容上移到顶部，避免两处重复） |

## 受影响状态

| 状态 | 影响 |
| --- | --- |
| loading | 徽标是静态文案，不随取数变化；加载中照常显示（结论恒成立：写操作逐条落库） |
| empty | 无成员（空池）时徽标照常显示（它讲写口径，不讲数据量） |
| error | 取数失败（画布 error 态）时徽标照常显示；写失败的 toast 反馈照旧 |
| success | 每次写入成功 toast 照旧；徽标组不随写操作变化（常驻） |

新建泳道模式（`isCorrection === false`）**不渲染**徽标组——该模式的出口是「起名并创建」，不需要生效时机说明。

## 复用组件与布局模式

- 布局模式：`workspace`（`AppPageShell mode="workspace"`）——**不变**，沿用修正工作面既有三栏（`380 / 1fr / 300`）。
- 复用组件：既有 `.bco__top` 标题行 + 既有 `.bco__timing` 徽标样式族（accent 实线 / 虚线 / 中性三种）；**不新增**基础组件、不新增弹窗、不新增自由宽度。
- 与左栏面板头徽标的分工（不重复）：**顶部徽标组 = 全局生效时机摘要**（覆盖加入 / 移出 / 首义 / 改名四类动作，故不加单一作用域词）；**左栏面板头徽标 = 分块作用域**（`LaneMemberList`「归属 · 立即生效」= 成员归属；`LaneAnchorScope`「次日日报生效」= 首义范围），属既有组件、本次不动。
- 徽标组宽度自适应（`flex-wrap` 兜底）；窄屏（<768px）沿用既有「需要宽屏或横屏」提示，不画残缺工作台。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 生效时机的承载形态：修正工作面**顶部标题行常驻结论型徽标组**（只给结论、不解释机制；不新增任何交互面）。判据 P1——工作面主模式是 canvas，结论型徽标挂在工作面内而非另起交互面 | `canvas` | `dialog`, `drawer` | `pinned` |
| D2 | 泳道名改名的承载形态（本次不改，仅锁定防回归）：标题处就地编辑。判据 P2——改单个字段且需看见上下文 | `inline-edit` | `dialog`, `drawer` | `pinned` |
| D3 | 写操作的承载形态：加入 / 移出 / 定首义 / 改名**逐条即时落库**，工作面不提供批量提交面。判据 P1——写操作对象是工作面内的多个节点 | `canvas` | `dialog`, `drawer` | `pinned` |

> 「无需保存」是**状态结论**（该界面没有保存出口），不属操作教学——它落在 D1 的决策正文里，不另立模式；模式词表（page / panel / drawer / dialog / inline-edit / canvas / command）之外不发明模式名。

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 「立即生效」徽标（加入 / 移出 / 改名逐条落库） | `none` | `needs-approval`：静态结论文案，非数据值；用户 2026-11-22 报障对话确认「工作面顶部常驻说明」方向与文案口径（记录见 `stage.md`） |
| 「次日日报生效」徽标（首义 / 重心） | `none` | `needs-approval`：同上（静态结论文案，非数据值） |
| 「无需保存」徽标（该工作面无全局保存出口） | `none` | `needs-approval`：同上（静态结论文案，非数据值） |

三行取 `none` 均按 `interaction.md`「取 none 者 MUST 标 needs-approval 并在对话中获用户确认」处理；**用户已于 2026-11-22 明确确认三枚徽标定稿**（对话原话：「就这三枚」），确认记录见 `stage.md`「用户拍板口径」节与 `ui-verification/report.md`（A-1 实测三枚徽标逐字渲染）。

## 明确不做

- 不在修正工作面加入任何「保存」按钮或全局提交入口。
- 不把生效时机说明写成段落 / 说明句 / 教学语。
- 不改写加入 / 移出 / 定首义 / 改名的写路径与提示文案。
- 不动新建泳道模式的页脚「起名并创建」出口。
- 不改首义范围面板的「用选中的 N 个节点定首义」按钮。
- 不动左栏面板头既有徽标（`LaneMemberList` / `LaneAnchorScope`）：那两处是分块作用域，与顶部全局摘要分工不重复（本次不扩范围去重）。

## Prototype

无需原型（minor 档：复用既有标题行 + 既有徽标样式族，无新界面结构、无新交互模式）。

## Acceptance

- 组件测试：`front/app/features/tags/components/console/BoardComposeOverlay.correction.test.ts` —— 修正模式渲染顶部徽标组且含三枚徽标文案；底部页脚不再渲染；新建模式不渲染徽标组。
- 命令：`cd front && pnpm test:unit app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → 全绿。
- 视觉：`1440×900` 与 `1920×1080` 两档下顶部徽标组不被裁切、标题不被挤压（agent-browser 截图核）。
