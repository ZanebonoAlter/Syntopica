
## 画布几何实数（行距 52 而非 44）

LaneCorrectionCanvas.vue（路径 front/app/features/tags/components/console/，不是 front/app/components/）：
- :161 MEMBER_BASE_RADIUS = 7.5（spec 写 5.5）
- :194 LABEL_BOX_PAD = 4
- :195 NODE_BLOCK_H = MEMBER_BASE_RADIUS*2 + LABEL_TOP_GAP + LABEL_MAX_LINES*LABEL_LINE_H + LABEL_BOX_PAD
- :376 行距 52
spec（lane-correction-surface:312,324）写「圆心→标签底 34.5 / 得 38.5」「40+4=44px 量级」——44 < 实际占高 48，照 spec 改会复现 spec 自己在 :312 警告的「标签框底与下一圆顶相切」。
漂移源 ad6e1841（同提交改了 spec 其它段却漏这两处；代码头注释 :9-11 也还留 5.5/44）。

**引用**：front/app/features/tags/components/console/LaneCorrectionCanvas.vue:161、front/app/features/tags/components/console/LaneCorrectionCanvas.vue:195、front/app/features/tags/components/console/LaneCorrectionCanvas.vue:376

<!-- pinned 2026-11-26T08:41:25Z -->

## 三栏宽度的窄档覆盖（240/280 @≤1280px）

BoardComposeOverlay.vue（front/app/features/tags/components/console/）：
- :860 宽档 grid-template-columns: 380px minmax(0,1fr) 300px
- :1029-1033 @media (max-width:1279.98px) .bco__workbench { 240px minmax(0,1fr) 280px }，组合选择器同特异性且源码在后 → ≤1280px 覆盖生效
时序：窄档规则 4d79ea5b(11-02) 早于 380/300 定稿 e33821d3(11-17)；spec（lane-correction-surface:14,49）只写 380/300，从未记录断点。

**引用**：front/app/features/tags/components/console/BoardComposeOverlay.vue:860、front/app/features/tags/components/console/BoardComposeOverlay.vue:1030

<!-- pinned 2026-11-26T08:41:25Z -->

## 版块态势页已是三视图（matrix|archive|events）

laneMatrixModel.ts:11 ConsoleView = 'matrix'|'archive'|'events'；BoardConsoleHeader.vue:97/107/116/124 顶栏 4 个按钮。
但 workshop-board-console:14（R14）仍写「两个同级视图」——上一轮 merge-lane-ops-specs 并入 archive 视图时没回头修 R14，与同文件 R26 自相矛盾。

**引用**：front/app/features/tags/components/console/laneMatrixModel.ts:11、front/app/features/tags/components/console/BoardConsoleHeader.vue:97、openspec/specs/workshop-board-console/spec.md:14

<!-- pinned 2026-11-26T08:41:25Z -->

## 两份过期审计的分布

workshop-board-console（1120 行/33 req）：有效 26 / 过期 6 / 无法核实 1 / 缺写 8。
必须修的三条：E-1 两视图→三视图（严重，我上轮造成）；E-2 R2 图例要求「颜色=行内相对强度」但代码图例只有三态文案（ObservationMatrix.vue:132），与同文件 R11「仅一条图例」互斥；E-3 R16「24px 固定步长」实为 Math.min(24, usable/(count-1)) 钳制（KeyEventTimelineView.vue:689-691）。
轻微：R30 多出「id 倒序」档（代码只有质量/时间）；R13「治理抽屉」已退役为「单级宽工作面」、顶栏入口名「上下文」；R26 归档入口在编年头、导航块在工具行上方。

lane-correction-surface（728 行/19 req）：有效 15 / 过期 4。
E-1 三栏宽度缺窄档（严重）；E-2 行距数值全错（严重）；E-3 左栏「四块」实为五块（LaneConstraintScope.vue 辅助标签约束，BoardComposeOverlay.vue:501，spec 出现 5 处「四块」，:161 还是验收条款）；E-4「首义来源三态」实为 source: auto|manual + anchor_origin.resolved 布尔两字段（daily_report_anchor_read.go:22-28）；E-5 :275 整条曲线 Requirement 讲的是新建泳道（属 workshop），修正模式自己的 365 日曲线（useLaneCorrection.ts:735-780）spec 一字未写。

重复（碎片风险）：高度贴合容器 / 横向吸顶标签列 / 缩放滚动外壳 / 窄屏门限 / 批内相对归一 与 workshop-lane-compose:395/510/547 逐字重复；首义读端点零写副作用 与 lane-ops:29 重复。
缺写高价值：左栏第五块约束、修正侧 365 日曲线、质心池含更早成员向量（useLaneCorrection.ts:308-329）、节点卡「↩加回」第三写入口（LaneCorrectionCanvas.vue:1438）、块内生效徽标。

**引用**：openspec/specs/workshop-board-console/spec.md、openspec/specs/lane-correction-surface/spec.md

<!-- pinned 2026-11-26T08:41:25Z -->
