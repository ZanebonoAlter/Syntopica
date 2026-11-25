## 1. 基线（动手前）

- [x] 1.1 读齐两侧画布外壳代码与既有用例，确认三处漂移与几何常量：`front/app/features/tags/components/console/LaneCorrectionCanvas.vue`（`onWheel` 无拖拽守卫、`onWindowKeydown` 只判 `dragging`、`LEFT_PAD=150`/`FOLD_W=68`/`FOLD_GAP=8`/`axisX0`/`MIN_COLUMN_WIDTH=64`）、`front/app/features/tags/components/compose/LaneComposeCanvas.vue`（`LABEL_W=132`、`rowLabelTransform`、`pinnedTransform`、`STICKY_SHADOW_BANDS`、3 处内联吸顶）。验收：把六项常量与三处漂移逐条对照 design.md §Context 表格，全部对上。
- [x] 1.2 跑基线绿：`cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2`。验收：三文件零失败（这是后续三步的回归基线）。
- [x] 1.3 记录吸顶视觉基线：在编辑画布与新建画布各截一张「横向滚动后」的对照图，存 `openspec/changes/unify-lane-canvas-shell/ui-verification/before/`。验收：两张图可分别看到「编辑侧分区名已滚出视口」「新建侧行标签列仍在左缘」。

## 2. 第一步：抽画布外壳 composable

- [x] 2.1 新建 `front/app/features/tags/composables/useLaneCanvasShell.ts`，收拢 `containerRef`/`containerWidth`/`containerHeight`/`measure`/`ResizeObserver`/`window.resize`/`zoom`/`zoomPercent`/`onWheel`/`applyZoom`/`changeZoom`/`resetZoom`/`scrollTop`/`scrollLeft`/`scrollTopLogical`/`scrollLeftLogical`/`overflowing`/`overflowingX`/`syncScroll`。验收：`pnpm exec nuxi typecheck` 通过，且文件内不出现任何画布业务类型（`LaneCorrectionNode`/`ComposeCandidate` 等）。
- [x] 2.2 新建侧接上 composable：`LaneComposeCanvas.vue` 删除本地外壳实现改调 composable，`defineExpose({ measure, debugPointsEpoch })` 对外面保持不变，`watch([overflowing, overflowingX, canvasHeight, vbWidth, zoom], ...)` 位置保持在 `zoom` 之后。验收：`pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts --maxWorkers=2` 零失败。
- [x] 2.3 编辑侧接上 composable：`LaneCorrectionCanvas.vue` 同样改调，`defineExpose({ measure })` 保持不变。验收：`pnpm test:unit app/features/tags/components/console/LaneCorrectionCanvas.test.ts --maxWorkers=2` 零失败。
- [x] 2.4 统一三处漂移（D3）：`Esc` 同时取消拖拽与平移；拖拽期间屏蔽滚轮缩放。验收：在 `LaneCorrectionCanvas.test.ts` 补两条用例（拖拽中滚轮不改 `zoomPercent`；平移中按 `Esc` 停手后 `scroll` 不再跟随指针）并跑绿。
- [x] 2.5 第一步自检：`cd front && pnpm lint && pnpm exec nuxi typecheck`。验收：零 error、零新增 warning；`git diff --stat` 显示两个画布文件各净减约 150–200 行。

## 3. 第二步：抽吸顶标签列单元（先只给新建侧用）

- [x] 3.1 新建 `front/app/features/tags/components/compose/LaneCanvasStickyRail.vue`：props `width`/`overflowing`/`height`/`y`/`tint`/`bands`，默认 slot 放列内图元；内部渲染整组反向 `translate(scrollLeftLogical, 0)` + `label-bg` + 可选 `sticky-tint` + N 级右缘阴影。验收：组件单测或类型检查通过，且 `STICKY_SHADOW_BANDS` 只在组件内定义一次（`grep` 全仓仅 1 处）。
- [x] 3.2 `LaneComposeCanvas.vue` 三处内联吸顶（只读梯子 / 吸顶本次泳道行 / 普通行）改调该单元，**原样保留** `data-role="row-label-group"` / `"row-label-bg"` / `"row-label-tint"` / `"row-label-shadow"` 四个钩子。验收：`pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts --maxWorkers=2` 零失败——特别是「不溢出时 `row-label-bg` 数量为 0」与「`row-label-shadow` 数量 === `row-label-group` 数量」两条断言仍通过。
- [x] 3.3 第二步视觉对照：重截「新建画布横向滚动后」图，与 1.3 的 after 基线逐像素比对（或逐区域人工比对固定列底色/阴影/文字位置）。验收：`ui-verification/report.md` 记一条「第二步 === 第一步后，视觉无差异」。

## 4. 第三步：编辑侧接入吸顶（唯一有视觉变化的一步）

- [x] 4.1 编辑侧横向吸顶：把话题带标签、分区名（候选区 / 只读参照）、折叠带纳入吸顶列，宽度取 `axisX0`（动态 150 ↔ 226）。**逐条核对三处分区标签与话题带标签的 `x`**：整组平移后仍应画在 `x=14`，不得出现双重平移偏移。验收：新增 `front/app/features/tags/components/console/LaneCorrectionCanvas.sticky.test.ts` 的「横滚后分区名仍可见」「折叠带存在时固定列不吞掉勾选框」两条用例绿。
- [x] 4.2 编辑侧纵向吸顶：日期轴刻度层钉在画布顶部，随横向滚动移动，并被裁切到标签列右侧。验收：同文件「纵滚后日期轴仍可见」用例绿——断言 `scrollTop > 0` 时日期刻度 `y` 仍在 `[0, DATE_AXIS_H]` 且 `x` 随 `scrollLeft` 变化。
- [x] 4.3 溢出门控回归：编辑侧不溢出时不下发 transform、不画固定列底。验收：同文件「不溢出时不出现吸顶痕迹」用例绿（`data-role="row-label-bg"` 计数为 0）。
- [x] 4.4 第三步视觉对照：重截「编辑画布横向滚动后」图，与 1.3 的 before 基线对照。验收：`ui-verification/report.md` 记一条（固定列出现、阴影、无内容遮挡错位、折叠带宽度跳变各一行）。
  - 实拍替代：本批禁止 `pnpm build`、静态托管跑的是旧产物 → 截图不可用，改走 DOM 断言 + `before/` 图对照（原因与对照表见 `ui-verification/report.md` §4 / §5）。
- [x] 4.5 三步合并后全量相关用例：`cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/compose/LaneComposeInspector.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/console/LaneCorrectionCanvas.sticky.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2`。验收：零失败。

## 5. Review

- [x] 5.1 派只读子线程聚焦 review（审查 proposal/design/specs 制品 + 三步 diff），意见按 High/Medium/Low 处置，结论与处置写 `openspec/changes/unify-lane-canvas-shell/review.md`。验收：review.md 存在且每条 High/Medium 都有处置记录（修复或明确不修的理由）。

## 6. 测试

- [x] 6.1 受影响前端用例（按上面四组逐步跑齐）：`cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/compose/LaneComposeInspector.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/console/LaneCorrectionCanvas.sticky.test.ts app/features/tags/components/console/BoardComposeOverlay.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts app/features/tags/composables/laneComposeLogic.test.ts --maxWorkers=2`。验收：全部零失败（本 change 影响包内的红必须修复，不走记账通道）。
- [x] 6.2 后端零改动，不跑 `go test`；若 review 期间误碰后端，则按影响包补跑。验收：`git diff --name-only` 中无 `backend-go/` 路径。

## 7. 文档

<!-- doc-impact: flow -->

- [x] 7.1 更新 `docs/reference/flow/semantic-board.md` §泳道工作面（unify-lane-workbench）：补一节「两支画布共用外壳与吸顶单元」——外壳（缩放/平移/测量/滚动/Esc）同源、吸顶标签列同源、编辑侧两条吸顶轴、固定列宽度随折叠带动态（150 ↔ 226），以及三处已统一的行为（拖拽期屏蔽滚轮、Esc 同时取消拖拽与平移、不溢出不下发）。验收：`grep -n "吸顶" docs/reference/flow/semantic-board.md` 命中新增段落。
- [x] 7.2 在 7.1 新增段落末尾登记 change 名 `unify-lane-canvas-shell`，供 §12.2 变更溯源使用。验收：`grep -c "unify-lane-canvas-shell" docs/reference/flow/semantic-board.md` ≥ 1。
- [ ] 7.3 归档后按 §12.2 在 `docs/reference/flow/semantic-board.md` §变更溯源 补一行链接。验收：`bash scripts/harness/check-standards.sh --change unify-lane-canvas-shell` 的 E 段（归档后跑）零失败。

## 8. 验证

- [x] 8.1 `cd front && pnpm lint` → 期望 exit 0，零 error。
- [x] 8.2 `cd front && pnpm exec nuxi typecheck` → 期望 exit 0。
- [x] 8.3 `cd front && pnpm test:unit app/features/tags/components/compose/LaneComposeCanvas.test.ts app/features/tags/components/console/LaneCorrectionCanvas.test.ts app/features/tags/components/console/LaneCorrectionCanvas.sticky.test.ts app/features/tags/components/console/BoardComposeOverlay.correction.test.ts --maxWorkers=2` → 期望零失败（本 change 影响包）。
- [x] 8.4 构建（**不走裸 `pnpm build`**，树莓派上会 Node OOM；走仓库限载路径）：`bash scripts/dev/deploy-frontend.sh` 内含 `nice -n 19` + `NODE_OPTIONS=--max-old-space-size=3584` + 看门狗 + `pnpm generate`。期望 exit 0、产物落 `backend-go/frontend/`。
- [x] 8.5 `bash scripts/harness/doc-impact.sh verify openspec/changes/unify-lane-canvas-shell` → 期望对账通过（域注释 `flow` 与实际文档编辑一致）。
- [x] 8.6 `bash scripts/harness/check-standards.sh --change unify-lane-canvas-shell` → 期望 A–D + F + G 段零失败。
- [x] 8.7 域外红已记账：若 8.3 遇到非本 change 影响的失败用例，`bash scripts/harness/test-patrol.sh --register <test_id> --context unify-lane-canvas-shell` 登记并在本节写一句确认；本 change 影响包内的红不走记账通道，必须修复。
- [x] 8.8 前端收尾部署：`bash scripts/dev/deploy-frontend.sh` → 期望健康检查通过、页面可打开且编辑泳道横滚后左侧分区名可见。
