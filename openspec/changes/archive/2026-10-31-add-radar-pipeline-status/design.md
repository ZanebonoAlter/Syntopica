# Design: add-radar-pipeline-status

## Context

见 [proposal.md](proposal.md) 的 Why 与 [specs/](specs/) 的行为契约。本节只记录影响实现路径的现状事实：

- **日报生成闸是进程/数据库级单例**：`topicgraph/handler/daily_report_handler.go:151` 在受理请求时取闸（DB 级 advisory lock，不可达时回退进程内互斥），闸由持有管线在结束时释放。因此**一个请求 = 一条管线 = 一次终态广播 = 一条通知**是硬约束，不存在"一个请求拆成多条管线"的选项。
- **既有生成端点只支持单板或全量**：`triggerGenerateDailyReport` 的入参是 `board_id *uint`（nil → 全量）。全量路径的板块集来自 `CollectBoardIDsForDate(date)`（当日有 event tag 文章的板块），单板路径绕过该集合直接生成指定板。
- **`/api/poll` 已是常驻状态单一入口**：`usePollBundle` 把响应分发到 `useState` 键 `tag-queue-progress:status` 与 `poll:schedulers`；`client-poll-budget` 契约禁止为常驻状态另建周期性请求。
- **`schedule_time` 已存在但 `schedule_deadline` 未暴露**：`admin/handler/scheduler_handler.go:469` 的 `enrichStatus` 对 `daily_report` 填了 `LoadDailyReportTimeConfig()` 的值；`LoadDailyReportDeadlineConfig()` 存在于 `platform/aisettings/config_store.go` 但无任何 HTTP 出口。
- **雷达页不挂顶栏**：`BoardRadarView` 直接用 `AppPageShell`，不经过 `FeedLayoutShell`/`AppHeaderView`，故 `TagQueueProgressChip` 在该页不渲染。
- **overview 载荷的 `report_date` 语义**：`topicgraph/repository/board_radar_repository.go:32` 注释明确为"latest report period_date"，即**该板最新日报日（不限于今天）**；`out.Date` 是各板 `report_date` 的最大值。

## Goals / Non-Goals

**Goals:**
- 状态块零新增 HTTP 请求：全部数据来自已常驻的 `/api/poll` 分发与雷达页已有的 overview 载荷。
- 手动生成在闸的语义下保持"一次请求 = 一条管线 = 一次终态"，集合作用域不破坏既有互斥、通知、WS 契约。
- 状态块严格遵守 R-17：失败态与未取数态不得呈现业务结论。

**Non-Goals:**
- 不改日报生成**时机**（约束 23 的队列感知、21:00 门槛、23:30 兜底）——本 change 只**展示**这些时刻，不改调度逻辑。
- 不改 L0 雷达的信息架构与分档口径（关注度、热点/关注/常温分档、升温判定）。
- 不动 `TagQueueProgressChip`、不动 L1 版块态势页（`/tags/:boardId`）的任何界面。
- 不做定时/自动触发；手动生成始终是显式用户动作。

## Decisions

### D1. 集合作用域用 `board_ids` 数组扩展既有端点，不新增端点

- **结论**：`POST /api/daily-reports/generate` 增加可选 `board_ids []uint`；与既有 `board_id *uint` 互斥；缺省或空数组保持全量语义。
- **落点**：`backend-go/internal/topicgraph/handler/daily_report_handler.go:triggerGenerateDailyReport`
- **取舍**：被否方案（甲）新增 `POST /api/daily-reports/generate-batch` 端点——多一个端点就要多一处闸获取逻辑与多一份前端 API 封装，且现有端点的互斥/重建窗口守卫会被复制一遍，后续易漂移；（乙）前端循环调 N 次单板请求——闸是单例，第 2 次必被 409 拒绝，除非串行等待每次终态（WS 驱动），复杂度高且中途失败难恢复。

### D2. 集合内板块集合与全量板块集的关系：集合优先，不与"当日有内容"求交

- **结论**：`board_ids` 非空时**直接按请求集合生成**，不再用 `CollectBoardIDsForDate(date)` 过滤；某板当日无内容时走既有"版面无内容不算失败"路径（`report == nil` → `broadcastProgress("completed")` + 成功通知）。
- **落点**：`backend-go/internal/topicgraph/handler/daily_report_handler.go:generateSingleBoard`
- **取舍**：被否方案（与当日有内容集合求交）——用户显式勾选某板却因"当日无内容"被静默跳过，会让人以为生成成功但目标板没有日报；显式集合下"该板当日无内容 → 生成 0 篇并显示完成"更诚实（与单板语义一致）。

### D3. 多板块生成复用一个 goroutine 顺序循环，不并发

- **结论**：新增 `generateSelectedBoards(date, boardIDs, jobID)`，与 `generateAllBoards` 同构（顺序 for 循环、逐板广播 progress、末尾一次 `broadcastDone` + 一次 `notifyDailyReportTerminal`），但板块集来自请求而非 `CollectBoardIDsForDate`。
- **落点**：2.2
- **取舍**：被否方案（每板一个 goroutine 并发生成）——单板生成是重 LLM 管线，并发会放大 LLM 供应商限流与 `ai_call_log` 峰值，且"部分失败"的终态计数语义变复杂（现有终态只认成功/失败两类聚合）；顺序执行与既有全量路径行为一致。

### D4. 闸在集合场景的持有者仍是单个 goroutine

- **结论**：`generateSelectedBoards` 与 `generateAllBoards`/`generateSingleBoard` 一样，在函数入口 `defer releaseGenerationGate()`，闸从 handler 受理到管线结束全程被持有。
- **落点**：`backend-go/internal/topicgraph/handler/daily_report_handler.go:generateAllBoards`
- **取舍**：无备选：闸的持有/释放语义由 `guard-daily-report-generate` 约束固化，"每条管线一个持有者"是既有不变量。

### D5. `schedule_deadline` 从 `enrichStatus` 旁路填入，不改 `GetStatus` 协议

- **结论**：在 `admin/handler/scheduler_handler.go` 的 `enrichStatus` 中，与既有 `schedule_time` 同处为 `daily_report` 补 `ScheduleDeadline`；值取 `aisettings.LoadDailyReportDeadlineConfig()`（自身已有"缺省 23:30"与"早于生成时刻回落 23:30"逻辑）。
- **落点**：`backend-go/internal/admin/handler/scheduler_handler.go:enrichStatus`
- **取舍**：被否方案（在 `platform/scheduler/base.go` 的 `GetTaskStatusDetails` 里加字段）——那是通用调度器层，会把日报专属配置语义下沉到平台层并影响所有调度器；`enrichStatus` 已是"按调度器 key 分派专属字段"的既有位置（`board_upgrade_suggest` 同款）。

### D6. 前端不新建轮询：状态块复用 `usePollBundle` 的模块级单例

- **结论**：`useRadarPipelineStatus` 直接读 `usePollBundle` 已分发的 `useState`（`tag-queue-progress:status` / `poll:schedulers`），并 `ensureStarted()` 启动单例（幂等）；**不**新增任何 `setInterval` 或新端点请求。
- **落点**：`front/app/features/tags/composables/useRadarPipelineStatus.ts`
- **取舍**：被否方案（状态块自己 `GET /api/poll` + 自己定间隔）——直接违反 `client-poll-budget` 契约（"MUST NOT 为常驻状态类数据各自建立独立的周期性请求"），且与 `useNotifications`/`useSchedulerStatus` 重复对账。

### D7. "今日是否已出"按各板 `report_date` 与**本地**今天比对，不用 `out.Date`

- **结论**：`out.Date`（overview 顶层 date）是各板 `report_date` 的最大值，语义是"最新日报日"，**不等于今天**；当日判定 SHALL 用 `boards.some(b => b.report_date === todayKey())`。`todayKey` 用本地时间拼 `YYYY-MM-DD`。
- **落点**：`front/app/utils/todayKey.ts`
- **取舍**：被否方案（用 `out.Date` 当"今天"）——`out.Date` 在最旧的一天就会整体回退一整天，且与"今日该读"列表的 `report_date` 混用同一字段会误判"今日已出"；被否方案（`new Date().toISOString().slice(0,10)`）——UTC 截断在 UTC+8 的 00:00–08:00 会得到昨天（既有 `DailyReportGenerateDialog.vue` 正犯此错，本 change 不复刻）。

### D8. 队列门槛用 `pending + processing > 0` 判"有积压"，与 `useTagQueueProgress` 的活跃量口径一致

- **结论**：生成按钮的禁用判据复用既有 `activeCount = pending + processing`（`useTagQueueProgress` 同一口径）；`failed > 0` 单独以红色提示呈现，**不**因此禁用生成。
- **落点**：`front/app/features/tags/composables/useTagQueueProgress.ts:activeCount`
- **取舍**：被否方案（把 `failed` 也算进积压从而禁用生成）——失败任务不会自行被消费成打标结果，但它们不代表"还有文章要打标"；混在一起会让"生成可用性"的判据不可解释。取舍代价：失败项仍需用户去设置页重试，状态块只做提示不做重试入口（重试是写操作，超出本 change 的只读诊断定位）。

### D9. 状态块独立组件，右栏两段容器在 `BoardRadarView` 改造

- **结论**：新建 `BoardRadarStatusBlock.vue`（自身持有渲染与交互）；`BoardRadarView.vue` 把右栏从单列滚动区改为「上段可滚内容 + 下段常驻状态块」两段容器。
- **落点**：`front/app/features/tags/components/console/BoardRadarView.vue`
- **取舍**：被否方案（把状态块直接塞进 `BoardRadarReadingRail.vue`）——该组件的既有定位是"纯展示、不改口径"，塞入带写操作（手动生成）的状态块会破坏其边界；被否方案（新建 `BoardRadarPipelinePanel.vue` 包裹阅读栏+状态块）——多一层无信息增量的包装组件。

### D10. 手动生成走"状态块内就地展开清单"，不弹窗

- **结论**：点「立即生成」在状态块内就地展开板块勾选清单（默认全选、可全选/全不选、可取消勾选），确认后发起一次生成；不引入 `AppDialog`。
- **落点**：3.5
- **取舍**：被否方案（复用 `DailyReportGenerateDialog.vue` 弹窗）——该弹窗是单板 `<select>` + 日期选择器 + 进度卡，且用了旧的 `width="400px"` 自由宽度 prop（新代码禁止）；要支持集合勾选得重写它，反而牵连 L1 版块页既有界面。

### D11. 生成进度复用既有 WS 事件，不新增推送

- **结论**：`daily_report_progress` / `daily_report_done` 已是全局广播，状态块订阅之即可；集合生成沿用同一事件格式（`progress` 字段为 `n/total`），前端不新增事件类型。
- **落点**：`front/app/utils/eventTypes.ts:DAILY_REPORT_PROGRESS`
- **取舍**：无备选：既有事件格式已能表达集合进度（`progress: "current/total"`），新增事件类型会制造两套同义协议。

### D12. 终态通知与 WS 终态消息共用三态口径（不再自算“总版面数”）

- **结论**：通知中心的日报终态通知不再自己推导“总版面数”，改为与 `daily_report_done` 终态消息共用同一套三态计数（`total_boards` / `total_saved` / `failed_count` / `empty_count`）：通知文案里的「共 N 个版面」的 N 恒等于 `total_boards`（**含**当日无内容的板块），当日无内容的板块以「其中 M 个当日无内容」显式给出且不进失败数。新增 `notification.DailyReportTerminal(date, DailyReportCounts{...})` 作为三态口径的唯一写入口（成功侧写「N 个版面已产出日报”，不再写“保存 N 条条目”）。
- **适用范围**：该口径**手动路径与定时调度路径共用**——定时调度路径（`internal/admin/scheduler/job_daily_report.go`）同样统计三态（产出日报 / 生成报错 / 当日无内容），其终态通知走同一个 `notification.DailyReportTerminal` 写入口，不走旧的两参入口。否则同一个「生成日报」动作在两条路径下会给出不同的「共 N 个版面」（旧口径下定时路径的失败汇总按 `success+failed` 推导总数，漏掉当日无内容的板块；完成通知的成功数也不含无内容板块）。
- **落点**：`backend-go/internal/platform/notification/service.go:DailyReportTerminal`、`backend-go/internal/topicgraph/handler/daily_report_handler.go:notifyDailyReportTerminal`、`backend-go/internal/admin/scheduler/job_daily_report.go:adjudicateDailyReportTerminal`
- **取舍**：备选是“改签名 + 改全部调用点”——`internal/admin/scheduler/job_daily_report.go` 与 `internal/admin/handler/notification_handler_test.go` 都持有 `DailyReportSuccess` / `DailyReportFailedSummary` 的函数类型，改签名会连带改动跨包代码。选“加新函数 + 旧入口保留签名（降级为无当日无内容计数的旧口径）”能让手动 / 集合路径立即同口径而不动跨包签名。**代价**：定时调度路径切到三态写入口后，旧两参入口在生产代码里已无调用方（仅余 `internal/platform/notification/*_test.go` 与 `internal/admin/handler/notification_handler_test.go` 的测试引用），其“按 `success+failed` 推导总版面数”的旧口径成为**待清理的死代码**，本轮保留仅为不动跨包签名，后续可随迁移一并清理。
- **定时路径的 WS 广播不变**：`daily_report_complete`（`report_count` / `date` / `timestamp`）协议本轮 SHALL NOT 扩字段（当前全仓无消费方，但与手动路径的 `daily_report_done` 是两种消息类型，扩字段会引入第三种变体），三态计数只在终态通知里给出。

### D13. 闸探测失败路径必须归还专用连接（T4 补 `conn.Close()`）

- **结论**：DB 级闸的**探测失败分支（T4）**在 `killSession` 终止会话之后必须补 `_ = conn.Close()` 归还那条专用连接，与 T5（未抢到锁）与 `release()`（释放）两处保持同一纪律。不 Close 时 `*sql.Conn` 永不归还连接池——**每次探测失败漏一条池连接**。
- **落点**：`backend-go/internal/topicgraph/service/generation_gate.go:dailyReportGate.tryAcquire`（T4 分支）；回归用例 `backend-go/internal/topicgraph/service/generation_gate_test.go:TestDailyReportGateProbeFailureReturnsConnection`
- **理由**：(a) **生产侧连接泄漏**——闸探测失败（DB 重启 / 网络抖动 / 探测超时）在生产会反复发生，每次漏一条池连接，池最终被耗尽，全站 DB 访问阻塞；(b) **测试侧单连接池挂死**——`internal/admin/scheduler` 的测试库是 SQLite 且 `MaxOpenConns(1)`，真实闸必探 `pg_try_advisory_lock`（SQLite 无此函数）→ 走 T4 → 漏 Close 即占满单连接池 → 后续查询永久阻塞。这正是该包“测试挂死”的根因（改动前后同样复现，与本 change 的功能改动无关）。
- **取舍**：顺序固定为**先 kill 后 Close**。只 Close 不 kill 会把“会话仍存活、session 级 advisory lock 仍持有”的会话归还池（review H-1 的永久 `already_running`）；先 kill 会话则其 session 级锁随会话终结一并释放，归还/丢弃该连接都不会把锁带回池。不 Close 的代价是漏连接（生产耗尽 + 测试挂死），故取“先 kill 后 Close”。
- **附带（测试面）**：上一轮为绕过该 bug 加的测试 stub（`backend-go/internal/admin/scheduler/job_daily_report_test.go` 的 `stubDailyReportGateInProcess`）已删除——它把真实闸探测路径整个绕开，等于不测真实逻辑；该包用例现走**真实闸路径**。
- **spec**：该条属内部资源管理（不泄漏连接池），但与 Requirement「日报生成 API — 异步触发」已有的闸失败语义（“DB 不可达时 SHALL 回退进程内互斥”“SHALL NOT 造成永久死锁”）同族，故在 `specs/daily-report-system/spec.md` 的该 MODIFIED Requirement 互斥段落补一句「闸探测失败路径 SHALL 归还该次探测占用的数据库连接，SHALL NOT 泄漏连接池连接」，不新造 Requirement。

### D14. 状态块按**双队列**呈现事实，手动生成门槛与定时路径同款

- **结论**：状态块 SHALL 同时呈现**标签队列**与 **Embedding 队列**两个队列的事实（各带自己的计数与今日完成进度），手动生成的清空判据 SHALL 与定时路径一致——`tag_jobs`（pending+leased）与 `embedding_queues`（pending+processing）**双队列**均清空才允许生成。两个队列的量纲不同（文章 vs 标签），SHALL 分别标注。
- **理由**：(a) **只呈现标签队列会说谎**——定时路径的生成时机判据是双队列清空（night-window-alignment 约束 23），只盯标签队列时状态块会报「队列已清空」，而定时任务其实**不会**出日报（真实库 2026-10-30 实测：标签队列 0 积压、Embedding 队列 166 积压）；(b) **Embedding 未清空会静默降级**——日报泳道分桶依赖标签向量到质心的距离，embedding 缺失时文章落进 `unmatched`（未归位）桶，产出的是**桶分桶降级**的日报；(c) **后端手动路径不检查队列**（约束 23 明写「手动 `TriggerNowWithDate` 保持立即执行不等待」），门槛是客户端侧的诚实告知，不是后端拦截。
- **落点**：`backend-go/internal/tagmanagement/handler/embedding_queue_status.go:EmbeddingQueueStatusSnapshot`（新增快照）+ `backend-go/internal/admin/handler/poll_handler.go`（`data.embedding_queue` 段）+ `front/app/composables/useEmbeddingQueueProgress.ts`（新增）+ `front/app/features/tags/composables/useRadarPipelineStatus.ts`（`queuesDrained` 派生）+ `front/app/features/tags/components/console/BoardRadarStatusBlock.vue`（两行并列 + 门槛）
- **取舍**：门槛取**严**（Embedding 有积压也禁用手动生成），而非「只警示不阻断」。理由：手动生成与定时生成产出同构日报，放行降级产出不如让用户等队列跑完（Embedding 队列由 worker 正常消费，实测当天积压会收敛）。代价：Embedding 积压期间手动入口不可用，状态块须把原因说清楚（缺料说明分队列给出）。
- **未知即挡住**：`/api/poll` 响应不含 `embedding_queue` 段（旧后端）时，状态块呈现「状态未知」并挡住生成——R-17「没有数据不产出业务结论」，不得谎称「已清空」。
- **不新建轮询**：Embedding 队列计数随**同一个** `/api/poll` 请求下发（client-poll-budget：常驻状态数据由单一合并入口承载），SHALL NOT 为它另建周期性请求。

## Risks / Trade-offs

- **集合生成与既有"当日已存在则跳过"（约束 23 定时路径）语义不同**：手动路径是"同日重生成 = 整份替换"（`SaveReport` 走 update + 删旧 sections/threads）。用户若对已生成的当日日报再点一次生成，会**覆盖**当天那份。→ 缓解：状态块在"今日已出"时按钮文案改为「重新生成今日日报」并给二次确认（同日替换是破坏性动作）；spec 侧保持既有"同日重生成替换旧 sections/threads"语义不变，本 change 不改它。
- **勾选集合可能包含当日无内容板块**：生成会成功但产出 0 篇（D2 已定为诚实完成）。→ 缓解：状态块显示各板块的当日分区数（来自 overview `today_sections`），用户勾选前即可看出哪些板今天没料。
- **`/api/poll` 的 `schedulers` 数组体积随调度器数量增长**：新增一个字段影响可忽略。→ 无需缓解。
- **Embedding 队列积压时手动入口被锁死**：D14 的严门槛下，Embedding 队列未清空时按钮不可用；若该队列因故长期积压，手动补生成会长时间不可用。→ 缓解：状态块明示「Embedding 队列仍有 N 个标签待向量化」与其后果（未归位桶），用户知道在等什么；Embedding 队列由 worker 正常消费（真实库实测当天积压会收敛），不属常态长期积压。若日后出现长期积压，正确的修法是**修 worker / 提供队列重试入口**（`POST /api/embedding/queue/retry` 已存在），而不是放宽日报门槛。
- **前端状态块订阅全局 WS**：多个组件订阅同一事件是既有模式（`useDailyReportProgress` 已如此），不会产生重复生成（生成只由用户点击触发）。
- **review 未覆盖的风险**：集合生成路径是新代码，未经生产检验。→ 缓解：Review 节（tasks 4.1）明确要求子线程重点核"一个闸/一次终态"与"部分失败的终态计数"。
