<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

# Proposal: fix-observation-pool-curve

## Why

叙事工坊（版块态势控制台）矩阵页顶部「时间视窗」的折线缩略图（brush 背景曲线）**第一次打开永远只画得出当前档那 14 天**，历史曲线要等用户手动拖一下选区才出现（2026-11-02 用户实机截图：拖动前曲线只有右端一小段，拖动后历史趋势才补上）。

根因不是渲染，是**取数时机**：顶部图按 spec 是「365 池内已返回泳道的日线索合计」，数据来自 365 日池（`GET /semantic-boards/:id/lane-dynamics?days=365`）。而池的加载被挂在 `needsPool` 上——只有当观察范围**跨出当前档窗**（用户把选区拖到 14 天档之外）才自动拉池（`useObservationWindow.ensurePool`）。首屏观察范围 = 档默认区间（恰在档窗内），`needsPool=false` → 池从不加载 → 曲线里除档窗那几天外全是未知日（`null`，不补 0、不桥接）→ 视觉上就是「只有 14 天」。

这不是「按需加载」的合理取舍，而是**把两件事混在了一根触发线上**：

- 观察范围的**日内容解析**（历史格显示内容还是 `?`）确实该按需拉池（拖出档窗才需要）；
- 顶部**背景曲线**是常驻控件的画布，它要的就是整条 365 池曲线——按需加载让控件首屏残缺，用户第一眼看到的是坏图而不是空图。

归档 design 里其实已经写明了正确做法，只是实现漏了：`rework-workshop-into-radar-and-desk / implementation-matrix-stage.md` §G5「series 派生口径」末句——「组件不自行请求；**要完整背景曲线可早调 `ensurePool()`**」。实现只做了「拖出档窗才调」，没做「首屏就调」。

## What Changes

- **365 池预取（A 数据到位 + 锚点已知即拉）**：`useObservationWindow` 增加一条预取触发——A 档响应到位、且数据锚点（报告锚点或 A 最新日）已知时，立即 `ensurePool()` 拉 365 池，让首屏曲线就是完整历史趋势，不再等用户拖动。幂等由 `ensurePool` 既有守卫兜底（命中缓存/在途去重/失败不自动重发），不产生请求风暴；365 档本身即池（档/池共用一次请求，OX-8）仍不重复发。
- **池失败提示不再被「是否跨出档窗」门控**：历史池未取到/陈旧时，提示条与「重试历史池」入口无条件出现（此前 `needsPool=false` 时失败静默，用户只看到一条残缺曲线）。提示语义不变：A 可读、名单/排序不动、历史日显示 `?` 不补 0。
- **状态面清理**：舞台 props 去掉随此次判定失效的 `needsPool`（该判定的职责已从「是否要显示池失败提示」移走；`needsPool` 仍作为观察窗状态保留，用于「观察范围跨出档窗」的语义与池加载触发）。
- **回归锚**：新增用例锁住「首屏池已取到 + 曲线含档窗外历史日」，并修正既有因调用序列变化而失效的断言（池预取后请求序列从 `[14]` 变 `[14, 365]`）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `workshop-board-console`: 「观察窗与自由日期显示」Requirement 补「顶部时间视窗图 SHALL 在首屏即取到 365 池（不得只画当前档窗）」与「池未取到/失败 SHALL 无条件提示并可单独重试」两句，新增对应 Scenario。

## Impact

- **前端**：
  - `front/app/features/tags/composables/useObservationWindow.ts`（池预取触发 + 注释口径）
  - `front/app/features/tags/components/console/BoardConsoleStage.vue`（池失败提示条件）
  - `front/app/features/tags/components/console/consoleStageTypes.ts`、`consoleStageBinding.ts`、`BoardConsoleShell.vue`（去掉失效的 `needsPool` 舞台 prop）
  - 测试：`front/app/features/tags/composables/useBoardConsole.observation.test.ts`、`front/app/features/tags/components/console/BoardConsoleShell*.test.ts`（调用序列断言修正 + 新增首屏曲线用例）
- **后端 / 数据库 / API**：零改动。复用既有 `GET /semantic-boards/:id/lane-dynamics?days=365`，不改端点、不改参数、不改 SQL、不加索引。
- **部署后用户可见变化**：矩阵页顶部折线缩略图**首屏即显示完整历史趋势**（此前只有当前 14 天，拖动才补历史）；历史格在 365 池取到后可显示真实内容（此前显示 `?`）。板块打开时多一次 `days=365` 读请求（同板块会话内缓存复用，切档/切板块不重复发）。
- **旧数据降级**：不涉及数据变更、无迁移、无需回填、无需重新生成日报；回滚前端构建即可完全还原旧行为。
