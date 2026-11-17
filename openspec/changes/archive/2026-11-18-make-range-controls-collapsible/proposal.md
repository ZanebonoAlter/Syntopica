<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->

## Why

时间视窗（`ObservationRangeControls`）整块约 260–300px 高：标题+档位、日期表单、
两行图注、趋势曲线、操作说明、反馈行。在**小窗口**（窄视口 / 高度紧张）下它吃掉
了泳道画布的可视高度，用户看不到主体内容。

用户已确认期望：**折叠成一行摘要**（「时间视窗 · 近 30 天 · 10-19~11-17」），
**小窗口默认收起、宽屏默认展开**，手动切换后记住选择。

## What Changes

- 标题行改为**可点击的折叠开关**（整行可点，带 ▾/▸ 指示与 `aria-expanded`）。
- 折叠时只保留标题 + **范围摘要**（档位 + 起止日 MM-DD）；档位按钮、日历表单、
  图注、趋势曲线、操作说明、反馈行全部隐藏。
- 默认态：**窄视口（<768px）收起，宽屏展开**（复用 `useIsNarrowViewport`）。
- 手动切换后写 `localStorage`（`syntopica.orc.collapsed`），**手动选择优先于窄屏默认**。
- 折叠不改变任何受控语义：`range`/`sourceDays`/`series` 仍由上层给定，折叠只是
  视觉收起（主体用 `v-show` 保留 DOM，避免重建与焦点丢失）。

## Impact

- 三个使用处（矩阵页 `BoardConsoleStage`、泳道工作面 `BoardComposeOverlay` 两处）
  行为一致：宽屏不变，窄屏默认收起。
- 窄屏收起后，画布可视高度增加约 220–260px。
- 不涉及后端、接口、数据结构。
