# Tasks — night-window-alignment

## 1. 用例先行（复杂档强制）

- [x] 1.1 编写 `test-cases.md`：主链路表（firecrawl 暂停态照抓且下游不触发 LLM / tag 队列新任务优先与 priority 插队 / 日报墙钟-队列清空-兜底三段时机）串节拍，每 Scenario 标注层与落点；变体走查五组清单逐项给答案；白盒附加节出排序边界值与等待循环状态表；契约变更项（analysis-pause-control ×4、scheduler-accuracy ×1）填「继承与调整」表（旧 Scenario×处置×旧测试×动作）。验证：`grep -c '#### Scenario' openspec/changes/night-window-alignment/specs/*/spec.md` 的总数 ≤ test-cases.md 主链路+变体覆盖的落点数

## 2. firecrawl 摘出暂停/健康门

- [x] 2.1 `internal/app/runtime.go` 移除 `job_firecrawl` 注册处的 `scheduler.PauseAware(...)` 包裹，与 auto_refresh 同列不受门禁；验证：新增/调整 scheduler 包用例断言「analysispause.IsPaused()=true 时 firecrawl tick 正常执行、content_completion 仍 skipped」通过（`go test ./internal/admin/scheduler/...`）
- [x] 2.2 下游不触发 LLM 的不变式用例：暂停态下 firecrawl 完成回调仅落状态位 + tag_jobs 入队（pending 不被消费）；验证：tagmanagement 侧既有暂停不消费用例 + 新增 firecrawl 完成路径 enqueue 断言通过（`go test ./internal/tagmanagement/... ./internal/reader/...`）

## 3. tag 队列新鲜度优先

- [x] 3.1 tag worker lease 查询排序改为 `priority DESC, created_at DESC`（设计期修正：available_at ASC 会复活 FIFO，见 design D2）（repository 层，testcontainer PG 用例：新旧并存新的先 lease、高优旧任务插队、退避中不被 lease）；验证：`go test ./internal/tagmanagement/...` 新增排序用例通过
- [x] 3.2 「恢复后自动续跑」顺序契约调整：定位断言 created_at 顺序的既有测试并按新排序改写（处置记录进 test-cases.md「继承与调整」表）；验证：`go test ./internal/admin/scheduler/... ./internal/tagmanagement/...` 无红


## 4. 日报队列感知（单版完整制）

- [x] 4.1 `AISettings` 新 key `daily_report_deadline`（HH:MM，默认 23:30，非法回退+warn，早于 `daily_report_time` 回退默认+warn）；验证：配置解析用例五场景（默认/非法/早于墙钟/正常/更新生效）通过
- [x] 4.2 `daily_report` 触发改造：墙钟到点后 JobFunc 等待循环（60s 复查 tag_jobs 与 embedding_queues 双队列 pending+leased，双零即生成；到达 deadline 强制生成；当日已存在不重复；手动 TriggerNow 在等待期命中 409 重入保护）；验证：编排用例（提前清空准点生成/等待清空生成/兜底强制/已存在幂等/409）通过
- [x] 4.3 重启语义：墙钟已过且兜底未到且当日未生成 → 重启后继续按队列感知等待；兜底后启动 → 顺延次日（缺档由既有补档兜住）；验证：调度时刻计算用例通过（`go test ./internal/admin/scheduler/...`）

## 5. 测试

- [x] 影响包全绿：`cd backend-go && go test ./internal/admin/... ./internal/tagmanagement/... ./internal/topicgraph/... ./internal/reader/...`（按 change-scope.sh 判定结果补齐），期望全 PASS
- [x] test-cases.md 主链路故事全绿核对：逐行勾对落点测试通过证据（命令+结果），期望无缺漏落点

## 6. 文档

<!-- doc-impact: flow, configuration -->

- [x] `docs/reference/flow/scheduler.md`：约束 7 暂停生效范围移出 firecrawl（含健康门段）、daily_report 行补队列感知时机；grep 核对无「firecrawl 受暂停」残留表述
- [x] `docs/reference/flow/content-enrichment.md`：约束 7 追加「不受 analysis_paused/健康门管制，白天照抓；下游入队不消费」+ 溯源行
- [x] `docs/reference/flow/daily-report.md`：生成时机改单版完整制（墙钟最早 + 双队列清空 + 23:30 兜底，新约束 23），变更溯源表补本 change；额外回补隐含截止语义（约束 24）
- [x] `docs/reference/flow/topic-graph.md`：tag 消费顺序（新任务优先）对打标覆盖面的影响说明 + 溯源行
- [x] `docs/reference/configuration.md`：补 `daily_report_deadline` 新 ai_settings key 行，`daily_report_time` 描述同步队列感知语义

## 7. 验证

- [x] `cd backend-go && golangci-lint run ./...`，期望 0 issue（全量实跑：0 issues）
- [x] `cd backend-go && go vet ./... && go build ./...`，期望退出码 0（实跑 BUILD_OK）
- [x] `bash scripts/harness/change-scope.sh` 判定影响包并逐包 `go test`，期望全 PASS（admin/tagmanagement/topicgraph/reader 四域全绿，repository 层 testcontainer PG）
- [x] `grep -rn 'PauseAware' backend-go/internal/app/runtime.go | grep -i firecrawl`，期望无输出（A4 实跑无输出，firecrawl 已摘除包裹）
- [x] 验收期变更（2026-09-24）：TTL 淘汰项取消（design D3），代码/制品/文档同步摘除；摘除后重跑全量 `golangci-lint run ./...`（0 issues）、`go vet`/`go build`、影响包测试（tagmanagement/aisettings/admin 全绿）；`GET /api/schedulers/status` 确认 17 个任务无 `tag_job_ttl_sweep`，部署生效（start-dev.sh 重启 PID 2673111）
- [ ] 人工（上线后观察项，随自然窗口验证）：白天关闭本地 LLM 盒（健康门未过）状态下观察一个 firecrawl tick，期望 scheduler 状态显示 firecrawl 正常执行、tag_jobs 只增 pending 无 LLM 调用（ai_call_logs 无新行）
- [ ] 人工（上线后观察项，随自然窗口验证）：晚队列未清空时过 21:00，期望日报不在 21:00 生成；队列清空（或 23:30）后生成且包含当日已打标文章

### Scenario → 测试文件映射（scenario-trace 对账）

| Scenario | 测试文件 |
|---|---|
| 暂停时调度任务不 lease | backend-go/internal/admin/scheduler/pause_test.go |
| 暂停时 tag worker 不消费 | backend-go/internal/admin/scheduler/pause_test.go |
| 暂停时 firecrawl 的下游不触发 LLM | backend-go/internal/admin/scheduler/job_firecrawl_test.go |
| 暂停时 RSS 继续入库 | backend-go/internal/admin/scheduler/pause_test.go |
| 暂停时日志清理继续 | backend-go/internal/admin/scheduler/job_log_cleanup_test.go |
| 暂停时 firecrawl 照常抓取 | backend-go/internal/admin/scheduler/pause_test.go |
| 模型未就绪时调度任务不 lease | backend-go/internal/admin/scheduler/pause_test.go |
| 模型未就绪时 firecrawl 照常抓取 | backend-go/internal/admin/scheduler/pause_test.go |
| 模型未就绪时 tag worker 不消费 | backend-go/internal/admin/scheduler/pause_test.go |
| 启动竞态期视为不健康 | backend-go/internal/platform/aihealth/aihealth_test.go |
| 手动启动不被健康状态拒绝 | backend-go/internal/platform/aihealth/aihealth_test.go |
| 恢复时重新探活 | backend-go/internal/platform/aihealth/reprobe_test.go |
| 恢复后消化堆积任务 | backend-go/internal/admin/scheduler/pause_test.go |
| 队列提前清空则在墙钟时刻准时生成 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 墙钟时队列非空则等待清空 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 队列持续非空则兜底强制生成 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 服务在目标时刻前启动 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 服务在目标时刻后启动 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 重启不丢失调度 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 默认时刻 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 非法配置值回退默认 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 兜底早于墙钟的非法关系 | backend-go/internal/admin/scheduler/job_daily_report_window_test.go |
| 新旧任务并存时新的先消费 | backend-go/internal/tagmanagement/repository/tag_job_queue_test.go |
| 高优先级旧任务可插队 | backend-go/internal/tagmanagement/repository/tag_job_queue_test.go |
| 退避中的任务不被 lease | backend-go/internal/tagmanagement/repository/tag_job_queue_test.go |

