# change-review 审查报告：add-key-event-matrix-view

- 审查档：change-review（只读通用档）｜审查日期：2026-11-23｜审查范围：制品 + 实现 diff
- 审查基线：`AGENTS.md`、`front/AGENTS.md`、`docs/reference/开发执行规范.md` §0.6「review 执行要点」（步骤 0 design 落点复核 / 步骤 1 精读高风险文件 / 步骤 1.5 迁移去向表 + 3 条真人主故事 / 步骤 2 grep 全包验证 / 步骤 3 High·Medium·Low 分级 / 步骤 4 建账）
- 业务红线：`docs/reference/flow/semantic-board.md`（红线 37）与 `docs/reference/flow/topic-graph.md`（红线 10）

## 0. 读了什么 / 跑了什么

**制品**：`openspec/changes/add-key-event-matrix-view/` 下 `proposal.md`、`design.md`（D1–D14）、`ui-design.md`、`specs/topic-key-event-markers/spec.md`、`specs/workshop-board-console/spec.md`、`tasks.md`、`stage.md`、`explore-findings.md`、`ui-verification/report.md`（34 条：pass 31 / fail 1 / 未覆盖 2）。

**精读的高风险文件（8 个）**：

| 类别 | 文件 |
| --- | --- |
| 并发/缓存/晚到隔离 | `front/app/features/tags/composables/useBoardKeyEvents.ts` |
| 缓存/生命周期/降级 | `front/app/features/tags/components/console/KeyEventTimelineView.vue`（1019 行，分段读） |
| 布局算法 | `front/app/features/tags/components/console/keyEventLayout.ts`、`KeyEventLabel.vue` |
| 事务/竞态（query 写入） | `front/app/features/tags/composables/useBoardConsole.ts`（`batchQueryWrite`/`applyQueryPatch`/`setView`）、`useConsoleShellActions.ts`、`useObservationWindow.ts`（独立 query writer） |
| raw SQL / 参数化 | `backend-go/internal/topicgraph/repository/key_event_markers.go`（`ListBoardKeyEventMarkers`） |
| handler 边界（404/400/IDOR） | `backend-go/internal/topicgraph/handler/key_event_handler.go`（`parseBoardKeyEventDays`/`listBoardKeyEventMarkers`） |
| 接入点 / 旧能力去向 | `BoardConsoleStage.vue`、`BoardConsoleShell.vue`、`BoardConsoleBody.vue`、`BoardConsoleHeader.vue`、`useConsoleSelection.ts`（行序来源 `buildObservationRows`） |

**命令**（均为只读）：`git status --short`、`git diff -- <路径>`（后端 2 文件 + 前端 9 文件）、`grep -rn` × 12 组（只读性 / suggested·dismissed / 硬编码色值 / text-overflow / 写端点调用 / 行集合过滤 / route 冲突 / query.view 消费者 / 落点符号存在性）、`cd backend-go && go vet ./internal/topicgraph/...`（退出码 0）、`npx vitest run` 定向 11 个测试文件（148 用例全绿：KeyEventTimelineView / KeyEventLabel / keyEventLayout / boardKeyEvents / useBoardKeyEvents / BoardConsoleShellRailViewSwitch(+Query) / useBoardConsole / ObservationMatrix / BoardConsoleShellStates / BoardConsoleShellSelection / BoardNarrowSummary）。未跑全量 `go test ./...` / `pnpm test:unit`，未部署、未用 agent-browser、未改任何实现代码。

## 1. 步骤 0：design 落点复核（D1–D14）

| 决策 | 落点 | 可解析 | 说明 |
| --- | --- | --- | --- |
| D1 | `console/BoardConsoleHeader.vue` | ✅ | 文件存在；`.console-seg` 第四项「关键事件」+ `aria-pressed` 已落地 |
| D2 | `observationLanes.ts:buildObservationRows` | ✅ | 符号存在；事件视图行序经 `buildConsoleLaneGroups → buildObservationRows` 与矩阵同源（`useConsoleSelection.ts:41-44`） |
| D3 | `key_event_handler.go:RegisterKeyEventRoutes` | ✅ | 路由已注册（`/semantic-boards/:id/key-events`） |
| D4 | Scenario「标签不截断」 | ✅ | 标题逐字存在 |
| D5 | Scenario「候选话题合并为一行」 | ❌ | **该 Scenario 不存在**（实际为「候选话题不占行」）；且「合并为一行」恰是 D2 取舍里被否的方案 → 指针语义相反，见 R-5 |
| D6 | `console/BoardConsoleShell.vue` | ✅ | 文件存在（视图读写 URL 在 `useBoardConsole.ts`） |
| D7 | Scenario「层数超限才收起」 | ⚠️ | 实标题为「层数超限才收起标签」，前缀近似 → 见 R-5 |
| D8 | 无代码落点：`ui-design.md`「明确不做」+ `topic-graph` 约束 10 | ✅ | 两处承接制品均存在（红线 10 在册） |
| D9 | Scenario「时间视窗两侧共用」 | ✅ | 逐字存在 |
| D10 | Scenario「宽列横向滚动」 | ✅ | 标题存在；但**结论与 D11 互斥**（见 R-2） |
| D11 | `composables/laneComposeLogic.ts:panScroll` | ✅ | 符号存在（:221），实装复用 `panScroll` + `isDragBeyond` |
| D12 | `console/BoardConsoleShell.vue` | ✅ | 文件存在；emit 链 `KeyEventDayPanel → KeyEventTimelineView → Stage → Body → Shell → ArticlePreviewModal` 逐段核对通 |
| D13 | 无代码落点：`ui-design.md`「Layout Contract · 双主题」 | ✅ | 章节与条目均在 |
| D14 | `composables/useBoardConsole.ts` + 右轨既有落点逻辑 | ✅ | 符号存在（`setView`/`activeView`）；实装在 `useConsoleShellActions.locateLaneFromRail` + `BoardConsoleShell.handleSelectOtherFromRail` |

**结论**：D1–D14 无「有结论无承接」项（**无可删项**）。D5 落点指针失效（结论本身有承接：新增组件 + delta spec Requirement「关键事件节点与名称」），D7 指针为近似标题 → 合并记 R-5（Low，不按「可删项」处理）。

## 2. 步骤 1.5：旧能力去向表（迁移/替换型复核）

| 旧能力 | 旧入口（文件:行 / 组件·事件） | 新承接 | 判定 | 证据 |
| --- | --- | --- | --- | --- |
| 矩阵 / 追踪视图 | `BoardConsoleHeader.vue:95-102` `.console-seg` 首项 → `setView('matrix')` | 同（`setView` 增加同值早退 + `writeViewQuery`） | 已迁移 | `useBoardConsole.ts:325-329`；`ObservationMatrix.test.ts` 26 用例全绿 |
| 文章归档视图 | 顶栏「文章归档」→ `handleViewArchive`（`useConsoleShellActions.ts:63-72`） | 同；新增 `?view=archive` 落 URL（`archiveInitialDate` 仍在壳层 ref，日期不丢） | 已迁移 | `useBoardConsole.ts:168-174` |
| 研究工作台 | 顶栏「研究工作台」→ router.push | 未改 | 已迁移 | — |
| 右轨「话题追踪」定位泳道 | `useConsoleShellActions.ts:95-110` `locateLaneFromRail` | 同一动作：先切矩阵 + `batchQueryWrite` 合并成一次 replace | 已迁移（行为增强，A-34 复验 pass） | `BoardConsoleShellRailViewSwitchQuery.test.ts` 2 用例绿 |
| 右轨「当日其他话题」选行 | `BoardConsoleBody.vue:188` rail `@select-other` → `handleSelectOther` | 改派 `handleSelectOtherFromRail`（**先切矩阵**） | 已迁移，但作用域扩大到归档视图 → R-1 | `BoardConsoleShell.vue:376-388` |
| 舞台内「当日其他话题」选行 | stage `@select-other` → `handleSelectOther` | **未改**（仍留在当前视图） | 已迁移 | `BoardConsoleBody.vue:152` |
| 矩阵当日工作面阅读槽 `.stage-chronicle` | `BoardConsoleStage.vue`（原无条件渲染） | 事件视图下 `v-if="activeView !== 'events'"` 隐藏 | 主动退役（事件视图自带当日事件阅读区） | 同上 diff |
| 窄屏降级 `BoardNarrowSummary` | stage `isNarrow` 分支 | 未改；事件视图不进窄屏分支 | 已迁移 | `BoardNarrowSummary.test.ts` 13 用例绿 |
| 观察窗 `days/start/end` 的 URL 写 | `useObservationWindow.ts` `writeQuery`（独立 writer） | 未改（与新 `batchQueryWrite` 不同源） | 已迁移，带残留风险 → R-8 | `useObservationWindow.ts:99-107` |
| 归档往返滚动恢复 | `handleSetView` 的 `archiveReturnScroll` | 未改 | 已迁移 | `useConsoleShellActions.ts:48-56` |
| 事件写动作归属 | `KeyEventMarkerActions.vue`（`mode="review"` 挂 `DailyReportTopicSection.vue:487`） | 未改，仍唯一挂在日报阅读台 | 已迁移（红线 10 守住） | `grep -rn KeyEventMarkerActions front/app` |

**三条真人主故事**（结论与「新页面存在」分开写）：

1. **无活跃泳道但有日报 → 能读**：矩阵视图未变，故事仍可完成（`readingHint` + 报日列头 + 当日其他话题；`BoardNarrowSummary.test.ts` / `BoardConsoleShellSelection.test.ts` 绿）。**新视图不承载该故事**：事件视图在 `emptyState='no-lanes'` 时只给「该版块暂无持久化话题」且 `.stage-chronicle` 被隐藏 → 该视图内是死路，需切回矩阵。判定：旧故事**未断**（矩阵可达），新视图范围外（ui-design State Matrix 已定），不记 finding。
2. **归档 → 站内读正文 → 回原日期**：**仍可完成**（`BoardArchiveView` 未改，`archiveInitialDate` 在壳层 ref 不随视图切换丢失，`ArticlePreviewModal` 链路未改）。但右轨联动把归档视图里的点击变成「静默切走视图」→ 记 R-1（Medium，非断链）。
3. **事件 → 文章 → 阅读 → 回原卡**：**可走通**。emit 链逐段核对（DayPanel→TimelineView→Stage→Body→Shell→`overlays.handleOpenArticle`→`ArticlePreviewModal`）；`KeyEventDayPanel` 用既有 `LaneRelatedArticles` + 壳层 `reading` store（不新建第二份缓存）；A-14/A-15 实机复验 pass（卡片保持展开、`.console-stage` scrollTop 前后同为 435）。

## 3. 步骤 2：grep 全包验证关键不变量（结果）

| 不变量 | 命令 | 结果 |
| --- | --- | --- |
| 本视图只读（写动作不得进本视图） | `grep -rn "KeyEventMarkerActions\|patchKeyEvent\|MarkConfirmed\|ApplyThreadKeyEvent" KeyEvent*.vue boardKeyEvents.ts useBoardKeyEvents.ts` | ✅ 仅 1 处注释提及归属，无任何写调用；`grep -rn "apiClient\.(post\|put\|patch\|del)" ` 同样为空 |
| 只透 confirmed | `grep -rn "suggested\|dismissed" KeyEvent*.vue boardKeyEvents.ts normalizers/boardKeyEvents.ts useBoardKeyEvents.ts` | ✅ 前端零命中；后端 `ListBoardKeyEventMarkers` WHERE 硬编码 `state = KeyEventStateConfirmed`（`key_event_markers.go:638`） |
| 候选不占行 | `KeyEventTimelineView.vue:110-125` `rowTopicIds`（active+dormant 行话题集）过滤 `dayEventsByDate` | ✅ 时间轴与当日阅读区同时过滤（无孤儿事件） |
| 无日期伪造 0 | 模板 `v-for` 遍历 `projection.byDate`（实际事件）而非全轴 | ✅ 无事件日只有虚线轴 + 行头「无确认事件」 |
| 双主题不写死色值 | `grep -nE '#[0-9a-fA-F]{3,8}\|rgba?(\|hsla?(' KeyEvent{TimelineView,Label,DayPanel}.vue keyEventLayout.ts` | ✅ 零命中（全走 `--color-*`） |
| 标签不截断 | `grep -n "text-overflow" KeyEventLabel.vue` | ✅ 仅注释提及「不加 text-overflow」；`.kev-label-text{white-space:nowrap}` 无 ellipsis（**但当日阅读区卡片标题另有一处，见 R-3**） |
| raw SQL 参数化 | `key_event_markers.go:636-641` | ✅ 全部 `?` 占位；日期比较用 `Format("2006-01-02")` 字符串，与既有 `ListTopicKeyEventMarkers` 同款（`date` 列，DB 会话时区 Asia/Shanghai，与前端 `todayInShanghai` 同口径） |
| handler 边界 | `key_event_handler.go:163-200` | ✅ `:id` 非数字→400；`days` 非正整数→400（独立校验，不动话题端 `days=0=全部` 旧契约）；版块不存在/非 board→404；`semantic_board_id = ?` 硬过滤 → 无跨版块泄露（无越权面） |
| 路由不冲突 | `grep -rn "semantic-boards/:"` | ✅ 全部 `:id` 同名（gin trie 约束），`/semantic-boards/overview` 为静态段既有先例 |
| 既有写端未被改 | `git diff` 话题级读端/写端 | ✅ `parseKeyEventDays`、`ListTopicKeyEventMarkers`、`putThreadKeyEvent`、`patchKeyEventMarker` 均未改（红线 10 守住） |

## 4. 问题清单

### R-1 2026-11-23【Medium】右轨联动（D14）的实现作用域超出 delta spec：归档视图内点右轨也会被切走视图
- **状态**：已修复
  - 处置说明（主线程 2026-11-23）：实现子线程 2026-11-23：切视图改为**只在 `activeView === 'events'` 时**发生（`useConsoleShellActions.locateLaneFromRail` + `BoardConsoleShell.handleSelectOtherFromRail`）。归档视图恢复旧行为（不切视图、不消费归档返回滚动位），矩阵视图不变。spec 已是「在关键事件视图里」，实现向它对齐，未改 spec。回归：`BoardConsoleShellRailViewSwitch.test.ts` +2 例（归档视图两条右轨入口均不得切走），反向验证回滚两处条件 → 2 例变红 `expected false to be true`。

**问题**：delta spec 的 Scenario「右轨选中切回矩阵视图」把 WHEN 明确限定为「**用户在关键事件视图里**从右轨选中」；design D14 的措辞也只针对「右轨发起的选中在事件视图里是死选择」。但实现把「切回矩阵」做成了**无条件**动作：`useConsoleShellActions.locateLaneFromRail` 对任何当前视图都先 `handleSetView('matrix')`，`BoardConsoleShell.handleSelectOtherFromRail` 同样无条件切矩阵。右轨 aside（`BoardConsoleBody.vue` 的 `.console-gov`）在归档视图下同样常驻渲染，因此**归档视图里点右轨「话题追踪」/「当日其他话题」会把主舞台静默切到矩阵**。

**证据**：`useConsoleShellActions.ts:95-110`（`deps.batchQueryWrite(() => { handleSetView('matrix'); deps.selectLaneFromStage(topicId) })`）、`BoardConsoleShell.vue:376-388`（`handleSelectOtherFromRail`）、`BoardConsoleBody.vue:186-190`（rail `@select-other="emit('selectOtherFromRail', $event)"`）、`BoardConsoleStage.vue:107-110`（archive 分支与右轨并存）；对照 `specs/workshop-board-console/spec.md`「WHEN 用户在关键事件视图里从右轨（当日其他话题 / 话题条目）选中一个话题」。旧行为（`git show HEAD:front/app/features/tags/components/console/useConsoleShellActions.ts`）：只 `selectLaneFromStage`，视图不动（归档视图里是「点了没反应」的空点击，但归档上下文不丢）。

**影响**：旧主故事「归档 → 站内读正文 → 回原日期」**未断链**（归档视图本身未被改、归档日期仍在壳层 ref），但归档视图内的右轨点击从「空点击」变成「静默换视图」，且 `handleSetView` 的 `archiveReturnScroll` 在这条路径上被消费（`useConsoleShellActions.ts:48-56`）→ 用户对「我在归档里」的心智被打断。这是**行为扩大而非规格要求**，验收账本 A-31/A-32 只在事件视图里验过，归档视图路径无覆盖。

**建议**（二选一，需主线程拍板）：
- (a) 收窄实现：仅当 `activeView === 'events'` 时才切矩阵，其它视图（含归档）保持旧行为；
- (b) 扩写契约：把 D14 与 delta spec 的 Scenario 改成跨视图口径（「右轨发起的话题选中一律切回矩阵 / 追踪」），并在 `ui-verification/report.md` 补一条归档视图内的右轨联动验收条目。

### R-2 2026-11-23【Medium】design 内部结论自相矛盾：D10（固定列宽）与 D11（均分列宽）互斥且 D10 未标注作废；D4 的行高公式与实现不符
- **状态**：已修复
  - 处置说明（主线程 2026-11-23）：主线程 2026-11-23 直接改 design.md：D10 标注【已作废，由 D11 取代】并写明实装/验收按 D11；D4 行高公式校订为 70 / 46+44L（封顶 246），与 ui-design Layout Contract 与实装一致。

**问题**：`design.md` D10 的结论写「列宽取舒适固定值（7 天 84px / 14 天 72px / 30 天 64px）」，D11 的结论写「列宽 = `max(64px, 主舞台可用宽 / 可见日数)`」——两条决策互斥，而**实现与 delta spec 走的是 D11**（`keyEventLayout.ts:computeEventColumnWidth` = `max(64, available/days)`；本轮为过 `openspec validate --strict` 还把 spec 正文从「舒适固定值」改成了「均分可用宽度」，见 `ui-verification/report.md` 末「附：规格措辞修订」）。D10 没有标注「被 D11 取代」。同族问题：**D4 的行高公式「无标签 46px / 有标签 38 + 层数 × 24」与实现不符**（实现 `keyEventRowHeight`：基础 70px、`46 + 44×层数`、封顶 246px；ui-design Layout Contract 与 flow 文档口径均为 70px / +44px / 246px 封顶）。

**证据**：`design.md` D10/D11/D4 三节 vs `front/app/features/tags/components/console/keyEventLayout.ts:60-66,113-118` vs `docs/reference/flow/semantic-board.md`「行高 = 无标签 70px，有标签按层数每层 +44px（D4）」——注意 flow 文档把 70px/+44px 也挂在了 D4 名下，说明 D4 正文才是过时的一方。

**影响**：design 是随 change 归档的永久制品。归档后按 D10 读的人会以为列宽是固定值（进而得出「2560 右侧留白是 bug」的误判，A-12 正是反的）；按 D4 读的人会算出 38+24L 的行高，与 246px 封顶契约对不上。当前**无运行时缺陷**（实现是自洽的），属制品一致性缺陷。

**建议**：在 design.md 里把 D10 标注为「已由 D11 取代（列宽均分）」、把 D4 的行高公式改成实现口径（或注明「行高口径以 ui-design Layout Contract 为准」），并在 tasks 里加一行同步动作。

### R-3 2026-11-23【Medium】当日阅读区的事件标题仍用单行省略号截断，与 D7「名称由选中日阅读区承担」的兜底相冲突
- **状态**：已修复
  - 处置说明（主线程 2026-11-23）：实现子线程 2026-11-23：`.kdp-card-title` 去掉 `white-space: nowrap` / `overflow: hidden` / `text-overflow: ellipsis`，改 `white-space: normal` + `overflow-wrap: anywhere`（**不用 tooltip 糊弄**）。只动标题规则块，话题快照 `.kdp-card-topic` 未动。回归：`KeyEventDayPanel.test.ts` +2 例（长标题逐字渲染 + 源码契约断言标题块无 `text-overflow`/`nowrap`）。

**问题**：D7 规定「堆叠层数超上限（4 层 / 行高封顶 246px）的行才退化为『节点 + 当日事件计数（★N）』，**名称由选中日阅读区承担**」。但当日阅读区的事件卡标题 `.kdp-card-title` 是 `white-space: nowrap; overflow: hidden; text-overflow: ellipsis`（话题快照 `.kdp-card-topic` 同款）→ 行收起（轴上已无名称）时，**兜底位置的名称仍可能被省略号切成残句**，用户只剩原生 `title` 悬浮提示可看全文。

**证据**：`front/app/features/tags/components/console/KeyEventDayPanel.vue`（`.kdp-card-title` / `.kdp-card-topic` 样式块）与 `design.md` D7「名称由选中日阅读区承担」；对照时间轴标签侧是干净的：`KeyEventLabel.vue` 的 `.kev-label-text{white-space:nowrap}`、`.kev-label{max-width:none}`，全仓 `grep -nE '#[0-9a-fA-F]{3,8}|rgba?\('` 对三个新组件零命中。

**影响**：属**潜在**缺陷而非已观测失败——验收样本里的标题长度（如「GPT-6.2 Astra与Sol双模型发布，分层定位专业与日常」）在 1440 宽下未触发溢出；一旦标题更长或窗口更窄，截断即出现，且恰好落在「标签已收起、只能靠阅读区看名字」的场景。

**建议**：卡片标题允许换行（`white-space: normal`，或两行 `-webkit-line-clamp`），至少在「该行已收起标签」时不做省略号；如坚持单行，须在 D7 里改写「名称由选中日阅读区承担（悬浮提示可见全文）」的口径。

### R-4 2026-11-23【Medium】请求窗口被 365 上限截断：「近 1 年」档在版块最新日报落后今天时，轴最左侧若干天的关键事件取不到（静默缺数据）
- **状态**：已修复
  - 处置说明（主线程 2026-11-23）：实现子线程 2026-11-23：**去掉 365 封顶**，公式抽成纯函数 `computeKeyEventRequestDays(axis, today, sourceDays) = max(1, max(sourceDays, daysBetweenInclusive(axis[0], today)))`。有界性：轴左沿被池边界钳住（`axis[0] ≥ anchor−364`、`anchor ≤ today`）→ `span ≤ 365 + (today − anchor)` = 池跨度 + 日报滞后天数。函数头注释写明「池 365 是泳道数据口径、关键事件读端 days 接受任意正整数、封顶 = 轴最左 N 天静默少事件」。回归：`keyEventLayout.test.ts` +5 例、`KeyEventTimelineView.test.ts` 接线断言 captured days=376（>365）；反向验证塞回 `Math.min(365,…)` → 3 例变红 `expected 365 to be 376`。

**问题**：事件批量读的窗口由前端算：`requestDays = max(sourceDays, daysBetweenInclusive(axis[0], today))`，**再被 `Math.min(365, ...)` 截断**；后端窗口是 `[今天−(days−1), 今天]`。而事件视图的观察范围可以回看到 **池左沿** `[anchor−364, anchor]`（`anchor` = 该版块最新 completed 日报日，**通常落后今天 1 天以上**）。当 `anchor < today` 时：`daysBetweenInclusive(axis[0], today) = 365 + (today − anchor) > 365` → 被截成 365 → 后端窗口 = `[今天−364, 今天]`，**而轴的起点 `anchor−364 = 今天−364−(today−anchor)` 早于窗口起点** → 轴最左侧 `(today − anchor)` 天的事件**不在载荷里**，视图把它们渲染成「无确认事件」，与真相不符且无从分辨。

**证据**：`front/app/features/tags/components/console/KeyEventTimelineView.vue:88-95`（`Math.max(1, Math.min(365, Math.max(props.sourceDays, span)))`）、`front/app/features/tags/components/console/observationRange.ts:71-74`（`defaultRangeForTier(anchor, 365)` = `[anchor−364, anchor]`，即 365 档的**默认**范围就是池左沿）、`front/app/features/tags/components/console/useObservationWindow.ts:118-124`（`poolBounds = [anchor−(POOL_DAYS−1), anchor]`）、`backend-go/.../key_event_handler.go:184-189`（后端窗口 = `[today−(days−1), today]`，`days` 无上限）。触发条件：切「近 1 年」档（默认范围即池左沿）+ 版块最新日报落后今天 ≥1 天（常态）→ 轴最左 1..N 天的事件静默丢失；N = 今天−anchor（日报越旧丢得越多）。验收账本 A-20/A-29 只核了「标签是否照常显示」，没核「窗口内事件条数 = 接口返回条数」，故此缺口未被实机发现。

**影响**：本视图的主卖点就是「哪几天出了大事」，静默少几天 = 结论不可信（且空态文案「这段时间没有确认的关键事件」会让用户以为真的没有）。属**正确性缺陷**，触发路径是默认档 + 常态数据滞后，不算冷门；但丢失量小（通常 1 天、最多少量天）且不影响其它交互。

**建议**（一行级修法）：把上限从「天数」改成「不早于轴起点」——即 `requestDays = max(sourceDays, daysBetweenInclusive(axis[0], today))` **不再截 365**（后端接受任意正整数），或把上限放宽到 `POOL_DAYS + (today − anchor)`；并在验收账本补一条「请求天数 ≥ 轴跨度」的机械断言（轴长 vs 载荷覆盖区间）。

## 5. 审毕无发现（显式声明）

- 审毕无发现（审查范围：后端只读批量读端点 —— `repository/key_event_markers.go:ListBoardKeyEventMarkers` 与 `handler/key_event_handler.go:parseBoardKeyEventDays`/`listBoardKeyEventMarkers`：SQL 全 `?` 参数化、日期比较与既有 `ListTopicKeyEventMarkers` 同款（`date` 列 + DB 会话时区 Asia/Shanghai，与前端 `todayInShanghai` 同口径，无时区偏移）、`state='confirmed'` 硬过滤、`semantic_board_id = ?` 硬隔离（无跨版块泄露）、`days` 严格正整数且**不复用**话题端 `days=0=全部` 的旧契约、版块不存在/非 board → 404、`:id` 非数字 → 400、空结果 200 + 空数组、路由 param 名与既有 `/semantic-boards/:id/*` 一致（gin trie 无冲突）；`go vet ./internal/topicgraph/...` 退出码 0）
- 审毕无发现（审查范围：前端只读性与状态机 —— 新增组件/composable/API 层零写调用（`grep` 写端点与 `KeyEventMarkerActions` 仅注释命中）、零 `suggested`/`dismissed` 路径、候选话题事件在时间轴与当日阅读区同时被 `rowTopicIds` 过滤（无孤儿事件）、无事件日不落节点不伪造 0、`state` 四态（loading/empty/error/stale）各自分支互斥且都诚实（失败不冒称 0 条、刷新失败保留旧内容并标 stale）、组件卸载时 `ResizeObserver` 解绑且函数 ref 在容器重建时重新挂载（`setScrollRef` 的 `observedSurface` 判定）、`measuredLabelWidths` 兜底缓存使「收起/展开」不振荡、双主题零硬编码色值、标签不截断（`KeyEventLabel` 无 `text-overflow`））
- 审毕无发现（审查范围：行序/时间视窗/选中态共用 —— 事件视图行集合经 `buildConsoleLaneGroups → observationLanes.ts:buildObservationRows` 与矩阵同源（同一 `visibleAxis`、同一行序与休眠判定、同一 `selectedLane`/`selectedDate` 受控契约），未新建第二份行序/跨度实现；`requestDays` 与轴跨度对齐（除 R-4 的 365 截断外）、`visibleAxis` 与观察范围一致；切换视图不重置选中态（A-5/A-33 实机 + 组件用例双证））
- 审毕无发现（审查范围：迁移/替换型能力去向 —— 旧能力去向表 11 行逐行核对，除 R-1 的作用域扩大外无「遗漏」；三条真人主故事（旧矩阵故事、归档往返、事件→文章→阅读）均可走通；既有矩阵/窄屏/归档回归用例 56 条全绿）
- 审毕无发现（审查范围：制品与文档一致性 —— `proposal.md`/`design.md`/`ui-design.md`/`tasks.md`/`stage.md` 与两份 delta spec 的行为条款逐条对齐（除 R-2/R-5 指出的 design 内部矛盾与指针失效）；`docs/reference/flow/semantic-board.md` 红线 37 与「变更溯源」行、`docs/reference/flow/topic-graph.md` 版块级批量读节与溯源行、`docs/reference/api/semantic-boards.md` 新端点章节齐备；`KeyEventMarkerActions` 仍唯一挂在日报阅读台 `DailyReportTopicSection.vue`，写动作主权未外溢（红线 10））
### R-5 2026-11-23【Low】design 落点指针失效 2 处：D5 指向不存在的 Scenario「候选话题合并为一行」，D7 写成「层数超限才收起」
- **状态**：已修复
  - 处置说明（主线程 2026-11-23）：主线程 2026-11-23 改 design.md 落点：D5 → Scenario「候选话题不占行」（真实存在，且与 D2 被否方案区分）；D7 → Scenario「层数超限才收起标签」。两处现已可解析。

**问题**：D5 的落点写「Scenario『候选话题合并为一行』」——该 Scenario 在 delta spec 中不存在（实际标题是「候选话题不占行」），且「候选话题合并成一行」恰是 **D2 取舍里被否掉的方案**，指针语义与决策相反；D7 的落点写「Scenario『层数超限才收起』」，实际标题为「层数超限才收起标签」（前缀近似）。按 §0.6 步骤 0，二者按「未解析」计。

**证据**：`design.md` D5 / D7 的「落点」行 vs `specs/workshop-board-console/spec.md` 的 Scenario 标题清单（「候选话题不占行」「层数超限才收起标签」）。

**影响**：**不构成「可删项」**——两条决策的结论都有下游承接（D5 → 新增 `KeyEventTimelineView.vue`/`KeyEventLabel.vue` + delta spec Requirement「关键事件节点与名称」「关键事件标签布局与横向滚动」；D7 → Scenario「层数超限才收起标签」「长窗仍看得见事件名称」+ ui-design Layout Contract + flow 红线 37）。只属指针失效，后续按落点反查会查不到。

**建议**：D5 落点改为真实 Scenario 标题（或改为仓库文件 `front/app/features/tags/components/console/KeyEventTimelineView.vue` + 新增组件一句），D7 补全为「层数超限才收起标签」。

### R-6 2026-11-23【Low】取数缓存随视图切换整体失效：composable 实例挂在事件视图组件内，切一次视图就重发一次板级请求
- **状态**：已驳回
  - 处置说明（主线程 2026-11-23）：需把 `useBoardKeyEvents` 移出事件视图组件的 `v-if` 作用域（挂到壳层），会改动 `BoardConsoleStage`/`BoardConsoleShell` 的分支结构，风险大于收益：请求量小（一次板级批量读），切回重取无用户可感延迟。属独立优化议题，不在本 change 收尾阶段动结构。

**问题**：`useBoardKeyEvents` 在 `KeyEventTimelineView.vue` 内部实例化，而该组件由 `v-if="activeView === 'events'"` 挂载 → 视图一切换即卸载，实例内的 `cache`（board+days → 事件）随之丢失。因此「同一窗口往返不重发」只在组件存活期间成立，**每次切回「关键事件」都会重新请求一次**。

**证据**：`BoardConsoleStage.vue`（`v-if="activeView === 'events'"` 挂 `KeyEventTimelineView`）、`useBoardKeyEvents.ts:71`（`const cache = new Map(...)`，实例内私有）、`useBoardKeyEvents.ts:132-134`（`watch([deps.boardId, deps.days], ...)`，挂载即 `immediate: true` 触发一次请求）；对照 tasks 2.2 的表述「按 board + days 缓存、loading/error/stale 三态、切板块晚到隔离」与 design D3「一次请求取全观察窗内的 confirmed 事件」。

**影响**：功能与数据正确（late-arrival 隔离与三态都在位），只是与「往返不重发」的意图有落差——切视图频繁时多打板级请求；`:3` 每次切回都会看到一次「正在读取关键事件…」（`eventsLoading && windowEventCount === 0`）。

**建议**：把 composable 提升到壳层（`useBoardConsole` 或 `BoardConsoleShell`）持有、把 `events` 透传给视图；或在 tasks 2.2 里把口径改成「同窗同挂载期间不重发」。

### R-7 2026-11-23【Low】切版块且事件批量读失败时，会显示上一个版块的事件（stale 条未点明版块）
- **状态**：已修复
  - 处置说明（主线程 2026-11-23）：实现子线程 2026-11-23：`useBoardKeyEvents` 新增 `contentSource`（**发布时快照** `{boardId, boardLabel}`，避免切版块后当前版块名指错），失败保留旧快照；视图拼文案——同版块「版块「X」之前的数据」、跨版块追加「（不是当前版块的数据）」、名缺失回落 `版块 #<id>`。未改成切版块清空数据（会破坏 stale 语义）。回归：`useBoardKeyEvents.test.ts` +2、`KeyEventTimelineView.test.ts` +3。

**问题**：`useBoardKeyEvents.load()` 的失败分支只置 `error` / `staleNotice`，**不清 `events.value`**；视图的「事件层首错」判据是 `eventIndex.size === 0` → 有旧数据时不走首错分支，于是上一个版块的事件继续渲染在当前版块下，提示条只说「当前显示的是之前的数据」，不说是哪个版块的数据。

**证据**：`useBoardKeyEvents.ts:96-104`（失败分支不清 events）、`KeyEventTimelineView.vue:466-470`（`eventsFirstError` 判据）、`useBoardKeyEvents.ts:104`（`if (events.value.length > 0 || cache.has(key)) staleNotice.value = true`）。

**影响**：单用户本地应用无越权风险；口径与 A 通道一致（`useBoardConsole.loadSource` 失败同样保留旧 `data.value`），故按「与既有模式一致」记 Low。但本视图把「事件按版块取」做成了新维度，跨版块残留比跨窗口残留更容易误导（行头写的是当前版块的话题名，节点却是上一个版块的）。

**建议**：失败分支同时把 `events` 标记为「非当前版块」并在提示条里点名旧版块（或失败即清空 + 走首错分支），与切档失败的口径区分开。

### R-8 2026-11-23【Low】窄屏 + `?view=events` 深链：事件视图不渲染（符合 spec），但阅读槽被连带隐藏且头部无按下态视图按钮
- **状态**：已修复
  - 处置说明（主线程 2026-11-23）：实现子线程 2026-11-23：双保险——① `BoardConsoleShell` 窄屏时把 `activeView === 'events'` 回落为 `matrix` 并清掉 URL `view`（窄屏忽略该深链，URL 与画面一致）；② `BoardConsoleStage` 阅读槽条件改 `activeView !== 'events' || isNarrow`（窄屏永远有阅读槽，避免任何一帧空舞台）。窄屏 `BoardNarrowSummary` 分支与矩阵窄屏形态零变化。回归：`BoardConsoleShellStates.test.ts` +1 例（窄屏 + `?view=events` → 不进事件视图、摘要与阅读槽都在、URL 已清）。

**问题**：窄屏降级是对的（`BoardNarrowSummary` 的 `v-if="isNarrow"` 分支在事件视图分支之前 → 事件视图不渲染，符合 delta spec「窄屏不渲染」），但两处副作用没被 spec 覆盖：① `.stage-chronicle`（唯一阅读槽）被加上了 `v-if="activeView !== 'events'"`，而该 div 在 `isNarrow` 分支**之外**（窄屏也渲染）→ 窄屏下按 `?view=events` 打开会**丢掉阅读槽**；② 头部「关键事件」按钮 `v-if="!isNarrow"` 不渲染，而 `activeView` 仍是 `'events'` → 「矩阵 / 追踪」与「文章归档」两个按钮都 `aria-pressed=false`，**没有任何按钮处于按下态**。

**证据**：`BoardConsoleStage.vue`（`BoardNarrowSummary v-if="isNarrow"` 在前、事件视图分支在其 `v-else` 内；`.stage-chronicle` 的 `v-if="activeView !== 'events'"` 加在窄屏分支之外）、`BoardConsoleHeader.vue`（「关键事件」按钮 `v-if="!isNarrow"`）、`useBoardConsole.ts`（`initialView = parseViewParam(route.query.view)` → `activeView='events'`，窄屏不回落 matrix）。

**影响**：窄屏用户打开分享链接（`?view=events`）→ 摘要照常渲染、但阅读槽消失、seg 组无按下项；可点「矩阵 / 追踪」恢复（`activeView` events→matrix 会重挂 chronicle），不至于卡死，故记 Low。

**建议**：窄屏下若 `activeView === 'events'` 则在初始化回落 `matrix`（或至少保留 `.stage-chronicle`），并在 delta spec 的「窄屏不渲染」Scenario 里补一句「窄屏打开 `?view=events` 深链时的落点」。

### R-9 2026-11-23【Low】残留竞态：`useObservationWindow` 仍是独立 query writer，与新增的 `batchQueryWrite` 不同源
- **状态**：已驳回
  - 处置说明（主线程 2026-11-23）：需把 `useObservationWindow` 的 query 写入并入 `batchQueryWrite`，属「观察窗 query 通道」重构，与本 change 的视图切换修复不是同一处，且人手不可复现（两次写入跨事件循环任务）。记为结构性风险、后续独立处理。

**问题**：A-34 的根因是「同一动作里两次独立 `router.replace` 各自基于尚未更新的 `route.query` 重建 query，后写覆盖前写」。修复只在 `useBoardConsole` 内部加了一个合并作用域（`batchQueryWrite` / `applyQueryPatch`），**观察窗的写入仍是另一套独立 writer**（`useObservationWindow.writeQuery` 自行 `router.replace({query:{...route.query}})`）。只要观察窗写入与视图写入发生在同一次导航落地之前，后者仍会被抹掉：`view=events` 丢失 → `useBoardConsole` 的 view watcher 把 `activeView` 打回 `matrix` → 用户被静默踢出事件视图。

**证据**：`useObservationWindow.ts:99-107`（独立 `writeQuery`）、`useBoardConsole.ts:118-135`（`applyQueryPatch` / `batchQueryWrite`）、`useBoardConsole.ts:360-366`（view watcher：`requested ?? 'matrix'` → 直接改 `activeView`）。

**影响**：人工操作下窗口极小——`router.replace` 的导航在微任务内落地，两次点击跨事件循环任务，人手不可复现；实现线程也已自曝该残留。**不是 High**。但它与 A-34 是同一类风险，且「视图状态由 URL 派生 + URL 有多个独立 writer」的结构性前提还在，后续任何人再加一个 query writer 都会重蹈。

**建议**：把观察窗写入并入同一 `batchQueryWrite`（或让 `useObservationWindow` 与 `useBoardConsole` 共用一个 query writer），消除同类风险的复发面。

