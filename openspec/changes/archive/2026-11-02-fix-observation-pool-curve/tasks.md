# Tasks: fix-observation-pool-curve

## 1. 根因与口径（制品）

- [x] 1.1 定位根因：`useObservationWindow` 把 365 池加载挂在 `needsPool`（观察范围跨出档窗）上，首屏观察范围=档默认区间 ⇒ 池从不加载 ⇒ 顶部 brush 曲线里档窗外全是未知日（`null` 不补 0）⇒ 视觉上「只有当前 14 天，拖一下才出历史」。证据：`front/app/features/tags/composables/useObservationWindow.ts`（`needsPool`/`ensurePool`）、`useConsoleSelection.ts`（`poolSeries` → `buildPoolSeries` → `resolveLaneDay`）、归档 design `rework-workshop-into-radar-and-desk/implementation-matrix-stage.md` §G5 末句「要完整背景曲线可早调 `ensurePool()`」（实现漏做）。
- [x] 1.2 口径落 spec：`specs/workshop-board-console/spec.md` 的「观察窗与自由日期显示」Requirement 补两句——① 顶部时间视窗图 SHALL 首屏即取到 365 池（按需拉池只适用于历史日**内容**解析，不适用于常驻背景曲线）；② 池未取到/失败 SHALL 无条件提示并可单独重试；新增 Scenario「首屏曲线即含历史」「池未取到有提示可重试」。
- [x] 1.3 `ui-design.md` 记 `ui-impact: none` 最小 N/A（只改数据到达时机，界面结构/文案/交互零变更）。

## 2. 实现

- [x] 2.1 `front/app/features/tags/composables/useObservationWindow.ts`：新增池预取触发——`watch([anchorDate, sourceWindow])`，A 数据到位且锚点已知即 `void ensurePool()`；幂等沿用 `ensurePool` 既有守卫（缓存命中/在途去重/失败不自动重发）；365 档即池时 `ensurePool` 仍 no-op（不重复发第二次请求）。注释写明「按需拉池只管历史日内容，背景曲线首屏就要整条池」。
- [x] 2.2 `front/app/features/tags/components/console/BoardConsoleStage.vue`：历史池提示条条件由 `needsPool && (poolError || poolStaleNotice)` 放宽为 `poolError || poolStaleNotice`（池失败不再因「观察范围尚未跨出档窗」而静默）；提示文案与「重试历史池」按钮沿用现有。
- [x] 2.3 review R-1 修复：切到 365 档时清掉池失败/陈旧态（`onTierSwitch`）+ 舞台提示条件补 `sourceDays !== POOL_DAYS`（365 档即池，无独立池可失败，提示不适用）；补 composable 与 DOM 两层用例。
- [x] 2.4 状态面清理：舞台 props 去掉随判定失效的 `needsPool`（`consoleStageTypes.ts` / `consoleStageBinding.ts` / `BoardConsoleShell.vue` 三处同步删）；`needsPool` 在观察窗状态里保留（仍表示「观察范围跨出档窗」，且仍是池加载的触发之一）。

## 3. 测试

- [x] 3.1 `front/app/features/tags/composables/useBoardConsole.observation.test.ts`：新增「首屏即预取 365 池」用例（未做任何拖动/切档即含 365 请求 + 池窗口非空 + 档窗外历史日可解析为 `content/pool` + 观察范围/档位不变）。
- [x] 3.2 同文件修正因预取引入的调用序列变化断言（OX-9 深链三处 `[30]/[14]` → `[30,365]/[14,365]`、CA-2 四档缓存序列 → `[14,365,7,30]`、OX-10 迟到池响应不回填、OX-12 池失败改由「首取即失败」构造、切板块池随板块复位改按板块感知 mock 断言）。
- [x] 3.3 `front/app/features/tags/components/console/useConsoleSelection.test.ts`：新增池曲线 series 两条用例——池已就绪时档窗外历史日在 series 里有真实值（6）、档窗内当日仍按 A 口径、池已取到的断档日=已知 0；池未就绪时历史日是 `null`（不补 0、不伪造曲线）。
- [x] 3.4 `front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts`：新增「池首取失败：首屏即提示 + 可重试（不等用户拖出档窗；A 侧照常可读）」DOM 级用例。
- [x] 3.5 review R-3/R-4 修复：OX-10 的自比自恒真断言改为「切板块前名单快照 vs 切板块后名单」真实对比；补 V-8「无锚点不发池请求」用例。
- [x] 3.6 `front/app/features/tags/composables/useBoardConsole.test.ts` / `BoardConsoleShellNavigation.test.ts` / `BoardConsoleShellStates.test.ts`：把「最后一次调用 = 档响应」类断言改为「按 days 过滤的 A 档调用」断言（预取后末位是 365 池请求），语义不变。

## 4. 文档

<!-- doc-impact: flow -->

- [x] 4.1 `docs/reference/flow/semantic-board.md` §泳道动态：把「可见日跨出档窗时才按需读取 365 池（B）」改为「365 池（B）在 A 数据到位且锚点已知即预取（顶部时间视窗背景曲线首屏即需整条池），可见日跨出档窗只是复用已取到的池」；并注明池未取到时历史日显示 `?` 不补 0、提示与重试无条件出现。
- [x] 4.2 `docs/reference/flow/semantic-board.md` §变更溯源：补本 change 一行（指向 `archive/2026-11-02-fix-observation-pool-curve`）。

## 5. 验证

- [x] 5.1 `cd front && pnpm test:unit app/features/tags/components/console app/features/tags/composables/useBoardConsole.observation.test.ts app/features/tags/composables/useBoardConsole.test.ts --maxWorkers=2` → 54 文件 / 733 用例全绿（影响面；树莓派红线不跑全量）
- [x] 5.2 `cd front && pnpm test:unit app/pages/tags --maxWorkers=2` → 1 文件 / 2 用例全绿（宿主页面）
- [x] 5.3 `cd front && pnpm lint` → 退出 0、0 error（8 个 warning 全在无关文件：ArticleCardView.test / useArticleContentView / DailyReportTopicSection.test / detective-wall utils.test / schedulerMeta.test；本增量 0 warning）
- [x] 5.4 `cd front && pnpm exec nuxi typecheck` → 退出 0、无类型错误
- [x] 5.5 `bash scripts/harness/change-scope.sh` → 退出 0（前端影响面已按 5.1/5.2 全量覆盖；本 change 自身无后端改动）
- [x] 5.6 `bash scripts/harness/scenario-trace.sh openspec/changes/fix-observation-pool-curve` → 退出 0（8 个 Scenario 映射齐全）
- [x] 5.7 `bash scripts/harness/review-trace.sh openspec/changes/fix-observation-pool-curve` → 退出 0（review.md 建账）
- [x] 5.8 `bash scripts/harness/ui-verification-trace.sh openspec/changes/fix-observation-pool-curve` → 退出 0（none 档：N/A 一致性）
- [x] 5.9 `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-observation-pool-curve` → 退出 0（声明: flow）
- [x] 5.10 `bash scripts/harness/check-standards.sh --change fix-observation-pool-curve` → A-D/F/G 段零失败
- [x] 5.11 `bash scripts/harness/archive-readiness.sh fix-observation-pool-curve` → 六项全绿（实跑输出「✓ 六项全绿，可以 openspec archive」）→ 已执行归档命令（openspec archive 子命令，--yes）→ 归档为 `2026-11-02-fix-observation-pool-curve`，主 spec `workshop-board-console` 同步（实跑输出「~ 1 modified」）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 拖选不重排 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 精确档回切 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 明确切独立档 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 档池分层失败 | front/app/features/tags/composables/useBoardConsole.observation.test.ts, front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 首屏曲线即含历史 | front/app/features/tags/composables/useBoardConsole.observation.test.ts, front/app/features/tags/components/console/useConsoleSelection.test.ts |
| 池未取到有提示可重试 | front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 池失败后切 365 档不残留误述 | front/app/features/tags/composables/useBoardConsole.observation.test.ts, front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 非法日期反馈 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 深链恢复 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
