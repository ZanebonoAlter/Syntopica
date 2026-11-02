# 8C.6 typecheck 修复包报告（本 change 15 条类型错误）

> 线程：全新 implementer、本包唯一 writer；未派子线程、未 commit/归档/部署、未起服务、未换 provider/model/CLI。
> 开工 HEAD（未 commit 混合树）：`f0012930b8aab4b71e994e63acd5a3c65b925896`（develop）。本包零 commit，after SHA 为工作树实算。
> 只记本线程**实跑**证据；`SignalReportView.test.ts` 7 条他域旧债按父令原样保留、逐字未碰。

## 1. 基线与 typecheck 运行次数（透明记账）

| 轮次 | 时机 | 命令（`cd front`） | 结果 |
| --- | --- | --- | --- |
| ① 修前诊断 | 开工后、改动前 | `pnpm exec nuxi typecheck` | **exit 1 / 22 error**，逐条与父清单同文件同 line/col（15 条本 change + 7 条 `SignalReportView.test.ts`） |
| — | 中断前 | `pnpm exec nuxi typecheck`（修后第 1 次启动） | 被会话中断截断，**未取得输出，未采用、不计入结论** |
| ② 修后自证 | 改动完成、全部收尾后 | `pnpm exec nuxi typecheck` | **exit 1 / 7 error，全部在 `SignalReportView.test.ts`**；`components/console/` 命中 **0** 条（见 §4） |

即：有效 typecheck 共 **2 次**（1 次修前诊断 + 1 次修后自证），符合 brief「恰一次自证」的预算；中断的那次无输出、不作为证据。

## 2. 逐条修法（15 条 → 全消）

行号为**修前**实跑行号（与父清单一致）。全部只动类型/签名/导出/mock 字面量。

| # | 文件:行 | 错误 | 修法（旧 → 新） |
| --- | --- | --- | --- |
| 1–4 | `console/useConsoleReading.ts` 89 / 91 / 103 / 112 | TS2551 ×4 `Property 'value' does not exist on type 'string'` | `const boundSelectedDate = shallowRef<Ref<string \| null> \| null>(null)` → `shallowRef<{ current: Ref<string \| null> } \| null>(null)`；读取侧 `boundSelectedDate.value?.value` → `…value?.current.value`（89/91/103）、`bound.value` → `bound.current.value`（112） |
| 5 | 同上 99 | TS2322 `Ref<string \| null>` 不可赋给 `string` | `bindSelectedDate`：`boundSelectedDate.value = selectedDate` → `boundSelectedDate.value = { current: selectedDate }` |
| 6 | `console/useConsoleReadingWiring.ts` 92 | TS2322 `ComputedRef<ReadonlySet<string> \| null>` 不可赋给 `ComputedRef<ReadonlySet<string>>` | 传给 `useOtherTopicSelection` 的 `reportDates,`（三态派生 computed）→ `reportDates: readingBridge.reportDates,`（G1 store 原始 `ComputedRef<ReadonlySet<string>>`；其 deps 声明与文档即「空集 = 索引未就绪」） |
| 7 | `console/BoardConsoleShell.vue` 174 | TS2739 `Ref<boolean>` 缺 `ComputedRef<boolean>` | `useConsoleShellOverlays({ isNarrow, … })` → `isNarrow: computed(() => isNarrow.value),` |
| 8 | 同上 229 | TS2739 `Ref<ConsoleView>` 缺 `ComputedRef<ConsoleView>` | `useConsoleShellActions({ activeView, … })` → `activeView: computed(() => activeView.value),` |
| 9 | 同上 230 | TS2739 `Ref<string \| null>` 缺 `ComputedRef<string \| null>` | 同上 → `latestReportDate: computed(() => latestReportDate.value),` |
| 10 | `console/ObservationMatrixGrid.test.ts` 16 | TS2459 `LaneDayResolution` 本地声明未导出 | 测试**零改动**；在 `console/observationLanes.ts` 补 `export type { LaneDayResolution }`（该类型本就已 import 自 `useObservationWindow`，只增再导出） |
| 11 | `console/OtherTopicReading.signals.test.ts` 182（修 :175 mock 声明） | TS2345 `Promise<ObservationResponse<…>>` 不可赋给 `Promise<{success: true; …}>` | mock 默认实现补显式返回类型：`vi.fn(async () => ({ success: true as const, data: MULTI }))` → `vi.fn(async (): Promise<ObservationResponse<ObservationTimelineData>> => ({ … }))` |
| 12–14 | `console/useConsoleReading.test.ts` 243 / 269 / 323 | TS2345 `{success: boolean}` 不可赋给 `{success: true}` | `gate.resolve(okList(INDEX))` → `gate.resolve({ success: true, data: INDEX })`（`okList()` 返回字面量被放宽成 `boolean`；同文件 216 行已有同形字面量先例） |
| 15 | `console/useConsoleReturnContext.test.ts` 111 | TS2322 `{query: {[x: string]: unknown}, path: string}` 不可赋给 `ConsoleReturnRouteLike` | `import type { LocationQuery, Router }`；`query?: Record<string, unknown>` → `query?: LocationQuery`；`const route = {…}` → `const route: { query: LocationQuery, path: string } = {…}`；`replace` 桩参数 `{ path: string, query: Record<string, unknown> }` → `{ path: string, query: LocationQuery }` |

未使用 `as any` / `@ts-ignore` / `@ts-expect-error`，未放宽被测数据结构（`ObservationMatrixGrid.test.ts`、`SignalReportView.test.ts` 均零改动；全包新增 `as` 为 0）。改动文件内既有的 `router: () => ({ replace, push } as unknown as Router)` 是 G4a 旧代码，非本包引入、未触碰。

### 2.1 与 brief 诊断的两处校正（真因不同、修法仍守原约束）

1. **错误 1–5 的真因不是 deps 的 `() => string` vs Ref `.value` 契约**：`ConsoleReadingDeps.today/anchorDate/axis` 与 `useLaneDayReading`/`useOtherTopicObservation` 的 deps 全是 getter，调用方 `useConsoleReadingWiring` 也按 getter 传（`() => deps.today.value`），与 Shell 真实接线一致，无需改契约。真实原因是 **Vue 3.5.26 `shallowRef` 的分布条件类型**：
   `shallowRef<Ref<T> | null>` 实际解析为 `(Ref<T> | null) | ShallowRef<Ref<T>>`（`node_modules/@vue/reactivity/dist/reactivity.d.ts:463` 的 `T extends Ref` 分布式返回），于是 `.value` 的联合里带 `string`，`?.value` 报 TS2551、setter 报 TS2322。
   修法：持有对象包一层 `{ current: Ref<string|null> }`（不是 Ref 结构 → 不触发该条件分支），运行时仍是同一个被绑定 Ref 实例。
2. **错误 6 的声明侧在 `otherTopicSelection.ts:60`（brief 7 文件写域之外）**：其 `OtherTopicSelectionDeps.reportDates` 写的是 `ComputedRef<ReadonlySet<string>>`，而 G4b 接线传三态。按「brief 写域优先」，本包不改该文件，改为在接线侧传 G1 原始集合——该控制器的依赖文档原文就是「G1 真实报日集合（列头可选性；**空集 = 索引未就绪**，不据此断言无日报）」，null 与空集在其两个消费点等价（见 §3 边界说明）。三态 `null` 在**真正需要区分「未知 vs 已知 0 报日」**的地方全部保留：`useConsoleSelection`（选中日三级校正）、G2 列头 `stageProps.reportDates`、`BoardConsoleStageProps.reportDates`。
   > 若父更希望「向上收宽声明」，等价 follow-up（同 change G4b 文件、类型+一处空态守卫，非本包写域故未做）：`otherTopicSelection.ts` 声明改 `ComputedRef<ReadonlySet<string> | null>` + `selectDayHeader` 加空态 early return。

## 3. 行为不变的依据（逐条：改了什么类型面）

| 改动 | 为什么运行时不变 |
| --- | --- |
| `useConsoleReading.ts` 持有对象包装 | 绑定值仍是**同一个** `selectedDate` Ref 实例（只是放进 `{ current }`）；`watch([boundSelectedDate, () => ….current.value])` 与原来一样：换绑触发一次、绑定后追踪内层 `.value`；`retryReport` / `refreshAfterRebuild` / `selectionMovedAway` 的读值与判定逻辑逐字同构，请求时序（索引 await 后复核、旧日读取前停止）不变 |
| `BoardConsoleShell.vue` 三个 `computed(() => x.value)` | 仅把普通 `Ref` 包成只读 `ComputedRef` 满足下游 deps 声明；取值与下游读取时机（动作触发时 `.value`）不变，DOM/事件/时序/缓存零影响；与同文件既有 `today: computed(() => today.value)` 写法一致 |
| `observationLanes.ts` 再导出 | `export type { … }` 纯类型、编译期擦除，无运行时代码 |
| `OtherTopicReading.signals.test.ts` mock 返回类型注解 | 只约束 mock 的返回类型（`Promise<ObservationResponse<ObservationTimelineData>>`）；mock 行为、resolve 值、时序不变 |
| `useConsoleReading.test.ts` 三处 `gate.resolve` | `okList(INDEX)` 运行时就是 `{ success: true, data: INDEX }`，改写为内联字面量后**对象形状与值完全相同**（只是让字面量在上下文里收窄成 `true`） |
| `useConsoleReturnContext.test.ts` route 桩类型 | `LocationQuery` 是 vue-router 的真实 query 形状（`string \| null \| (… )[]`），测试传的 `{ lane: '11' }` / `{ focus: 'report', lane: '10' }` 本就是该形状；桩对象字段与 `replace` 桩体（`route.path = …; route.query = {…}`）逐字未改 |
| `useConsoleReadingWiring.ts` 传 G1 原始集合 | 见下「唯一语义边界」 |

**唯一语义边界（已核不可达）**：派生三态在「报日集合为空 **且** 索引 loading/error」时返回 `null`，原始集合此时为空集。两个消费点的差别：
- `reportDaysInAxis(axis, null)` → `[]`，空集 → `[]`：**完全相同**（`laneConsoleView.ts:84-89` 本就是 null-safe）；
- `selectDayHeader` 的 `reportDates.value.has(date)`：旧代码在该态会抛 `TypeError`（null 解引用），新代码是空集 → `false`（静默不选）。而该态在**产品路径不可达**：G2 列头与窄屏日期条只在集合内日期渲染原生 `<button>`（`ObservationMatrixDayHeader.vue:73/76`、`BoardNarrowDayStrip.vue:36`），`null` 时一个按钮都没有，因此事件不会触发。即：可达路径行为逐值等价，边界上只消除了一个不可达空指针。

## 4. 验证（Linux 本机串行；参数不带 `--`）

### 4.1 受影响测试：修前 / 修后同清单、例数一致

命令（`cd front`）：
`pnpm test:unit app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/OtherTopicReading.signals.test.ts app/features/tags/components/console/useConsoleReading.test.ts app/features/tags/components/console/useConsoleReturnContext.test.ts app/features/tags/components/console/BoardConsoleShell.test.ts --maxWorkers=2`

| 文件 | 修前（21:05 基线） | 修后（终态） |
| --- | --- | --- |
| `ObservationMatrixGrid.test.ts` | ✓ 9 | ✓ 9 |
| `OtherTopicReading.signals.test.ts` | ✓ 8 | ✓ 8 |
| `useConsoleReading.test.ts` | ✓ 13 | ✓ 13 |
| `useConsoleReturnContext.test.ts` | ✓ 13 | ✓ 13 |
| `BoardConsoleShell.test.ts` | ✓ 10 | ✓ 10 |
| **合计** | **5 files / 53 passed（0 failed / 0 skipped）** | **5 files / 53 passed（0 failed / 0 skipped）** |

不丢例、不新增 skip、不新增用例（本包只改类型，无断言改写）。中断前另有一次修后同清单实跑亦为 53 passed，与终态一致。

### 4.2 改动 `.ts` eslint

`pnpm exec eslint <useConsoleReading.ts、useConsoleReadingWiring.ts、observationLanes.ts、OtherTopicReading.signals.test.ts、useConsoleReading.test.ts、useConsoleReturnContext.test.ts、BoardConsoleShell.vue>` → **exit 0，0 error 0 warning**（`BoardConsoleShell.vue` 被现配置忽略：`File ignored because no matching configuration was supplied`——**不称 Vue lint 绿**，仅报告配置事实）。

### 4.3 typecheck 终态（§1 的 ② 轮）

```
TYPECHECK_EXIT=1
total_errors=7        signal_report_view=7        console_dir=0
app/features/tags/components/SignalReportView.test.ts(545,5): error TS18048: 'thesis' is possibly 'undefined'.
app/features/tags/components/SignalReportView.test.ts(560,12): error TS2532: Object is possibly 'undefined'.
app/features/tags/components/SignalReportView.test.ts(595,12): error TS2532: Object is possibly 'undefined'.
app/features/tags/components/SignalReportView.test.ts(596,12): error TS2532: Object is possibly 'undefined'.
app/features/tags/components/SignalReportView.test.ts(597,12): error TS2532: Object is possibly 'undefined'.
app/features/tags/components/SignalReportView.test.ts(598,12): error TS2532: Object is possibly 'undefined'.
app/features/tags/components/SignalReportView.test.ts(602,12): error TS2532: Object is possibly 'undefined'.
```

- 本 change 15 条**全消**（`components/console/` 命中 0）。
- 他域 7 条**原样**：与修前诊断逐条同文件/同 line/同 col/同 message；该文件磁盘 mtime `2026-10-29 00:58`（早于本线程全部改动），相对路径 `front/app/features/tags/components/SignalReportView.test.ts`，SHA256 `79423fac554a285ba98e245edb32db127807d5f83bc7ed79fbdaf8176fbe1fb2`——未触碰。
- 仍未绿（他域旧债，非本包授权）：这 7 条使 `nuxi typecheck` 退出码仍为 1，需他域线程/父裁决处理。

### 4.4 快照时间与并发写者（不属本包结果，但须知）

- 上述 typecheck 自证对应**本机 2026-10-29 22:20 前后**的工作树快照。
- 同一工作区此时还有**另一个写者**（活跃 change `board-health-triage-advice` 的线程）：`22:24–22:25` 写入 `front/app/api/semanticBoards.ts`、`front/app/api/normalizers/boardHealthTriage(.test).ts`、`front/app/features/tags/components/console/board-health/*`、`BoardWideWorkface.vue/.test.ts`——均**非本包**（本包只写 §5 那 7 个文件，其中 `ObservationMatrixGrid.test.ts` 零改动）。
- 因此：本包结论只能声称「在该快照上 15 条全消、仅剩 7 条他域旧债」；全树 typecheck 的最终绿仍由父在所有写者落定后统一复跑。

## 5. 改动文件 SHA 与尺寸

`before` 为开工实采（本线程改动前），`after` 为工作树终态（`sha256` 全值）。

| 文件（`front/app/features/tags/components/console/`） | before | after | 行/字节 before → after |
| --- | --- | --- | --- |
| `useConsoleReading.ts` | `f570c597ec23a39deec125db833dfdf94283af78a6d9b816b7631ba9ef35ecd8` | `8b164d752cde9443f8a1cb7b0493aaf557f421c32c0109c760be3c39b230ac33` | 184/8818 → **186/9103** |
| `useConsoleReadingWiring.ts` | `70d8f42d12d139677e35b728a7a425965802dbca8e2986950cb8093b72e75d53` | `e56615e92861541378b6d2582d9b1dfff91d62ab2c46aa6a90730c4826b67069` | 200/9403 → **203/9803** |
| `BoardConsoleShell.vue` | `3bc8752d23cdca27a53447ec733c795f702ff0769772f19b1a9bfbc8d0321e0c` | `367d13860293c1b4ee9f0e386aeb0d599fa319847435678cf3bf633ba5130f8e` | 444/14486 → **446/14817** |
| `observationLanes.ts` | `2d5c8b9e7e34648df5e3aeca5ca06cd9fb7d601cc570b3ffea14a0ca19f4326d` | `ac1fbeeae3b27b2e799c02dc8a33774d53abc6bfcc97533356ee1634c3358dac` | 229/9779 → **232/9949** |
| `OtherTopicReading.signals.test.ts` | `563c41912131798ce89089576c378ad96b2ddfab46b1b07d849185b13d6317d7` | `5e90a88fab175a7badaf309a9a00d482bd28fdae728159fae6daa1ad08e000db` | 255/10902 → **258/10974** |
| `useConsoleReading.test.ts` | `77ad266f4077433c8c51955584dc27438a0018fbbdac9361e300464e469f3996` | `de349c27c843c271b2d772195d38198013cceb58a43217af671d064f1e8dd790` | 331/13540 → **331/13591** |
| `useConsoleReturnContext.test.ts` | `de75cdc9bd0f6dca379625ef008792d529428cb958b6bf39776ec5cc8397e2a8` | `d29f2fe6b48d8269acab6df1a0a7e5358a0fc0d99a8efdf7edb63223fb9eba78` | 367/14499 → **372/14753** |
| `ObservationMatrixGrid.test.ts` | `3cbec322243e5770898663741a7152f2093e72cdd5708c64d65e1e3d42d892e3` | **同左（零改动）** | 180/8625 → 180/8625 |

尺寸纪律：全部改动文件均 ≤500 行且 ≤15360B（最紧 `BoardConsoleShell.vue` 14817B、`useConsoleReturnContext.test.ts` 14753B，仍在上限内）。
口径说明：`useConsoleReading.ts` / `useConsoleReadingWiring.ts` / `BoardConsoleShell.vue` / `observationLanes.ts` 的 before 尺寸为**本线程开工实测**；`useConsoleReading.test.ts` 的 before 取 G4b 报告实测值 331/13540，并经本包 delta 复算一致（3 × `okList(INDEX)`→`{ success: true, data: INDEX }` 各 +17B = 13591 ✓）；`OtherTopicReading.signals.test.ts` / `useConsoleReturnContext.test.ts` 的 before 尺寸由「把本包已知 hunk 反向套用到副本」复算（目标文件未动，仅量口径），与 after 实测差值 = 本包增行（+3 / +5）。

## 6. 未做 / 遗留

1. `SignalReportView.test.ts` 7 条他域旧债按令原样保留（文件 mtime 早于本线程、SHA 未变），`nuxi typecheck` 整体退出码仍 1。
2. `useConsoleReadingWiring.ts` 选的是**接线侧直传 G1 原始集合**（brief 7 文件写域约束）；若要改为「声明侧收宽三态」，需动 `otherTopicSelection.ts`（同 change G4b 文件，非本包写域）：声明 `ComputedRef<ReadonlySet<string> | null>` + `selectDayHeader` 空态守卫。两者可核等价，留父裁决。
3. `BoardConsoleShell.vue` 三处包装沿用同文件既有 `computed(() => today.value)` 先例；若父更希望改 `useConsoleShellActions.ts` / `useConsoleShellOverlays.ts` 的 deps 声明为 `Ref<…>`（同样非本包写域），可作后续纯类型 follow-up。
4. 按 brief 未跑：`pnpm build`/`generate`、全量 `test:unit`、浏览器/agent-browser、`pnpm lint`、Go/SQL/DB/AI/业务写、服务起停、部署、commit/归档。本报告不称产品验收或 typecheck 全绿。
5. `tasks.md` 未改动（8C.6 由父统一勾）。

## 7. 一句话结论

本 change 的 **15 条类型错误全部消除**（`components/console/` 0 命中），改动**只落在类型/签名/再导出/mock 字面量**（无 `as any`/`@ts-ignore`、无数据结构放宽、无断言改写、零运行时语义变更）；受影响 5 个测试文件修前修后**同为 53 passed / 0 failed / 0 skipped**；改动 `.ts` eslint exit 0；typecheck 仅剩 `SignalReportView.test.ts` 的 7 条他域旧债且逐字未碰。
