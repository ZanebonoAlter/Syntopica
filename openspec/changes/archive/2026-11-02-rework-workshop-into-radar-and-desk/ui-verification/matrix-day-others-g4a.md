# G4a 状态/读取/返回契约准备报告（8C.5A）

> 线程：全新短 implementer、本写域唯一 writer（未 resume 旧长线程、未派子线程、未 commit/归档/部署、未换 provider/model/CLI）。
> 基线：批准矩阵增量 `ui-prototype/revision-2026-10-28-matrix-day-others/standalone.html`（SHA256 `cdb0b70e…`）；G1/G2/G3/8C.4S 已目标绿（旧 189 用例 multiset 一致）。
> 本页只记录本线程**实跑**证据；**控制器绿 ≠ 真实 UI 已接**；旧账（根账 S11/53/1/2、他域债）不因本包清零。

## 1. 范围与红线

改了什么（全部在 G4a 授权辖区）：

| 类别 | 文件 |
| --- | --- |
| 新增（G4a 专有内聚控制器） | `front/app/features/tags/components/console/useConsoleReading.ts`、`otherTopicSelection.ts` |
| 既有返回载体/恢复/刷新（必要小 hunk） | `consoleReturnContext.ts`、`useConsoleReturnContext.ts`、`useConsoleRefresh.ts` |
| 新增/扩展测试 | `useConsoleReading.test.ts`、`otherTopicSelection.test.ts` + `otherTopicSelection.select.test.ts` + `otherTopicSelectionTestHarness.ts`（共享 fixture）、`useConsoleReturnContext.test.ts`、`useConsoleRefresh.test.ts`（新）；`consoleReturnContext.test.ts`（扩） |

**未触碰**（红线核对）：任何 `.vue`（Shell/Body/Stage/Chronicle/G1/G2/G3 视图）、`consoleStageTypes.ts`、G1/G2/G3 运行时源与测试、`restoreScroll.ts`、`api/**`/normalizer、后端/Go/SQL/AI/业务数据、foreign/原型/`docs/reference/**`/部署/commit/归档。`BoardConsoleShell.test.ts` 只读跑回归，未改一字。

关键约束继承：A 名册/排序/休眠/计数与 sourceDays 生命周期不变；单板块 1 份报日索引、详情 14/文章 20、cache epochs 复用既有 store（未造第二个读取器/缓存）；API 边界字符串 ID/camelCase 只在既有边界；不新增 `date=` query。

## 2. before → after（SHA256 前 16 位，开工实采 / 完工实算）

| 文件 | before | after | 行/字节 before → after |
| --- | --- | --- | --- |
| `console/consoleReturnContext.ts` | `de3d9292c71eea45` | `f49a3bb48e3ff745` | 112/4507 → **160/6851** |
| `console/consoleReturnContext.test.ts` | `c1dc40109dce3520` | `8cfde0350a3bf067` | 120/4478 → **190/7472** |
| `console/useConsoleReturnContext.ts` | `9074c7ffeb6d49f5` | `388ab6572d30be2f` | 370/12926 → **412/15301** |
| `console/useConsoleRefresh.ts` | `bac0b93878a2d01d` | `d43c04ba5fdad0fa` | 32/985 → **44/1612** |
| `console/useConsoleReading.ts` | 不存在 | `bdf8693597b4717b` | 0 → **172/8067** |
| `console/otherTopicSelection.ts` | 不存在 | `21925beb09e242ca` | 0 → **207/9096** |
| `console/useConsoleReading.test.ts` | 不存在 | `3cd9205e0aca2e73` | 0 → **277/11294** |
| `console/otherTopicSelection.test.ts` | 不存在 | `0ddee235445a0758` | 0 → **150/5192**（默认态/组视图） |
| `console/otherTopicSelection.select.test.ts` | 不存在 | `1c1fd4b2d5b0929f` | 0 → **158/6406**（选择/清场） |
| `console/otherTopicSelectionTestHarness.ts` | 不存在 | `4b9b9d05b628d898` | 0 → **140/5302**（共享 fixture，非 `*.test.ts`） |
| `console/useConsoleReturnContext.test.ts` | 不存在 | `de75cdc9bd0f6dca` | 0 → **367/14499** |
| `console/useConsoleRefresh.test.ts` | 不存在 | `ddab3e1ff9d2b621` | 0 → **97/3400** |

- 全部 source/test/harness ≤500 行且 ≤15360B（`useConsoleReturnContext.ts` 15301B 最接近；测试最大 `useConsoleReturnContext.test.ts` 14499B）。`otherTopicSelection.test.ts` 初版超阈（15533B→修剪后仍 15511B），已按 8C.4S 惯例拆出 `select` 子套件 + 共享 harness（用例总数与断言不变：7 + 8 = 15）。
- 新文件 before 均不存在（开工前 `ls` 实查）；未用 HEAD/stash/reset 重建任何文件。
- 未改 `useConsoleReturnContext` 的既有 deps 必需项、`useConsoleRefresh` 的既有语义、`consoleReturnContext` 的旧六字段形状。

## 3. 唯一状态与构造顺序（G4b 直接照此接）

**唯一状态**：

- `selectedDate` 仍只在 `useConsoleSelection`（现有）持有一份；G4a 控制器只读跟随，不建第二份。
- 单份读取：`useConsoleReading.reading`（既有 `useLaneDayReading` 实例：索引 1 份/详情 14/文章 20/双 epoch）与 `.observation`（G3 单份 section-timeline 缓存）。**产品路径只创建一次**。
- `selectedOther` / `othersOpen` 只在 `useOtherTopicSelection` 持有；`otherTopics` 由 G1 `buildOtherTopics` 派生（不复制排除/计数/状态词）。
- 返回载体仍走同一 board sessionStorage 键（`syntopica:board-console:return:<boardId>`）；阅读 key 分离为「lane 原 key 或 `other:<sectionId>`」，**不写进 `laneId`/URL `lane`**。

**构造顺序（Shell setup，建议严格按序）**：

```ts
// 1) 状态源（现有）+ 观察可见轴
const consoleState = useBoardConsole(boardIdRef, { route: () => route, router: () => router, reportAnchor: () => latestReportDate.value })
const { sourceWindow, axis, visibleAxis, today, anchorDate, sourceDays, selectedLaneId, resolveLaneDay, observation, /* ... */ } = consoleState

// 2) 单份读取组合（索引首取在此触发，不依赖 LaneChronicle 挂载）
const readingBridge = useConsoleReading({
  boardId: boardIdRef,
  today: () => today.value,
  anchorDate: () => anchorDate.value,
  axis: () => visibleAxis.value,
})

// 3) 选中日投影（注入同一份 reportDates → G1 三级校正）
const selection = useConsoleSelection({
  sourceWindow, axis, visibleAxis, selectedLaneId, resolveLaneDay, poolBounds,
  reportDates: readingBridge.reportDates,
})

// 4) 绑定唯一 selectedDate → 报告优先读取（同日不重复、跨日先清显示）
readingBridge.bindSelectedDate(selection.selectedDate)

// 5) 其他分区选择/默认展开/组视图（需要上面三份）
const other = useOtherTopicSelection({
  boardId: boardIdRef,
  selectedDate: selection.selectedDate,
  selectedLaneKey: selection.selectedLaneKey,
  visibleAxis,
  reportDates: readingBridge.reportDates,
  indexLoading: readingBridge.indexLoading,
  indexError: readingBridge.indexError,
  reportDay: readingBridge.reportDay,
  allLanes: selection.allLanes,
  resolveLaneDay,
  sourceWindow,
})

// 6) 返回恢复（既有 deps 不变，追加两个可选 ref）
const returnContext = useConsoleReturnContext({
  /* ...既有 deps... */,
  selectedOther: other.selectedOther,
  othersOpen: other.othersOpen,
})

// 7) 刷新链（既有防重/顺序不变，追加 after）
const { busy: consoleRefreshing, refresh: refreshConsole } = useConsoleRefresh({
  loadReportMeta, loadSource: load, reportMetaLoading,
  after: () => readingBridge.refreshAfterRebuild({
    date: () => selection.selectedDate.value,
    otherTopicId: () => other.selectedOther.value?.topicId ?? null,
  }),
})
```

`source A 未到达`（`sourceWindow === null`）与「真正成功 0 泳道」（`lanes: []`）由 `othersOpen` 初始化 gate 分开：只有 `sourceWindow !== null` 才消费一次默认态。

## 4. 对 G4b 的接口（props/events/methods/settled key）

### 4.1 `useConsoleReading`（新增）

| 导出 | 说明 |
| --- | --- |
| `reading` / `observation` | 既有 store 实例；分别注入 `LaneChronicle`（可选 `reading` prop）与 `OtherTopicReading`（`reading` + `observation`）。 |
| `bindSelectedDate(selectedDate)` | 构造第 4 步；内部 watch 跟随唯一选中日 → `ensureReportDay(date)`（折叠组也读，供 summary 计数）。 |
| `retryReport({date, sectionId})` | 受控重试：`await ensureReportIndex({force:true})` → `await ensureReportDay(date,{force:true})` → 按稳定 sectionId 核对。返回 `{ applied, sectionFound }`：`applied=false`（板块/选中日已变）时 G4b 忽略；`sectionFound=null` 只表示详情未到/失败，不得当「缺分区」。不触发观察 GET、不清选择。 |
| `refreshAfterRebuild({date:()=>…, otherTopicId:()=>…})` | 生成完成/显式刷新：force 索引 → 当前阅读 force → `observation.invalidate()`；`otherTopicId` 非 null 才 `ensureObservation(topicId)`，否则**零观察 GET**。异步间切板块直接停止。 |
| `reportDates`/`reportDay`/`reportStale`/`indexLoading`/`indexError` | 直接下发给 `ObservationMatrix*`/`BoardNarrowSummary`/`OtherTopicReading`（同一份 store 状态）。 |
| `ensureReportIndex` / `ensureReportDay` | 供既有 stage `retry` 事件复用（见 §5）。 |

### 4.2 `useOtherTopicSelection`（新增）

| 导出 | 说明 |
| --- | --- |
| `selectedOther` | `Ref<{ sectionId, date, reportId, topicId } \| null>`（不存标题）。 |
| `selectedOtherSectionId` / `selectedOtherTopicId` / `selectedOtherReadingKey` | 阅读槽高亮 / 观察带 topic / 阅读 key（`other:<sectionId>`）。 |
| `othersOpen` | 首份成功 A：`active+dormant == 0` → `true`，否则 `false`；只一次/板块；toggle/切档/裁范围不强改；切板块复位。 |
| `otherTopics` | G1 `buildOtherTopics` 视图（含 `windowReportDayCount`）。 |
| `toggleOthers()` | 仅 `state==='ready'` 生效（与组件 disabled 一致）。 |
| `selectOther({sectionId,date,topicId?})` | 行/格同一入口：要求 `date === selectedDate`、sectionId 合法且真实存在于当前 ready 详情；写入 `reportId`（取自 reportDay）与 `topicId`。不写日期/URL。 |
| `selectDayHeader(date)` | 只改同一 `selectedDate`；要求轴内且 `reportDates.has(date)`。 |
| `selectObservedDay({date,sectionId,reportId,topicId})` | **原子**写 `selectedDate` + 稳定目标；轴外/非法 ID 忽略。G3 节点 payload 直接透传。 |
| `clearOther()` / `resetForBoard()` | 显式清场/复位。 |
| `selectedLaneKey` 变化（sync）/ `selectedDate` 变化（date 不匹配才清）/ 板块变化 | 清场规则内置。缺 section（报告已更新）**不清**选择。 |

### 4.3 既有 `useConsoleReturnContext`（兼容扩展）

新增可选 deps：`selectedOther?: Ref<ConsoleOtherSelection | null>`、`othersOpen?: Ref<boolean>`。行为：

- `saveContext()`：无其他选择时**保持旧六字段形态**（Shell 既有测试 `Object.keys` 断言不变）；有选择时追加 `other` + `othersOpen`，`laneId` 仍是泳道实体。
- 恢复：`other.date` 必须与恢复的 `date` 一致才写入；`othersOpen` 随 `other` 恢复。
- 阅读 key：`currentReadingKey = selectedOther ? 'other:'+sectionId : selectedLaneKey`；`readingSettled` 与滚动 gate 都按 **readingKey + date** 匹配（旧 lane 信号/旧 other 信号不会误触发）。
- URL 显式 `lane`：缓存不覆盖显式值；显式 lane **不同**于保存 lane → 不恢复 other（显式导航优先）；显式 lane **相同**（从其他话题打开日报再回来的真实往返）→ 恢复原阅读槽。任何情况下 other 不写 `laneId`/URL `lane`/`date=`。
- `readingSettled` 的 G3 payload `laneKey='other:<sectionId>'` 直接命中；G4b 按「最近一次」消费并负责在切上下文时清旧 `readingSettled`（G3 hook 自身也会作废旧排队）。
- 归档内部往返：`activeView` 切换不离开壳层，`selectedDate`/`selectedOther`/`othersOpen`/滚动状态自留，无需额外调用；阅读台/研究工作台真实路由往返由 `openReport`/`openResearch` 内置 `saveContext()`。

### 4.4 既有 `useConsoleRefresh`（兼容扩展）

新增可选 `after?: () => Promise<void>`：在同一 `refreshing` 防重窗口内、`loadReportMeta()` → `loadSource()` 之后执行；`after` 抛错被吞掉（只影响附属刷新，索引陈旧由 `indexError` 自述，矩阵/编年仍可读），`refreshing` 必回落。未传 `after` 的旧调用行为零变化。

### 4.5 G4b 事件接线速查

- `ObservationMatrix@selectDayHeader` → `other.selectDayHeader`；`@toggleOthers` → `other.toggleOthers`；`@selectOther` → `other.selectOther`；`@toggleDormant` 仍走 `toggleDormant`（A 名单/休眠不变）。
- `BoardNarrowSummary` 同名事件同接法。
- 阅读槽：`selectedOtherSectionId` 非空 → `OtherTopicReading`（`selectedDate`、`selectedSectionId`、`reportDay`、`reading`、`observation`、`topicId=selectedOtherTopicId`）；否则既有 `LaneChronicle`（注入 `readingBridge.reading`）。
- `OtherTopicReading@retryReport` → `readingBridge.retryReport(payload)`；`@selectObservedDay` → `other.selectObservedDay(payload)`（日期变化由 `bindSelectedDate` 自动重读，无需额外调用）；`@readingSettled` → 既有 `readingSettled` ref。
- `OtherTopicReading@openReport` → `returnContext.openReport(payload.date)`；`@viewArchive` → 既有归档切换；`@openArticle` → 既有站内阅读。
- 索引失败重试：现有 stage `retry` 事件 → 同时 `load()` 与 `readingBridge.ensureReportIndex({ force: true })`（G2 索引提示无独立按钮，G4a 不扩 G2 视图）。

## 5. 受控重试与生成刷新时序（与 G3 M1/M2 对齐）

1. `OtherTopicReading@retryReport({date, sectionId})` →
2. `await ensureReportIndex({ force: true })`（同日重建会换 reportId，旧索引/旧详情缓存必须失效）→
3. `await ensureReportDay(date, { force: true })`（绕过详情成功缓存；同日失败保留旧内容 + stale）→
4. 按稳定 `sectionId` 核对：`sectionFound=true` 命中 / `false` 缺分区（UI 显示「报告已更新」+ 可重试）/ `null` 未知；
5. 生成完成/手动刷新：既有 `useConsoleRefresh`（meta→A 防重）→ `after`：
   - `ensureReportIndex({force:true})`（新报日列头立即可选）；
   - 当前选中日 `ensureReportDay(date,{force:true})`（切日/切板块中途停止）；
   - `observation.invalidate()`：无其他行 → 到此为止（零观察 GET）；有选择 → `ensureObservation(topicId)`（当前 topic，不沿旧 topic 发无谓请求）。

## 6. 新字段 sanitizer 与旧兼容（`consoleReturnContext.ts`）

```ts
other?: { sectionId: string, date: string, reportId: string | null, topicId: string | null }
othersOpen?: boolean
```

- 旧六字段 payload 反序列化后**不新增键**（`Object.keys` 仍为 `laneId/date/days/start/end/scrollTop`），既有 Shell 测试原样通过。
- `other` 只在 `sectionId` 与 `date` 均合法且 `other.date === date` 时保留；`reportId/topicId` 非法只置 `null`；整个 other 非法静默丢弃（不影响旧字段恢复）。
- `othersOpen` 仅接受 boolean；只有 `other` 有效时随载体出现。
- `otherReadingKey(sectionId)` → `other:<sectionId>`；与 G3 `useOtherReadingSettled` 的 `laneKey` 同口径（G3 常量仍是字面量，字符串相等即命中，本包不引 G3 源）。

## 7. 验证（Linux 本机串行，参数不带 `--`）

| 检查 | 命令（`cd front`） | 结果 |
| --- | --- | --- |
| 本包目标（新增 + 扩展 + 旧 consoleReturnContext） | `pnpm test:unit app/features/tags/components/console/useConsoleReading.test.ts app/features/tags/components/console/otherTopicSelection.test.ts app/features/tags/components/console/otherTopicSelection.select.test.ts app/features/tags/components/console/useConsoleReturnContext.test.ts app/features/tags/components/console/consoleReturnContext.test.ts app/features/tags/components/console/useConsoleRefresh.test.ts --maxWorkers=2` | **6 files / 56 tests passed**（11 / 7 / 8 / 13 / 13 / 4） |
| 相邻旧 Shell 回归（只读，未改其测试） | `pnpm test:unit app/features/tags/components/console/BoardConsoleShell.test.ts --maxWorkers=2` | **1 file / 38 tests passed**（含「从阅读台返回 URL 显式 lane 优先」与 R34 滚动两例） |
| 合并一次跑 | 上述 7 文件 | **7 files / 94 tests passed** |
| 本组 `.ts` eslint（12 文件） | `pnpm exec eslint <§2 全部 .ts>` | **exit 0**（0 error 0 warning） |
| 尺寸 | 逐文件 `wc -lc` | 全部 ≤500 行 / ≤15360B（§2） |

计数口径：`consoleReturnContext.test.ts` 原 8 例逐字保留 + 新增 5 例 = 13；Shell 38 例为旧资产回归，不与新例相加冒充覆盖。**本包新增用例不存在「改动前实现」可先红**（控制器/载体字段为本包新建或纯扩展），因此不声称红→绿过程；真正红→绿的既有行为由 G1/G3 各自报告承担。

本包未跑：全量 `pnpm test:unit`、`nuxi typecheck`、`pnpm lint`、`pnpm build`/generate、浏览器、Go/SQL/DB/AI/业务写、服务起停、部署、commit/归档（按 brief 与树莓派红线）。

## 8. 未验 / 风险 / 归 G4b 或父

1. **未接真实 UI**：没有任何 `.vue` 改动，`useConsoleReading`/`otherTopicSelection` 尚无 Shell 调用点；props/events 接线、单一阅读槽渲染、真实滚动宿主与返回恢复的集成验证归 8C.5B/8C.5（G4b）。
2. **typecheck 未跑**：新类型（`RetryReportResult`、`OtherSelectionTarget`、`ConsoleReturnDeps` 新可选 deps、`ConsoleReturnContext.other`）由父在收口批集中 `pnpm exec nuxi typecheck`；本包只以 Vitest SFC/TS 编译 + 本组 eslint 验证。
3. **URL 显式 lane 与 other 的取舍**：本包采用「显式 lane 不同 → 不恢复 other；相同 → 恢复」以满足 S11-20 往返；若父认为应一律不恢复，只需删 `useConsoleReturnContext` 恢复分支一处条件（有对应测试可改），不影响其它契约。
4. **索引失败重试入口**：G2 网格提示无独立按钮（G2 已冻结），G4b 需把索引重试挂到既有 stage `retry`（同时 `load()` + `ensureReportIndex({force:true})`）；若要求独立按钮需另开 G2/G3 视图小 hunk，本包不越界。
5. **`othersOpen` 只随 other 保存**：用户仅手动展开组（未选行）时返回不恢复展开态（旧六字段兼容优先）；如产品要求恢复纯折叠偏好，需要扩载体（会破坏旧 `Object.keys` 断言，本包按 brief「必要 othersOpen」保守处理）。
6. **真实数据/浏览器**：0 正式泳道（246030）与成熟（1974）的真实组行数、计数、状态词、滚动几何仍待 8C.7；本包用受控 fetcher/rAF driver 模拟。
7. **G3 终态信号的清理责任**：`readingSettled` 由 G4b 在换 context 时清旧 ref（G3 hook 已作废其内部排队）；本包只按 key+date 匹配，不主动清 G4b 的 ref。

## 9. tasks 8C.5A 勾选依据

- 本包目标 **6 files / 56 tests passed**、相邻旧 Shell **38 passed**、12 个 `.ts` eslint **exit 0**、全部 source/test/harness 过 500 行/15KB、`openspec validate --strict` 与 `check-tasks-tail.sh` 通过（见 tasks 验收行）。
- 仍**只此包**结论：不代表 G4b 接线、真实 UI、整 change 完成；8C.5B/8C.5/8C.6–8C.8 不勾；根账旧失败/未覆盖不洗绿。
