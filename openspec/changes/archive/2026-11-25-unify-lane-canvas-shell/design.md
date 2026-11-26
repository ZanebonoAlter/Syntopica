## Context

见 `proposal.md` §Why。以下是决定实现形态的现状事实（均为本 change 动手前的实测）：

**两支画布的关系**：`console/BoardComposeOverlay.vue` 的两个模式各自挂一个画布组件——`mode=create` → `compose/LaneComposeCanvas.vue`（2513 行），`mode=correct` → `console/LaneCorrectionCanvas.vue`（1935 行）。两者已共享纯函数层 `composables/laneComposeLogic.ts`（`clampZoom` / `zoomStep` / `zoomAnchorScroll` / `panScroll` / `isDragBeyond` / `canvasContentPoint` / `pointAppearance` / `relativeProximities` / `scaledPointRadius` / `relatedNodeIds` / `estimateTextWidth` / `titleBudget` / `placeRelatedTitles` 等）与同一套 `.lcc-*` 类名前缀。

**已确认的三处漂移**（不是"未来可能漂移"，是当下已不同）：

| # | 行为 | 编辑侧 | 新建侧 |
|---|---|---|---|
| 1 | 拖拽期间滚轮 | 响应缩放（`onWheel` 无守卫） | 屏蔽缩放（`if (dragging.value) return`） |
| 2 | `Esc` 取消 | 只取消 `dragging` | 取消 `dragging` 或 `isPanning` |
| 3 | 吸顶轴 | 无（分区标签 `x=14`、日期轴 `y=14`，均在内容坐标） | 横滚钉行标签列 + 纵滚钉本次泳道行 |

**新建侧吸顶机制的全部参数**：整组 `<g :transform="rowLabelTransform">`（反向 `translate(scrollLeft/zoom, 0)`）；不透明底 `label-bg { fill: var(--color-bg-base) }`；提档底色 `sticky-tint { fill: var(--color-bg-elevated) }`；6 级右缘阴影（`width: 1`，`x = LABEL_W + band - 1`，`opacity = 0.28 - (band-1)*0.045`，`fill: var(--color-bg-base)`）；全部由 `overflowingX` 门控。该组在新建文件内**内联了 3 份**（只读梯子 / 吸顶本次泳道行 / 普通行）。

**编辑侧几何（决定固定列宽度）**：`LEFT_PAD = 150`，`FOLD_W = 68`，`FOLD_GAP = 8`，`axisX0 = LEFT_PAD + (hasFold ? FOLD_W + FOLD_GAP : 0)`，即 **150 或 226**。折叠带内含首义范围勾选框（`x ≈ 184`），是可点操作入口。

**编辑侧内容溢出是常态**：`MIN_COLUMN_WIDTH = 64`，`vbWidth = max(视口, axisX0 + 天数 × 列宽 + RIGHT_PAD)`；86 天窗口下 ≈ 5682px vs 容器 ≈1900px。

## Goals / Non-Goals

**Goals:**

- 两个画布的**外壳层**（缩放 / 平移 / 容器测量 / 滚动同步 / `Esc`）只有一份实现，且三处已知漂移统一。
- 新建侧吸顶标签列从 3 处内联收敛为 1 个共用单元；编辑侧接入同一次元，补齐两条吸顶轴。
- 外壳重构**以既有用例为回归兜底**，不改变任何缩放/平移/接近度语义。

**Non-Goals:**

- 不统一列宽下限（64 vs 124）、不统一纵向语法（分区 vs 行）、不统一节点标签策略（换行+跳过 vs 截断+整档关闭）。理由见 `proposal.md` §非目标。
- 不改变任何数据口径（候选链、质心推荐、阈值门控、成员归属、首义）。
- 不为编辑侧引入行式布局——`zones` 与 `layoutRows` 保持两套。

## Decisions

### D1. 外壳以 composable 形式抽取，不用 mixin、不抽成组件

- **结论**：新建 `composables/useLaneCanvasShell.ts`，接收宿主 `containerRef` 与若干回调，返回 `containerWidth` / `containerHeight` / `zoom` / `zoomPercent` / `scrollTop` / `scrollLeft` / `scrollTopLogical` / `scrollLeftLogical` / `overflowing` / `overflowingX` / `onWheel` / `applyZoom` / `changeZoom` / `resetZoom` / `measure` / `syncScroll`；宿主只保留各自的业务状态。
- **落点**：`front/app/features/tags/composables/useLaneCanvasShell.ts`
- **取舍**：否掉 mixin——`defineExpose` 与 `props` 泄漏、两组件对外暴露面不同（编辑侧暴露 `measure`，新建侧暴露 `measure` + `debugPointsEpoch`），mixin 会把这些揉在一起。否掉抽成 `<LaneCanvasSurface>` 组件——两个画布的 SVG 内容、`viewBox` 计算、吸顶策略差异太大，包成组件会逼出十几个 slot 与透传 props。

### D2. 外壳把「缩放钳制常量」保持现状，不引入新档位

- **结论**：`LANE_ZOOM_MIN` / `LANE_ZOOM_MAX` / `LANE_ZOOM_DEFAULT` / `zoomStep` 全部沿用 `laneComposeLogic.ts` 现有导出，composable 只做状态编排，不改常量、不加档位、不改锚点换算公式。
- **落点**：`front/app/features/tags/composables/laneComposeLogic.ts`
- **取舍**：无备选：spec「画布缩放不承担解拥挤职责」与「编排画布高度贴合容器」两条约束都建立在现有等比拉伸语义上，换档位会连带改两条既有契约。

### D3. 三处漂移统一为「新建侧口径」

- **结论**：(a) 拖拽期间屏蔽滚轮缩放；(b) `Esc` 同时取消拖拽与平移；(c) 吸顶门控以 `溢出为假则不下发 transform` 为准。全部以新建侧现有行为为准，编辑侧向它对齐。
- **落点**：Scenario「拖拽节点时滚轮不缩放」
- **取舍**：否掉"以编辑侧为准"——编辑侧在拖拽中触发缩放会让拖拽坐标基准中途变化（`canvasContentPoint` 除以 `zoom`），属缺陷而非特性；否掉"保持两侧各自现状"——那正是本 change 要消除的漂移。

### D4. 吸顶单元做成组件，且由它自己算阴影与底色

- **结论**：新增 `components/compose/LaneCanvasStickyRail.vue`，props 为 `width`（列宽）、`overflowing`（是否溢出）、`height`（覆盖高度）、`y`（覆盖起点）、`tint`（是否提档）、`bands`（渐隐级数，默认取 `STICKY_SHADOW_BANDS`）、`offset`（整组反向位移 = `scrollLeftLogical`）；默认 slot 放列内图元。组件内部负责：反向 `translate(offset, 0)`、不透明底 `rect`、可选提档 `rect`、N 级右缘阴影，并把 `STICKY_SHADOW_BANDS` 收到组件内唯一一处。
- **落点**：`front/app/features/tags/components/compose/LaneCanvasStickyRail.vue`
- **取舍**：否掉"只抽 CSS 类、模板仍内联"——新建侧 3 处内联的同构代码里，阴影的 `y`/`height` 各不相同，只抽 CSS 无法消掉重复的 `v-for` 与参数换算。否掉"抽成 render 函数"——需要与行组共处同一 `<g>` 层、共享 `rowLabelTransform`，模板组件更贴合现有写法。否掉"组件自己 inject 外壳拿 `scrollLeftLogical`"——**改为显式 `offset` prop**（实现期补充）：单元 inject 外壳就与具体 composable 绑死，而它应该也能给未接入外壳的画布用。

### D5. 编辑侧固定列宽度取 `axisX0`（动态），不取 `LEFT_PAD`

- **结论**：编辑侧吸顶列宽度 = `axisX0`（150 或 226），随折叠带存在与否变化；`x ∈ [LEFT_PAD, axisX0)` 的折叠带（含首义勾选框）一并纳入吸顶。
- **落点**：Scenario「折叠带存在时固定列不吞掉勾选框」
- **取舍**：否掉固定 150——折叠带勾选框会随横滚消失，那是操作入口不是装饰，消失即退化。接受代价：折叠带出现/消失时固定列宽度变化一次（一次布局变化，不重排内容）。

### D6. 编辑侧纵向吸顶只钉日期轴，不钉分区带

- **结论**：纵向吸顶范围 = 日期轴刻度层（`y ∈ [0, DATE_AXIS_H]`）。分区名随吸顶列钉在左侧（横向），不再纵向钉。不引入"吸顶成员带"。
- **落点**：Scenario「纵滚后日期轴仍可见」
- **取舍**：否掉"连话题带一起钉顶"——编辑画布只有一条泳道，钉顶等于让整块上部常驻，在 993px 视口里会吃掉候选区可视高；新建侧钉本次泳道行是因为那一行是**拖拽落点**，编辑侧不存在需要常驻的落点区。

### D7. 重构分三步落地，每步独立可绿

- **结论**：第一步只抽外壳、两份调用点都接上（行为不变，修三处漂移）；第二步抽吸顶单元、先只让新建侧接（视觉不变，纯内联去重）；第三步编辑侧接入吸顶（唯一有视觉变化的一步）。任一步失败可独立回退。
- **落点**：`tasks.md` 任务 2 / 3 / 4
- **取舍**：否掉"一次改完"——第三步有视觉变化，混在纯重构里会让回归定位失去参照点；`ui-verification` 的对照截图也需要"第二步后 === 第一步后"这个中间基线。

### D8. 吸顶列不新增语义色 token

- **结论**：固定列底色沿用 `--color-bg-base`，提档色沿用 `--color-bg-elevated`，阴影同为 `--color-bg-base`；不新增 token、不改变色。
- **落点**：`front/app/features/tags/components/compose/LaneComposeCanvas.vue`
- **取舍**：否掉"给吸顶列一个专属底色"——吸顶列是**中性面**（表达层序，不承载语义），本仓纪律要求同一颜色不得表达两义；把 `--color-accent` / `--color-event` 挪来当层序色会撞上既有语义。

## Risks / Trade-offs

- **[回归面大] 外壳抽取动的是 4000+ 行组件的初始化路径** → 分三步（D7），第一步以既有 `LaneComposeCanvas` / `LaneCorrectionCanvas` 相关用例全绿为门；`defineExpose` 的对外面（`measure`、`debugPointsEpoch`）在两份调用点逐一核对，不得因抽取丢失。
- **[净减行数不是三步等分] 第一步实测编辑侧只净减 53 行（新建侧 −175）**，因为编辑侧外壳本身只有 ~90 行可删，而第三步还要给它**加**吸顶代码。tasks 2.5 里「两个文件各净减 150–200 行」只在三步合并后成立，单看第一步不成立——验收时不以此单项判红。
- **[吸顶带的通栏底色不属于该单元] 新建侧的 `sticky-bg` / `sticky-tint`（通栏吸顶带底，`LaneComposeCanvas.vue:1438/1451`）与标签列底色是两个元素**：前者铺满 `vbWidth`（盖住纵向滚上来的行），后者只管左侧列。本 change 只把**标签列**抽成单元，通栏层保持内联，不得被误读为漏抽。
- **[TDZ / 声明顺序] 新建侧 `canvasHeight` 依赖 `zoom`，而 `zoom` 声明在其后，现有代码用注释明确记录了「watch 的 source 会立即求值，必须留在 `zoom` 声明之后」** → 抽到 composable 后这个前后关系变成「composable 先返回 `zoom`、宿主再声明 `canvasHeight`」，天然满足；但宿主的 `watch([overflowing, overflowingX, canvasHeight, vbWidth, zoom], ...)` SHALL 保持在同一位置（composable 返回之后），不得上移。
- **[固定列遮挡] 编辑侧吸顶列宽 226px，占 1900px 视口的 12%** → 与新建侧 132px 同性质的既有代价；不溢出时不下发（D3），窄屏本就只出提示、不画工作台。
- **[折叠带宽度跳变] 折叠带出现/消失时固定列从 150 跳到 226** → 接受（D5）；在 `ui-verification/report.md` 记录一次该跳变的对照，若实测明显干扰再考虑给宽度变化加过渡（本 change 不加动画，避免与 `prefers-reduced-motion` 交互）。
- **[既有用例依赖 `data-role` 钩子] 新建侧吸顶已有覆盖（`LaneComposeCanvas.test.ts` 的「本次泳道行吸顶（P-1）」组，含「不溢出时 `row-label-bg` 数量为 0」「`row-label-shadow` 数量 === `row-label-group` 数量」等断言）** → 抽吸顶单元时 MUST 原样保留 `data-role="row-label-group" / "row-label-bg" / "row-label-tint" / "row-label-shadow"` 四个钩子及其数量关系；这是第二步「视觉不变」的机械验证基准，不得为了让组件干净而重命名。
- **[编辑侧 `x=14` 标签进吸顶列后坐标系变化]** 标签原本在内容坐标 `x=14`，纳入反向平移组后必须同步改到「视口坐标 = 14 - scrollLeftLogical」的等价写法（即整组平移后仍画在 14） → 实现时逐条核对三处分区标签与话题带标签的 `x`，避免出现"吸顶后偏移一个滚动量"的双重平移。

## Migration Plan

纯前端重构，无数据迁移、无 API 变更、无配置变更。

1. 第一步落地后先跑既有用例（`pnpm test:unit` 指定受影响文件，树莓派加 `--maxWorkers=2`）+ `pnpm lint` + `pnpm exec nuxi typecheck`。
2. 第二步落地后确认新建画布视觉与第一步后逐像素一致（人工对照截图进 `ui-verification/`）。
3. 第三步落地后按 `ui-design.md` §Acceptance 建账。
4. 回滚策略：三步各自是一个 commit，任一步回退不影响其它两步的收益（第一步的重复消除在第二步之前已完成，第二步的视觉不变可单独留）。
5. 前端改动收尾按仓库习惯跑 `bash scripts/dev/deploy-frontend.sh` 铺静态产物。

## Open Questions

（无——本 change 涉及的选择均已在 D1–D8 收敛；列宽口径统一、节点标签策略统一、纵向语法统一三条已作为非目标显式排除，属后续 change 议题。）
