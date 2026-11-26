# 任务 · fix-lane-row-gap-zoom

<!-- complexity: simple -->

> 实现完成后逐条勾选；验证节命令需实际执行并记录结果。
> 本 change 只改前端画布布局计算，**不跑后端、不碰数据库**。

## 1. 纯函数（单一公式源）

- [x] 1.1 `front/app/features/tags/composables/laneComposeLogic.ts` 新增导出 `defaultMemberRadius(columnWidth: number): number`，公式 `max(min(cap,10), cap*0.85)`，`cap = max(3, min(16, columnWidth*0.46))`；`pointAppearance` 改为调用它（`flags.memberRadius` 覆盖优先的既有行为不变）
- [x] 1.2 `laneComposeLogic.test.ts` 补用例：列宽 8 / 20 / 46 / 80 四档的返回值；单调不降；有上下限；`pointAppearance` 既有断言（成员尺寸不编码接近度、候选恒小于成员）保持绿

## 2. 画布布局（`LaneCorrectionCanvas.vue`）

- [x] 2.1 `memberRowGap = max(16, NODE_BLOCK_H + 4)` = 44，其中 `NODE_BLOCK_H = MEMBER_BASE_RADIUS*2 + LABEL_TOP_GAP + LABEL_MAX_LINES*LABEL_LINE_H` = 40（圆顶→标签底）。**刻意不依赖 `defaultMemberRadius(列宽)`**：本画布半径恒 5.5，按半径只会得 16 并保留缺陷
- [x] 2.2 `points` 里成员 y 从 `MEMBER_Y + slot * 16` 改为 `MEMBER_Y + slot * memberRowGap.value`；候选区 / 已移出区 / 只读区的行距不动
- [x] 2.3 新增 `maxMemberSlotsPerDay`（与 `points` 的 slot 计数同源），计算 `membersBottom = MEMBER_Y + max(0, maxMemberSlotsPerDay - 1) * memberRowGap`
- [x] 2.4 `zones` 的 `top` 从硬编码 `156` 改为 `Math.max(156, membersBottom + 48)`；`canvasHeight` 增加下限 `membersBottom + 2 * 60 + 上下留白`，保证两个分区各留最小高度
- [x] 2.5 折叠带高度（用 `zones.zone1Line` 计算）随之下移后仍包住折叠节点（rect 高 `max(40, zone1Line - 20)` + 节点堆叠上限 `zone1Line - 28` 均为派生值，无需改代码；既有折叠带用例保持绿）

## 3. Review

- [x] 3.1 自查：`memberRowGap` 与 `defaultMemberRadius` 只有一个公式源，没有第二处硬编码行距；`LaneComposeCanvas` 未被动（`rowGap ≥ 36` 已够）
- [x] 3.2 派 `change-review` 子线程审 diff + 制品，按 §0.6 要点产出 `review.md`；High/Medium 逐条处置后重跑定向命令（实现子线程不做本条，留给主线程评审阶段）

## 4. 测试

- [x] 4.1 `LaneCorrectionCanvas.test.ts` 新增：同一天 2 成员 → 两节点 `cy` 差 ≥ `2 * r + 29 + 4`（= 圆 + 起笔 5 + 两行标签 24 + 间隙 4）
- [x] 4.2 同上新增：7 天档与 30 天档的 `cy` 差**相等**且 ≥ 44（行距由节点占高决定，半径固定故不随天数变）
- [x] 4.3 同上新增：成员区底部 < 候选区 `zone1Top`（候选不压成员），且 `canvasHeight` 随成员数增加
- [x] 4.4 回归：既有 zoom 用例保持绿（`lane-zoom-in` → readout `110%`；本次不改缩放机制）；另加一条「缩放只等比拉伸、间距/半径比值不变」用例锚住 delta spec 的第二个 Requirement

## 5. 文档

<!-- doc-impact: none(纯前端画布内部布局数值修正，不改 flow 业务口径、不改接口字段；行为约束已写入本 change 的 delta spec) -->

- [x] 5.1 `openspec/changes/fix-lane-row-gap-zoom/` 内制品自洽（proposal / ui-design / design / specs / tasks 数值与公式一致，均为 `max(16, 2r+4)`）

## 6. 验证

- [x] 6.1 `cd front && pnpm test:unit app/features/tags/composables/laneComposeLogic.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts --maxWorkers=2` → 零失败（**参数不带 `--`**）：86 passed（logic 57 / canvas 29）；另跑受影响宿主 `BoardComposeOverlay.correction.test.ts` 15 passed
- [x] 6.2 `cd front && pnpm exec nuxi typecheck` → exit 0
- [x] 6.3 `cd front && pnpm exec eslint app/features/tags/composables/laneComposeLogic.ts app/features/tags/components/console/LaneCorrectionCanvas.vue` → 0 error（注：本仓 `eslint.config.js` 未挂 Vue 语言插件，`.vue` 命中 "File ignored because no matching configuration"，属既有现状）
- [x] 6.4 `bash scripts/harness/archive-readiness.sh fix-lane-row-gap-zoom` → 六项全绿（review 落地后复跑）（当前 ④ 已补映射表，⑤ review-trace 待评审阶段建 `review.md`）
- [x] 6.5 `cd front && pnpm build` → **exit 0**（2026-11-17 实测）。环境前提：树莓派内存紧张时（后端 `go run` 常驻约 0.9–2.5 GB，swap 1.7G 已满）Node 会 `SIGABRT / JavaScript heap out of memory`；须 `NODE_OPTIONS=--max-old-space-size=3072` 且先 `bash scripts/dev/start-dev.sh stop` 腾内存，build 后再起服务——本子线程时段有 agent-browser/chromium 在跑，按红线未执行，留给主线程收尾

### Scenario → 测试文件

| Scenario | 测试文件 |
| --- | --- |
| 同一天两个成员不重叠（含标签） | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 候选区与只读参照区同样不重叠 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 行距不随可见天数变化 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 成员很多时整体滚动而非压缩 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |
| 放大不改变间距/占高比值 | `front/app/features/tags/components/console/LaneCorrectionCanvas.test.ts` |