# Review：fix-observation-pool-curve

> 审查方式：派只读审查子线程（`change-review-glm`，glm-5.3 max）按《开发执行规范》§0.6「review 执行要点」执行，brief 见 `/tmp/fix-observation-pool-curve-review-brief.md`（含审查维度与输出格式）。
> 审查范围：change 制品（proposal/ui-design/specs delta/tasks/test-cases）+ 本次实现 diff（`useObservationWindow.ts`、`BoardConsoleStage.vue`、`consoleStageTypes.ts`、`consoleStageBinding.ts`、`BoardConsoleShell.vue` + 6 个测试文件）+ `docs/reference/flow/semantic-board.md` 文档改动。

## 总评

实现与 proposal/tasks/spec delta 的落点一致；预取触发线在请求风暴、365 档共用、epoch 隔离、切板块复位四个风险面均闭环，测试实跑全绿。审查发现 1 处 Medium 回归（本次放宽提示门控引入的 365 档残留提示/死重试键）、1 处 Medium 账实不符、2 处 Low（恒真断言、无锚点变体未锁）。四条已全部处置，见下。

### R-1 2026-11-02【Medium】365 档下残留池失败提示且重试按钮变死键

- **状态**：已修复
- **位置**：`front/app/features/tags/composables/useObservationWindow.ts`（`onTierSwitch`）、`front/app/features/tags/components/console/BoardConsoleStage.vue`（池提示条件）
- **问题**：14 档下池首取失败 → `poolError` 置位 → 用户切「近 1 年」档；`onTierSwitch` 不清池失败态，365 档下 `ensurePool` 又直接早退，于是提示条持续误述「365 日历史池未取到，历史日显示 ?」（实际整年数据已由 A 覆盖），且「重试历史池」按钮点了没反应（`retryPool` 同样早退）。旧实现该提示被 `needsPool` 门控（365 档恒 false）恰好屏蔽此路径，本次去掉门控后暴露。
- **处置**：`onTierSwitch(days === POOL_DAYS)` 时清 `poolError` / `poolStaleNotice`（365 档即池，没有独立池可失败）；舞台提示条件同时补 `sourceDays !== POOL_DAYS` 双保险。delta spec 与 flow 文档同步补该例外口径。新增两层用例：composable 层「池失败后切 365 档 → 失败态清空」、DOM 层「切近 1 年 → 提示条消失」。

### R-2 2026-11-02【Medium】tasks.md 归档账实不符（5.7/5.11 声称已完成但 review.md/归档尚未发生）

- **状态**：已修复
- **位置**：`openspec/changes/fix-observation-pool-curve/tasks.md`（5.7 / 5.11）
- **问题**：5.7 声称 review.md 已建账且脚本退出 0、5.11 声称已归档并同步主 spec，但当时 review.md 不存在、change 未归档、主 spec 仍是旧文本——属「说了但没做」的口径漂移。
- **处置**：本文件即 review.md 建账（复跑 `review-trace.sh` 退出 0）；tasks.md 的 5.11 改为**未勾选**状态，待 `openspec archive` 真实执行后再勾选并复跑 `archive-readiness.sh`（归档后再回填勾选，不预支）。

### R-3 2026-11-02【Low】OX-10 中一处恒真断言（自比自），名单不变性义务实际未被断言

- **状态**：已修复
- **位置**：`front/app/features/tags/composables/useBoardConsole.observation.test.ts`（OX-10）
- **问题**：`expect(rosterSignature(state)).toEqual(rosterSignature({ ...state, rows: state.rows }))` 两侧读同一 ref，恒等成立、永不失败；「迟到池响应不改名单」这一义务实为零覆盖。
- **处置**：改为真实快照对比——切板块前捕获 `rosterBeforeSwitch`，旧板块迟到响应回放后断言名单与该快照逐项一致。

### R-4 2026-11-02【Low】V-8（锚点未知 → 不发池请求）无测试锚定

- **状态**：已修复
- **位置**：`front/app/features/tags/composables/useBoardConsole.observation.test.ts`
- **问题**：机制已实现（预取 watcher 的 `if (!anchor || !window) return`），但全测试集无「A 成功而锚点未知 → 无 365 调用」断言，test-cases.md 的 V-8 属于「写了但没锁住」。
- **处置**：补用例——不注入 reportAnchor、A 返回 `has_reports:false / lanes:[]`（`latestDataDate=null`）→ 断言 `anchorDate` 为 null 且请求序列不含 365。

## 核实记录（审查子线程）

1. 精读 `useObservationWindow.ts` / `useBoardConsole.ts` / `laneDynamicsChannels.ts` / `BoardConsoleStage.vue` / `consoleStageTypes.ts` / `consoleStageBinding.ts` / `BoardConsoleShell.vue` / `useConsoleSelection.ts` / `laneConsoleView.ts` 全文：预取 watcher 为 `watch([anchorDate, sourceWindow])`，A 到位与锚点后到两种时序均可达；`ensurePool` 守卫链 = 365 档早退 → 在途防重 → 非 force 时 `poolWindow/poolError` 短路（失败后不经 watcher/needsPool 自动重发）；365 档与池共用 `channelStore` 同一键不二次发；切板块 `resetForBoard` + `invalidateBoard` 双保险。
2. `grep needsPool front/app`：舞台 props / Shell / Binding / Types 已无残留，仅观察窗状态与测试引用。
3. `grep "跨出档窗|按需读取|按需拉池"`：无旧口径写死（除已更新的注释与 `ObservationMatrix.vue` 的 footnote 语义）。
4. `poolSeries` 口径核对：仍是 `buildPoolSeries(bounds, A名单ID, resolveLaneDay)`（unavailable→null 不补 0、断档→已知 0、部分未知只计已知）。
5. 实跑：`useBoardConsole.observation.test.ts` 24 例全绿；`useConsoleSelection.test.ts` + `BoardConsoleShellSelection.test.ts` + `useBoardConsole.test.ts` 41 例全绿。
6. 实机复核（agent-browser，静态托管 `http://localhost:5100/tags/1980`，未做任何拖动）：brush `polyline.orc-line` 覆盖 x=0..1000（全 365 天轴，365 个点、单一连续段，选区 x=964.3..1000 = 右端 14 天），历史区段 351 点中 113 点在基线之上、26 个不同 y 值——首屏即有真实历史曲线（修复前历史区段为空、曲线只覆盖右端 14 天）。截图：`/tmp/fix-observation-pool-curve/curve-first-paint.png`（本地留存，不入 git）。
