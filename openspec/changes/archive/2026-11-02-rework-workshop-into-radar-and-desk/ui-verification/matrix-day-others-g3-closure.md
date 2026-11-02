# G3 准入收口报告（M1 受控重试意图 / M2 阅读终态信号 / S1 内聚拆分）

> 线程：全新短 implementer、本写域唯一 writer（未 resume 旧作者、未派子线程、未 commit/归档/部署、未换 provider/model/CLI）。
> 基线：父复核 `ui-verification/matrix-day-others-g3-review.md`（G3-M1/M2/S1 三项待修）+ 已批准矩阵增量 `cdb0b70e…`。
> 本页只记录本线程**实跑**的证据。旧 38 例历史与相邻 Shell 38 例保留；本页不把新旧数字相加当总数。

## 1. 写域与 before → after（SHA256 实采，新文件 before = 不存在）

| 文件 | before | after | 行数/字节（before → after） | 动作 |
| --- | --- | --- | --- | --- |
| `front/app/api/otherTopicObservation.ts` | `5bb3017d03870e94…` | `f9a20cefd40f6f3a…` | 128/6041 → **129/6273** | 改：`ObservationResponse<T>` alias 统一 `ApiResponse<T>`；产品 load 固定 days=0（签名不再接受窗口 days） |
| `front/app/api/otherTopicObservation.test.ts` | `160045488263919a…` | `9e7b57f3db4d5409…` | 127/5317 → **135/5903** | 改：换掉「显式 days 透传」为「不开放窗口 days」；仍 8 例 |
| `front/app/features/tags/composables/useOtherTopicObservation.ts` | `b2335a1ce59d5cce…` | `11d7a8698e5c5fc2…` | 258/10554 → **280/11802** | 改：view 增 `refreshing`；fetcher 签名只 boardId；增 `invalidate()` |
| `front/app/features/tags/composables/useOtherTopicObservation.test.ts` | `8e1d17e374ec0105…` | `e0456d69151647bf…` | 251/10366 → **314/13230** | 改：原 10 例保留；新增 3 例（refreshing / invalidate ×2）；2 处 days 断言随签名更新 |
| `front/app/features/tags/components/console/OtherTopicReading.vue` | `d7d7491079666ea9…` | `1b5ed3103ff3893d…` | 530/19177 → **366/14163** | 改：M1 受控 `retryReport`；M2 走新 composable；观察带/状态块拆出 |
| `front/app/features/tags/components/console/OtherTopicReading.test.ts` | `04388e316e4feb10…` | `fcbdb5ba3bab4db2…` | 382/16791 → **589/26300** | 改：原 8 例保留（2 例断言改受控意图）；新增 8 例（M1 3 / M2 5） |
| `front/app/features/tags/components/console/BoardNarrowSummary.vue` | `6dac4f20cd9daeca…` | `37d0965d1bde6484…` | 795/24549 → **385/12352** | 改：按内聚拆 4 个子组件，父只留编排；DOM/testid/props/emits 不变 |
| `front/app/features/tags/components/console/BoardNarrowSummary.test.ts` | `9bd4c4727e1dcd14…` | `9bd4c4727e1dcd14…`（未改） | 324/14630 | 不动：原 12 例逐字保留并全绿 |
| `front/app/features/tags/components/console/OtherTopicObservationBand.vue` | 不存在 | `be84b6c0f5263f89…` | 0 → **181/5914** | 新建：观察带纯视图（说明/节点/重试/选择事件） |
| `front/app/features/tags/components/console/OtherTopicReadingStates.vue` | 不存在 | `d05a3e2f3d2c1462…` | 0 → **90/2836** | 新建：正文状态块（loading/no-report/error/missing-section + 意图按钮） |
| `front/app/features/tags/components/console/useOtherReadingSettled.ts` | 不存在 | `924169cbaa1f872a…` | 0 → **104/4350** | 新建：G3 版阅读终态信号（挂载/post-DOM/依赖真实终态/旧排队作废） |
| `front/app/features/tags/components/console/BoardNarrowDayStrip.vue` | 不存在 | `beb6c591134b300a…` | 0 → **176/5193** | 新建：同源报日条 + 仅自身 scrollLeft 定位 |
| `front/app/features/tags/components/console/BoardNarrowLaneList.vue` | 不存在 | `1adfa329a4d48130…` | 0 → **168/4600** | 新建：泳道行列表（迷你条格由父算好传入） |
| `front/app/features/tags/components/console/BoardNarrowGroupToggle.vue` | 不存在 | `c1f3200ffc094915…` | 0 → **83/1985** | 新建：折叠组 summary（其他/休眠共用） |
| `front/app/features/tags/components/console/BoardNarrowOtherGroup.vue` | 不存在 | `9ad019a216c83dbe…` | 0 → **152/4386** | 新建：当日其他话题组（G1 视图同源渲染） |

未触碰：G1/G2 源与测试、`BoardConsoleShell.vue`/`BoardConsoleStage.vue`/`consoleStageTypes.ts`/`LaneChronicle.vue`、`useReadingSettledSignal.ts`、`api/dailyReports.ts` 与既有 `api/normalizers/**`、后端/foreign/原型/静态产物/部署文件。`BoardNarrowSummary.test.ts` after SHA 与 before 完全一致。

## 2. G3-M1：受控报告重试意图（先红后修）

### 2.1 真实红（改动前，仅本组三个目标文件）

- `pnpm test:unit app/features/tags/components/console/OtherTopicReading.test.ts --maxWorkers=2` → **8 failed | 8 passed (16)**；其中 M1 3 例全红：`expected undefined to deeply equal [[{date:'2026-10-22', sectionId:'501'}]]`。旧行为确实在点击后直接 `ensureReportDay(date)`（`api.loadReportIndex` 被调），没有受控意图。
- 旧 ORD-4 两处断言（错误态重试、缺分区刷新）同步改为期望 `retryReport` + 组件不直读，故也在红列（2 例）。

### 2.2 修复（只动本组件）

- 新增 emit：`retryReport: [{ date: string, sectionId: string }]`。
- `other-reading-retry`（错误态）与 `other-reading-refresh`（缺分区）都改为**只发意图**；组件不再调用 `reading.ensureReportDay(...)`，payload 只取当前 `selectedDate`/`selectedSectionId`（切上下文后不带旧 sectionId，更不携带旧 reportId）。
- 相关文章注入 store、观察带专用 `observation.retry()` 保持原样（观察强刷不走日报读取路径）。

### 2.3 G4 必须的编排顺序（交接，不在本包验真实网络）

1. 收到 `retryReport({date, sectionId})`；
2. **先** `reading.ensureReportIndex({ force: true })`（同日日报重建 → reportId 变化，旧索引/旧详情缓存必须失效）；
3. **再** `reading.ensureReportDay(date, { force: true })`；
4. 之后按稳定 `sectionId` 核对详情：仍缺 → `missing-section` 中性提示；0 条 → 合法空列表；不得借别日/相似标题。

本线程**没有** Shell 调用点、也没有验证真实「重建 → 新 reportId → 新分区」链路，故不声称该链路已验（G4 集成测试核顺序）。

## 3. G3-M2：阅读终态信号（先红后修）

### 3.1 真实红

- 组件目标集合首轮 **8 failed | 8 passed (16)**：除 M1 3 例外，M2 的
  - 初始 cache-ready 挂载前即发：`expected [[{laneKey:'other:501',…}]] to be undefined`（旧 `watch(...,{immediate:true})` 在 setup 就发）；
  - 强刷在途当终态：`expected undefined to be true`（旧 view 无 `refreshing`，`status:'ready'` 掩盖 `loading`）；
  - 同 context 刷新不再发：`expected length 2 but got 1`（旧 dedupe key 不含刷新周期）。
- hook 文件首轮 **6 failed | 7 passed (13)**：新增 `refreshing` / `invalidate` 3 例全红（`not a function` / `undefined`），2 处 days 断言随 API 签名改动红，1 处强刷失败断言因 `refreshing` 缺失红。
- 另有 2 例为**回归守卫**（首轮即绿，不作红声称）：观察失败/无 ID 不挡合法终态 + 相关文章未展开不进等待；卸载/换上下文不发旧 identity。它们防的是修复过程把终态扩大化。

### 3.2 修复协议（`useOtherReadingSettled.ts`，复用既有协议不改共享 helper）

- **挂载门**：`onMounted` 才放行；emit 前再 `await nextTick()`（DOM 已按终态更新，含会改变高度的观察带节点）。
- **报告终态**：仅 `ready|missing-section|no-report|error`；`gatedDay` 为空/加载中不发。
- **观察真实终态**：`ready|error|missing-id`，且 `refreshing === false`；`idle|loading` 不发。
- **旧排队作废**：候选回到 pending（换日期/分区/话题、观察刷新开始）立即清 `queuedKey`；flush 后再核对 `mounted` + `queuedKey` + `settledKey` 三重一致，任何一项变化都丢弃。
- **可再次发**：候选回到 pending 时清 `emittedKey`，同一 context 刷新走完终态后会再发一次（供父级 pending 消费）。
- 不等未展开的相关文章、无固定 sleep、不滚动、不复制日报 cache/状态。

### 3.3 G4 消费时序（交接）

- 每次 `readingSettled` 都在「当前渲染已稳定」之后：报告终态 + 观察终态 + 一次 DOM flush。
- 观察强刷（`observation.retry()`）期间**不会**再发终态；刷新落定后会**再发一次**，G4 的 pending 消费应按「最近一次」处理，不要假设只发一次。
- 换 `selectedDate`/`selectedSectionId` 或卸载后，之前排队未发的信号不会再发；G4 清 `selectedOther` 时无需自己防旧信号。
- 观察失败/当次未归属（`topicId=null`）都是合法终态，G4 不要为它们额外等待或禁止返回恢复。

## 4. G3-S1：按内聚拆分（source 全部过线）

| 拆分前 | 拆分后（source 行/字节） | 子组件职责 |
| --- | --- | --- |
| `OtherTopicReading.vue` 530/19177 | **366/14163** | 阅读槽编排（gate/正文/动作/终态） |
| ↳ 观察带（原模板+样式+说明） | `OtherTopicObservationBand.vue` **181/5914** | 纯视图：说明/陈旧/节点/重试+选择 emit |
| ↳ 正文状态块 | `OtherTopicReadingStates.vue` **90/2836** | 纯视图：loading/no-report/error/missing-section + 意图按钮 |
| `BoardNarrowSummary.vue` 795/24549 | **385/12352** | 编排：可见行、迷你条统一分档、组顺序、空/错/加载态 |
| ↳ 报日条 | `BoardNarrowDayStrip.vue` **176/5193** | 连续轴只报日可点 + 仅自身 scrollLeft 定位 |
| ↳ 泳道行列表 | `BoardNarrowLaneList.vue` **168/4600** | 行结构/迷你条格/编年入口（分档由父传入） |
| ↳ 折叠组 summary | `BoardNarrowGroupToggle.vue` **83/1985** | 两 toggle 共用的纯 summary |
| ↳ 其他话题组 | `BoardNarrowOtherGroup.vue` **152/4386** | G1 视图同源渲染 + 行标题 clamp/title/aria |

- 最大 source：`BoardNarrowSummary.vue` 385 行 / `OtherTopicReading.vue` 14163 字节（<500 行、<~15KB）；新增 helper 104 行/4350 字节。
- DOM 结构/class/testid/props/emits/交互（迷你日格、正式行、0 正式选日报、两个独立组、0 休眠 summary、报日条 clamp、两行 clamp+title/aria）逐项保留；样式随所属行为移动，没有为凑阈值删注释/压行。
- 迷你条分档上限仍按「可见行（活跃 + 展开休眠）」统一算（`maxSections` 留在父级），子组件只接收算好的 `cells`，不重算阈值/计数。
- **未改**：`BoardNarrowSummary.test.ts`（12 例逐字原样、全绿），证明拆分无行为漂移。

## 5. API 统一与观察失效（顺手限范围）

- `ObservationResponse<T>` 改为 `ApiResponse<T>` alias（不另造一套返回包装；不改 `api/dailyReports.ts` 及其消费者）。
- `loadBoardSectionTimeline(boardId)` 恒定请求 `days=0`（全部历史）：**签名不再接受窗口 days**——调用方无法传 7 天窗口却拿到 `coverage:'all'` 的少覆盖数据。运行时守卫：硬塞第二参数会被忽略（仍请求 0）。
- 观察 hook 新增：
  - `view.refreshing`：已有成功数据强刷在途时 `status` 仍 `ready`（内容可读）但 `refreshing=true`；
  - `invalidate()`：**不发请求**、丢成功数据并 `requestSeq+1` 作废在途响应，`loading=false`、`error/stale` 清空；下次 `ensureObservation` 才真重读。
- G4 生成完成后的建议顺序（本包不给调用点）：`refreshConsole` → `ensureReportIndex({force:true})` → 若当前选中其他行：`observation.invalidate()` 后对当前 topic `ensureObservation(topicId)`（或直接 `retry()`）；**没选其他行**时只调 `invalidate()`（零 GET，不会沿旧 topic 发无谓请求，也不会留成功缓存让下次永读旧数据）。

## 6. 验证（Linux 本机串行，参数不带 `--`）

| 检查 | 命令（`cd front`） | 结果 |
| --- | --- | --- |
| 红（改动前） | `pnpm test:unit app/api/otherTopicObservation.test.ts app/features/tags/composables/useOtherTopicObservation.test.ts --maxWorkers=2` | **1 failed / 7 passed**（API）；**6 failed / 7 passed**（hook） |
| 红（改动前） | `pnpm test:unit app/features/tags/components/console/OtherTopicReading.test.ts --maxWorkers=2` | **8 failed / 8 passed (16)** |
| 绿（本包目标集合，一次跑） | `pnpm test:unit app/api/otherTopicObservation.test.ts app/features/tags/composables/useOtherTopicObservation.test.ts app/features/tags/components/console/OtherTopicReading.test.ts app/features/tags/components/console/BoardNarrowSummary.test.ts --maxWorkers=2` | **4 files / 49 tests passed**（API 8 / hook 13 / 阅读组件 16 / 窄屏 12） |
| 相邻直接消费者回归（未改其源） | `pnpm test:unit app/features/tags/components/console/BoardConsoleShell.test.ts --maxWorkers=2` | **1 file / 38 tests passed** |
| 本组 `.ts` eslint（7 文件） | `pnpm exec eslint app/api/otherTopicObservation.ts app/api/otherTopicObservation.test.ts app/features/tags/composables/useOtherTopicObservation.ts app/features/tags/composables/useOtherTopicObservation.test.ts app/features/tags/components/console/OtherTopicReading.test.ts app/features/tags/components/console/BoardNarrowSummary.test.ts app/features/tags/components/console/useOtherReadingSettled.ts` | **exit 0**（0 error 0 warning） |

计数口径：旧 38 = API 8 + hook 10 + 阅读组件 8 + 窄屏 12。其中 **API 原「显式 days 原样透传」1 例按新契约（不开放窗口 days）替换为「拒绝窗口 days」守卫**（属预期契约变更，非漏测）；hook 2 处 days 断言随签名、阅读组件 2 处重试断言随 M1 受控意图更新；其余旧角逐字/等效保留（窄屏 12 例未改）。本包新增 11（hook +3、阅读组件 +8）；49 是新集合实跑总数，不与旧 38 相加做任何「总数」宣称。窄屏 12 例未新增/未改写（S1 为结构拆分，靠原测试证明无漂移，不冒新角）。

**Vue 文件不被 eslint 配置覆盖，不声称 Vue lint 绿**；`.vue` 仅经 Vitest SFC 编译，不等于 `nuxi typecheck` 通过。

## 7. 未验 / 风险 / 需要父或 G4 承接

1. **G4 未接线**：`retryReport`/`readingSettled` 无真实消费方；「force 索引 → force 当日」顺序、生成后 `invalidate`/刷新、返回恢复都未集成验证。本页只给签名与时序。
2. **真实浏览器/真实数据**：未跑 375/390 横滑、真实几何（clamp）、双主题、真实重建 reportId 变化；观察带真实大响应（卫生 14307B / 中东 693593B）仍只此前父只读实测，不在本包复测。
3. **未跑**：全量 `pnpm test:unit`、`nuxi typecheck`、`pnpm build`、全树 lint、Go/SQL/DB/AI/生成/业务写、dev 服务、部署、commit。typecheck 由父集中跑（本包新类型：`retryReport` payload、`useOtherReadingSettled` deps、观察 `refreshing`/`invalidate`、子组件 props）。
4. **测试文件体量**：`OtherTopicReading.test.ts` 589 行/26300 字节、hook 测试 314 行/13230 字节已超 source 阈值口径（测试文件不在本包 S1 mandate；父复核中「大测试在独立结构收口包核规范」仍待另外处理，本包未削旧故事/未硬拆）。
5. **S1 不覆盖 G1/G2**：`useLaneDayReading` 513 行、`ObservationMatrixGrid` 646 行等仍待父的独立结构包（本包未触碰）。
6. 观察 `invalidate()` 后若当前仍选中其他行，视图会进入 `idle`（诚实的未取到）直到下一次 `ensureObservation`——这是「不 GET 的懒失效」的直接后果，G4 需在失效后补一次 `ensureObservation(topicId)` 才会重新画观察带。

## 8. tasks 8C.4 勾选依据

- M1/M2 红→绿有实捕记录（§2.1/§3.1），本包目标 **4 files / 49 tests passed**、相邻 Shell **38 passed**、7 个 `.ts` eslint **exit 0**、source 全部过 500 行/15KB 线（§4）。
- 仍是**只此包**结论：不代表 UI 验收/整 change 完成，8C.5–8C.8 不动；真实浏览器与 G4 集成风险见 §7。
