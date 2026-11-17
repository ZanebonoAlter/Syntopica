<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计契约 · fix-lane-row-gap-zoom

## 入口与入口变更

**入口不变**：矩阵行头「⋯」→「编辑泳道」→ 泳道修正工作面（`BoardComposeOverlay` 修正模式）→ 中央画布 `LaneCorrectionCanvas`。

本次不改入口、不改三栏栏宽（左 380 / 中画布 / 右 300）、不改任何按钮与文案。

## 受影响状态

| 状态 | 是否受影响 | 说明 |
| --- | --- | --- |
| success（有节点） | **是** | 三个区行距统一为 44px（圆直径 + 起笔 5 + 两行标签 24 + 间隙 4）；每区高度按自身同日最大堆叠数计算；候选区 / 只读参照区随成员区下移；画布总高超出容器时纵向滚动 |
| success（无节点） | 是（轻微） | 无节点时画布最小高由 360 抬到 416（`membersBottom(86) + 330`）；容器高 < 416 时出现纵向滚动条。这是为保证任意一区有节点时不重叠而付出的统一代价 |
| loading | 否 | 骨架屏不画节点 |
| empty | 否 | `emptyNote` 文案位置在画布下方，不受影响 |
| error | 否 | 错误提示为行内文本，不受影响 |
| 窄屏（<768px） | 否 | 工作台整体卸载（既有行为），本次不触及 |

## 复用组件与布局模式

- **复用**：仍为 `LaneCorrectionCanvas`（改造内部布局计算），不新建组件、不改 `AppDialog` 尺寸档、不改 `AppPageShell` 布局模式。
- **布局模式**：`workspace`（既有），画布区继续遵守 layout.md「画布类工作面按容器真实像素 1:1 绘制」——`viewBox` **宽**恒 = 容器 `clientWidth`（缩放靠 SVG 元素尺寸等比实现，实测 `width 900→990`、`viewBox` 宽不变）；**高**随节点堆叠自适应（0/1/4 成员 → 416/456/588），超出容器由滚动承接。
- **溢出策略**：纵向内容变高 → 容器 `overflow: auto` 滚动（既有）。这是刻意取舍：**宁可滚动，不可重叠**。

## 验收映射

| 验收点 | 落点（真实路径） |
| --- | --- |
| 同日两成员不重叠（含标签）：圆心距 ≥ 44px（圆 11 + 起笔 5 + 两行标签 24 + 4） | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` 用例 4.1（读真实 `cy`/`r` 属性） |
| 行距由节点占高决定、不随天数变化（半径固定） | 同上用例 4.2（7 天 vs 30 天档 `cy` 差相等且 ≥ 44） |
| 成员区变高后候选区下移、不压成员 | 同上（断言 `zone1Top > 最大成员 y`） |
| 纯函数边界：`defaultMemberRadius` 在列宽 8/20/46/80 下单调不降且有上下限（供 `pointAppearance` 用；**画布行距不依赖它**） | `front/app/features/tags/composables/laneComposeLogic.test.ts` |
| 滚轮缩放仍等比拉伸（回归，防本次改动破坏缩放） | `LaneCorrectionCanvas.test.ts` 既有 zoom 用例保留（`lane-zoom-in` → readout `110%`，SVG `width × 1.1`） |

不需 agent-browser 验收：本次只改画布内部布局数值，不改交互、不改路由、不改主题；`ui-impact: minor` 按 minor 档不强制可视原型。归档前以组件测试机械断言为证据。
