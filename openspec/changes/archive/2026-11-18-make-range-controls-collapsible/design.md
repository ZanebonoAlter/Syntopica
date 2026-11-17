## Context

`ObservationRangeControls.vue` 是受控纯 UI（不请求 API、不写 router）。它被矩阵页与
泳道工作面（两处）复用。整块高度约 260–300px，在窄视口下挤压主体。

## Decisions

### D1 折叠整块为一行摘要（而非只收起图表）

折叠后只渲染 `时间视窗 · <档位> · <起始 MM-DD>~<结束 MM-DD>` 一行；点击整行展开。

- **落点**：`ObservationRangeControls.vue` 的 `collapsed` / `rangeSummary` 与模板
  `orc-toggle`；验收落到 Scenario「折叠后只留一行摘要」。
- **取舍**：被否方案——只收起图表、保留档位按钮（约 90px）。否决理由：窄屏下 90px
  仍是常驻开销，且用户明确选择「只留一行摘要」。

### D2 默认态：窄屏收起、宽屏展开；手动选择优先并持久化

`collapsed = 手动偏好 ?? isNarrow`；手动切换写 `localStorage`。

- **落点**：`collapsePref` ref + `useIsNarrowViewport()` + `syntopica.orc.collapsed`；
  验收落到 Scenario「窄屏默认收起、宽屏默认展开」与「手动选择覆盖默认并持久化」。
- **取舍**：被否方案——总是展开只做手动。否决理由：窄屏用户每次进来都要手点一次。
- **取舍**：被否方案——总是收起。否决理由：宽屏有足够空间，收起反而多一次点击。
- **健壮性**：`localStorage` 只在 `onMounted` 后读（Nuxt 预渲染期无 `window`），
  读失败不影响功能（退回 `isNarrow` 默认）。

### D3 折叠用 `v-show` 保留 DOM

- **落点**：主体容器 `v-show="!collapsed"`；`aria-expanded` / `aria-controls` 绑定。
- **取舍**：被否方案——`v-if` 销毁重建。否决理由：拖动选区过程中若因折叠重建，
  会丢失曲线与手势状态；`v-show` 的 `display:none` 已足够把内部控件移出 Tab 序。

## Risks

- 窄屏默认收起是**可见行为变化**（此前任何视口都完整展开）。用户已确认此取向。
- `localStorage` 键与页面无关（所有使用处共享一次选择），属刻意——同一控件在不同
  页面表现一致更可预测。
