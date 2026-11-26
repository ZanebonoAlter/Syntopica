# 8E「矩阵关联日黄点」增量独立评审（change-review，2026-10-30）

> 审查对象：change `rework-workshop-into-radar-and-desk` 的 8E 增量（矩阵内复用视觉与关联日黄点）实现态——
> `laneConsoleView.ts` / `useConsoleSelection.ts` / `otherTopicSelection.ts` / `useConsoleReadingWiring.ts` /
> `ObservationMatrixDayHeader.vue` / `BoardNarrowDayStrip.vue` / `BoardConsoleStage.vue` 及其测试。
> 编号延续本 change `review.md` 的 R-1…R-40（`review.md` 本轮不改写）。
> 本轮为**只读评审 → 复现 → 修复**：先补复现测试（红）再改实现（绿），未触真库、未改其他 change 文件、
> 未提交/未归档、未起 dev server、未跑浏览器（浏览器与部署由主线程协调）。

## 一、结论

**确认 1 项 High（已修复）**：报日索引缺该日时，**黄点（关联日）点击会被选中日普通回落静默撤销**——
用户点的日期与阅读目标一起被搬回别的报日，桌面列头与窄屏日期条同样中招。修复口径：**只保护当前
这一个真实观察目标**（已验证记录日），普通日期/缩窗排出/切板块/切泳道/手动清选择的回落语义不变。

## 二、问题清单

### R-41 2026-10-30【High】报日索引缺该日时，黄点点击被选中日普通回落静默撤销

- **状态**：已修复（定向单测 4 文件 / 61 用例绿，其中本轮新增 10 例；`pnpm lint` 0 error；`pnpm exec nuxi typecheck` exit 0）

**原评审主张（复核后成立）**：`ObservationMatrixDayHeader.isReportDay`（现 L58）与
`BoardNarrowDayStrip.hasReport`（现 L48）把 `observedDays` 的日期当作可点（即使成功 `reportDates` 索引里
没有该日）；`selectObservedDay` 原子写日期+选择后，`laneConsoleView.reconcileSelectedDate`（现 L124，
修复前无 `verifiedDay` 分支）按「不在 reportDates 且该泳道当日无分区」把日期回落到「最后有分区日 /
窗内最新报日」，随后的日期 watcher 再把 `selectedOther` 清掉。

**复核补正（重要，影响复现口径）**：选中日校正是**值变化驱动**的——`useConsoleSelection` 里
`watch(reconciledSelectedDate, …)` 只在 computed 的**值**变化时才回写 `selectedDate`。因此：
若回落值恰好等于回落前的选中日（例如默认选日=最后有分区日=最新报日），computed 值不变、回写不发生，
点击**看似**成功；只有**从非默认回落日出发**（先切到另一个真实报日，再点缺索引的观察日），回写才真正
发生，表现为「点黄点 → 日期与阅读目标被一起搬走」。本轮复现测试即按此口径构造（先切到 `2026-10-16`
报日，再点索引里没有的 `2026-10-21` 观察日）。

**复现证据（修复前，红）**：
`front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts` 新增段
「8E review R-41：缺索引观察日黄点点击不被回落搬走」三例（桌面列头 / 窄屏日期条 / 连续第二次点击）：
- 桌面：`expected '2026-10-23' to be '2026-10-21'`（点 `2026-10-21` 黄点后选中日被搬到 `2026-10-23`）；
- 窄屏：`other-topic-reading` 整块消失（阅读目标被日期 watcher 清掉）；
- 连续第二次点击：第一次点完即被搬走，第二个观察日无从点起。

**代码链路（修复前）**：
1. `ObservationMatrixDayHeader.vue` `isReportDay` / `BoardNarrowDayStrip.vue` `hasReport`：
   `observedByDate.has(date)` 即视为可点；
2. `otherTopicSelection.ts` `selectObservedDay`（现 L174）：写 `selectedDate` + `selectedOther`
   （修复前无「该日已被验证」的标记）；
3. `laneConsoleView.ts` `reconcileSelectedDate`（现 L124）：`reportDates`（已就绪 Set）不含该日、该泳道当日
   无分区 → 回落到 `lastContentDate ?? latestReportDay`；
4. `useConsoleSelection.ts` `watch(reconciledSelectedDate, …)`（现 L68）：computed 值变化 → 回写 `selectedDate`；
5. `otherTopicSelection.ts` 日期 watcher（现 L189）：`current.date !== next` → 清 `selectedOther` → 阅读槽整块卸载。

**为什么这算 High（不只是体验）**：黄点是被 UI 明确承诺可点的入口（`button` + `aria-pressed` + title
「同归属记录 · 读取该日首个真实分区」），点击却**静默无效**（日期与目标一起被撤销）；且观察带
（section-timeline）是**独立只读链**，其记录自带真实 `report_id`/`section_id`/`period_date`，
证据强度**高于**报日索引（一个 lookback 列表，可陈旧/不全）。即：把更强的证据源判成更弱，是口径倒挂。

**修复口径（最小、不放开所有无报日）**：
- 新增「**已验证记录日**」概念：经黄点选中的那一天（观察带里同 ID 的真实分区记录日，仍须在可见轴内）。
- `laneConsoleView.reconcileSelectedDate` 新增第 6 参 `verifiedDay`：该日与报日同等对待（规则 1 保留），
  **轴外仍回落**（范围排出语义不变）；`null`/未接线 = 无已验证目标 → 普通回落。
- `useOtherTopicSelection` 持有 `observedTargetDay`（只认 `selectObservedDay` 原子写入的那一个日子），
  普通选行/清场/切泳道/切板块/换日立即失效。
- `useConsoleReadingWiring` 用**同步 watcher**把它喂给 `useConsoleSelection`（构造顺序 3 在 5 之前，
  故由壳层持有该值，不反向依赖选择控制器）。

**为什么不破坏原回落逻辑**：
- 规则顺序未变：轴外检查仍最先执行（缩窗排出照旧回最新报日）→ 泳道内容日 → **已验证记录日** → 报日索引三态。
- 保护只覆盖**当前这一个**已验证目标日：未接线（`undefined`）、无目标（`null`）、保护别的日子 → 全部按原口径回落
  （`laneConsoleView.test.ts` R41-2 断言）。
- 清场路径全覆盖：普通选行（`selectOther`）、`clearOther`、切泳道（sync watcher）、切板块（`resetForBoard`）、
  换日（日期 watcher）→ 保护同步失效，普通日期的回落语义原样保留（`otherTopicSelection.select.test.ts`
  「已验证记录日生命周期」逐项断言）。
- 目标实际无日报时：日期与目标保留，阅读槽显示诚实 `no-report` 态（「所选日没有日报；不借别日或相似标题补摘要。」），
  不借旧正文、不退回别日（display gate 仍按 `reportDay.date === selectedDate`）。

**修复后验证**：
- `front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts`（4 例新）：桌面黄点点击、
  窄屏日期条黄点点击、普通报日仍清旧选择、连续第二次黄点点击——均绿。
- `laneConsoleView.test.ts`（R41-1…4）、`useConsoleSelection.test.ts`（R41 保护/撤销）、`otherTopicSelection.select.test.ts`
  （保护生命周期）——均绿。
- `cd front && pnpm test:unit app/features/tags/components/console --maxWorkers=2` → 49 文件 / 582 用例通过。
- `cd front && pnpm lint` → exit 0（0 error，8 条他文件既有 warning）；`pnpm exec nuxi typecheck` → exit 0。

**未做（留给主线程）**：真实浏览器亮暗/宽窄验收、build/部署（本 writer 未跑浏览器、未起 dev server、未触 DB）。

## 三、已读文件

`front/app/features/tags/components/console/`：`laneConsoleView.ts`、`useConsoleSelection.ts`、
`otherTopicSelection.ts`、`useConsoleReadingWiring.ts`、`useConsoleReading.ts`、`ObservationMatrixDayHeader.vue`、
`BoardNarrowDayStrip.vue`、`BoardConsoleStage.vue`、`OtherTopicReading.vue`、`OtherTopicReadingStates.vue`；
`front/app/features/tags/composables/useOtherTopicObservation.ts`、`useLaneDayReading.ts`、`laneDayReportIndex.ts`、
`laneDayReportDay.ts`、`useBoardConsole.ts`；后端 `internal/topicgraph/handler/daily_report_handler.go`、
`internal/topicgraph/repository/daily_report_repository.go`（`ListReports` 只按 `period_date` 窗口过滤，
不按状态过滤 → 索引与观察带是两条不同源证据）；本 change `specs/workshop-board-console/spec.md`、
`test-cases-matrix-date-markers.md`、`tasks.md`、`review.md`。

## 四、给父的裁决点

1. 浏览器复验入口：`http://localhost:5100/tags/1974`（成熟板）——先切到与观察日不同的报日，再点关联日黄点，
   断言选中日不被搬走、阅读槽不整块消失；窄屏（<768px）同链路。
2. 索引陈旧的产品口径：观察带记录证明该日**有**日报、但报日索引缺该日时，阅读槽显示「所选日没有日报」
   （诚实但偏保守）。是否要让「已验证记录日」也参与当日详情读取（按 timeline 的 `report_id` 直读），
   属**产品口径变更**，本轮未做，建议下一 change 评估。
3. delta spec 未新增 Scenario：既有 Scenario「黄点点击原子保留阅读目标且普通日期不变」已承诺
   「原子写日期/report/section/topic 并读取真实分区」，本次修复是**回归该承诺**（修复前实现违背它），
   故不新增 Scenario（避免扩大全 change 既有 117 个待对账 Scenario 的旧账）；若父要求把
   「已验证记录日」写成显式 Scenario，本轮可补一条并同步映射表（需重跑 `scenario-trace.sh`）。
