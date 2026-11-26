# fix-daily-report-ke-row-layout 实现任务

修复日报线索行 scope 断链布局错乱，落地 50/50 等宽分栏 + 动作组行底右对齐。根因与方案见 design.md；UI 契约见 ui-design.md（minor，无需原型审批）；用例见 test-cases.md。

## 1. 实现（子线程）

- [x] 1.1 `front/app/features/tags/components/KeyEventMarkerActions.vue`：scoped 样式承接自身根节点摆放——`.ke-detail { grid-column: 2; grid-row: 1 }`（右半列）；`.ke-actions` 改横排全宽底行（`grid-column: 1/-1; grid-row: 2; flex-direction: row; flex-wrap: wrap; justify-content: flex-end; align-items: center; max-width: none`，去 min/max-width 纵排约束）；新增 ≤720px 媒体查询回退（`.ke-detail` r2 全宽、`.ke-actions` r3，注释与宿主断点互指）。
- [x] 1.2 `front/app/features/tags/components/daily-report/DailyReportTopicSection.vue`：`.drm-thread-row` 改 `grid-template-columns: minmax(0, 1fr) minmax(0, 1fr)`；删除两处 scope 断链死规则（`> .ke-detail`、`> .ke-actions` 及 720px 媒体查询内同款），留注释指明摆放规则归属子组件与原因（fragment 根不继承宿主 scope）。不碰模板结构与脚本。

## 2. 测试（用例先行：本节与 1 并行可交叠，全绿为准）

- [x] 2.1 `front/app/features/tags/components/KeyEventMarkerActions.test.ts`：「splits detail from action column」用例断言继承（兄弟块分离/按钮不嵌套/理由不截断），注释改写为新分区语义；新增源码守卫用例（readFileSync 先例）：子组件样式含 `.ke-detail`/`.ke-actions` grid 摆放声明、宿主样式不含 `.drm-thread-row > .ke-detail`/`> .ke-actions` 选择器。
- [x] 2.2 `cd front && pnpm lint && pnpm exec nuxi typecheck`；`pnpm test:unit app/features/tags/components/KeyEventMarkerActions.test.ts app/features/tags/components/daily-report/DailyReportTopicSection.test.ts --maxWorkers=2`（参数不带 `--`）→ 影响面测试全绿。

## 3. Review（子线程聚焦 review，意见 High/Medium/Low 处置后 review.md 建账）

- [x] 3.1 派只读 review 子线程审查制品 + diff，重点：摆放规则归属是否完备（是否残留宿主死规则/断点是否同步）、四状态布局行为、注释锚定是否足以防复发。（review 产出 R-1【Medium】+R-2~R-5【Low】；R-1/R-2/R-3/R-4 当轮修复、R-5 随验收建账闭环，详见 review.md；处置后 44/44 绿）

## 4. 端到端 UI 验收（agent-browser，建账 ui-verification/report.md）

- [x] 4.1 静态 :5100（两轮 deploy-frontend.sh 部署：初版 + R-2 处置后最终构建）验证：suggested 态左右各 50%、理由带分隔线长文本不截断、徽标+按钮横排行底右对齐；confirmed/dismissed/无标记三状态动作位置一致右下；≤720px 单列回退；标记实点因真实建议不可还原未执行，由组件回归兆底（44/44 绿）；逐场景机械量测 + 截图建账 ui-verification/report.md（A-1/A-2/A-4/A-5 pass、A-3 未覆盖→兆底说明）。
- [x] 4.2 `bash scripts/dev/deploy-frontend.sh` 铺静态托管（收尾习惯，别让用户开着旧版）。（2026-10-17 两次：初版验证用 + R-2 处置后终版；NODE_OPTIONS max-old-space-size=2560 避树莓派构建 OOM）

## 5. 文档

<!-- doc-impact: none(纯样式摆放修复，不新增/修订 reference 文档内容；归档时按 §12 补 flow/daily-report.md 变更溯源链接) -->

- [x] 5.1 归档后按 §12 在 `docs/reference/flow/daily-report.md` 变更溯源节补本 change 链接；`docs/reference/standard/frontend/layout.md` 若线程行分区模式有既有记载则同步一句（先 grep 确认辖区，无则不动）。

## 6. 验证（Scenario → 测试文件映射）

| Scenario | 测试文件 |
| --- | --- |
| 建议行左右等宽且动作右下 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |
| 建议行左右等宽且动作右下 | 人工：agent-browser 端到端量测建账 ui-verification/report.md（A-1/A-2/A-4） |
| 摆放规则不因scope断链失效 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |
| 建议只在日报展示 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |
| 总览不承担新选择 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |
| 标记不误展开文章 | front/app/features/tags/components/daily-report/DailyReportTopicSection.test.ts |
| 未知状态与重复提交反馈 | front/app/features/tags/components/KeyEventMarkerActions.test.ts |

映射注记：首行映射断言 detail/actions 兄弟块分离与按钮不嵌套（布局载体结构）；「人工」行为 4.1 浏览器机械量测留痕；继承场景由既有用例回归跑绿（2.2，44/44）。
