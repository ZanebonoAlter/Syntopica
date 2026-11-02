# G3 阅读、跨日观察与窄屏交付报告（tasks 8C.4）

> 线程：全新短线程、本写域唯一 writer（未 resume 旧线程、未派子线程、未提交/归档/部署、未换 provider/model）。
> 顺序：先写新例（API → hook → 阅读组件 → 窄屏），再实现；逐组跑目标 Vitest。**未跑**全量、typecheck、build、pnpm lint 全树、浏览器/agent-browser、Go/SQL/DB/AI/生成、dev 服务、部署。下述「绿」只指本页列出的命令，不外推。

## 1. 写域与实际改动（before → after，SHA256）

开工前实采既有文件 SHA、并确认 6 个新文件不存在（新文件 before = 不存在）：

| 文件 | before | after | 行数（before→after） | 动作 |
| --- | --- | --- | --- | --- |
| `front/app/api/otherTopicObservation.ts` | 不存在 | `5bb3017d03870e94…` | 0→128 | 新建：section-timeline 只读边界归一 |
| `front/app/api/otherTopicObservation.test.ts` | 不存在 | `160045488263919a…` | 0→127 | 新建 8 例 |
| `front/app/features/tags/composables/useOtherTopicObservation.ts` | 不存在 | `b2335a1ce59d5cce…` | 0→258 | 新建：板块单份只读观察依赖 |
| `front/app/features/tags/composables/useOtherTopicObservation.test.ts` | 不存在 | `8e1d17e374ec0105…` | 0→251 | 新建 10 例 |
| `front/app/features/tags/components/console/OtherTopicReading.vue` | 不存在 | `d7d7491079666ea9…` | 0→530 | 新建：单一阅读槽（纯受控） |
| `front/app/features/tags/components/console/OtherTopicReading.test.ts` | 不存在 | `04388e316e4feb10…` | 0→382 | 新建 8 例 |
| `front/app/features/tags/components/console/BoardNarrowSummary.vue` | `b9c783fc4bf777a1…` | `6dac4f20cd9daeca…` | 389→795 | 改：报日条/其他组/休眠组/0 正式泳道 |
| `front/app/features/tags/components/console/BoardNarrowSummary.test.ts` | `0ad1937678f2b77b…` | `9bd4c4727e1dcd14…` | 125→324 | 改：旧 4 例保留 + 新增 8 例 |
| `openspec/.../ui-verification/matrix-day-others-g3.md` | 不存在 | 本文件 | — | 新建证据 |
| `openspec/.../tasks.md` | `e29e141e3a74d670…` | `b03bbf8e16bed4c5…` | 166→167 | 仅 8C.4 勾选 + 验收结果 |
| `openspec/.../test-cases-matrix-day-others.md` | `20af274c8377b267…` | `b22a619671d4280e…` | 234→236 | NAR-1 改成条内滚动、补 G3 测试落点/实际命令 |

未触碰：G1/G2 源与测试、`BoardConsoleShell.vue`/`BoardConsoleStage.vue`/`consoleStageTypes.ts`/`LaneChronicle.vue`、`api/dailyReports.ts` 与既有 `api/normalizers/**`、`useReadingSettledSignal.ts`、后端/foreign/原型/静态产物/部署文件。`front/app/api/otherTopicObservation.ts` 只 import 既有 `useDailyReportsApi`，未改共享类型。

## 2. 真实读取边界与覆盖承诺（已核后端只读 handler/仓储）

- 复用既有 `GET /semantic-boards/:id/section-timeline?days=N`（`front/app/api/dailyReports.ts:286` 原样透出），后端零改动。
- 字段来源（`backend-go/internal/topicgraph/repository/daily_report_repository.go` 的 `SectionTimelineNode`）逐项消费：`id`（→字符串 sectionId）、`report_id`（→字符串）、`period_date`（RFC3339 → `YYYY-MM-DD`）、`cluster_label`（历史分区原题）、`thread_count`（缺失/非法 → null）、`persistent_topic_id`（→字符串，null = 当次未归属）。
- **刻意不映射**节点 nested `persistent_topic`（label/status/hit_count/can_activate）——它是**当前**话题属性，不是 section 生成时的历史快照；归一记录里不存在这些字段（单测断言 JSON 不含当前 label/status）。
- **覆盖承诺（关键实核，brief 与实际后端冲突处按真实 handler 记录）**：正数 `days` 会被后端**静默钳到 90 天**，窗口锚定该板块 `MAX(period_date)`（completed）；`days<=0`（含省略默认 0）= 100000 天 = **板块全部历史**。若按 poolBounds 计算 365/369 天正数请求，只会拿到 90 天且无法从记录反推覆盖（brief 也禁止按记录最小/最大日猜覆盖）。因此观察带产品路径**固定 `days=0`（全部历史）**，`coverage:'all'` 只描述请求口径；这才能真正覆盖现有 365 日观察池、停更差额不漏、不依赖名册 `sourceDays`。代价是首个其他行读取时一次较大 GET（见 §8 风险）。
- 端点返回的 `relations` 与当前话题 nested 属性一律不参与观察（只按稳定 `persistent_topic_id` 关联历史 section 记录）。

## 3. 读取隔离（板块单份 / late guard / 失败重试 / 覆盖）

`useOtherTopicObservation`（一个实例 = 一个板块作用域，G4 必须在壳层创建一次并注入，不得在阅读面板挂载时新建）：

- **单份成功数据**：`dataset`（全部话题的历史 section 记录）每板块 1 份；换 topic 只换 `nodes` 投影，0 额外 GET；拖观察范围/预览只按 `axis()` 重投影，**不发请求**。
- **epoch/seq**：`boardEpoch`（切板块/卸载复位）+ `requestSeq`（每次请求递增）。旧响应晚到不写 dataset/error/loading；同板块强刷时旧响应不得覆盖更新的强刷结果；loading 归最新请求。
- **在途合乘**：非 force 且同板块在途 → 直接 await 同一 promise；force 启动新请求超越旧请求。
- **失败**：首败 `status:'error'` 可重试（失败不入成功缓存）；已有成功数据时刷新失败保留旧数据并 `stale:true` + error 文案（明确陈旧，不冒空/无记录）。
- **无可靠 ID**：`ensureObservation(null)` 只置 `missing-id`，**不发请求**；这是「缺身份」不是「无记录」。
- **投影口径**：只取 `persistentTopicId === 请求 topicId` 且 `date ∈ axis` 的真实记录日；同日多分区保持接口原序，节点 `threadCount` = 同日峰值（任一分区缺失 → null 未知，不求和/不补 0）；`reportId/sectionId` = 同日首个分区（稳定目标），`sectionIds` 全量原序，供 G4 切日后按 ID 核对详情。
- 与 G1 单 store 的边界：本 composable **不建**日报 index/detail 读取器；阅读正文/相关文章仍由注入的 `useLaneDayReading` 单 store 服务，两路 epoch 互不干扰。

## 4. 阅读槽接口（G4 直接照此接线）

### 4.1 `OtherTopicReading.vue`

```ts
props: {
  selectedDate: string | null
  selectedSectionId: string | null          // selectedOther.sectionId
  reportDay: LaneReportDay | null           // G1 store.reportDay（组件内再按 date gate）
  reading: LaneDayReadingStore              // 壳层上提的单 store（相关文章/force 刷新）
  observation: OtherTopicObservationStore   // 壳层创建的板块级观察实例
  topicId?: string | null                   // G2 selectOther 的 row.persistentTopicId；null=当次未归属
}
emits: {
  openReport: [{ date: string, reportId: string | null }]
  viewArchive: [date: string]
  openArticle: [articleId: string]
  selectObservedDay: [{ date: string, sectionId: string, reportId: string, topicId: string }]
  readingSettled: [{ laneKey: string, date: string, status: string, sectionId: string, topicId: string | null }]
}
```

- 正文：分区 `clusterLabel` 逐字、全量 threads；复用 `LaneChronicleThread`（含 `LaneRelatedArticles`），单条互斥展开 `summary` 逐字、空摘要「暂无摘要记录」；0 线索合法句。
- 状态分层：`pending`（未选/日期不匹配/详情加载中）、`no-report`、`error`（可重试）、`missing-section`（「当日报告已更新，未找到该分区」+ force 刷新）、`ready`；全部不借别日/相似标题。
- `readingSettled`：**阅读状态终态 + 观察依赖终态（非 idle/loading）后**才发，且不等待未展开的相关文章、无固定 sleep、不主动滚动。`laneKey = 'other:<sectionId>'`（与泳道 topicId 命名空间区分，G4 可直接填既有 `PendingScroll.lane` 做 R34 兑现）；`status ∈ ready|missing-section|no-report|error`。
- 未改 `useReadingSettledSignal.ts`（其 deps 是 lane 专属类型，跨域改签名需父裁决；本组件用同协议本地 computed+watch，不触碰该 helper）。

### 4.2 `BoardNarrowSummary.vue`（新 props/events 全部可选；旧 Stage 未接时不破）

新增 props：`reportDates?: ReadonlySet<string> | null`、`selectedDate?: string | null`（报表条高亮/点行携带日期，必需）、`indexLoading?: boolean`、`indexError?: string | null`、`otherTopics?: OtherTopicsView | null`、`othersOpen?: boolean`、`selectedOtherSectionId?: string | null`。
新增 events：`selectDayHeader(date)`、`toggleOthers()`、`selectOther({ sectionId, date, topicId? })`。旧 props/events（`selectLane/toggleDormant/retry/generate/gotoChronicle/openContext`）与 `narrow-dormant-toggle`、`bns-cell` 迷你条选择器保留。

G4 接线示例：

```vue
<BoardNarrowSummary
  ...既有 props...
  :report-dates="reportDates"
  :selected-date="selectedDate"
  :index-loading="reading.indexLoading.value"
  :index-error="reading.indexError.value"
  :other-topics="otherTopics"
  :others-open="othersOpen"
  :selected-other-section-id="selectedOther?.sectionId ?? null"
  @select-day-header="date => selectDate(date)"
  @toggle-others="othersOpen = !othersOpen"
  @select-other="p => { selectedOther = p }"
/>

<OtherTopicReading
  v-if="selectedOther"
  :selected-date="selectedOther.date"
  :selected-section-id="selectedOther.sectionId"
  :report-day="reportDay"
  :reading="reading"
  :observation="observation"
  :topic-id="selectedOther.topicId"
  @select-observed-day="p => { selectedOther = { sectionId: p.sectionId, date: p.date, topicId: p.topicId }; selectDate(p.date); void reading.ensureReportDay(p.date) }"
  @open-report="...现有阅读台路由..."
  @view-archive="..."
  @open-article="..."
  @reading-settled="readingSettled = $event"
/>
```

G4 仍需自担（不在本包）：`selectedOther`/`othersOpen` 归属与清场（selectedDate/范围排出/切板块/切泳道清空）、首份 A 响应初始化一次默认态、观察实例在壳层创建一次（`useOtherTopicObservation({boardId, axis: () => visibleAxis.value})`）、生成完成后 `observation.retry()`（有选中其他分区时强制刷新）、返回载体恢复 `board/report/section/date + 阅读槽`（旧载体兼容、不新增 `date=` query）、把其他阅读 `laneKey='other:<sectionId>'` 接入既有 R34 pending 语境（`useConsoleReturnContext.readingReady` 只比 `lane/date`）。用户意图取消沿用既有 wheel/touch/pointer/keydown 监听（本组件不注册滚动/不抢焦点）。

## 5. 窄屏行为（同源，不是第二套导航）

- 报日条 = 同一连续 `visibleAxis`；只有 `reportDates` 内日期渲染为原生 `BUTTON`（`aria-pressed`），非报日保留为 `SPAN aria-hidden`（不压轴、不可聚焦）；不新增日历/第二日期条/独立 picker。索引加载/失败只给说明，不冒「无日报」。
- 选中日定位：`onMounted` + `watch(selectedDate, axis.length, reportDates)` → `nextTick` 后测 `scrollWidth/clientWidth/offsetLeft/offsetWidth`，**只写日期条自身 `scrollLeft`（居中 + clamp）**；无固定 sleep、无 `scrollIntoView`、无 `window.scrollTo`、不改纵向滚动/焦点。单测用可控几何 + 双向 clamp 断言，且断言两个全局滚动 API 未被调用。
- 组顺序：正式行（`activeRows`，按 A 组 `row.dormant` 分流，不重算休眠阈值）→ 当日其他话题（summary + 自身行）→ 休眠泳道（summary + 自身行）；两个 toggle 独立（`othersOpen`/`expandedDormant` 全由 G4 持有，组件不初始化）。
- 0 休眠仍保留 summary「休眠泳道 · 0 · 本窗无」并禁用展开、不造行；0 正式泳道不再只剩空态：保留报日条 + 其他组 + 旧空态降为行内说明；其他行两行 clamp + 完整 `title`/`aria`（含当时状态词与「点下方读全文」）、只显示所选日线索数（0 合法可点），行内不堆第二行工程元信息。
- 其他组计数/状态/行集全部来自 G1 `OtherTopicsView`，窄屏只做同源简单渲染，不复制排除/状态词/计数公式。

## 6. 验证（Linux 本机串行，参数不带 `--`）

红 → 绿轨迹（各阶段最终计数以最后一次为准；不把多轮数字相加）：

| 阶段 | 命令（`cd front`） | 红 | 绿 |
| --- | --- | --- | --- |
| API 边界 | `pnpm test:unit app/api/otherTopicObservation.test.ts --maxWorkers=2` | 模块不存在（`1 failed / no tests`） | **8 passed** |
| 观察 hook | `pnpm test:unit app/features/tags/composables/useOtherTopicObservation.test.ts --maxWorkers=2` | 未单独捕获：测试先写、实现在同一编辑段落下补齐（不虚称红） | **10 passed** |
| 阅读组件 | `pnpm test:unit app/features/tags/components/console/OtherTopicReading.test.ts --maxWorkers=2` | 模块不存在；实现后 **1 failed / 7 passed**（用例选择器把 thread 容器与 toggle 都算入，6≠3），修选择器为 `.lct` 后绿 | **8 passed** |
| 窄屏 | `pnpm test:unit app/features/tags/components/console/BoardNarrowSummary.test.ts --maxWorkers=2` | **7 failed / 5 passed**（报日条/其他组/0 休眠 summary 均不存在；旧 4 例 + legacy 1 例绿） | **12 passed**（旧 4 + 新 8） |

最终目标集合（一次跑）：

```
cd front && pnpm test:unit app/api/otherTopicObservation.test.ts app/features/tags/composables/useOtherTopicObservation.test.ts app/features/tags/components/console/OtherTopicReading.test.ts app/features/tags/components/console/BoardNarrowSummary.test.ts --maxWorkers=2
→ 4 files / 38 tests passed
```

细分：API 8、hook 10、阅读组件 8、窄屏 12。

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| 相邻直接消费者回归（未改其源） | `pnpm test:unit app/features/tags/components/console/BoardConsoleShell.test.ts --maxWorkers=2` | **1 file / 38 tests passed**（旧壳/矩阵/窄屏旧 props 未破） |
| 本组 `.ts` eslint（6 个文件：两个新源+两个新测试+两个测试） | `pnpm exec eslint app/api/otherTopicObservation.ts app/api/otherTopicObservation.test.ts app/features/tags/composables/useOtherTopicObservation.ts app/features/tags/composables/useOtherTopicObservation.test.ts app/features/tags/components/console/OtherTopicReading.test.ts app/features/tags/components/console/BoardNarrowSummary.test.ts` | **exit 0**（0 error 0 warning；首跑 1 个 unused import warning 已删） |

**Vue 文件未被 eslint 配置覆盖，不声称 Vue lint 绿**；`.vue` 只经 Vitest 的 SFC 编译（模板/脚本可编译），不等于 `nuxi typecheck` 通过。

用例覆盖锚点（对应 `test-cases-matrix-day-others.md` S11-10/11/12/15/16/17/18 与 ORD/OBV/NAR 白盒）：分区原题/全量线索/收束与互斥展开、空摘要与 0 线索、display gate 跨日不挂旧 sections、no-report/error/missing-section + 重试与 force、相关文章按需与同文章跨线索复用、打开日报/归档完整日期、观察带缺身份/0/1/多日/峰值/未知/窗外/其他 ID、节点 `selectObservedDay` 稳定目标、观察失败不阻正文与 settled 等终态、窄屏连续轴只报日可选/非报日非交互、仅条内 scrollLeft 双向 clamp 且不 scrollIntoView/window.scrollTo、两 toggle 独立、0 休眠 summary、0 正式泳道不整块空态、旧 props/events 未接线兼容。

## 7. 字段来源与「不冒历史」声明

| 展示字段 | 唯一来源 | 降级/边界 |
| --- | --- | --- |
| 分区标题 | 当日报告详情 `section.cluster_label`（经 G1 normalizer） | 缺失 = missing-section 中性提示，不借相似标题 |
| 线索标题/摘要 | `section.threads[].title/summary` 逐字 | 空摘要 = 「暂无摘要记录」；0 条 = 合法空列表 |
| 相关文章 | `thread.related_article_ids` → 注入 store 的 ≤20 缓存 | 空 ids = 「相关文章列表为空」；单条失败局部重试 |
| 观察节点日期 | section-timeline 历史 `period_date` ∩ 当前 `axis` | 窗外/其他 ID 不进入；无 ID = missing-id；未加载/失败 = loading/error，不补 0 |
| 观察节点计数 | 同 ID 同日分区 `thread_count` 峰值 | 任一分区缺失 = `?`（未知），不求和、不累计命中、不冒 0 |
| 观察节点目标 | 同日接口原序首个分区的 `report_id`/`id` | 报告改写/缺分区由 G4 按 ID 核对后给中性提示，不借标题补 |
| 观察文字 | 「同归属记录日」+「不称同一事件/正式泳道」 | 1 日只说明；0 日「本窗没有…记录日」 |
| 报日条可点 | G1 `reportDates`（真实 period_date 集合）∩ 连续轴 | 索引未就绪/失败 → 不可点 + 说明；不冒无日报 |
| 其他行计数/状态词 | G1 `OtherTopicsView`（报告详情 threads.length / `topic_status_at_report`） | 窄屏不重算；0 合法；状态词不读当前 topic |

## 8. 未验 / 风险 / 需披露的偏差

1. **`days` 口径偏差（有意，已核后端）**：brief 写「用 poolBounds/起止与 today 计算实际 days」，但 handler 对正数 `days` 静默钳 90 天，无法覆盖 365 池；本包按真实端点语义固定 `days=0`（全部历史），并在 API 边界头注释/测试/本报告留证。若父坚持计算正数 days，需要后端先支持 >90 天窗口（本包禁改后端），或接受观察带只覆盖最近 90 天（与 spec「本窗真实记录日」冲突）。
2. **payload 体积**：板块全部历史 section-timeline 在成熟板块可能较大（一次 GET/板块，懒触发；无每像素请求）。jsdom/happy-dom 无网络体积验证；真实库负载留 G5/父核。
3. **真实几何/触屏**：报日条居中与 clamp 只在 happy-dom 用覆盖几何断言；真实 375/390 触摸横滑、长轴（365 天）、双主题视觉、`elementFromPoint`/焦点回归留 G5（本包禁浏览器）。`BoardNarrowSummary.vue` 795 行（样式为主，含旧的泳道卡/迷你条），未拆分；若 review 要求可按「G3 专属最小子组件」拆，但当前无逻辑复制。
4. **无真实多日样本**：OBV 只做夹具/组件证据，未在真实库核对某分区本窗多记录日与峰值；S11-C 留 8C.7，不冒真库通过。
5. **G4 未接线**：`OtherTopicReading` 尚未挂载、`selectedOther/othersOpen` 未持有、返回恢复/生成后强刷/索引失败重试入口未实现；默认展开态、仅一个观察实例、`laneKey='other:<id>'` 的 R34 语境接入都是 G4 义务。当前窄屏未接新 props 时按旧结构渲染（兼容），但 0 休眠 summary 已恒在场（预期可见变化，待 G4/G5 复核）。
6. **hook 无独立红**：测试先写，实现与测试同一编辑段落补齐，未单独捕获失败断言；不把该阶段称作红→绿。API/组件/窄屏均有实捕红。
7. **未跑**：全量 `pnpm test:unit`、`nuxi typecheck`、`pnpm build`、全树 lint、浏览器、Go/SQL/AI/生成/业务写、部署、commit。`typecheck` 由父 8C.6 集中跑（本组新类型：optional `ReadonlySet<string> | null`、`ObservationSectionRecord`、`selectOther/selectObservedDay` payload、`readingSettled` 扩展字段）。

## 9. tasks 勾选依据

- 8C.4：本页 §6 目标 4 文件 38 例 + 相邻壳 38 例 + 6 个 `.ts` eslint exit 0 真实绿，已勾；**不代表 UI 验收/整 change 完成**，8C.5–8C.8 不动。
- 其他阶段（8C.1/8C.2/8C.3/8C.5–8C.8、9.x、10.x、11.x）未勾、未改。

## 10. G3 收口增量（2026-10-29，全新短线程；上文 §1–§9 为历史交付记录，不覆盖）

父复核 `matrix-day-others-g3-review.md` 的 G3-M1/M2/S1 已在本轮关闭，完整证据（before/after SHA、红→绿、拆后每文件行数/字节、G4 交接签名与时序）见 [`matrix-day-others-g3-closure.md`](matrix-day-others-g3-closure.md)。本节只留最新接口，替换 §4 中过时的重试/settled 描述：

- `OtherTopicReading` 新增 emit **`retryReport: [{ date: string, sectionId: string }]`**：错误态「重试」与缺分区「刷新」都只发意图，组件不再自读旧 reportId。G4 顺序固定：`ensureReportIndex({force:true})` → `ensureReportDay(date,{force:true})` → 按稳定 sectionId 核对详情。
- `readingSettled` 载荷不变（`laneKey/date/status/sectionId/topicId`），但**时序**改为：挂载 + 一次 nextTick（DOM 已按终态更新）+ 报告终态 + 观察依赖真实非刷新终态后才发；观察强刷在途中不发，刷新落定后再发一次；换上下文/卸载旧排队不发。
- 观察 hook：`view.refreshing`（已有数据强刷在途）、`invalidate()`（生成后懒失效，不发 GET；下次 `ensureObservation` 才重读）。生成后无选中其他行时只调 `invalidate()`，不要 `retry()`。
- `loadBoardSectionTimeline(boardId)` 恒定 `days=0`（全部历史），签名不再接受窗口 days；`ObservationResponse<T>` 为 `ApiResponse<T>` alias。
- S1 拆分：`OtherTopicReading` 530→366 行（观察带 `OtherTopicObservationBand` + 状态块 `OtherTopicReadingStates`）；`BoardNarrowSummary` 795→385 行（`BoardNarrowDayStrip` / `BoardNarrowLaneList` / `BoardNarrowGroupToggle` / `BoardNarrowOtherGroup`）；`BoardNarrowSummary.test.ts` 未改、12 例全绿。
- 本包验证：目标 4 files/49 tests、相邻 Shell 38 tests、7 个 `.ts` eslint exit 0；未跑 typecheck/build/浏览器/全量/Go/部署。
