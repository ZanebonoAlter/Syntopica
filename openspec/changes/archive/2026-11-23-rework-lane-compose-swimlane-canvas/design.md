## Context

见 `proposal.md` — Why。实现侧需要知道的现状：

- 画布是**单个 SVG 组件**（`front/app/features/tags/components/compose/LaneComposeCanvas.vue`，约 1280 行），几何 1:1 于容器像素（`vbWidth = clientWidth`），日期列 `columnWidth = span / dates.length`，同日槽位 `rowGap = max(36, min(110, 360/(maxSlots-1)))`。
- 质心/距离**全部前端算**：`laneComposeLogic.ts` 的 `centroidOf` / `cosineFor` / `centroidRecommendations`；`matchThreshold` 由 `compose-candidates` 响应下发。
- 候选链是服务端唯一口径（`compose-candidates` 的 `chains`），前端只渲染；`bandLanes` 取成员数降序前 6。
- 候选节点池与「已入 active 泳道」的只读参照是同一份 `sections`，靠 `persistentTopic.status` 区分。
- 现有手势只有「空白处按住拖 = 平移（4px 阈值）」与「滚轮缩放 0.5–3×」；点击 = 聚焦钉住。
- 修正模式用**另一个组件** `LaneCorrectionCanvas.vue`，本期不动。

## Goals / Non-Goals

**Goals**

- y 轴从「同日内 id 序号」换成「所属候选链」，使行 = 泳道成立，且不引入任何新的服务端口径。
- 选入手势（拖拽）与查看手势（点击）在同一坐标系里互不干扰，并能与既有平移/缩放共存。
- 阈值门控只影响**散料层**，且「被收起」永远可展开、可计数。
- 检索范围与检索态是**纯前端派生**，不改后端。

**Non-Goals**

- 不改候选链资格、质心公式、阈值来源（`ai_settings` / 接口下发）。
- 不改修正模式画布（`LaneCorrectionCanvas.vue`）。
- 不改右栏的写路径与保存接口。
- 不做触屏拖拽、不做批量选入。

## Decisions

### D1. 行归属 = 候选链，用「链成员集合 → 行序」映射而非重新计算

- **结论**：把 `chains`（服务端返回的候选链，含成员 id 列表）展开成 `rowIndexOf: Map<sectionId, rowIndex>`；行序 = `本次泳道(0) → chains 顺序 → 地板(N+1)`。同一日内节点按 `rowIndexOf` 升序排槽位，**不再按 id**。不在任何链里的节点 → 地板行。
- **落点**：`front/app/features/tags/composables/laneComposeLogic.ts:buildRowIndex`
- **取舍**：否掉「前端按 `persistentTopicId` 自行分组建行」——那会在前端复制候选链口径，与「服务端唯一口径」红线冲突，且跨分量链会被合并。

### D2. 行几何：行高由「当日最大堆叠数」决定，槽距固定 24px

- **结论**：行结构 = `行标签 32px + maxSlots(行内同日最大节点数) × 24px + 行间 26px`；槽内「点 9px + 标题 10px」。单日超过 `MAX_SLOT_PER_DAY = 6` 时封顶，超出部分以 `+N` 折行提示，不再拉高。
- **落点**：`front/app/features/tags/components/compose/LaneComposeCanvas.vue`
- **取舍**：否掉「同日折叠成计数点 `●3`」——会把用户正要区分的那几条藏起来（用户在 design 讨论中明确要求同日多条各自可读）。

### D3. 拖拽与平移共用 pointer 状态机，靠「起始命中目标」分流

- **结论**：`pointerdown` 时判定命中对象：命中**节点或行标签** → 进入 `drag-object` 状态；命中空白 → 进入 `pan` 状态。`drag-object` 状态下位移超过 4px 才真正起拖（保留 tap 语义），起拖后 `setPointerCapture`；拖拽期间屏蔽滚轮缩放与 hover 浮层；`pointerup` 落在本次泳道行区间内 = 选入，落在行外 = 移出/取消。拖拽结束后的第一次 `click` 必须被吞掉（沿用现有 `isDragBeyond` + 250ms 抑制）。
- **落点**：`front/app/features/tags/components/compose/LaneComposeCanvas.vue`
- **取舍**：否掉「拖拽单独一套 pointer 监听」——两套监听会在 pointerdown 阶段互相抢捕获，产生「拖点变成拖画布」的偶发；否掉「用 HTML5 drag&drop」——它无法与 SVG 内缩放坐标系对齐，且不能自定义 ghost。

### D4. 阈值三档 = 散料可见集的过滤，候选链成员走白名单旁路

- **结论**：可见集 = `已选 ∪ 只读参照 ∪ 候选链成员 ∪ {散料 | distance ≤ thresholdOf(tier)}`。三档映射：`tight = matchThreshold × 0.5`、`standard = matchThreshold`、`all = 1`（全部散料）。折叠计数 = `散料总数 − 本次显示的散料数`。阈值切换只重算可见集与右栏批次，不重取数据。
- **落点**：`front/app/features/tags/composables/laneComposeLogic.ts:visibleScatter`
- **取舍**：否掉「阈值同时门控候选链成员」——等于把距离当准入门槛，违反「推荐不得当置信度」；否掉「三档存后端」——阈值是展示密度旋钮，不应产生配置面。

### D5. 检索范围 = 未归类，用「不在任何链成员集合里」派生

- **结论**：`unclassifiedIds = 窗口内 sections − ⋃ chain.memberIds`（不是 `persistentTopicId == null`，否则挂着 candidate 话题但未成链的节点会被漏掉）。检索 = 在这批 id 上按标题 `includes` 过滤；命中集同时驱动画布高亮与右栏列表。检索态期间的 canvas 呈现 = 候选链行与地板降透明度（0.20），本次泳道行不降级。
- **落点**：`front/app/features/tags/components/compose/LaneComposeInspector.vue`
- **取舍**：否掉「按 persistentTopicId 判空」——会把大量未成链节点漏掉；否掉「下沉后端全文检索」——本期范围外（已列为独立 change）。

### D6. 节点标题档：由列宽推导，整档开/关不做碎片降级

- **结论**：`titleVisible = columnWidth >= 40`；显示时截断到 `floor(columnWidth / 9.2)` 字（14 天窗口 ≈5 字），超出加省略号。散料节点不参与（无论列宽）。30 天窗口列宽 ≈23px 即整档关闭。
- **落点**：`front/app/features/tags/components/compose/LaneComposeCanvas.vue`
- **取舍**：否掉「按列宽线性缩短到 2 字」——2 字标题的信息量接近 0，反而制造噪声；否掉「散料也显示标题」——违反母 spec 既有边界（散料读全文靠悬停）。

### D7. 重排性能：可见集变化才重算布局，拖拽只动 ghost

- **结论**：`points`（含坐标与外观）用 `computed` 缓存，依赖 = 可见集 / 窗口 / 容器宽度 / 阈值；拖拽过程中**不重算 `points`**，只更新一个 `dragGhost` 的 x/y（rAF 节流）。目标行高亮用 CSS class 切换，不触发 Vue 重渲染整棵树。
- **落点**：`front/app/features/tags/components/compose/LaneComposeCanvas.vue`
- **取舍**：否掉「拖拽时实时预览落位后的布局」——树莓派上 200+ 节点的重排会掉帧，且预览反而干扰判断；否掉「canvas 2D 重写」——现有 SVG + 命中区方案已满足 1:1 契约，重写风险大于收益。

### D8. 撤销只覆盖「移出」方向，用一次限时操作而非历史栈

- **结论**：维护 `lastRemoval = { id, fromRow, slot }`，移出后 3 秒内可恢复；新的一次移出覆盖上一笔；超时或任何其他成员变更清空。
- **落点**：`front/app/features/tags/composables/useLaneCompose.ts`
- **取舍**：否掉「通用撤销栈」——编排全过程都是会话态、单步可逆（再拖回来），栈的价值低且会与「取消本次清空草稿」语义冲突。

### D9. 本次泳道不参与行归属计算，是恒定的第 0 行

- **结论**：`rowIndexOf` 只从候选链派生；本次泳道成员单独成一个 `Set`，渲染时优先归到第 0 行。因此某链成员全被取走时，该链行自然空掉，由「空行不渲染」规则收掉，**不需要额外的「已采纳」状态**。
- **落点**：`front/app/features/tags/composables/laneComposeLogic.ts:buildRowIndex`
- **取舍**：否掉「把候选链的采纳进度写进行状态」——那会把本次泳道绑到某条链上（用户明确否掉），且引入第二份真值。

## Risks / Trade-offs

- **拖拽命中精度**（点半径 3–9px）→ 拖起后 ghost 放大到 1.6× 跟随指针，视觉反馈弥补抓取误差；`pointerdown` 命中区沿用既有 `r = max(7, r+4)`。
- **平移与拖拽误判** → 用「起始命中对象」在一次 pointerdown 内一次性分流，不做二次猜测；两者共用 4px 阈值。
- **200+ 节点 + 标题常显的渲染开销** → 标题只在 `titleVisible` 时生成 `<text>`；30 天窗口自动关闭标题，节点数与文本数同时下降。
- **`+N` 折行上限** 可能让用户以为「这一天就这么几条」→ 折行标记本身可点开（展开该日全部），且计数常显。
- **检索范围收窄后**，用户可能找不到某个明明存在的候选链成员 → 左栏候选组的事件列表仍可逐条查看，且该链可整组采纳；这属于换取「检索结果干净」的已知代价。
- **修正模式与新建模式手势不一致**（本期已知偏差）→ 在 `design` 里记录为后续 change 的对齐项；两组件无共享手势代码，不会互相污染。

## Migration Plan

- 纯前端改造，**无数据迁移、无后端改动**；回滚 = 回退该组件与 composable。
- 上线后旧操作习惯（点击选入）失效：点击只出卡。因为选入一直是「先点开卡再点按钮」两步，新路径（拖拽）步数不多于旧路径。
- 阈值三档默认 `standard` = 现有 `matchThreshold`，不改变既有默认推荐集。

## Open Questions

- 同日超过 6 条时的 `+N` 展开：是在行内展开（行高临时变高）还是出浮层？——实现时按「行内展开 + 可再次收起」先做，原型已批准的三态图不涉及该组合，不构成审批阻塞。
- 拖拽整组时的 ghost 是「一簇点」还是「一条轮廓」？——原型里画的是整行成员半透明跟随；实现时若性能不足，降级为「行标签跟随 + 该行成员高亮」。
