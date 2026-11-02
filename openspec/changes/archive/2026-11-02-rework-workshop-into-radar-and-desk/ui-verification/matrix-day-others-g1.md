# G1 数据与模型（tasks 8C.2）交付报告

> 包：G1（实现合同 §6 独占写域）。范围：报日索引/报告优先读取/双 epoch、normalizer 历史字段、`dayOtherTopics` 纯模型、选中日三级校正与目标测试。
> **未做**：任何 `.vue`（G2/G3/G4）、后端/Go/SQL/DB、G3 的 section-timeline 观察依赖、部署/构建/commit/archive。旧账（根账 53/1/2、其他包未覆盖）不因本包改写。
> 白名单自查：只改下方 10 个 `.ts`；`front/app/api/laneDayReading.ts`、`LaneChronicle.vue`、`BoardConsoleShell.vue`、`ObservationMatrix*.vue` 等均未触碰（仍为他域 untracked 脏文件，未计入本包）。

## 1. 实际改动（before → after，SHA256 前 16 位）

| 文件 | before | after | 行数 | 动作 |
| --- | --- | --- | --- | --- |
| `front/app/api/normalizers/laneDayReading.ts` | `0b6a677034076bad` | `be0edcefe13f82c3` | 222 | 改：section 补 `clusterIndex`/`topicStatusAtReport`/`persistentTopicId`（边界归一） |
| `front/app/api/normalizers/laneDayReading.test.ts` | `ba1f6d605706ead7` | `a611f115cf9fbae4` | 195 | 改：+2 例（新字段透传 / 旧数据全 null） |
| `front/app/features/tags/composables/useLaneDayReading.ts` | `421efc75d66b387d` | `1e43b08ac2baebc4` | 495 | 改：索引暴露 + `ensureReportDay` + 双 epoch + 索引在途合乘 |
| `front/app/features/tags/composables/useLaneDayReading.test.ts` | `eea823d38330ba3f` | `5793fc3d2ffa855d` | 744 | 改：+19 例（RDC/报告优先/双 epoch/边界） |
| `front/app/features/tags/components/console/dayOtherTopics.ts` | 新建 | `23837cdb8621d3b4` | 211 | 新增纯模型 |
| `front/app/features/tags/components/console/dayOtherTopics.test.ts` | 新建 | `510a5908af1adcf4` | 344 | 新增 20 例 |
| `front/app/features/tags/components/console/laneConsoleView.ts` | `1960459067544bd7` | `f4c904dc149f2cda` | 180 | 改：`reconcileSelectedDate(..., reportDates?)` + `latestReportDay`/`reportDaysInAxis` |
| `front/app/features/tags/components/console/laneConsoleView.test.ts` | `13ad6b47ec5527ef` | `1226c21dda2cb0bf` | 280 | 改：+11 例 RDY，改 1 条 CH-1 旧断言（口径变更见 §5） |
| `front/app/features/tags/components/console/useConsoleSelection.ts` | `fa5f90d49d7c499f` | `712e4fb12aa1f0e3` | 80 | 改：可选 `reportDates` 依赖 + A 名单未到达 gate |
| `front/app/features/tags/components/console/useConsoleSelection.test.ts` | 新建 | `922610fd0189d66a` | 180 | 新增 8 例 |

未新增第二读取器/第二缓存：报日索引与报告优先读取全部跑既有 store 的 `index`（1 份）+ `reportCache`（≤14）+ `articleCache`（≤20）。

## 2. 新增接口与类型（G2/G3/G4 直接照此接线）

### 2.1 `useLaneDayReading`（单 store，G4 上提后由壳层注入）

```ts
// ── 报日索引（日期轴 / 窄屏报日条 / 报告优先读取共用 1 份） ──
ensureReportIndex(options?: { force?: boolean }): Promise<void>   // 生成完成后传 { force: true }
reportDates: ComputedRef<ReadonlySet<string>>                     // 真实 period_date 集合（可为空集）
indexLoading: Ref<boolean>                                        // 既有字段，语义不变
indexError: Ref<string | null>                                    // 索引失败/陈旧可重试（旧索引保留）

// ── 报告优先读取（只需日期，不依赖选中 lane / lane-dynamics 当日桶） ──
ensureReportDay(date: string, options?: { force?: boolean }): Promise<void>
reportDay: ShallowRef<LaneReportDay | null>
reportStale: Ref<boolean>          // 同日报日刷新失败保留旧内容

export interface LaneReportDay {
  date: string                     // 必须与调用方 selectedDate 比对后再渲染（display gate）
  status: 'loading' | 'ready' | 'no-report' | 'error'
  error: string | null
  errorSource: 'index' | 'detail' | null   // 索引失败≠无日报；详情失败只影响当日
  reportId: string | null
  sections: LaneDayReportSection[]         // 全量 sections（0 个 = 合法 ready）
}
```

口径要点（写进 G2/G4 断言时照此）：

- `ensureReportIndex` 的 lookback = `reportIndexDaysFor(pool首 = anchor-(POOL_DAYS-1) 或 today, today)`，**不用板块截止日冒充服务器 today**，停更差额自动扩大；已有更大覆盖不重复取。
- `ensureReportDay` 的 lookback = `requiredIndexDays(date)`（覆盖池首与目标日更早者）。
- `force` 语义：`ensureReportIndex({force})` 必重取列表（新报日可见，失败保留旧索引 + `indexError`）；`ensureReportDay(date,{force})` **只绕过详情成功缓存**，不动索引（索引刷新是壳层的独立动作）。
- 索引请求**在途合乘**：壳层启动 `ensureReportIndex()` 与编年首读并发时只发 1 次列表 GET（RDC-1）。
- `ensureReportDay` 不取消 lane 读取，反之亦然（`laneSeq`/`reportSeq` 独立）；切板块 `resetForBoard()` 同时作废两路并清 `reportDay/indexError/reportDates`。
- 跨日报日先清显示（`loading` + 空 sections），同日刷新失败才 stale 保留；晚到响应按各自 seq 丢弃。

### 2.2 normalizer（`api/normalizers/laneDayReading.ts`）

```ts
export interface LaneDayReportSection {
  sectionId: string
  clusterIndex?: number | null          // 新：报告 cluster_index（行序键）
  clusterLabel: string
  bucketTitle: string | null
  topicStatusAtReport?: string | null   // 新：原值透传（不 lower/不猜），未知由模型降级
  persistentTopicId?: string | null     // 新：数字 ID 字符串化；null/缺省 = 当次未归属
  threads: LaneDayThread[]
}
```

**三个新字段是 optional**：既有 typed fixture（如 G4 的 `LaneChronicle.test.ts`）不需要改即可通过 typecheck；未归一路径按 `null` 处理（`?? null`）。`snake_case → camelCase`、数字 ID → string 全部只在本边界。

### 2.3 `dayOtherTopics`（新纯模型，唯一排除口径）

```ts
buildOtherTopics(input: OtherTopicsInput): OtherTopicsView

interface OtherTopicsInput {
  selectedDate: string | null
  reportDay: LaneReportDay | null            // store 的 reportDay（date 不符即视为未读到）
  allLanes: readonly ObservationLaneRow[]    // A 名单 active + dormant
  resolveLaneDay: LaneDayResolver
  indexLoading: boolean
  indexError: string | null
  windowReportDayCount?: number | null       // 可见轴内报日数（0 → 「窗内没有日报」文案）
}

interface OtherTopicsView {
  state: 'no-date' | 'index-loading' | 'index-error' | 'loading' | 'no-report' | 'error' | 'ready'
  rows: OtherTopicRow[]      // { sectionId,label,clusterIndex,threadCount,persistentTopicId,statusKind,statusText }
  count: number | null       // 仅 ready 给数字（0 合法）；其余 null（不冒 0）
  statusText: string | null  // 非 ready 的状态文案；ready+0 行 = 合法 0 行句；ready 有行 = null
  note: string | null        // 失败技术文案 / 部分泳道未取到
  retryable: boolean
}
```

导出的稳定文案常量（G2/G5 直接引用，不要手抄字符串）：
`OTHER_TOPIC_STATUS_TEXT`（`当时已转正/当时待观察/当时已归档/当时状态未记录/当次未归属`）、`OTHER_TOPICS_EMPTY_TEXT`（`当日分区都已在泳道内可读`）、`OTHER_TOPICS_UNAVAILABLE_NOTE`、`OTHER_TOPICS_{NO_DATE,NO_WINDOW_REPORT,INDEX_LOADING,INDEX_ERROR,LOADING,NO_REPORT,ERROR}_TEXT`。

排除口径固定为并集：`sectionId ∈ 所选日任一 A 行 content 解析的 sections` ∪ `persistentTopicId ∈ A 名单 topicId`；行序 `clusterIndex` 升序（`null` 排末尾、相对序稳定）；计数 = `threads.length`（与 lane 聚合 `thread_count` 无关）。`ready` 之外任何状态行集恒为空、计数恒 `null`。

### 2.4 选日（`laneConsoleView` + `useConsoleSelection`）

```ts
reconcileSelectedDate(lane, axis, resolve, current, reportDates?: ReadonlySet<string> | null): string | null
latestReportDay(axis, reportDates): string | null
reportDaysInAxis(axis, reportDates): string[]        // 列头/窄屏报日条可选集合（轴保持连续）

// useConsoleSelection 新增可选依赖（G4 接线时传 store.reportDates）
interface ConsoleSelectionDeps { ...; reportDates?: { readonly value: ReadonlySet<string> | null } | null }
```

`reportDates` 三态语义（**重要**，G4 按需选）：

| 传入 | 含义 | 行为 |
| --- | --- | --- |
| 省略 | 未接线（旧调用/旧测试） | 完全保持旧口径（非内容日回最后内容日；无日期推导） |
| `null` | 已接线但索引未就绪/失败 | 当前日在轴内时**不搬走**（不把暂未知误报无日报） |
| `Set`（可空集） | 索引就绪 | 按真实报日校正（RDY-1..RDY-10） |

`useConsoleSelection` 额外 gate：`sourceWindow === null`（A 名单未到达）且尚无选中日时**不先按「0 泳道」选中最新报日**，否则名单到达后该报日会被规则 1 保留、默认日再也回不到「最后有分区日」。G4 实际顺序（`loadSource` 同 tick 写 `sourceWindow` + 自动选 lane）天然满足。

## 3. 字段来源与状态分流

| 展示/状态 | 唯一来源 | 分流 |
| --- | --- | --- |
| 日期列头可选 | `reportDates`（列表端点 `period_date`）∩ 可见轴（`reportDaysInAxis`） | 索引未就绪/失败 → 可选集空（G2 渲不可点 + 提示），不写「无日报」 |
| 组计数 | `dayOtherTopics.count`（= ready 行数） | 非 ready 一律 `null` |
| 行计数 | 详情 `section.threads.length` | `0` 合法可点 |
| 行标题 | 详情 `cluster_label` 逐字 | 不截断（clamp 由 G2） |
| 行状态词 | `topic_status_at_report` 原值 | `active/candidate/archived` → 三种「当时…」；其他/空 + 有 ID → `当时状态未记录`；无 ID → `当次未归属`；**不读当前 `topic.status`** |
| 组状态机 | `reportDay.status`/`errorSource` + `indexLoading`/`indexError` | `index` 失败 → `index-error`（可重试）；`detail` 失败 → `error`；`no-report` 只给「所选日没有日报」 |
| 身份排除 | `resolveLaneDay(topicId, selectedDate)` + `reportDay.sections[].persistentTopicId` | 泳道 `unavailable` 不判组失败，只追加备注句 |

## 4. 验证（本包实跑，Linux 本机直跑，串行）

红 → 绿轨迹（不把多轮数字相加，最终计数以最后一次为准）：

| 阶段 | 命令（`cd front` 后） | 结果 |
| --- | --- | --- |
| store 红 | `pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts --maxWorkers=2` | 20 绿 / **19 红**（`ensureReportIndex is not a function` 等） |
| `dayOtherTopics` 红 | `pnpm test:unit app/features/tags/components/console/dayOtherTopics.test.ts --maxWorkers=2` | 18 绿 / **1 红**（OTH-10 断言与本模型并集口径冲突，已按 spec「不把该泳道分区补进组」修正用例） |
| 选日红 | `pnpm test:unit app/features/tags/components/console/laneConsoleView.test.ts --maxWorkers=2` | 14 绿 / **9 红**（`reportDaysInAxis` 缺失等） |
| 壳层投影红 | `pnpm test:unit app/features/tags/components/console/useConsoleSelection.test.ts --maxWorkers=2` | 5 绿 / **2 红**（默认日被报日抢走） |
| **本包目标套件（绿）** | `pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/composables/laneDayReadingCache.test.ts app/api/normalizers/laneDayReading.test.ts app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/laneConsoleView.test.ts app/features/tags/components/console/useConsoleSelection.test.ts --maxWorkers=2` | **6 files / 106 tests passed**（2.99–3.11s） |
| eslint（仅本组 `.ts`，10 个文件） | `pnpm exec eslint <上述 10 个文件>` | **0 error 0 warning**（exit 0） |
| 相邻回归（只读跑，确认未破 G4 尚未改的壳/编年） | `pnpm test:unit app/features/tags/components/console/LaneChronicle.test.ts app/features/tags/components/console/BoardConsoleShell.test.ts --maxWorkers=2` | **2 files / 66 tests passed** |

细分计数：normalizer 9、useLaneDayReading 39、laneDayReadingCache 7、dayOtherTopics 20、laneConsoleView 23、useConsoleSelection 8。

未跑（按 brief 禁止）：全量 `pnpm test:unit`、`nuxi typecheck`、`build`、`lint` 全树、Go/浏览器/dev server、部署。**未声称** typecheck/build/Vue lint/后端任何一项通过。

## 5. 兼容策略与语义变更留痕

1. **旧调用零改动**：`ensureDayReading` 签名/返回不变；`reconcileSelectedDate` 第 5 参可选（4 参调用走 `undefined` = 旧口径）；`useConsoleSelection` 依赖可选；normalizer 新字段 optional。
2. **CH-1 旧断言改 1 条**（有意变更，非凑绿）：`laneConsoleView.test.ts`「当前日被范围排出」由「回最后有分区日」改为 `null`（未接线时）。依据：父裁决 P2「范围排出回窗内最新报日，无报日 null，不跳 today/active 日」；用例账本「继承与调整」已登记 CH-1 改语义。
3. **`suspendDisplay()` 现在递增 `laneSeq`**：此前注释承诺「失效在途显示」但实现只清 `current`，在途响应仍会重新发布。改为真正失效在途显示（成功缓存不动，重入即命中）。旧 R24 用例照绿。
4. `LaneChronicle.forceRefresh` 语义不变（`force` 仍绕过索引+详情成功缓存）；编年内网与旧 testid 未动。
5. `useLaneDayReading.ts` 495 行（+191）：仍是**单 store 单缓存**，未拆出第二实例/第二缓存；未新增 helper 文件（索引/报告的闭包耦合 `reportCache`/epoch，拆出会形成第二个状态所有者）。

## 6. 未覆盖 / 风险 / 各包接入方法

**未覆盖（不属本包）**

- G2：报日列头渲染与不可点文案、0 正式泳道的网格保留、组 DOM 顺序/默认态/只标所选日/clamp、`other-topics-toggle` testid。
- G3：`OtherTopicReading`、跨日观察带（section-timeline 只读依赖）、窄屏报日条。
- G4：store 上提注入（`LaneChronicle` 可选 `reading` prop）、`selectedOther`/`othersOpen` 与单一阅读槽清场、生成完成 `ensureReportIndex({force:true})`、返回上下文恢复 section/date、索引失败提示与重试入口。
- 真实库/浏览器：行集合 = 详情减泳道可读集合、`topic_status_at_report` 覆盖率、真实 0 行报日与多行报日（8C.5/8C.7）。

**风险**

1. `topic_status_at_report` 旧数据可能 NULL → 一律「当时状态未记录」；`persistent_topic_id` 缺省 → 「当次未归属」。不补写、不回填（符合迁移预期）。
2. 索引失败但旧索引仍可用时，`ensureReportDay` 对该旧索引中**存在**的报日仍会正常读详情；只有「旧索引里查不到该日」才升级为 `index-error`（不把陈旧索引当「无日报」）。G2/G4 断言「索引失败」请用 `indexError`/`index-error`，不要依赖 `reportDates.size === 0`。
3. `ensureReportDay` 的 `force` 不刷新索引 → 同日期被重新生成（reportId 变化）时，必须先 `ensureReportIndex({force:true})` 再读当日，否则详情仍是旧 reportId（G4 生成完成链路已按此设计）。
4. `useConsoleSelection` 的 A 名单未到达 gate 只在「尚未有选中日」时生效；若 G4 先写入 URL `?lane=`/pending date 再加载，则按规则 1 保留，不会被默认日覆盖。
5. 本包未跑 typecheck：`LaneDayReportSection` 新字段刻意 optional 以免 G4 既有 fixture 编译坏；若父在 8C.6 集中 typecheck 报错，优先看本报告 §2.2 的 optional 契约是否被改成必填。

**接入顺序建议**

1. G2 从 `dayOtherTopics.buildOtherTopics({...})` 取 `state/rows/count/statusText/note`，并只渲染 `reportDay.date === selectedDate` 的模型（display gate）。
2. G3 的观察带单独建只读依赖，**不复制**本 store 的索引/详情读取；阅读面板复用 `reportDay.sections` 中同 `sectionId` 的分区。
3. G4 上提单实例后：`ensureReportIndex()` 于板块首帧；`watch(selectedDate)` → `ensureReportDay(date)`（与编年 `ensureDayReading` 并行，索引自动合乘）；`selectedOther` 用 `sectionId + date` 双键，`selectedDate` 变化即清空。
