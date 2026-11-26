<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

<!--
  minor 复用契约。2026-11-24 重写：原稿基于已归档的 lane-manual-correction-loop（0/60 未开工）
  作前置假设，与现状不符。当前事实（摸底证据）：
    - lane-manual-correction-loop 已于 2026-11-16 归档，非「未开工」；
    - 之后叠了 unify-lane-workbench（11-17 三栏）、replace-lane-lifeline-with-topic-events（11-19）、
      rework-lane-compose-swimlane-canvas（11-21 泳道行画布+拖拽）、lane-compose-topic-event-tags（11-23）、
      clarify-lane-correction-effect-timing（11-23）；
    - 「编辑泳道」与「新建泳道」是同一个全屏模态 BoardComposeOverlay.vue（979 行）的两种模式，
      根容器是 Teleport > div.bco（position:fixed; inset:0; z-index:1000），不是 AppDialog；
    - 修正模式 = 三栏：左 aside.bco__left（LaneMemberList + LaneAnchorScope）/ 中 LaneCorrectionCanvas /
      右 aside.bco__right-correction（LaneRecommendationList）。
  本 change 只在修正模式左栏追加一个配置块，不改布局模式、不改导航、不新增入口、不新增页面。
-->

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
|---|---|---|---|---|
| D1 | 辅助标签约束的增删改入口 | `panel` | `dialog`, `drawer` | `open` |
| D2 | 单条约束的移除 | `inline-edit` | `dialog` | `open` |

- **D1 依据 P4**（需在保留泳道上下文前提下编辑多条约束）：约束与首义同属「人工改写泳道定义面」，用户在配约束时要能同时看见左栏成员与首义范围、中间画布——用 `dialog` 会遮住上下文并退化为串行弹窗（P5 同时违反）。故固定为修正工作面左栏的 `panel` 块。
- **D2 依据 P2**（改单个字段且需看见上下文）：移除一条约束是单点动作，就地生效，不开弹窗。
- **与既有 pinned 台账不冲突**：`interaction.md` 的 D1「泳道**内容**的增删改入口 = `canvas`」约束的是**成员集合**（节点选入/移出），本 change 改的是**约束规则**，不改成员集合；成员仍只经画布拖拽/左栏勾选/右栏加入三路变更。

## Data Provenance

| 数据项 | source | 说明 |
|---|---|---|
| 约束列表（短词 + 模式 + 模型标识） | `api GET /api/daily-reports/topics/:id/constraints` | 本 change 新增端点 |
| 约束条数徽标 | `派生 约束列表长度` | 前端派生，无独立字段 |
| 生效时机徽标「次日日报生效」 | `none` | 静态结论文案，无数据源 |
| 模型标识（如 `qwen3-embedding`） | `api GET .../constraints` 的 `embedding_model` 字段 | 新增端点字段，让用户知道锚算在哪个向量空间 |
| 「计算中」/「失败可重试」态 | `派生 请求状态机` | 前端派生 |

## 入口与入口变更

- **不新增入口**。约束配置区随既有「修正工作面」进入，入口沿用 `lane-correction-surface` 既有接线表：
  - `E2` L1 矩阵行头「⋯」→ 进入修正工作面
  - `E3` L1 顶栏「板块管理」→「泳道管理」→ 进修正工作面
- **唯一变更点**：修正模式左栏（`aside.bco__left`）在 `LaneAnchorScope` 之后追加一个 `panel` 块。
  - 落点：`front/app/features/tags/components/console/BoardComposeOverlay.vue:481`（`</aside>` 之前）
  - 新组件：`front/app/features/tags/components/console/LaneConstraintScope.vue`（与 `LaneAnchorScope.vue` 同目录、同命名族）
  - 新 composable：`front/app/features/tags/composables/useLaneConstraint.ts`（与 `useLaneCorrection.ts` 同目录）
- **块形态复用 `LaneAnchorScope`**（`LaneAnchorScope.vue:100-105`）：`panel` 容器 + 块头「标题 + 计数徽标 + 结论型生效徽标」。
  - 块头：标题「辅助标签」+ 计数徽标（`0` / `2 条`）+ 生效徽标「次日日报生效」
  - **默认折叠**：左栏已有 4 块（本泳道成员 / 更早成员 / 已移出 / 首义范围），常态再展一块会让左栏在第 5 块独立内滚。块头常驻可点开，展开后才渲染约束行。
- **写操作逐条即时落库**，与修正工作面既有口径一致（加入/移出/定首义/改名均即时），**不提供全局保存按钮**——生效时机由块头徽标承担（`clarify-lane-correction-effect-timing` 已确立该表达方式）。
- **新建模式也提供（草稿模式）**：新建时话题尚不存在，所以该块**只维护本地行集**（不拉数据、不落库），保存时随建话题一并提交（后端同事务写入，见 design D12）；不发 `GET`/`PUT /constraints`。
  - 落点：新建模式左栏底部（`LaneComposeCandidates` 之下），容器 `div.bco__left-compose`（grid 两行：候选链 1fr / 约束块 auto）
  - 与修正模式的块是同一个组件 `LaneConstraintScope`，靠 `draft` prop 切换；块头、徽标、行形态、空态文案完全一致
  - 不放进命名弹窗（`lane-correction-surface`「轻量弹窗只承载单字段操作」），不放右栏（`workshop-lane-compose`「右栏只做三件事」）

## 受影响状态

- **empty**：块头折叠态显示计数徽标 `0`；展开后列表区为空（不渲染空态说明句——「未绑定辅助标签」属状态结论，可作为行内单行文案，但 SHALL NOT 附加「约束只影响吸附、不影响生命周期」这类派生机制解释）。
- **单条 required 陷阱（已实测验证）**：`max_distance` 为空时会取 `ai_settings` 全局默认 0.55，所以**单条 required 真的会拦**。对双边话题而言，只绑「以色列」会变成「只要以色列、反而挡黎巴嫩」——效果与意图相反。界面不得用解说句讲这件事（属于派生机制），由下一条约束的存在本身约束使用方式；本条只在 ui-design 留档提醒实现/评审。
- **loading**：提交一条新约束后，该行进入「计算中」态（短词已显示、锚向量未返回），该行的模式切换与删除禁用。
- **error**：锚向量计算失败 → 行内错误态 + 重试动作，**保留用户已输入的短词**（不静默丢弃）；`role="alert"`。
- **success**：行内展示短词 + 模式徽标 + 模型标识；块头计数徽标同步。
- **长文本**：短词上限 100 字符，输入框内截断，不撑破 240px 窄栏；行内单行省略。
- **窄栏（≤1280px）**：左栏塌至 240px，约束行**纵向堆叠**（短词输入一行 / 模式组一行 / 删除贴行尾），不横向挤压。
- **重复提交**：同话题内重复短词 → 行内提示「该辅助标签已存在」，不产生第二行。
- **新建模式保存失败（2026-11-25 补丁）**：带约束建泳道失败（如后端 503「锚向量算不出来」）→ 命名弹窗内行内提示（`role="alert"`，`data-testid="compose-save-error"`），文案**原样使用后端给的可操作中文原因**（含失败的短词）；弹窗保持打开、已选成员与名字不丢；重开弹窗清空上次错误。此前失败只写进内部 `error` ref，界面上什么都不显示（用户只能从 DevTools 看到英文 sentinel）。

## 复用组件与布局模式

- **布局模式**：不新增，随修正工作面的全屏模态（`Teleport > div.bco`，`z-index:1000`）承载；本 change 不引入新 layout mode、不写自由宽度。
- **基础组件**：`AppInput`（短词输入，`maxlength=100`，配 `input-id` 关联 label）+ `AppButton`（添加 / 删除 / 重试）+ 原生 `<button aria-pressed>`（`required`/`preferred` 两态切换，沿用工具体既有的 segmented 写法 `BoardComposeOverlay.vue:430-441`）。
- **不新造组件族**：块骨架、块头徽标、计数徽标、生效徽标全部复用 `LaneAnchorScope` 的既有样式 token。
- **不暴露 `margin` / `max_distance`**：两者走 `ai_settings` 全局默认（`persistent_topic_constraint_margin_default` / `..._max_distance_default`），UI 只让用户输入短词与模式——用户心智负担优先，阈值标定由后端配置承担。
- **主题与可访问性**：沿用既有主题 token；模式按钮 `aria-pressed`；删除按钮 `aria-label` 含短词（「移除约束 以色列」）；折叠块头 `aria-expanded`。

## 档位判定依据（为何不升 major）

`开发执行规范` §3 将 major 判据列为「新页面/面板/弹窗/导航/多步/信息架构/布局模式」。本 change 逐条对照后保持 **minor**，依据：

| major 判据 | 本 change 情况 |
|---|---|
| 新页面 | 无。仍只在既有修正工作面的全屏模态内 |
| 新面板（信息架构级） | 无。不是在页面级新增面板区，而是在既有 `panel` 族（`LaneAnchorScope`）里追加一个**同形态、同组件族**的可折叠块；块骨架、块头徽标、计数徽标、生效徽标全部复用既有样式 token |
| 新弹窗 / 新尺寸档 | 无。不使用 `AppDialog`，不新增尺寸档 |
| 新导航 | 无。不新增入口，沿用 E2 / E3 |
| 多步流程 | 无。逐条即时落库，无向导、无提交序列 |
| 布局模式变更 | 无。不新增 layout mode、不写自由宽度 |

另：块**默认折叠**，左栏常态高度不因本 change 改变——若展后展开再算「信息架构变化」，那也发生在用户主动交互之后，不是页面结构变更。

## 验收映射

- **组件测试**：`front/app/features/tags/components/console/LaneConstraintScope.test.ts` —— 折叠默认态与 `aria-expanded`、计数徽标、添加后行渲染、`required`/`preferred` 切换（`aria-pressed`）、删除行、计算中禁用、失败保留短词并可重试、重复短词提示、240px 窄栏纵向堆叠不横向溢出；`BoardComposeOverlay.test.ts` —— 带约束建泳道失败时命名弹窗内展示后端原因、成员不丢。
- **后端**：`backend-go/internal/topicgraph/handler/` 约束读写 handler 测试 —— 读取不返回 `term_embedding`、批量替换幂等、非法载荷不部分写入、404 语义。
- **真库效果核对**：绑定「以色列」+「黎巴嫩」两条 `required` 约束到 id=2 后跑一次日报生成，核对 2026-11-12 之前 32 条纯以色列内政 section 是否被拦下，记录触发原因 / 方法 / 量化结果 / 结论，并据此标定 `margin` 全局默认值。
- **视觉/交互**：agent-browser 主链路（E2 进入修正工作面 → 展开「辅助标签」→ 添加「以色列」→ 见行与模式徽标 → 删除）在 `ui-verification/report.md` 建账；两档视口（1440×900 / 1920×1080）各截图一次，另需 1280px 窄栏一次（左栏塌至 240px 的堆叠态）。
- **不适用**：无需 `ui-prototype/`（minor 档；无新交互范式，形态完全复用 `LaneAnchorScope`）。
