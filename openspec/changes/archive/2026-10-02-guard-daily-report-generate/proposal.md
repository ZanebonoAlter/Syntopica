<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, scheduler -->

## Why

2026-09-30 晚实锤：手动生成端点 `POST /api/daily-reports/generate` 无任何防重入保护，与调度管线并发双跑同一批版面（`daily_report_handler.go:96` 裸 `go generateAllBoards(...)`），导致 10 个版面 × 2 条完整 LLM 管线重复执行（token 白烧一遍）、通知中心同日期出现两条「日报已生成」（23:16:20 / 23:17:42），用户感知为"日报生成了好几次"。数据靠 `(semantic_board_id, period_date)` 唯一索引 + `SaveReport` upsert 兜住未重复，但两管线若几乎同时首写同一版面，后到者 Create 撞唯一索引会被误报为"该版面生成失败"。

## What Changes

- `POST /api/daily-reports/generate` 接入与调度器共享的防重入闸：已有日报生成（定时 or 手动）在跑时返回 409 `already_running`（口径与 `TriggerNowWithDate` 既有文案一致："日报生成正在执行中，请稍后再试。"）。
- 调度路径（`DailyReportJob`）与手动端点收敛到同一个包级原子闸（新增 `internal/topicgraph` 域内小闸，两处共用）；`TriggerNowWithDate` 既有的 `TrySetExecuting` 检查保留为调度层二道防线。
- 端点被拒时返回结构化错误体（`success:false, reason:"already_running"`），前端走既有错误 toast 通道提示"正在生成中"，不新增界面结构。
- 不改 DB schema，不改生成管线本体，不改"同日窗口内重建=整份覆盖"语义。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-system`: 「日报生成 API — 异步触发」requirement 增加并发约束——同一时刻至多一条日报生成管线（调度 or 手动同闸），重复触发 SHALL 返回 409 `already_running` 且不产生第二条完成通知。

## Impact

- `backend-go/internal/topicgraph/handler/daily_report_handler.go`：`triggerGenerateDailyReport` 过 rebuild-window 校验后加闸，被拒返回 409。
- `backend-go/internal/admin/scheduler/job_daily_report.go`：`DailyReportJob` 执行体包进同一把闸（获取失败→skip，reason `already_running`）。
- 新增共享闸（预计 `internal/topicgraph` 包级 `sync` 原子标志 + Try/Release 语义，数十行）。
- 前端无需改版：`DailyReportGenerateDialog.vue` / `LaneDynamicsPanel.vue` 已有 `success:false` 错误处理路径，透出后端 message 即可。
- 测试影响包：`go test ./internal/topicgraph/... ./internal/admin/scheduler/...`（按 change-scope.sh 机械判定）。
- 部署后行为变化：手动生成与定时生成撞车时不再双跑，用户会看到"正在生成中"提示而非静默双跑；无需数据迁移，旧数据零降级。
