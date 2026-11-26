# Tasks: add-radar-pipeline-status

> 变更：雷达总览页（L0 `/tags`）右栏底部新增管线状态块 + 手动日报生成入口（带板块勾选）；后端 `POST /api/daily-reports/generate` 扩展 `board_ids` 集合作用域；`/api/poll` 暴露日报兜底时刻。
> 用例文档：[test-cases.md](test-cases.md)（Scenario 即用例；本 change 声明 `complexity: complex`，白盒分支表与边界值清单见用例文档）。
> <!-- doc-impact: flow, api, standard -->
> <!-- doc-impact-excuse: database=本次只新增一份只读计数快照（查既有 embedding_queues 表），未改 schema/迁移/索引/字段，database 文档无对应变更 -->

## 1. 用例先行

- [x] 1.1 按 [test-cases.md](test-cases.md) 落三故事（S1 状态块 / S2 手动生成 / S3 兜底时刻）的主链路与变体走查用例；复杂档白盒义务（生成按钮状态机 6 态、`board_ids` 请求 6 分支、边界值 9 项）逐条落测试
- [x] 1.2 确认 `daily-report-system` MODIFIED 要求的旧 Scenario 继承清单（8 条）逐条在既有测试文件中保留原断言，集合场景为新增

## 2. 后端：日报生成集合作用域与兜底时刻

- [x] 2.1 `backend-go/internal/topicgraph/handler/daily_report_handler.go`：`triggerGenerateDailyReport` 解析 `board_ids []uint`，与 `board_id` 互斥校验（同时非空 → 400，不启动管线、不取闸）
- [x] 2.2 同文件：新增多板块生成 goroutine（一个闸、一个 `job_id`、逐板生成、单板失败不中断其余、一次 `daily_report_done` 广播、一次终态通知）；`board_ids` 缺省或空数组 → 保持既有全量语义
- [x] 2.3 `backend-go/internal/admin/handler/scheduler_handler.go`：`SchedulerStatusResponse` 加 `ScheduleDeadline`（json `schedule_deadline`）；`enrichStatus` 为 `daily_report` 填该值（`aisettings.LoadDailyReportWindowConfig()` 的 deadline 部分，兜底早于生成时刻时回落 23:30）
- [x] 2.4 `backend-go/internal/platform/aisettings/config_store.go`：确认 `LoadDailyReportDeadlineConfig` 已具备缺省回落与「早于生成时刻回落 23:30」逻辑；缺失则补齐并加单测
- [x] 2.5 **双队列增量**（D14）：`backend-go/internal/tagmanagement/handler/embedding_queue_status.go` 新增 `EmbeddingQueueStatusSnapshot()`（pending/processing/completed/failed/total/completed_today，单位=**标签**；清空判据 = pending+processing，failed 是终态不计入活跃量）；`backend-go/internal/tagmanagement/wire.go` 导出别名；`backend-go/internal/admin/handler/poll_handler.go` 在 `data` 里挂 `embedding_queue` 段（与 `tag_queue` 并列，不合并）
- [x] 2.6 **双队列增量**：`backend-go/internal/admin/handler/poll_test.go` 补 `embedding_queue` 段的键集断言（六个字段）

## 3. 前端：状态块与聚合层

- [x] 3.1 `front/app/utils/todayKey.ts`：本地日期 key 工具（`getFullYear/getMonth/getDate`，**不用 `toISOString()`**——UTC 截断在 UTC+8 会差一天）
- [x] 3.2 `front/app/features/tags/composables/useRadarPipelineStatus.ts`：聚合
  - 标签队列：`usePollBundle` 分发的 `tag-queue-progress:status`（pending/processing/completedToday/failed）+ `poll:schedulers` 中 `daily_report` 的 `schedule_time` / `schedule_deadline`
  - 日报落点：总览载荷各板 `report_date` 最大值；当日判定：`report_date === todayKey()`
  - 生成动作：`useDailyReportsApi().generateDailyReport({date, board_ids})` + `useEventStream` 的 `DAILY_REPORT_PROGRESS/DAILY_REPORT_DONE`
  - 门槛派生：`activeCount = pending + processing > 0` → 按钮禁用 + 缺料说明
- [x] 3.3 `front/app/api/dailyReports.ts`：`generateDailyReport` 参数扩 `board_ids?: number[]`
- [x] 3.4 `front/app/types/scheduler.ts`：`SchedulerStatus` 加 `schedule_deadline?: string`
- [x] 3.5 `front/app/features/tags/components/console/BoardRadarStatusBlock.vue`：状态块组件（四项事实 + 队列进度条 + 生成按钮 + 就地展开的板块勾选清单 + 全选/全不选 + 进度态 + 结果内联 + 失败文案）；token 只用 Layer 2 语义变量，底色用 `color-mix` 派生
- [x] 3.6 `front/app/features/tags/components/console/BoardRadarView.vue`：右栏改「上滚下驻」两段容器（上段内容区独立滚动 + 下段状态块常驻底部），保持 `workspace` 模式与既有断点行为
- [x] 3.7 **双队列增量**（D14）：`front/app/api/embeddingQueue.ts` 的 `EmbeddingQueueStatus` 加 `completed_today?`；`front/app/api/poll.ts` 的 `PollBundleData` 加 `embedding_queue?`；`front/app/composables/usePollBundle.ts` 分发 `embedding-queue-progress:status` + `embedding-queue-progress:ready`（响应缺该段时 ready 保持 false）
- [x] 3.8 **双队列增量**：新增 `front/app/composables/useEmbeddingQueueProgress.ts`（活跃量 / 本轮总量 / 进度 / 清空判据）；`useRadarPipelineStatus.ts` 派生 `embeddingActiveCount` / `embeddingBlocked` / `queuesDrained`，手动生成入口守卫改用**双队列**判据
- [x] 3.9 **双队列增量**：`BoardRadarStatusBlock.vue` 改为**两行并列**（标签队列 / Embedding 队列），各带计数与各自的今日完成进度条；缺料说明按队列分列；门槛改 `queuesDrained`（任一队列有积压即禁用）

## 4. Review

- [x] 4.1 派子线程聚焦 review（改动 diff + 三份制品）：重点核 (a) 集合生成是否真的只占一个闸/一次终态；(b) 门槛禁用是否覆盖所有"队列有积压"路径（含首次对账未成功的 0 值误判）；(c) R-17 语义（失败态不得呈现业务结论）；(d) `todayKey` 是否被误用回 `toISOString()`
- [x] 4.2 人工走查产物路由：编辑 `review.md` 建账（结论 + 处置），每条意见标 High/Medium/Low 并逐条闭环

## 5. 测试

- [x] 5.1 后端单测：`backend-go/internal/topicgraph/handler/daily_report_handler_test.go` 补集合作用域 6 分支 + 互斥 400 + 部分失败终态；`backend-go/internal/admin/handler/scheduler_handler_test.go` 补 `schedule_deadline` 三个场景
- [x] 5.2 前端单测：`front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts`（状态矩阵 + 按钮状态机 + 勾选交互）、`front/app/features/tags/composables/useRadarPipelineStatus.test.ts`（派生口径 + 旧后端降级）、`front/app/utils/todayKey.test.ts`（UTC 跨天回归）、`front/app/features/tags/components/console/BoardRadarView.test.ts`（两段容器）——**注**：`pnpm test:unit` 的路径参数不带 `front/` 前缀（相对 `front/` 匹配）
- [x] 5.4 **双队列增量**测试：后端 `backend-go/internal/tagmanagement/handler/embedding_queue_status_test.go`（计数映射 / completed_today 日界 / 清空判据与 failed 终态）+ `backend-go/internal/admin/handler/poll_test.go`（合并端点下发 embedding_queue）；前端 `BoardRadarStatusBlock.test.ts`（两行并列 + 双队列门槛 + 未知态 + 各自进度条 + 清单展开期间复堵）+ `useRadarPipelineStatus.test.ts`（双队列派生口径 + 生成入口守卫）
- [x] 5.3 效果核对：**真库生成未执行（显式豁免，已留痕）**。理由：该核对需真跑一次 LLM 日报生成管线并**整份替换当日日报**（破坏性、不可撤销），未获用户授权；任务本身已处理完毕（非遗漏）。替代覆盖：（a）`ui-verification/report.md` A-14/A-15 已显式判定为「未覆盖」并写原因；（b）双队列计数、清空判据、门槛与终态三态口径由后端白盒用例（`embedding_queue_status_test.go` / `poll_test.go` / `daily_report_handler_test.go`）与前端组件 + composable 用例覆盖；（c）51 个 delta Scenario 全部映射到实测测试文件（`scenario-trace` 零未映射）；（d）真实库只读侧核对已做：`GET /api/poll` 的 `embedding_queue` 与 `tag_queue` 逐字段与页面一致（2026-10-30 实测）

## 6. 文档

- [x] 6.1 `docs/reference/standard/frontend/layout.md`：更新 §已登记例外（固定视口栅格）表中 L0 `/tags` 雷达行，补「右栏底部新增常驻管线状态块（上滚下驻两段容器）」说明——已补：表格「界面」/「溢出策略」两列标注 `add-radar-pipeline-status` 增补并说明不新增第五区、页面既有布局枚举四种保持不变；表下新增「登记项的增补口径」段（原始登记 change 与增补 change 的归属写法）；§组件速查 增「右栏常驻底部状态块」行
- [x] 6.2 `docs/reference/flow/daily-report.md`：§业务约束与不变量 补一条——手动生成 API 支持 `board_ids` 集合作用域（一次请求=一个闸=一次终态），集合语义与单板语义并存——已补约束 34（集合作用域）/ 35（三态计数口径，手动与定时同一写入口）/ 36（闸探测失败路径归还连接），均按红线句格式写（顶层列表项首个加粗块为自含红线句）；另在 §代码入口 补「前端管线状态块与手动生成入口」条
- [x] 6.3 `docs/reference/architecture/map.md`：若雷达页入口映射需补状态块条目则同步——**判断：无需补条目，故不改 map.md**：map 索引粒度为「业务域 → 目录级入口」，`features/tags/` 已在日报/话题图谱/语义版块三行索引；新增 `console/BoardRadarStatusBlock.vue` + `composables/useRadarPipelineStatus.ts` 属 tags feature 内部实现（同级 radar console 组件本就不在 map 单列），域级入口细节归 flow 文档（已落 `flow/daily-report.md` §代码入口）；本条打勾表示判断已完成、判断结论为「无需补条目」，故该文件未作改动
- [x] 6.4 附带修复（闸探测失败路径归还连接，见 design.md D13）：`backend-go/internal/topicgraph/service/generation_gate.go` 的 T4 分支（探测失败）在 `killSession` 之后补 `_ = conn.Close()`——不 Close 则每次探测失败漏一条池连接，生产侧最终耗尽连接池、测试侧（SQLite `MaxOpenConns(1)`）直接挂死；同步删除上一轮为绕过该 bug 加的测试 stub `stubDailyReportGateInProcess`（该包用例改走真实闸路径）；`specs/daily-report-system/spec.md` 的 MODIFIED Requirement「日报生成 API — 异步触发」补一句「闸探测失败路径 SHALL 归还该次探测占用的数据库连接，SHALL NOT 泄漏连接池连接」——**文档落点**：业务不变量（生产侧不泄漏、先 kill 后 Close 顺序纪律）落 `flow/daily-report.md` 约束 36；测试环境侧表征与排查判据（SQLite 单连接池下整包测试永久挂死、独占连接归还硬约束、回归守卫）落 `standard/backend/testing.md` §集成测试常见陷阱 新增条；两处各写一层、互不重复
- [x] 6.5 **API 文档补漏**（doc-impact 门禁查出）：`docs/reference/api/system.md` 的 `GET /api/poll` 响应补 `embedding_queue` 段与双队列量纲说明；`docs/reference/api/daily-reports.md` 的 `POST /daily-reports/generate` 补 `board_ids` 集合作用域/互斥/去重/终态三态计数；`docs/reference/api/schedulers.md` 补 `daily_report` 的 `schedule_time` / `schedule_deadline` 与双队列时机说明
- [x] 6.6 **约束编号去撞车**：本 change 的四条约束在共享文件 `docs/reference/flow/daily-report.md` 中被重编为 **34/35/36/37**（原 33/34/35/36 与已提交的 add-intra-lane-theme-bucketing 尾部 32/33 撞车；另一并发 change 的未提交 32 也占位）——编号按「归档后改动先入者先占，本 change 顺延后重新编号」处理，交叉引用同步改为 约束 34/35/37 与 testing.md 的 36

## 7. 验证

- [x] 7.1 后端静态检查：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`，期望全部通过无输出错误
- [x] 7.2 后端影响包测试：`cd backend-go && go test ./internal/topicgraph/... ./internal/admin/...`，期望 PASS
- [x] 7.3 前端静态检查：`cd front && pnpm lint`，期望无 error
- [x] 7.4 前端类型：`cd front && pnpm exec nuxi typecheck`，期望无类型错误
- [x] 7.5 前端受影响单测：`cd front && pnpm test:unit app/utils/todayKey.test.ts app/features/tags/composables/useRadarPipelineStatus.test.ts app/features/tags/components/console/BoardRadarStatusBlock.test.ts app/features/tags/components/console/BoardRadarView.test.ts --maxWorkers=2`，期望全部通过（参数不带 `--`；**文件路径不带 `front/` 前缀**——vitest 的 include 相对 `front/`，带前缀会匹配不到任何文件并以 exit 1 退出）
- [x] 7.6 构建：`cd front && pnpm build`，期望构建成功
- [x] 7.7 双视口双主题验收：agent-browser 在 1440×900 与 1920×1080 打开 `/tags`，检查状态块常驻右栏底部、内容区独立滚动、无横向溢出；editorial 与 dark 两主题各留截图，证据写 `ui-verification/report.md`
- [x] 7.8 窄屏基线：375×667 打开 `/tags`，期望状态块落到页面下方、板块清单单列、无横向溢出
- [x] 7.9 归档门禁：`bash scripts/harness/ui-verification-trace.sh openspec/changes/add-radar-pipeline-status --delivery` 与 `bash scripts/harness/doc-impact.sh verify` 与 `openspec validate add-radar-pipeline-status --strict`，期望全部通过
- [x] 7.10 闸连接泄漏回归（6.4 附带修复）：`cd backend-go && go test -count=1 -timeout 180s ./internal/admin/scheduler/... && go test -count=1 -timeout 300s ./internal/topicgraph/service/...`，期望全部 PASS 且**不挂死**（`internal/admin/scheduler` 此前因闸探测失败路径泄漏单连接池而永久阻塞；`-timeout` 防无限挂）

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 触发单板日报生成 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 触发全板日报生成 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 触发多板块集合生成 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 多板块集合共用一个执行闸与一次终态 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 集合内单板失败不影响其余板块 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 单板块与集合参数互斥 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 集合内重复板块折叠 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 集合字段类型非法不降级为全量 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 单板块字段类型非法不降级为全量 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 生成进行中重复手动触发 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 生成进行中调度到期 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 跨实例互斥 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 持闸实例崩溃后闸自动释放 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 数据库不可达时回退进程内互斥 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 管线结束后闸释放 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 手动触发与定时调度互斥不产生重复通知 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 单板生成完成广播 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 全部生成完成广播 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 终态消息给出三态计数 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 当日无内容的板块既不算成功也不算失败 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 单板块生成失败 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 状态载荷同时给出两个时刻 | `backend-go/internal/admin/handler/scheduler_handler_test.go` |
| 兜底时刻缺省回落默认值 | `backend-go/internal/admin/handler/scheduler_handler_test.go` |
| 兜底时刻早于生成时刻时回落 | `backend-go/internal/admin/handler/scheduler_handler_test.go` |
| 1 成功 + 1 失败 + 1 无内容 → 通知与 WS 的总版面数一致 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 当日无内容的板块走完成通知 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 收集阶段失败的通知口径 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 定时调度路径与手动路径的总版面数一致 | `backend-go/internal/admin/scheduler/job_daily_report_test.go` |
| 定时调度路径收集阶段失败的通知口径 | `backend-go/internal/admin/scheduler/job_daily_report_test.go` |
| 当日无可生成板块不发终态通知 | `backend-go/internal/topicgraph/handler/daily_report_handler_test.go` |
| 状态块常驻右栏底部 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 状态块展示五项事实 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 首次对账未成功不呈现业务结论 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 对账失败保留旧值 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 总览取数失败不判未生成 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 默认全选并可取消勾选 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 生成以勾选集合为作用域 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 未勾选任何板块禁止提交 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 队列未清空时禁用生成 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 清单展开期间队列复堵仍禁止提交 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 今日已出时重生成给出替换警示 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 队列清空后生成可用 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 生成期间显示进度 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 生成完成后内联显示结果并刷新 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 终态计数直接读 `failed_count` / `empty_count`（旧后端两级回落） | `front/app/features/tags/composables/useRadarPipelineStatus.test.ts` |
| 无内容的板块不计入失败（显示成功 1 · 失败 0） | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 当日判定与今日日期（本地时区） | `front/app/utils/todayKey.test.ts` |
| 队列派生口径与旧后端降级 | `front/app/features/tags/composables/useRadarPipelineStatus.test.ts` |
| 右栏两段容器 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 双队列任一有积压时禁用生成 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts`, `front/app/features/tags/composables/useRadarPipelineStatus.test.ts` |
| 两个队列各自的今日完成进度 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts`, `front/app/features/tags/composables/useRadarPipelineStatus.test.ts` |
| Embedding 队列计数未知时挡住生成 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 双队列均清空时生成可用 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts`, `front/app/features/tags/composables/useRadarPipelineStatus.test.ts` |
| Embedding 队列计数随合并端点下发 | `backend-go/internal/admin/handler/poll_test.go` |
| 两个队列的计数互不混淆 | `backend-go/internal/admin/handler/poll_test.go`, `backend-go/internal/tagmanagement/handler/embedding_queue_status_test.go` |
| Embedding 队列计数缺失时不谎称已清空 | `front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts` |
| 窄屏基线 / 明暗双主题 / 双视口 | `ui-verification/report.md` |
