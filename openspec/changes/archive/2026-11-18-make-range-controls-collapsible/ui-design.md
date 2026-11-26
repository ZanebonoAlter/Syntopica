# ui-design · make-range-controls-collapsible

- ui-impact: **minor**（组件内新增折叠态，复用既有控件与视觉 token）
- 参考契约：`docs/reference/standard/frontend/interaction.md`（交互模式）、`layout.md`（窄视口断点 767.98px）

## Interaction Decisions

- 折叠开关为 `inline-edit` 级轻交互：**整行可点**（不是单独小图标），`aria-expanded` 表达状态。
- 展开/收起**不弹层**：原地展开，避免遮住画布。
- 窄屏断点沿用 `useIsNarrowViewport()`（767.98px，与 Tailwind md 对齐）。

## Layout Contract

- 布局模式：使用处既有（矩阵页 `workspace`、泳道工作面 `workspace`），本次不改模式。
- 折叠态高度：单行 ≈32–36px；展开态 ≈260–300px（既有）。
- 窄屏（<768px）默认收起；宽屏（≥768px）默认展开。**宽屏既有布局零变化**是回归红线。
- 折叠态摘要单行截断，不换行、不溢出。

## 受影响状态

| 状态 | 是否受影响 | 变化 |
| --- | --- | --- |
| 宽屏展开（默认） | 否 | 与现状一致（标题行由 `<h3>` 改为可点按钮，视觉不变） |
| 宽屏收起（手动） | 是 | 新增：折叠为一行摘要 |
| 窄屏（<768px） | **是** | 默认收起源；画布可视高度 +220–260px |
| 数据未就绪（disabled） | 是（轻微） | 摘要显示「数据未就绪」，展开后仍是既有空态 |
| 拖动刷选中 | 否 | 折叠用 `v-show`，DOM 不重建，手势状态不丢 |
| 展开态折叠开关 | 是 | 由裸 ▾ 图标改为**显式「收起」按钮**（带边框、右对齐）——实测裸图标被误认为分隔符，零点击 |
| 图下操作说明 | 是 | **删除**「日期轴 · 拖动选区…」一行（与工具行为重复，占 20px） |
| 缩放图标（`RangeDateForm`） | 是 | `放大 ＋`/`缩小 −` 小字 → 20px 放大镜 SVG（含 `+`/`−`），`aria-label`/`title` 保留 |
| 画布 zoom 控件 | 是 | `－/＋` 由 11px 字提到 17px 符号 + 26×26 点击区，hover 变主题色 |

## Data Provenance

摘要中的档位来自 `sourceDays` prop、起止日来自 `range` prop（均为上层真实受控值），
无新增数据源、无占位内容。

## Acceptance

- **组件测试映射**：`front/app/features/tags/components/console/ObservationRangeControls.test.ts`
  - 「折叠后只留一行摘要」→ 验证 Scenario「折叠后只留一行摘要」
  - 「窄屏默认收起、宽屏默认展开」→ 验证 Scenario「窄屏默认收起、宽屏默认展开」
  - 「手动选择覆盖默认并持久化」→ 验证 Scenario「手动选择覆盖默认并持久化」
  - 「折叠不触发范围变更」→ 验证 Scenario「折叠不触发范围变更」
