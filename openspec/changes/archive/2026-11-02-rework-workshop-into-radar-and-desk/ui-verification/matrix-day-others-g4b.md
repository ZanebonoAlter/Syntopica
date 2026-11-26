# G4b 实际壳层接线报告（8C.5A 前置收口 + 8C.5B/8C.5）

> 线程：全新 implementer、本包唯一 writer（未 resume 旧长线程、未派子线程、未 commit/归档/部署、未换 provider/model/CLI）。
> 基线：批准矩阵增量 `ui-prototype/revision-2026-10-28-matrix-day-others/standalone.html`（SHA256 `cdb0b70e…`，用户已批准授权实施）。
> 开工 before SHA（HEAD，未 commit 混合树）：`f0012930b8aab4b71e994e63acd5a3c65b925896`（develop）。本包零 commit，after 为工作树实算。
> 只记本线程**实跑**证据；控制器绿 ≠ UI 已验收，组件测试 ≠ 真实浏览器；根账旧失败/未覆盖不因本包清零。

## 1. A 入场前置小回归（先真红 → 最小修）

父 `matrix-day-others-g4a-review.md` 定的两个入场角，先写用例捕获真红，再最小修：

| 角 | 用例（新增） | 修前红 | 修法 |
| --- | --- | --- | --- |
| 旧日期重试抢写新选中日 | `useConsoleReading.test.ts` →「重试 A 的 force 索引在途时切到 B（B 已读就绪）：索引返回后不再读/发布 A，B 终态保持」 | ✗ `expected [ 'detail:900' ] to deeply equal []`（旧日详情被重读、reportDay 被抢回 A） | `retryReport` 在 `ensureReportIndex` await 之后、**任何旧 date 读取之前**复核 board + 绑定选中日（`selectionMovedAway`：已绑定且不等于 payload.date，含被清空）→ 直接 `applied=false` 停止；末尾原有核对保留 |
| 绑定 selectedDate 被清空（切板块前清场） | 同上 →「重试 A 索引在途时选中日被清空…applied=false」 | ✗ `expected { applied: true, sectionFound: true } to deeply equal { applied: false, sectionFound: null }`（还多读了一次旧日详情） | 同上（`selectionMovedAway` 覆盖 null） |
| 其他选择成员合法性 | `otherTopicSelection.select.test.ts` →「选行 guard：只接受 G1 其他组视图成员（正式/休眠泳道分区详情里存在也不得进入）」 | ✗ `expected { sectionId: '601', … } to be null`（正式泳道 601 被当其他话题选中） | `selectOther` 只接受 `otherTopics.state==='ready'` 且 `sectionId ∈ otherTopics.rows`（**直接复用 G1 `buildOtherTopics` 成员**，不再自行用详情 sections 判存在；不复制排除算法） |
| 休眠分区 / 组 ready 0 行 | 同上 →「休眠泳道分区 / 组 ready 但 0 行时一律不进入其他选择」 | ✗ `expected { sectionId: '602', … } to be null` | 同上 |

红灯实跑（修前）：`pnpm test:unit useConsoleReading.test.ts otherTopicSelection.select.test.ts --maxWorkers=2` → **4 failed / 19 passed (23)**。
绿灯实跑（修后，含 G4a 既有 6 文件）：**6 files / 60 tests passed**（56 + 4 新例）。原 56/94 与父独立 24 例历史保留，不冒称本包覆盖。

消费侧：G3 阅读槽状态全部由 props（`selectedOther` + 刷新后的 `reportDay`）派生，壳层**不留「上次核对结论」**状态，因此异步间切条时旧 `sectionFound` 不可能显示到新选中条上（brief「消费时 section 不同别用旧提示」结构性满足）；集成用例「G3 阅读槽重试」覆盖切条/缺分区/恢复路径。

## 2. B 实际接线（唯一状态与构造顺序）

`BoardConsoleShell.vue` 严格按 G4a 合同顺序构造（全部只创建一次）：

1. `useBoardConsole`（G1 唯一状态源，`reportAnchor` 接独立日报元信息）；
2. `useConsoleReadingWiring` 内部：`useConsoleReading`（**1 份 reading + 1 份 observation**，索引首取与编年挂载解耦）→ `useConsoleSelection`（注入报日集合三态 `null|Set`，不能把 null 当已知空）→ `bindSelectedDate`（唯一选中日跟随报告优先读取）→ `useOtherTopicSelection`；
3. `useConsoleReturnContext`（追加 `selectedOther`/`othersOpen`）；
4. `useConsoleRefresh({ after: () => wiring.refreshAfterRebuild() })`。

- **stage props**：`consoleStageBinding.buildConsoleStageProps` 装配纯视图字段；控制器 store（`reading`/`observation`）作为 **plain 对象**下发（不 reactive 深解包 Ref；下游读 `.current.value`/`.view.value` 不变）。
- **唯一单例**：`useLaneDayReading` 不再在编年里二次创建——`LaneChronicle` 新增可选 `reading` prop，产品路径注入壳层实例；未注入时保旧自建用法（`LaneChronicle.test.ts` 28 例原样绿）。子卸载只停自身 watcher/显示，不释放壳层 report/cache/observer；换板块由 store 的 boardId watcher 统一复位。

### 2.1 真实事件透传（Shell → Body → Stage → Grid/Narrow → OtherReading）

| 事件 | 落点 |
| --- | --- |
| `selectDayHeader(date)` | `ObservationMatrixDayHeader` / 窄屏 `BoardNarrowDayStrip` → Stage → Body → 壳层 `wiring.selectDayHeader`（只改同一 `selectedDate`，非报日/轴外忽略） |
| `toggleOthers` | `DayOtherTopicsGroup`/窄屏其他组 → `wiring.toggleOthers`（仅 ready 生效，与 `toggleDormant` 完全独立） |
| `selectOther(payload)` | 组行/格 → `wiring.selectOther`（复用 G1 成员合法性；不写日期/URL） |
| `selectObservedDay(payload)` | `OtherTopicReading` 观察带节点 → `wiring.selectObservedDay`（原子写同一 `selectedDate` + 稳定 report/section/topic） |
| `retryReport(payload)` | `OtherTopicReading` 重试/刷新 → `wiring.retryReport`（force 索引 → force 当日详情 → 稳定 section 核对；结果只驱动同一份 `reportDay`） |
| `retryReportChain` | Stage 新增 `matrix-report-chain-notice` 最小入口 → `wiring.refreshAfterRebuild`（force 索引 → force 当日报告；**不把重试只接旧 A load**） |
| `retry`（旧） | 矩阵/窄屏 A 失败 → `load()` **且** `ensureReportIndex({force:true})`（G4a §4.5） |
| 选泳道/普通格/演进日 | `wiring.selectLaneFromStage` / `selectCellFromStage` / `selectDateFromStage`：**显式 `clearOther`**（点已选中的同一泳道也清，不靠 registry watcher） |

### 2.2 单一阅读槽与清场

- `selectedOther !== null` → `OtherTopicReading`（含获批同归属跨日观察带）；否则 `selectedLaneRow` → 原 `LaneChronicle`；否则给**方向提示**（`console-reading-empty`，0 正式泳道文案指向「点报日列头 + 展开当日其他话题」）。
- 其他阅读中：**隐藏旧 lane 演进**（不叠两个演进/阅读面），并把矩阵/窄屏的 `selectedLaneId` 置空以**抑制泳道高亮**；底层 lane 选中与 URL `lane` 保留供切回。
- 清场规则沿用 G4a 控制器（换日/切泳道/切板块/范围排出）；壳层只在**显式用户动作**上补 `clearOther`。
- 0 正式泳道：日期轴/网格/其他组默认展开/下方同一阅读槽都在（旧 `console-no-lanes-fallback` 内联说明按既有 38 例断言保留，不再是唯一内容）。

### 2.3 生成完成/显式刷新（同一防重窗口）

`useConsoleRefresh`（meta → A → `after`）内 `after` = `refreshAfterRebuild`：

1. `ensureReportIndex({force:true})`（新报日列头立即可选）；
2. 当前选中日 `ensureReportDay(date,{force:true})`（异步间切日/切板块按最新上下文，不拉旧选择）；
3. `observation.invalidate()`（**无其他行选择 = 零观察 GET**；有选择按当前 topic 重读）；
4. **lane 编年重建**：`rebuildLaneReading()` 用 `ensureDayReading`（非 force）从**同一份已刷新缓存**重新 join，避免第二遍 force 列表，也避免「只换 reportDay、原编年永久旧」。

失败不静默：索引失败 `indexError` 自述 + Stage 重试入口；详情失败保留旧内容并标 stale（G1 语义不变）。

### 2.4 返回/滚动（真实组件链）

- 路由往返（阅读台/研究工作台）沿用 G4a：`other:<sectionId>` 阅读 key、四参 `days/start/end/lane` 一次原子写、URL 显式值优先、`other` **不塞** `laneId`/URL `lane`、无新 `date=`。
- G4a 返回 hook 原本没有「内部归档」入口：本包把滚动恢复拆到 `consoleReturnScroll`（同行为），并在 hook 上补最小 `captureStageScroll()`/`restoreStageScroll(target)`；壳层在切归档前记 `stage.scrollTop`，切回矩阵后交同一 pending 机制恢复（等阅读终态、用户 wheel/touch/key 取消、pending 不耗帧）。
- `readingSettled` 在阅读上下文（date/lane/other）变化时由壳层清空，旧信号不得兑现新槽滚动。

## 3. 内聚拆分与 before→after（§8 / ≤500 行 / ≤15360B）

SHA 为 `sha256` 前 16 位（before 开工实采 / after 工作树实算）；before 为 0 表示本包新建。

| 文件 | before | after | 行/字节 before → after |
| --- | --- | --- | --- |
| `console/BoardConsoleShell.vue` | `05dcc6c755210607` | `3bc8752d23cdca27` | 493/15045 → **444/14486** |
| `console/BoardConsoleBody.vue` | `831d8c4db0469e6b` | `45021f6561b00250` | 267/7312 → **285/8384** |
| `console/BoardConsoleStage.vue` | `6dc74ce05ef66153` | `7ba76729d413cce5` | 236/7293 → **335/12197** |
| `console/consoleStageTypes.ts` | `740931536bb60073` | `eeec9fac466cbe02` | 42/1410 → **80/3338** |
| `console/LaneChronicle.vue` | `dbb20b46fe5f2fd3` | `2a3d629eb9627504` | 399/14947 → **389/14597** |
| `console/useConsoleReading.ts` | `bdf8693597b4717b` | `f570c597ec23a39d` | 172/8067 → **184/8818** |
| `console/useConsoleReading.test.ts` | `3cd9205e0aca2e73` | `77ad266f4077433c` | 277/11294 → **331/13540** |
| `console/otherTopicSelection.ts` | `21925beb09e242ca` | `042eb553df9fac18` | 207/9096 → **211/9396** |
| `console/otherTopicSelection.select.test.ts` | `1c1fd4b2d5b0929f` | `99c64a487cccb86c` | 158/6406 → **218/8825** |
| `console/useConsoleReturnContext.ts` | `388ab6572d30be2f` | `110e653a697fdc4d` | 412/15301 → **264/10244** |
| `console/consoleReturnScroll.ts`（新） | — | `bb84f81490955db7` | 0 → **232/8847** |
| `console/consoleStageBinding.ts`（新） | — | `5d61cf30d420a262` | 0 → **84/3123** |
| `console/useConsoleReadingWiring.ts`（新） | — | `70d8f42d12d13967` | 0 → **200/9403** |
| `console/useConsoleShellOverlays.ts`（新） | — | `c9bd5106b5c3428e` | 0 → **131/4317** |
| `console/useConsoleShellRail.ts`（新） | — | `921281fdfd47d580` | 0 → **55/1560** |
| `console/useConsoleShellActions.ts`（新） | — | `30b02a53fd8cc9be` | 0 → **104/4047** |
| `console/laneChronicleMetrics.ts`（新） | — | `f8a411891a2269ce` | 0 → **35/1559** |
| `console/BoardConsoleShell.test.ts`（骨架组） | `baa8df1ffe3406b4` | `0cee8d8367f6b209` | 1094/44663 → **161/7503** |
| `console/BoardConsoleShellSelection.test.ts`（新） | — | `2734fc43c2acd41f` | 0 → **190/8392** |
| `console/BoardConsoleShellNavigation.test.ts`（新） | — | `b7f0ea388fb4fd1e` | 0 → **351/15294** |
| `console/BoardConsoleShellStates.test.ts`（新） | — | `157eb70061f6bd64` | 0 → **104/4356** |
| `console/BoardConsoleShell.integration.test.ts`（新） | — | `58f35b0be868e609` | 0 → **289/13881** |
| `console/boardConsoleShellTestHarness.ts`（新） | — | `5e3b9f3376990392` | 0 → **259/8938** |
| `console/boardConsoleShellFixtures.ts`（新） | — | `29ba2a4d37a82a30` | 0 → **163/4286** |
| `console/boardConsoleShellOtherFixtures.ts`（新） | — | `b7ac52b967989b72` | 0 → **144/4169** |

拆分口径：`BoardConsoleShell.vue` 的精读/写入/返回/动作职责分别落到 `useConsoleReadingWiring`（阅读接线）、
`useConsoleShellOverlays`（生成/新建/管理/原文阅读/工作面）、`useConsoleShellRail`（右轨装配）、
`consoleStageBinding`（舞台视图模型装配）、`useConsoleShellActions`（视图/归档往返/重试/定位）；
`LaneChronicle.vue` 的计数口径文案抽 `laneChronicleMetrics`；返回 hook 的滚动宿主抽
`consoleReturnScroll`（13301B 已逼近上限，属必须拆）。旧壳测试按 describe 组拆 4 份 + 3 个非
`*.test.ts` harness/fixtures，新增集成用例独立成文件。
未压行、未删注释、未造万能 harness；DOM/testid/公共 props/emits 只在 G2/G3 既有可选项上透传，
未改已批准样式与模型。全部 source/test/helper ≤500 行且 ≤15360B（最紧
`BoardConsoleShellNavigation.test.ts` 15294B、`LaneChronicle.vue` 14597B、`BoardConsoleShell.vue` 14486B）。

### 3.1 旧 38 例对账（JSON 多重集，不用重写断言凑绿）

- 拆前：`--reporter=json` → `g4b-evidence/shell38-baseline.json`（38 passed / 0 failed / 0 skip / 0 todo），名单 `shell38-baseline-names.txt`。
- 拆后：4 个分组文件合并跑 → `g4b-evidence/shell38-after-split.json`（38 passed / 0 failed / 0 skip / 0 todo），名单 `shell38-after-split-names.txt`。
- 对账：`fullName + status` 排序多重集**完全相等**；缺例 0、新增 0。分组：骨架 10 / 唯一选中日+档 7 / 入口·管理·路由 16 / 版块·归档·失败 5。
- 新集成用例 9 例**不加入**旧 38 计数（见 §4）。

## 4. B 新集成用例：真红 → 绿

`BoardConsoleShell.integration.test.ts`（9 例，真实 `Shell → Body → Stage → ObservationMatrix/ObservationMatrixGrid/DayOtherTopicsGroup` / `BoardNarrowSummary` → `OtherTopicReading` 链；只 mock API 与复杂弹层）：

1. 0 正式泳道：真实报日列头是 `button`、非报日仍是 `div`；其他组默认展开、真实行与计数；网格仍在；未选时阅读槽给方向提示；列头选日写同一 `selectedDate`。
2. 成熟板块：其他组默认收起（计数可见）→ 展开点行 → 单一阅读槽逐字标题/摘要、旧编年与演进消失、泳道高亮抑制；点行**不额外发详情 GET**（共享缓存）。
3. 点同一底层已选泳道 / 普通格都**显式清其他选择**（同 laneKey 情形覆盖）。
4. 观察带真实节点：同归属跨日 → 原子切同一 `selectedDate` + 稳定 report/section，阅读槽跟随真实分区（复用 G3 带，不重算）。
5. 报日链失败：Stage 最小重试入口 → force 索引**先于** force 当日详情（`invocationCallOrder` 断言），不只刷 A。
6. G3 阅读槽重试：报告重建缺分区 → 中性「报告已更新」→ 刷新 → force 索引 → force 详情 → 真实内容回来。
7. 生成完成：lane 编年基于刷新后共享缓存重建（标题变化可见），无其他选择时观察 **零 GET**。
8. 内部归档 → 矩阵同实例返回：其他阅读槽/报日/滚动位置都恢复。
9. 窄屏同源：报日条 `button` 可点、切日后其他组按该日报真实分区列出、行 → 下方同一阅读槽。

**反向检查（非空绿证明）**：临时断开 `consoleStageBinding` 里的 `...deps.reading`（等价恢复「未接线」的舞台输入）→ 集成用例 **9 failed / 9**；恢复后 **9 passed**。据此确认断言确实依赖本包接线，而非既有组件巧合通过。

## 5. 验证（Linux 本机串行，参数不带 `--`）

| 检查 | 命令（`cd front`） | 结果 |
| --- | --- | --- |
| A 前置（红） | `pnpm test:unit app/features/tags/components/console/useConsoleReading.test.ts app/features/tags/components/console/otherTopicSelection.select.test.ts --maxWorkers=2` | 修前 **4 failed / 19 passed (23)** |
| A 前置（绿）+ G4a 回归 | 上述 + `otherTopicSelection.test.ts`、`useConsoleReturnContext.test.ts`、`consoleReturnContext.test.ts`、`useConsoleRefresh.test.ts` | **6 files / 60 tests passed** |
| 旧 38 例（拆前） | `pnpm test:unit .../BoardConsoleShell.test.ts --maxWorkers=2 --reporter=json` | **1 file / 38 passed**（baseline JSON） |
| 旧 38 例（拆后） | 4 个分组文件 `--maxWorkers=2 --reporter=json` | **4 files / 38 passed**，multiset 一致 |
| 目标批次（本包全部受影响） | `pnpm test:unit <24 文件：5 Shell 分组/集成 + useConsoleReading/otherTopicSelection×2/useConsoleReturnContext/consoleReturnContext/useConsoleRefresh + LaneChronicle + ObservationMatrix×3 + DayOtherTopicsGroup + OtherTopicReading×3 + BoardNarrowSummary + restoreScroll + useConsoleSelection + laneConsoleView + dayOtherTopics> --maxWorkers=2` | **24 files / 265 tests passed**（0 failed / 0 skip） |
| 本组 `.ts` eslint | `pnpm exec eslint <§3 全部 .ts，21 文件>` | **exit 0，0 error 0 warning**（Vue 不被 eslint 配置覆盖，**不称 Vue lint 绿**） |
| 尺寸 | 逐文件 `wc -lc` | 全部 ≤500 行 / ≤15360B（§3） |
| 文档门禁 | `openspec validate rework-workshop-into-radar-and-desk --strict`、`bash scripts/harness/check-tasks-tail.sh openspec/changes/rework-workshop-into-radar-and-desk` | 见 tasks 验收行（本包实跑结果） |

未跑（按 brief 与树莓派红线）：全量 `pnpm test:unit`、`pnpm exec nuxi typecheck`、`pnpm lint`、`pnpm build`/`generate`、浏览器/agent-browser、Go/SQL/DB/AI/业务写、服务起停、部署、commit/归档。

## 6. 必要源 hunk 与理由（逐项）

| 文件/位置 | hunk | 理由 |
| --- | --- | --- |
| `useConsoleReading.ts` | `selectionMovedAway` + 索引后/详情后两处复核 | 父 G4a review 前置：旧日重试必须先于读取停止，末尾 `applied=false` 救不了已写 store |
| `otherTopicSelection.ts` | `selectOther` 改判 `otherTopics.rows` 成员 | 正式/休眠分区也在当日详情里，仅「详情存在」会误入其他选择；复用 G1 唯一排除口径 |
| `BoardConsoleStage.vue` | 新增 `matrix-report-chain-notice` + `retryReportChain` emit；阅读槽/演进/高亮三处条件 | G2 组 summary 没有独立 retry 事件，索引/当日报告链需最小正确入口；单一阅读槽与「其他模式隐藏旧演进」是合同要求 |
| `BoardConsoleStage.vue` | `console-reading-empty` 方向提示 | 0 正式泳道收口后，「无 lane 且未选其他」不能留空白；文案诚实指向报日列头/其他组 |
| `BoardConsoleStage.vue` | 复用 `.stage-pool` 样式承载索引/报告链提示 | 不新造视觉语言，双主题 token 不变 |
| `consoleStageTypes.ts` | 新增 `reportDates/index*/otherTopics/othersOpen/selectedOther*/reportDay/reportStale/reading/otherObservation` | 舞台受控契约扩展（G4b 唯一新增字段面）；`otherObservation` 特意与观察范围 `observation` 分名（同名会被 `...reading` 覆盖，已用集成用例守住） |
| `LaneChronicle.vue` | 可选 `reading` prop + `createOwnReading()` 兜底 | 产品路径单实例；未注入保旧自建（不无条件先建第二 store） |
| `useConsoleReturnContext.ts` / `consoleReturnScroll.ts` | 滚动内聚拆分 + `captureStageScroll`/`restoreStageScroll` | hook 已 15301B/15360B，新增能力必须按 §8 拆；内部归档同实例返回入口是 G4a review 明列义务 |
| `BoardConsoleShell.vue` | 全量接线 + 4 个新 helper | 超出 15KB 上限，必须内聚拆；构造顺序按 G4a §3 |
| `useConsoleShellOverlays.ts` | 收编 workface 开关 | 浮层与工作面同属「打开/关闭/焦点归还」，同文件内聚，减少 Shell 解构 |
| `useConsoleShellActions.ts` | 视图/归档往返/报告/研究/版块/重试/定位动作 | Shell 拆出后仍有 15.9KB，超 §8 上限；这些动作共用同一批入口（暂存滚动位置、wiring 重试、closeWorkface），同文件内聚且不改行为 |

### 6.1 未需要的 fallback hunk（实核结论）

brief 预授权的「0 正式泳道无日期轴时接 reportAnchor 的最小 fallback」**未使用**：`useBoardConsole.axis` 由
`buildDayAxis(anchorDate ?? today, windowDays)` 派生，与 A 名单是否为空无关；`useObservationWindow.poolBounds`
同样只依赖窗口/锚点。故 0 正式泳道下轴/列头/格/其他组/阅读槽全部照常，本包未改
`useBoardConsole.ts` / `useObservationWindow.ts`，也未动 A 名册/排序/休眠/计数与 max/sum 口径（相邻
`laneConsoleView.test.ts` 23 例、`ObservationMatrix*.test.ts` 34 例原样绿）。

## 7. 未验 / 风险 / 归 G5（8C.7）或父

1. **typecheck/lint/build 未跑**：新类型（`BoardConsoleStageProps` 扩展、`ConsoleReadingStageProps` Pick、`ConsoleReturnScrollDeps`、`ReadingSettledState` 重导出）由父 8C.6 集中 `nuxi typecheck` 一次；本包只经 Vitest SFC 编译 + `.ts` eslint。
2. **真实像素/触摸/双主题未验**：0 正式泳道（246030）与成熟（1974）的真实报日集合、行数、滚动几何、375/390 窄屏仍需 8C.7 agent-browser；本包集成用例用受控 API 夹具。
3. **0 正式泳道两处提示并存**：旧 `console-no-lanes-fallback`（G5 内联说明，旧 38 例按原文断言）与 G2 网格内「0 正式泳道 · 已展开当日其他话题」同时可见。功能上不阻断（网格/组/阅读都在），但视觉可能重复；改文案需同步旧例断言，按「不重写断言凑绿」本包未动，留 review/8C.7 决定。
4. **索引 force 频次**：生成完成链路为 meta → A → before-after 内 1 次 force 索引 + 1 次 force 当日详情 + lane 非 force 复用；`anchorDate` 若同时前移，G1 非 force 覆盖扩大可能合并/追加一次列表请求（G1 在途合乘保证不重复同范围）。
5. **返回恢复的滚动几何**：内部归档恢复复用同一 pending 机制（等阅读终态、用户意图取消、180 帧上限）；真实浏览器内容高度插值下是否仍有 1–2px 收敛误差留 8C.7 机械断言。
6. **`othersOpen` 仅随真实 other 保存**：用户只手动展开组但未选行时，路由往返不恢复展开态（G4a 保守决定，旧六字段 `Object.keys` 断言不破）；如产品要求恢复纯折叠偏好需扩载体（会破旧断言，属父裁决）。
7. **测试文件贴近上限**：`BoardConsoleShellNavigation.test.ts` 15294B、`BoardConsoleShell.vue` 15054B。后续再加用例/字段需先拆（例如把 R30/R31 返回与滚动子套件单独成文件）。

## 8. tasks 勾选依据

仅勾 **8C.5A / 8C.5B / 8C.5**：A 四例真红→绿（60 tests）、目标批次 24 files/265 tests、旧 38 例 multiset 一致、9 例集成真红→绿、20 个 `.ts` eslint exit 0、全部文件过 500 行/15KB。**8C.6–8C.8 不勾**：typecheck/lint/build、真实浏览器与独立 review 未做，不称产品验收/上线。
