## 1. 列宽下限与画布撑宽

- [x] 1.1 引入 `MIN_COLUMN_WIDTH = 64`，`columnWidth = max(64, 均分)`，`vbWidth = max(视口宽, axisX0 + 列宽×天数 + RIGHT_PAD)`（`LaneCorrectionCanvas.vue`）
- [x] 1.2 确认无循环依赖：`axisX0 ← props`、`columnWidth ← containerWidth + axisX0`、`vbWidth ← axisX0 + columnWidth`

## 2. 标签按列宽收窄

- [x] 2.1 `LABEL_PER_LINE` 改为 `LABEL_PER_LINE_MAX = 10`，新增 `LABEL_FONT_SIZE = 11`（与 `.lcc-label` 字号同源）
- [x] 2.2 新增 `labelPerLine` computed：`clamp(2, 10, floor((columnWidth − 6) / LABEL_FONT_SIZE))`
- [x] 2.3 `wrapTitle(text, perLine)` 加动态每行字数参数，调用处传 `labelPerLine.value`
- [x] 2.4 `estimateTextWidth` 的字号改用 `LABEL_FONT_SIZE`

## 3. 标签与节点圆碰撞兜底

- [x] 3.1 `labels` computed 内构建 `circles`（全节点圆）并增加 `circleHit` 分支（排除自身）
- [x] 3.2 更新 `labels` 注释，删去「已知边界（review R-5）」段改为实际行为说明

## 4. 用例

- [x] 4.1 `4.6 列宽有下限：天数多时撑宽画布横向滚动` —— 断言 7 天档 `svgWidth ≈ 900`、30 天档 `> 900` 且 `col30 ∈ [64, 70)`
- [x] 4.2 `4.7 标签每行字数按列宽收窄` —— 断言 30 天档每个 `<text class="lcc-label">` 内容 ≤ 5 字



## 5. 测试

<!-- doc-impact: none(纯前端画布横向布局参数修正，不改 flow 业务口径、不改接口字段；行为约束已写入本 change 的 delta spec) -->

- [x] 5.1 `pnpm test:unit app/features/tags/components/console/LaneCorrectionCanvas.test.ts --maxWorkers=2` → **32 passed**
- [x] 5.2 `pnpm exec nuxi typecheck` → exit 0（仅 DEP0205 噪音）
- [x] 5.3 `pnpm exec eslint LaneCorrectionCanvas.vue --no-warn-ignored` → 0 error

## 6. 验证

- [x] 6.1 `bash scripts/dev/deploy-frontend.sh` 构建并铺盘（产物 13:43），后端重启后 `/health` 200
- [x] 6.2 用户实机确认横向方案通过（「可以」），并据此另开 `make-range-controls-collapsible` 处理时间视窗占位

| Scenario | 测试文件 |
| --- | --- |
| 天数多时撑宽画布而非压缩列宽 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 标签宽度不超过所在列宽 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 标签不压其它列节点圆 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 放大不改变横向拥挤比值 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |

## 7. 文档与收尾

- [x] 6.1 `ui-design.md` Layout Contract 记录横向溢出策略与 1:1 绘制的兼容说明
- [ ] 7.1 归档前跑 `archive-readiness.sh fix-lane-label-horizontal-fit` 六项全绿
- [ ] 7.2 review 建账（review.md）
- [ ] 7.3 归档

## 8. 影响说明

- [ ] 8.1 「部署后影响 + 需要的操作」章节写入交付汇报：30 天以上档出现横向滚动条、标签变短、放大可恢复

## 9. 无 flow 影响

本 change 只调前端画布横向布局参数，不改业务规则、接口或数据结构，无 flow 文档需同步。
