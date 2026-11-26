<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: ui-prototype/states/02-editing.svg -->

# UI 设计契约 · 编排态节点「已确认关键线索」★

> minor：复用既有编排态三栏与画布形态（`rework-lane-compose-swimlane-canvas` 已批准），只追加一个角标与一行明细，不新增布局模式、不新增组件。
>
> **旧原型已作废**：本 change 目录下的 `ui-prototype/index.html` + `screenshots/` 基于旧的"话题带 + 勾选选入"画布（话题带已删），不接受为验收基准、不作为几何基准，保留仅为历史存档。
>
> **当前原型**：`ui-prototype/states/02-editing.svg`（1440×900，以已归档的 `2026-11-23-rework-lane-compose-swimlane-canvas` 编辑中态为几何基底，只叠加 ★ 增量）。

## 入口与入口变更

- 入口不变：版块控制台 →「新建泳道」→ `BoardComposeOverlay` 全屏编排面。
- 变更点（全部为纯展示追加）：
  1. `LaneComposeCanvas.vue` 节点：`section.confirmedKeyEvents` 非空 → 节点主体由圆点**换成星形**（空心星 = 未选，实心星 + ✓ = 已选）；空 → 保持圆点，不追加任何元素。
  2. `LaneComposeCanvas.vue` 节点卡：日期行之后追加「★ 已确认关键线索」区块；单条显示快照标题，多条逐条列出；空 → 整块不渲染。
  3. `LaneComposeCandidates.vue` 左栏成员条目：同一字段 → **日期行右端** ★；组头与计数不变。
  4. `LaneComposeInspector.vue` 右栏②「可补充的事件」条目：同一字段 → **日期行首** ★；不改「＋ 加入」与接近度。
  5. `BoardComposeOverlay.vue` 工具条：**移除**「显示已有泳道（只读）」复选框与「历史待启用话题」按钮（用户 2026-11-23 定档）；只读参照层随之不再出现（`showAssigned` 恒 `false`），历史管理弹窗无入口。
  6. `LaneComposeCanvas.vue` 拖拽：被拖对象（单节点 / 整组）**本体跟随光标**平移，拖拽期间画布**不选中任何文本**；松手回到落位/原位，取消（Esc / pointercancel）清空位移。这是既有拖拽契约的手感修正，不改落位规则。
- 不新增入口、不新增开关、不改任何手势。

## 受影响状态

- **loading**：候选链加载中节点不渲染，★ 随 `confirmedKeyEvents` 一起到达；无独立 loading。
- **empty**：无确认注记的 section / 节点 → 不显示 ★、卡内不出现区块（无占位符、无"暂无"文案）。
- **error**：候选端点失败沿用既有错误呈现；★ 不引入新错误路径（字段缺失归一为 `[]`，等同无标记）。
- **success**：带 ★ 节点在编辑中 / 已确认 / 检索态下均保留（检索态只做透明度降级，不单独隐藏星形）；已选节点 = 实心星 + 中心 ✓（✓ 按星形内切圆缩小，不溢出星形）；相关节点 = 星形加粗描边；焦点环照旧在外。
- **边界**：marker 来源指针为 NULL（脱离快照）→ 该事件无 ★；不在界面上做任何提示或补偿。
- **只读参照层**：本 change 起工具条无「显示已有泳道（只读）」开关，该层在新建模式**永不出现**（`showAssigned` 恒 `false`）；能力保留在组件与数据层，恢复只需加回控件。
- **历史待启用话题**：编排面无入口，弹窗不可达；服务端 `can_activate` 与管理名单数据口径不变。

## 复用组件与布局模式

- layout mode：`workspace`（沿用 `300px / 1fr / 320px`，本 change 不改栏宽与溢出策略）。
- 目标视口：1440×900、1920×1080；窄屏沿用既有 `<768px` 整屏提示。
- 复用：既有画布节点 `g` / 节点卡浮层 / 左栏成员行；不新增全局组件、不新增浮层。
- 星形为节点主体（SVG `<path>`，五角星）：外接半径**沿用该节点的 `pointRadius`**，SHALL NOT 因星形而放大/缩小；缩小时沿用**同一套通用像素下限（≥3px）**，不设星形专属放大下限（放大 = 伪造接近度）。50% 缩放下星形辨识度偏低属已知取舍。
- **拖拽与聚焦**：`pointerdown` 进入拖拽时阻止默认行为，因此**点击节点不再触发浏览器原生聚焦**（键盘 Tab/Enter 路径不受影响；节点卡由 `focusId` 驱动，不依赖 DOM focus）。
- 与既有形状语义不冲突：已选=实心+✓（中心）、相关=加粗描边、检索命中=四角括号、焦点=外环、**★=星形主体**（形状而非附加角标，不额外占地）。
- 与 `ThreadRelatedEvents.vue` 的 ★ 区分：那是"候选线索命中已确认注记"的推荐星；本界面是"该事件下有线索被人工确认"。卡内文案写「已确认关键线索」，不写裸星。

## Prototype

- 路径：`ui-prototype/states/02-editing.svg`（1440×900，深色主题），`file://` 直接打开；PNG 对照片同目录。
- 基底：`openspec/changes/archive/2026-11-23-rework-lane-compose-swimlane-canvas/ui-prototype/states/02-editing.svg`（已批准并已实现的画布形态），未改动其布局与几何。
- 本 change 的**全部增量**（只此四处）：
  1. 本次泳道行 1 个已选节点 → 实心星 + 缩小 ✓（`★` 形状，不是圆点 + 角标）；
  2. 候选链行 2 个未选节点 → 空心星；
  3. 左栏成员条目「美联储官员密集表态 **★**」（真实实现放日期行右端）；
  4. 右栏②条目「**★** 11-14 · 接近度 0.79」；右栏①追加「★ 已确认关键线索：降息预期升温」示例行。
- 未画进原型：hover 节点卡（300px 浮层）内的明细行——其形态在 `interaction-table.md` 与「入口与入口变更」第 2 条写明，实现时按既有卡片骨架追加一行。
- 刻意不做：不重画三态图、不改任何栏宽/行高/日期轴——本 change 不动布局。

## 验收映射

- 组件测试（真实文件）：
  - `front/app/features/tags/components/compose/LaneComposeCanvas.test.ts`：星形替换圆点（`[data-role="key-star"]`）、无标记仍是圆点、外接尺寸与同半径圆点一致、已选实心星 + ✓ 不溢出、卡内逐条列出。
  - `front/app/features/tags/components/console/BoardComposeOverlay.test.ts`：左栏成员条目日期行 ★ 与无标记不渲染；工具条两个入口 `lane-compose-show-assigned` / `lane-compose-history-open` 均不存在，只读参照层不渲染。
  - `front/app/features/tags/components/compose/LaneComposeInspector.test.ts`：右栏②条目日期行 ★，不影响「＋ 加入」。
  - `front/app/api/persistentTopics.test.ts`：`confirmed_key_events` 归一（缺字段→`[]`、thread_id / confirmed_at 可空）。
- 后端测试：`backend-go/internal/topicgraph/handler` 的 compose-candidates 用例（归属 / 空数组 / 只认 confirmed）。
- 视觉：实现后 1440×900 实拍一张带 ★ 的编辑中态，与 `ui-prototype/states/02-editing.svg` 逐项对账（星形替换 / 左栏日期行 ★ / 右栏② 日期行首 ★ / 卡内明细行），写入本 change 的 `ui-verification/report.md`（minor 至少 1 条判定条目）。
