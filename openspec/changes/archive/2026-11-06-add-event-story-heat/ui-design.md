<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约（minor · 复用既有结构）

## 入口与变更

| 入口 | 路径 | 变更 |
|---|---|---|
| 版块态势台 · 观察矩阵 | `/tags/:boardId` → seg「矩阵」 | **格值语义变更**：从「当日该泳道各主题桶事件数的最大值（桶峰值）」改为「当日该泳道下的事件数（事件线数）」；新增**颜色深浅**通道表达「当日最大事件的独立来源数（participant_count）」；图例补一条说明。 |
| 版块态势台 · 演进带 | 同页选中泳道下方 | **节点数字语义变更**：从「当日各桶事件数之和」改为「当日事件数」；节点提示补 participant 数。 |
| 今日雷达 | `/tags` 首屏 | **排序口径变更**：关注度从「分区数加权」改为 `lane_heat`（事件热度的半衰期求和）；分档阈值常量位置（`RADAR_HOT_ATTENTION` / `RADAR_WARM_ATTENTION`）不变，**取值按全量历史 `lane_heat` 的 P80/P20 重新标定**，不得沿用旧 `section` 计数量纲的 50/15（标定依据记 design 附录）。 |
| 今日雷达 · 今天可能升温 | `/tags` 首屏侧栏 | 判据从「当日分区数 ≥ 3 且 ≥ 近3日日均 2 倍」改为「当日事件线数 ≥ 3 且当日 `story_heat` 合计 ≥ 近三日日均 2 倍」；文案与倍数展示不变。 |

**不扩展界面结构**：无新路由、无新弹层、无新交互步骤、无新 seg。格内点击行为保持现状（进入当日工作面），不新增事件列表弹层。

## 受影响状态

| 组件 | loading | empty | error | success |
|---|---|---|---|---|
| 观察矩阵 | 不变（骨架行） | **新增一种降级**：版块有日报但全部事件 `story_id IS NULL` 时，格值按「每事件记 1 个事件」保守计算，并在矩阵 footnote 标「事件身份未回填」；MUST NOT 显示 0 或空态 | 不变（错误条 + 保留旧数据 + stale 提示） | 格值 = 事件线数；颜色 = participant 数分档；title/aria 同时给出事件线数、participant 数、未归组事件数三个可读文本 |
| 演进带节点 | 不变 | 不变（无分区「—」） | 不变 | 节点数字 = 当日事件线数；点径仍按数量分档（阈值随口径同步调整） |
| 雷达行 | 不变 | 不变（今天暂无日报） | 不变（右轨失败不阻塞） | 关注度 = `lane_heat`；分档阈值常量位置不变、取值按 `lane_heat` 分布重标定；行内「当日分区数」字段保留但**改标注为「当日事件线数」**，避免同一数字两个叫法 |
| 雷达「今天可能升温」 | 不变 | 无候选时整块不渲染 | 不变 | 判据与倍数展示不变，分子换 `story_heat` |

**颜色通道约束**：沿用现有主题色板的单一色相，只改不透明度/明度分 3–4 档（0 事件、1–2 来源、3–5 来源、6+ 来源），不引入新色相；必须同时提供带数字的 title/aria，**颜色不得是唯一信息载体**（色觉障碍可读）。

## 复用组件与布局模式

- **布局模式**：`workspace`（版块态势台既有壳，`BoardConsoleShell` → `BoardConsoleStage` → `ObservationMatrix`），不新增 layout mode、不改栏宽与溢出策略。
- **复用组件**：
  - `ObservationMatrixGrid.vue` / `ObservationMatrixLaneRow.vue`：只改格值渲染与新增颜色 class，不改 DOM 结构。
  - `ObservationMatrix.vue`：footnote 文案追加图例与「事件身份未回填」降级说明。
  - `BoardLaneEvolution.vue`：节点数字来源替换，结构不变。
  - `BoardRadarView.vue` / `BoardRadarReadingRail.vue`：关注度数值与标签文案替换，结构不变。
  - `boardRadarHelpers.ts`：分档阈值常量保留在既有位置，不新建常量文件。
- **不新增组件**。若实现中发现必须新增，先在 `ui-design.md` 补记并说明复用为何不成立。

## 验收映射

| 验收点 | 落点（真实文件） |
|---|---|
| 格值按事件线数渲染、图例文案、未回填降级 | `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`、`ObservationMatrix.test.ts` |
| 演进带节点口径 | `front/app/features/tags/components/console/BoardLaneEvolution.test.ts` |
| 雷达关注度分档与「今天可能升温」判据 | `front/app/features/tags/components/console/boardRadarHelpers.test.ts`、`front/app/features/tags/composables/useBoardRadarOverview.test.ts`、`front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 接口字段映射（story_count / participant_count / lane_heat） | `front/app/api/normalizers/boardRadar.test.ts` |
| 颜色通道非唯一信息载体 + 1440×900 / 1920×1080 视觉检查 | agent-browser 主链路断言 + 两档截图，建账 `openspec/changes/add-event-story-heat/ui-verification/report.md` |
