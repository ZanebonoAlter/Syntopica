## Context

`LaneCorrectionCanvas.vue` 的画布宽度 = 容器宽（`vbWidth`），列宽 = 均分
`(vbWidth − axisX0 − RIGHT_PAD) / dates.length`。30 天档得 ≈24px，而两行标签
≈116px。SVG 缩放是 `:width="vbWidth * zoom"` + 固定 `viewBox` 的等比拉伸，
列宽与标签同比放大，比值恒定。

## Decisions

### D1 列宽下限 + 横向滚动（而非继续压缩）

`MIN_COLUMN_WIDTH = 64`，`columnWidth = max(64, 均分)`，`vbWidth = max(视口宽, axisX0 + 列宽×天数 + RIGHT_PAD)`。

- **落点**：`app/features/tags/components/console/LaneCorrectionCanvas.vue` 的
  `MIN_COLUMN_WIDTH` / `columnWidth` / `vbWidth`；验收落到 Scenario「天数多时撑宽画布而非压缩列宽」。
- **取舍**：被否方案——保持均分不设下限。否决理由：均分到 24px 时标签无论如何都
  放不下，只能靠大面积丢弃标签（信息全丢）或放任压字（原缺陷）。
- **取舍**：被否方案——靠用户滚轮放大解决。否决理由：等比拉伸下列宽与标签同比
  变化，比值不变，放大不改变重叠事实。

### D2 标签每行字数按列宽推导（而非固定 10 字）

`labelPerLine = clamp(2, 10, floor((columnWidth − 6) / LABEL_FONT_SIZE))`，
`wrapTitle(text, labelPerLine)`。

- **落点**：`labelPerLine` computed + `wrapTitle` 的第二参数；验收落到 Scenario
  「标签宽度不超过所在列宽」。
- **取舍**：被否方案——只做 label↔圆碰撞检测、保留 116px 标签。否决理由：30 天档
  列宽 64px 时几乎每个标签都会与邻列圆冲突 → 被"宁缺勿叠"大面积丢弃（可能只剩
  1/5 标签），用户会认为"标题没了"，比短标签更糟。
- **取舍**：被否方案——固定 4 字/行（更保险）。否决理由：7 天档列宽可达 180px，
  固定 4 字浪费空间；动态推导让"列宽变大 = 显示更多字"成立，正是用户要的放大收益。

### D3 保留 label↔圆碰撞检测作兜底

即便 D2 让标签宽度 ≤ 列宽，仍保留与其它节点圆的相交检测（跳过）。

- **落点**：`labels` computed 的 `circles` + `circleHit` 分支；验收落到 Scenario
  「标签不压其它列节点圆」。
- **取舍**：无备选——D2 已保证标签 ≤ 列宽，硬检测只做浮点/字体度量的兜底。注：`Math.max(2, …)` 是**不可达**分支（`columnWidth ≥ 64` → 至少 5 字），不构成 D2 的保证来源。

## Risks

- 30 天档出现横向滚动是**用户可见的行为变化**（此前无滚动条）。已在 `ui-design.md`
  Layout Contract 与 proposal Impact 显式记录。
- 标签变短会减少单屏信息量；放大/切近档可恢复，属刻意的取舍。
