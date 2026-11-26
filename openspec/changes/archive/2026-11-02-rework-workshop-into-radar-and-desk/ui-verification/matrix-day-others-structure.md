# 8C.4S 结构规范包报告（G4 前纯重构 · 无新业务/布局）

> 2026-10-29 单 writer 短线程实跑。**结论：本包目标门禁全绿，8C.4S 可勾**——但仅代表「结构上限 + 行为回归 + 用例清单一致」，**不代表 UI 验收、真实像素/双主题、G4 接线或整 change 完成**。
> 依据：`standard/frontend/code-style.md` §8（内聚拆分 / 小 Interface / ≤500 行与 ~15KB）、`standard/shared/test-design.md`（断言与用例名保留、不删用例凑绿）、`implementation-matrix-day-others.md` §4.4/§6。

## 1. 范围与红线

改了什么（全部在本包授权辖区）：

| 类别 | 文件 |
| --- | --- |
| G1 组合根 | `front/app/features/tags/composables/useLaneDayReading.ts` |
| G1 新 helper（4） | `laneDayReportIndex.ts`、`laneDayReportDay.ts`、`laneDayCurrentReading.ts`、`laneDayRelatedArticles.ts` |
| G2 网格 | `front/app/features/tags/components/console/ObservationMatrixGrid.vue` |
| G2 新子组件（2） | `ObservationMatrixDayHeader.vue`、`ObservationMatrixLaneRow.vue` |
| 测试拆分 | `useLaneDayReading*.test.ts`（5）、`ObservationMatrix*.test.ts`（3）、`OtherTopicReading*.test.ts`（3） |
| 测试 harness（3，非 `*.test.ts`） | `laneDayReadingTestHarness.ts`、`observationMatrixTestHarness.ts`、`otherTopicReadingTestHarness.ts` |

**未碰**（红线核对）：G1 其它源（`api/normalizers/laneDayReading.ts`、`console/dayOtherTopics.ts`、`laneConsoleView.ts`、`useConsoleSelection.ts`、`laneDayReadingCache.ts`）、G2 `ObservationMatrix.vue`/`DayOtherTopicsGroup.vue`、G3 运行时源（`OtherTopicReading.vue`/`BoardNarrowSummary.vue`/`useOtherTopicObservation.ts`/`api/otherTopicObservation.ts`）、G4 `BoardConsoleShell.vue`/`BoardConsoleStage.vue`/`consoleStageTypes.ts`/`LaneChronicle.vue`、后端/foreign/原型/`docs/reference/**`/部署。`OtherTopicReading` 改的**只有测试**。

行为红线继承不变（结构包不重算任何公式）：A 名册/排序/休眠由 source 档、B 只补同 ID/裁切、正式 max/演进 sum；单板块 1 份报日索引、详情 14 / 文章 20 FIFO、在途合乘、失败不入成功缓存、lane/report 独立 epoch、force 有效覆盖取 max（R3）、索引请求序（R1）、相关文章跨板块晚到不发布（R2）。**没有第二份 cache / 第二个 store / 每次 get 新建 factory**：父 store 仍创建一次并委托。

## 父回包核验

父直接读取保存的before.json/after.json重新派生：两者189 passed、failed/pending均0；按`status + '|' + fullName`排序换行编码的SHA256两者同为`f990f5e7ada354b7a7eb5e2b215e24c0d66161c399b7c862fd747faf496a4658`。此编码与下表子报告序列化方式不同，不拿摘要值交叉混比；真实前后多重集一致。父未再跑全189，不冒称自己重跑。

`test-cases-matrix-stage.md`两处CA测试落点同步属于保持旧资产可追溯的必要小hunk，父认可该有限补正，不扩其他旧合同/计数/状态。G4a/G4b将继续全新短线程串行，旧Shell体量在G4b处理，本结构包不抢写。

## 2. 前后 SHA / 行 / 字节（全部 ≤500 行 且 ≤15360B）

| 文件 | before | after | 行/字节 before → after |
| --- | --- | --- | --- |
| `composables/useLaneDayReading.ts` | `4c6e309130d62554…`（前缀） | `f55f981830b76126…` | 522/25292 → **169/7913** |
| `composables/laneDayReportIndex.ts` | 不存在 | 新增 `78d053b3c70254b8…` | 0 → **181/9486** |
| `composables/laneDayReportDay.ts` | 不存在 | 新增 `1d213495cac45893…` | 0 → **141/6961** |
| `composables/laneDayCurrentReading.ts` | 不存在 | 新增 `b9b1ce308146df33…` | 0 → **177/8256** |
| `composables/laneDayRelatedArticles.ts` | 不存在 | 新增 `01536580b0e36247…` | 0 → **109/5199** |
| `composables/useLaneDayReading.test.ts` | `1bdef5ab51a20141…` | `22eca0dd83883bf4…` | 917/50337 → **226/14019** |
| `composables/useLaneDayReading.reportIndex.test.ts` | 不存在 | 新增 `157a0e2ee38e4029…` | 0 → **238/13154** |
| `composables/useLaneDayReading.reportDay.test.ts` | 不存在 | 新增 `9c803d9f9ff250ac…` | 0 → **261/14488** |
| `composables/useLaneDayReading.cache.test.ts` | 不存在 | 新增 `e7c28792d785d6ad…` | 0 → **125/6135** |
| `composables/useLaneDayReading.relatedArticles.test.ts` | 不存在 | 新增 `008bc4ec65d4e9ae…` | 0 → **65/3121** |
| `composables/laneDayReadingTestHarness.ts` | 不存在 | 新增 `8e00c9c46635b0ce…` | 0 → **116/4700** |
| `console/ObservationMatrixGrid.vue` | `b6695bc5f8bca1871277cced8a7aac01cbd20a95f2b078f272684fd03a5a2726` | `c9b1d75031aefd65…` | 646/21457 → **447/15224** |
| `console/ObservationMatrixDayHeader.vue` | 不存在 | 新增 `fbfbd158c4df86c6…` | 0 → **151/5077** |
| `console/ObservationMatrixLaneRow.vue` | 不存在 | 新增 `e829809094cd7558…` | 0 → **212/6012** |
| `console/ObservationMatrix.test.ts` | `a8714644e1b0b539b34a2c59ca9e403a35659bb142f86d6246616d5416fb2dcb` | `3cdf1bd3dd51fd99…` | 587/29954 → **267/13545** |
| `console/ObservationMatrix.dayHeaders.test.ts` | 不存在 | 新增 `57df792c22cde990…` | 0 → **90/4985** |
| `console/ObservationMatrix.groups.test.ts` | 不存在 | 新增 `1873db8b54d4e602…` | 0 → **163/9235** |
| `console/observationMatrixTestHarness.ts` | 不存在 | 新增 `43ed7900b25f324e…` | 0 → **133/5552** |
| `console/OtherTopicReading.test.ts` | `fcbdb5ba3bab4db2…` | `6014264c605bd46d…` | 589/26300 → **185/9000** |
| `console/OtherTopicReading.observation.test.ts` | 不存在 | 新增 `32e980feb141abb7…` | 0 → **159/6793** |
| `console/OtherTopicReading.signals.test.ts` | 不存在 | 新增 `563c41912131798c…` | 0 → **255/10902** |
| `console/otherTopicReadingTestHarness.ts` | 不存在 | 新增 `6d5c2eb81c86e9c5…` | 0 → **142/5360** |

- before 值：本包开工时实采（sha256 前 16 位，与派发 brief 记录一致）。说明：这些文件在当前 worktree 里**尚未入 git**（`git ls-files` 为空），故 before 不能从 HEAD 复算；除已登记在 `matrix-day-others-g2.md`/`g3-closure.md` 的全量 digest 外，只能保留开工实采前缀。未用 HEAD/stash/reset 重建任何文件。
- after 全量 sha256（实盘计算，可复核；路径相对 `front/app/features/tags/`）：

```text
f55f981830b761267358e8c6023a578ef46769230de0e5884a388843f39e75c2  composables/useLaneDayReading.ts
78d053b3c70254b8084f76c8767f7f8f58ec3cf5315b416265fbad106086942b  composables/laneDayReportIndex.ts
1d213495cac45893a010e3a3659f9b1973d62a7cac6be00986d108a9d8955829  composables/laneDayReportDay.ts
b9b1ce308146df3368b01476eafb24b6de7ef1e4046ba81ce6a89c841d18a32c  composables/laneDayCurrentReading.ts
01536580b0e3624792430584fbc9f8118c5f6ac6d0ef6909e1b2291b57d1e6af  composables/laneDayRelatedArticles.ts
22eca0dd83883bf42567ec7111382fef3aa2c0ad694864e4b4f71a8791710979  composables/useLaneDayReading.test.ts
157a0e2ee38e4029276e156c9220e0fb68308a05443d644a1425d891f2f2adb0  composables/useLaneDayReading.reportIndex.test.ts
9c803d9f9ff250ac2b18a69be82beb5fc18bc57efe26702103622b1848ec2756  composables/useLaneDayReading.reportDay.test.ts
e7c28792d785d6adf7ff8f7788cd5504b41e146b2120cdb7a00c6b50cb85992c  composables/useLaneDayReading.cache.test.ts
008bc4ec65d4e9aefc4fd7863172a77a82e0374b28392d42de8f32a54524f850  composables/useLaneDayReading.relatedArticles.test.ts
8e00c9c46635b0ce6b0558500840bc066f3e3e3dd6e73ea85cbd4488d207ead3  composables/laneDayReadingTestHarness.ts
c9b1d75031aefd65c0286665bead8b33d73312b076de1d1401f1c9e23e2c9589  components/console/ObservationMatrixGrid.vue
fbfbd158c4df86c6152e593e8145301695ed31e7133f22f30af6ba3f4e34fb82  components/console/ObservationMatrixDayHeader.vue
e829809094cd7558eb08445b74118a6796012b2e66623270b06cc988f71f57b1  components/console/ObservationMatrixLaneRow.vue
3cdf1bd3dd51fd995028dbc8c9abc087509ede673b8fce304714c6dd246f9c7a  components/console/ObservationMatrix.test.ts
57df792c22cde990be36e51e4f0476d319afb2c1042ca020cec2353f3e610fb3  components/console/ObservationMatrix.dayHeaders.test.ts
1873db8b54d4e602e50b4ae7af8abe007a3b15d266b704c932d8027b9f686fc1  components/console/ObservationMatrix.groups.test.ts
43ed7900b25f324ee91755f44d9b06f0c661090b0445cd85be48382336f095fd  components/console/observationMatrixTestHarness.ts
6014264c605bd46d8f02d001253d434207a4194f767db4d1baa8397e2f1bdf47  components/console/OtherTopicReading.test.ts
32e980feb141abb7800cacb60ca10bbc2b55171e9c1ab04cbc79100814a6c2dd  components/console/OtherTopicReading.observation.test.ts
563c41912131798ce89089576c378ad96b2ddfab46b1b07d849185b13d6317d7  components/console/OtherTopicReading.signals.test.ts
6d5c2eb81c86e9c5359160cb3578c89878ff05d81c2ed398c9525ebbb7749cc4  components/console/otherTopicReadingTestHarness.ts
```

## 3. 内聚拆分职责（谁拥有什么）

### 3.1 G1 `useLaneDayReading` = 组合根（仍唯一状态所有者）

| 模块 | 拥有 |
| --- | --- |
| `useLaneDayReading`（本文件） | fetcher 注入、**板块代际 `boardEpoch`**（所有晚到判定的统一失效源）、两份有界缓存（详情 14 / 文章 20）、`resetForBoard` 统一复位编排、对外返回键（原样 21 键） |
| `laneDayReportIndex` | 索引 1 份/板块：`index`/`indexLoading`/`indexError`/`reportDates`、在途合乘槽、请求序 `indexSeq`、覆盖（`poolStartDate`/`reportIndexDays`/`requiredIndexDays`/R3 有效 days=max）、`ensureReportIndex`、`reset` |
| `laneDayReportDay` | 报告优先读取：`reportSeq`、`reportDay`/`reportStale`、`ensureReportDay`（索引→精确 period_date→同一详情缓存→分层状态）、同日 stale 保留、`reset` |
| `laneDayCurrentReading` | 编年当前日：`laneSeq`、`currentContext`（laneKey/date/sectionKey）、`current`/`loading`/`error`/`staleNotice`、`ensureDayReading`（join 降级）、`suspendDisplay`、`reset` |
| `laneDayRelatedArticles` | 相关文章条目状态机（idle/loading/success/error）、跨板块/缓存失效代晚到不发布、单项重试、`getArticlePayload`、文章缓存清空（与条目同生命周期） |

`resetForBoard` 顺序即失效序：`boardEpoch += 1` → `index.reset()` → `currentReading.reset()` → `reportDay.reset()` → `related.reset()`（含 `articleCache.clear()`）→ `reportCache.clear()`。与拆分前逐项等价（同一组 ref/计数器被清，只是文件边界变了）。

依赖方向：四个 helper 只吃**小 Interface deps**（`boardId`/`today`/`anchorDate`/`loadIndex`/`requiredIndexDays`/`detailCache`/`articleCache`/`boardEpoch`），不 import UI、不 import 父 store、不互相 import。

### 3.2 G2 `ObservationMatrixGrid` + 两个受控子组件

| 文件 | 拥有 |
| --- | --- |
| `ObservationMatrixGrid.vue`（父） | 网格容器与横滚/列定义样式、逐可见日投影（`project`/`activeEntries`/`dormantEntries`）、`indexHint` 内联提示、**点格可开性判定与禁用格提示（唯一出口）**、休眠 summary、`DayOtherTopicsGroup` 接线、对「其他话题组」行的 `:deep()` 视觉覆盖 |
| `ObservationMatrixDayHeader.vue` | corner + 列头（多根片段，无 wrapper）：`reportDates` 三态（省略/null/Set）、`title`/`aria` 文案、可点报日按钮与 `selectDayHeader`、列头 CSS（8 条规则原样迁移） |
| `ObservationMatrixLaneRow.vue` | 单行（标签按钮 + 逐日格，多根片段）：正式/休眠共用（`dormant` 只切弱化与「休眠」标）、格值/可开性/title/aria 全由父投影好的 `cells` 携带、行/格 CSS（20 条规则原样迁移） |

- 子组件**只 props/emits**，不自建网络/日期/名册分组；点格先 emit `selectCell({row,cell,date})` 回父层，父层再判定/提示后 emit `selectDate` —— 与拆分前同一出口（`MX-4 断点格不 emit、不弹浮层` 照旧）。
- **不新 wrapper**：两个子组件都是 Vue 3 多根片段，元素仍是网格的直接子节点，`sticky` 标签列与 `grid-template-columns` 对齐不受影响（本轮全量跑无 attribute-fallthrough 警告）。

## 4. 公共接口 / DOM 可观察行为不变的证据

1. **G1 store 返回键（21 个，顺序也保持）**：`current, loading, error, staleNotice, indexLoading, ensureDayReading, suspendDisplay, reportDates, indexError, ensureReportIndex, reportDay, reportStale, ensureReportDay, relatedEntries, readRelated, ensureRelatedArticles, retryRelatedArticle, getArticlePayload, resetForBoard, reportCacheCount, articleCacheCount, indexDays`。`UseLaneDayReadingDeps`/`LaneDayReadingFetchers`/`REPORT_DETAIL_CACHE_LIMIT=14`/`ARTICLE_CACHE_LIMIT=20`/`LaneDayReadingStore` 仍在原模块导出；搬走的类型（`LaneReportDay`/`ReportDayStatus`/`ReportDayErrorSource`/`ReportIndexState`/`RelatedArticleEntry`/`LaneDayCurrentReading`/`LaneDayReadingContext`）在本模块 `export type … from` 再导出，`dayOtherTopics.ts`、`OtherTopicReading.vue`、`LaneChronicleBanners.vue`、`LaneChronicleThread.vue`、`LaneRelatedArticles.vue` 的既有 import 路径与名字零改动。G4 需要的「父创建一次 + `LaneChronicle` 可选 `reading` 注入」约定不受影响（未触碰壳/编年）。
2. **G2 Grid 公共契约**：14 个 props（`axis/activeRows/dormantRows/selectedLaneId/selectedDate/today/resolveDay/expandedDormant/reportDates?/indexLoading?/indexError?/otherTopics?/othersOpen?/selectedOtherSectionId?`）与 6 个 emits（`selectLane/selectDate/selectDayHeader/toggleDormant/toggleOthers/selectOther`）原样；testid（`matrix-scroll`/`matrix-grid`/`matrix-dayhead-<date>`/`matrix-lane-<id>`/`matrix-cell-<id>-<date>`/`matrix-report-index-hint`/`dormant-toggle`）与 `omx-*` 类名、`data-lane-id`/`data-date`/`aria-*` 原样。
3. **CSS 规则清单**：拆分前 60 条规则声明（按选择器列表计一条）按归属原值迁移——列头 8 条进 `ObservationMatrixDayHeader`、行/格 20 条进 `ObservationMatrixLaneRow`（其中 12 条原本与 `:deep(.omx-other-*)` 写在同一条选择器列表里，现按归属各持一份，两个文件头都写明「改动需两处同步」）、其余 32 条（横滚/网格/索引提示/其他组 `:deep`/组 summary）留在父文件。`is-report/is-selected/is-today/is-gap/is-unknown/is-b1..b4/is-dormant` 的复合选择器随元素所属文件迁移，未丢条目（逐文件 awk 清单见本节末尾）。

```text
父 ObservationMatrixGrid.vue（32）：.omx-scroll / .omx-index-hint / .omx-grid / .omx-grid.is-scrollable /
  :deep(.omx-other-lane)×2 / :deep(.omx-other-name)×2 / hover / is-selected-name / :deep(.omx-other-cell)×3 /
  :deep(.omx-other-cell.is-b1..is-b4)×4 / :deep(.omx-other-cell.is-selected) / :deep(.omx-other-blank) /
  .omx-group-summary+deep / .omx-group-btn+deep (+hover / [aria-expanded=true] / :disabled) /
  .omx-group-title+deep / .omx-group-count+deep / .omx-group-note+extra+deep×2 / :deep(.omx-group-noterow)
列头 ObservationMatrixDayHeader.vue（8）：.omx-corner / .omx-dayhead / .is-report / .is-report:hover /
  .is-report.is-selected / .omx-day-month+.omx-day-day / .omx-day-month / .omx-dayhead.is-today
行 ObservationMatrixLaneRow.vue（20）：.omx-lane / .omx-lane-name / hover / .is-selected name / .is-selected::before /
  .omx-lane-watch+.omx-lane-dormant-tag / .omx-lane-watch / .omx-cell / :hover:not(:disabled) / :disabled /
  .is-b1..is-b4 / .is-today / .is-gap+.is-unknown / .is-gap / .is-selected / .is-dormant（cell+lane）
```
4. **DOM 顺序/焦点/格值**：G2 55 例（含 DOM 顺序 3 例、365 列横滚 2 例、0 休眠 summary、组折叠离 DOM、只标所选日）与 G3 16 例原样通过；本轮跑无 Vue warn（多根片段未触发属性透传告警）。

## 5. baseline → refactor → replay（JSON 多重集对账）

**方法**：同一批精确文件、`--maxWorkers=2`、`--reporter=json --outputFile=…`；对账按 `status + fullName` 多重集（不按文件名），把每行排序后取 sha256，并逐项比较「before 有、after 少」与「after 有、before 少」。日志与原始 JSON 在 `.pi/subagents/test-receipts/matrix-structure/`（`before.json` / `after.json` / `before-multiset.txt`，均为已忽略产物）。

**重构前（before，10 文件 / 189 例全过）**

```bash
cd front && pnpm test:unit \
  app/features/tags/composables/useLaneDayReading.test.ts \
  app/features/tags/composables/laneDayReadingCache.test.ts \
  app/api/normalizers/laneDayReading.test.ts \
  app/features/tags/components/console/dayOtherTopics.test.ts \
  app/features/tags/components/console/laneConsoleView.test.ts \
  app/features/tags/components/console/useConsoleSelection.test.ts \
  app/features/tags/components/console/ObservationMatrix.test.ts \
  app/features/tags/components/console/ObservationMatrixGrid.test.ts \
  app/features/tags/components/console/DayOtherTopicsGroup.test.ts \
  app/features/tags/components/console/OtherTopicReading.test.ts \
  --maxWorkers=2 --reporter=json --outputFile=.../before.json
```

→ `10 files / 189 tests passed`（G1 六文件 118 = 47+11+9+20+23+8；G2 三文件 55 = 34+9+12；G3 阅读 16）。

**重构后（after，同行为集合 18 文件 / 189 例全过）**

```bash
cd front && pnpm test:unit \
  app/features/tags/composables/useLaneDayReading.test.ts \
  app/features/tags/composables/useLaneDayReading.reportIndex.test.ts \
  app/features/tags/composables/useLaneDayReading.reportDay.test.ts \
  app/features/tags/composables/useLaneDayReading.cache.test.ts \
  app/features/tags/composables/useLaneDayReading.relatedArticles.test.ts \
  app/features/tags/composables/laneDayReadingCache.test.ts \
  app/api/normalizers/laneDayReading.test.ts \
  app/features/tags/components/console/dayOtherTopics.test.ts \
  app/features/tags/components/console/laneConsoleView.test.ts \
  app/features/tags/components/console/useConsoleSelection.test.ts \
  app/features/tags/components/console/ObservationMatrix.test.ts \
  app/features/tags/components/console/ObservationMatrix.dayHeaders.test.ts \
  app/features/tags/components/console/ObservationMatrix.groups.test.ts \
  app/features/tags/components/console/ObservationMatrixGrid.test.ts \
  app/features/tags/components/console/DayOtherTopicsGroup.test.ts \
  app/features/tags/components/console/OtherTopicReading.test.ts \
  app/features/tags/components/console/OtherTopicReading.observation.test.ts \
  app/features/tags/components/console/OtherTopicReading.signals.test.ts \
  --maxWorkers=2 --reporter=json --outputFile=.../after.json
```

→ `18 files / 189 tests passed`，`numPendingTests=0`、`numTodoTests=0`、`numFailedTests=0`。

**对账结果**

| 指标 | before | after |
| --- | --- | --- |
| 文件数 / 用例数 | 10 / 189 | 18 / 189 |
| passed / failed / skip / todo | 189 / 0 / 0 / 0 | 189 / 0 / 0 / 0 |
| 多重集「before 多出」 | — | **0** |
| 多重集「after 多出」 | — | **0** |
| `status+fullName` 排序后 sha256（前 16） | `1c6d82f37dfd4357` | `1c6d82f37dfd4357` |

**迁移归属（每例只搬一次，用例名与断言未改）**

| 旧文件（例数） | 新落点（例数） | 内容 |
| --- | --- | --- |
| `useLaneDayReading.test.ts`（47） | `useLaneDayReading.test.ts`（13） | RD-1/2/3（2）、RD-4（2）、RD-5（3）、join 分层（4）、R24 离开上下文（2） |
| | `useLaneDayReading.reportIndex.test.ts`（12） | 停更 lookback（2）、RDC-1（2）、RDC-6（3）、R1（3）、R3（2） |
| | `useLaneDayReading.reportDay.test.ts`（14） | ensureReportDay（5）、RDC-2/3/4b（6）、RDC-5（2）、R24 报告优先级（1） |
| | `useLaneDayReading.cache.test.ts`（5） | CA-3/4/5/7/8 |
| | `useLaneDayReading.relatedArticles.test.ts`（3） | R2（跨板块晚到不发布） |
| `ObservationMatrix.test.ts`（34） | `ObservationMatrix.test.ts`（18） | MX 派生（8）、名单稳定（4）、交互与状态（6） |
| | `ObservationMatrix.dayHeaders.test.ts`（6） | 报日列头三态 + 0 正式泳道列头可点 |
| | `ObservationMatrix.groups.test.ts`（10） | 组顺序/折叠独立性（6）、其他行只标所选日（4） |
| `OtherTopicReading.test.ts`（16） | `OtherTopicReading.test.ts`（5） | ORD 正文/互斥摘要/失败分层/相关文章/日期动作 |
| | `OtherTopicReading.observation.test.ts`（3） | OBV 缺身份/0-1 日/多日峰值 + 失败重试 |
| | `OtherTopicReading.signals.test.ts`（8） | M1 受控重试意图（3）+ M2 终态信号（5） |

- 每个测试文件里的 `vi.mock`/`vi.hoisted`/`beforeEach`/`afterEach` **留在该文件内**（模块 mock 必须在自己的 import 图里注册），harness 只提供 fixture/装配，不 mock 模块、不跨文件共享可变状态。

## 6. ESLint（只覆盖 `.ts`）

```bash
cd front && pnpm exec eslint \
  app/features/tags/composables/useLaneDayReading.ts \
  app/features/tags/composables/laneDayReportIndex.ts \
  app/features/tags/composables/laneDayReportDay.ts \
  app/features/tags/composables/laneDayCurrentReading.ts \
  app/features/tags/composables/laneDayRelatedArticles.ts \
  app/features/tags/composables/laneDayReadingTestHarness.ts \
  app/features/tags/composables/useLaneDayReading{,.reportIndex,.reportDay,.cache,.relatedArticles}.test.ts \
  app/features/tags/components/console/observationMatrixTestHarness.ts \
  app/features/tags/components/console/ObservationMatrix{,.dayHeaders,.groups}.test.ts \
  app/features/tags/components/console/otherTopicReadingTestHarness.ts \
  app/features/tags/components/console/OtherTopicReading{,.observation,.signals}.test.ts
```

→ **exit 0，无任何输出（0 error 0 warning）**。ESLint flat config 不含 Vue 插件，因此本轮**不声称** `.vue` lint 通过；`ObservationMatrixGrid/dayHeader/laneRow` 的 Vue 侧保障只有编译期（Vitest 内 `@vitejs/plugin-vue` 编译）+ 55 例组件行为。

## 7. 文档 / 资产路径同步

- `tasks.md`：勾选 **8C.4S**，追加「实跑验收」与「拆分落点（before→after 行/字节）」两条；§11「8C 新增 Scenario 落点」表按实际迁移改 5 行——零正式泳道保留日期轴（+`ObservationMatrix.dayHeaders.test.ts`）、报日索引失败不误报（→`useLaneDayReading.reportIndex.test.ts`+`dayHeaders`）、组位置与默认态 / 零休眠仍保留同级组标题（→`ObservationMatrix.groups.test.ts`）、零泳道仍按报日读分区（→`useLaneDayReading.reportDay.test.ts`）。
- `test-cases-matrix-day-others.md`：主链路步 11/13、变体 6/7/8、继承与调整「矩阵视图 / 空格提示 / 相关文章按需 / 休眠泳道折叠 / 矩阵观察窗读取边界」按实际落点改写；「实际命令」节新增 8C.4S 对账记录（前后 189、multiset 一致、18 文件清单）。
- `test-cases-matrix-stage.md`：CA 组两处路径改指 `useLaneDayReading.cache.test.ts`（CA 子套件已迁，旧表不再指空）。
- `test-cases.md`：S11 入口索引原为指针段（不含具体测试文件路径），**无需改**；旧 53/1/2 历史未动、未洗绿。
- 证据：本文件 + `.pi/subagents/test-receipts/matrix-structure/{before.json,after.json}`（已忽略）。

## 8. 未验 / G4 入口 / 风险

**未验（按派发约束不跑）**：全量 `pnpm test:unit`、`pnpm exec nuxi typecheck`、`pnpm build`、`pnpm generate`、agent-browser/真实像素/双主题/双视口、Go/SQL/DB、部署、commit。本包**不**声称 Vue lint、类型安全、真实视觉或整 change 完成。

**交 G4 的入口（零改动）**：
- store 仍由壳创建一次：`ensureReportIndex({force})`、`reportDates`、`indexError`、`indexLoading`、`ensureReportDay(date,{force})`、`reportDay`（消费前须按 `.date` 比对）、`reportStale`、编年 `ensureDayReading`/`suspendDisplay`/`current`；`LaneChronicle` 的 `reading?:` 可选注入约定未变。
- Grid 事件链不变：`selectDayHeader(date)` 由列头 → Grid；`selectLane/selectDate/toggleDormant/toggleOthers/selectOther` 原样；`testid` 未变，G4 现有 `BoardConsoleShell.test.ts` 选择器不需要改。
- G4 的 `BoardConsoleShell.vue`（约 494 行/15045B）与其测试（约 1095 行/44663B）**不归本包**，接线包仍需按内聚拆。

**风险 / 关注点**
1. `ObservationMatrixGrid.vue` 现为 **447 行 / 15224B**（占 ~15KB 上限 99%），任何再往父文件加注释/逻辑都会越线——G4 不应改 Grid；若必须改，先迁走「其他组 `:deep` 视觉覆盖」或组 summary。
2. 行/格视觉语言现分居 `ObservationMatrixLaneRow.vue`（自有行）与父文件 `:deep(.omx-other-*)`（其他组行），两处数值必须同步（两个文件头已写明）；后续统一视觉改动需同批改。
3. 多根片段组件依赖 Vue 3 fragment；本轮无属性透传告警，但若将来有人给它传 `class`/`style`，Vue 会告警（fallthrough 不可继承）——需继续只传 props/emits。
4. harness 抽公共后，单个子套件的 fixture 变更会横向影响 2–5 个测试文件；这是「小 fixture 共享」的取舍，已通过「mock/钩子留在各自文件、harness 无模块 mock」把隔离风险压到最低。
5. 对账口径只覆盖本包受影响集合（10→18 文件）；相邻 `LaneChronicle`/`BoardConsoleShell` 未在本次重跑（G4 未接线且未改动它们，历史 66/38 绿仍按旧证据），G4 接线包需按自己的集合重跑。
