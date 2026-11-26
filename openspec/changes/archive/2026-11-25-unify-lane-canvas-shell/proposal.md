<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

# unify-lane-canvas-shell

## Why

泳道工作面的两个模式（新建 `LaneComposeCanvas.vue`、编辑 `LaneCorrectionCanvas.vue`）共用同一个全屏弹层与同一份纯函数层（`laneComposeLogic.ts`），但**外壳各写一份**：画布缩放、平移、容器测量、滚轮绑定、`ResizeObserver`、`Esc` 取消六件事在两个组件里各 ~200 行，已开始漂移（编辑侧滚轮在拖拽期间不屏蔽，新建侧屏蔽）。

更实质的是**吸顶能力只有新建侧有**：新建画布横滚时行标签列钉住、纵滚时本次泳道行钉顶；编辑画布的分区标签画在 `x=14`、日期轴画在 `y=14`，都在内容坐标里 —— 横滚 150px 分区名整片飘走，纵滚一屏就不知道自己在看哪天。同一个弹层，一个模式能横滚三屏不迷路，另一个滚一屏就失向。

这不是新发现的结构债：`rework-lane-compose-swimlane-canvas` 的非目标明写「修正工作面本期不动……两工作面语义对齐留作后续 change」，`align-lane-correction-workface` 随后只对齐了**操作手感**（拖拽/ghost/4px 阈值/吞 click/Esc/相对归一/尺寸图例），**画布语法与外壳仍未对齐**。本 change 补的是外壳这一层。

## What Changes

**A. 抽出画布外壳 composable（`useLaneCanvasShell`）**

- 把两个组件各自实现的「缩放（滚轮 + 锚点换算 + 钳制）/ 平移（指针分流 + 4px 阈值）/ 容器测量（`clientWidth`/`clientHeight` + `ResizeObserver` + `window.resize`）/ 滚动位置同步 / `Esc` 捕获」收敛为一个 composable。
- 纯重构：缩放范围（0.5–3×）、步进、锚点换算、平移阈值、生命周期行为**逐项保持现有语义**，不改任何视觉或交互结论。
- 顺带修一处已漂移的行为：**编辑侧滚轮在拖拽期间也响应缩放**（新建侧早已屏蔽），统一为「拖拽期间屏蔽」。

**B. 抽出吸顶标签列能力（`LaneCanvasStickyRail`）**

- 把新建侧现有机制（整组反向 `translate(scrollLeft/zoom, 0)` + 不透明底 + 提档底色 + 6 级渐隐阴影 + `overflowingX` 门控 + 裁切）抽成可复用单元。
- **新建侧自用**：收掉该文件内部已有的 3 处重复实现（只读梯子 / 吸顶本次泳道行 / 普通行各一份 bg + tint + 6 级阴影）。
- **编辑侧接上，补齐两条缺失的吸顶轴**：
  - 横向：分区标签（话题带 / 候选区 / 只读参照）钉在最左；
  - 纵向：日期轴刻度钉在画布顶部。
- **固定列宽度取 `axisX0`（动态）而非 `LEFT_PAD`（常量）**：折叠带（`FOLD_W = 68`，内含首义范围勾选框）落在 `x ∈ [150, 218]`，若按 `LEFT_PAD` 固定则勾选框会随横滚消失 —— 那是可点的操作入口，消失即退化。代价是折叠带出现/消失时左侧固定区宽度变化一次（一次可容忍的布局变化），换操作入口常驻。

**C. 契约补齐（spec 层）**

- `lane-correction-surface` 新增 Requirement：修正画布 SHALL 有两条吸顶轴（横滚钉分区标签列、纵滚钉日期轴），且 SHALL 复用与编排画布同一能力。
- `workshop-lane-compose` 补一条：吸顶标签列的实现 SHALL 是两个画布共用的单元，SHALL NOT 各自维护一份。

## Capabilities

### New Capabilities

（无——本 change 不引入新能力，只把两个既有能力里重复的外壳层收敛，并补齐修正画布缺失的两条吸顶轴。）

### Modified Capabilities

- `lane-correction-surface`：新增「修正画布的吸顶轴」Requirement（横滚钉分区标签列 + 纵滚钉日期轴 + 固定列宽随折叠带动态），修订既有「画布缩放不承担解拥挤职责」的相邻语境（缩放外壳改为共用实现，语义不变）。
- `workshop-lane-compose`：补「吸顶标签列是共用单元」约束（新建侧现有吸顶行为不变，只把实现从 3 处内联收敛到 1 处共用）。

## Impact

**前端（`front/app/`）**

- 新增 `features/tags/composables/useLaneCanvasShell.ts` —— 缩放/平移/测量/滚动/`Esc` 外壳
- 新增 `features/tags/components/compose/LaneCanvasStickyRail.vue`（或 `console/`，最终归位以 design 为准）—— 吸顶标签列（反向 translate + 底 + 阴影 + 门控）
- `features/tags/components/compose/LaneComposeCanvas.vue`：外壳改调 composable、3 处内联吸顶改调共用单元（行为不变）
- `features/tags/components/console/LaneCorrectionCanvas.vue`：外壳改调 composable、新增横向分区标签吸顶 + 纵向日期轴吸顶
- 测试：既有 `LaneComposeCanvas` / `LaneCorrectionCanvas` 相关用例须全绿（外壳重构以既有用例为回归兜底），新增吸顶轴的位置/门控用例

**后端**：无改动。

**文档**

- `docs/reference/flow/semantic-board.md` §泳道工作面：补「两条画布共用外壳与吸顶单元」一节
- `docs/reference/standard/frontend/layout.md`：若吸顶行为构成布局契约（固定列宽度可变），补一句说明

**非目标（明确不做的）**

- **不统一列宽下限**：编辑侧 `MIN_COLUMN_WIDTH = 64`、新建侧 `LANE_MIN_COLUMN = 124` 是两次独立设计为不同目标拍的（编辑侧要一屏读完一条泳道的历史密度，新建侧要读懂每条候选链的名字），本 change 只统一「谁提供机制」，不统一数字。列宽口径显式化留作后续 change。
- **不统一纵向语法**：编辑侧=分区（话题带/候选区/只读参照/更早折叠带）、新建侧=行（本次泳道/候选链/散料地板）。两者回答的是不同问题（「这条泳道的边界在哪」vs「N 条候选链谁值得采纳」），布局函数（`zones` vs `layoutRows`）不同源，**不做合并**。
- **不统一节点标签策略**：编辑侧「两行换行 + 碰撞检测、装不下跳过」vs 新建侧「单行截断 + 列宽 <124 整档关闭」。牵动两侧可读性条款与节点卡信息量，属节点层（L2）议题。
- **不引入缩放新档位或新密度模型**：缩放语义（等比拉伸、不承担解拥挤职责）原样保留。
- **不动任何数据口径**：候选链、质心推荐、阈值门控、首义与成员归属一律不变。
