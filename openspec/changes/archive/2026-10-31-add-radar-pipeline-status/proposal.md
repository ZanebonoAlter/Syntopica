<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, semantic-board -->

## Why

L0 今日雷达（`/tags`）现在只回答"今天有什么可读"，不回答"今天为什么还没有日报"。日报受约束 23 约束（不早于 `daily_report_time` 21:00、`tag_jobs` 与 `embedding_queues` 双队列清空才生成、`daily_report_deadline` 23:30 强制兜底），但这些前置条件在页面上完全不可见：用户看到"今天还没有日报"，无从判断是还没到生成时刻、队列没清空、还是生成已经失败了。

同时标签队列进度只存在于顶栏芯片，而 `/tags` 页面用的是 `BoardRadarView`（不挂 `FeedLayoutShell`/`AppHeaderView`），**顶栏芯片在该页根本不渲染**——队列积压对雷达页读者是完全隐形的。

本 change 在雷达页右栏底部加一个常驻状态块，把"待打标积压 → 日报截止时刻 → 今日日报是否已出"这条因果链显式化，并提供带板块勾选的手动生成入口。

## What Changes

- **雷达页右栏底部新增常驻状态块**（`/tags` L0 雷达总览）：内容区（今日该读 / 今天可能升温）独立滚动，状态块固定在右栏底部不随之滚动。
- **状态块展示四项事实**：
  1. 标签队列待打标数（`pending + processing`）与今日完成进度（`completed_today / 总量`）；
  2. 日报生成时刻（`daily_report_time`，默认 21:00）与强制兜底时刻（`daily_report_deadline`，默认 23:30）——**兜底值显式展示**；
  3. 最新日报落点日期（各活跃板块最新 `report_date` 的最大值）；
  4. 当日日报是否已生成（取各板块 `report_date` 与本地今天比对）。
- **手动生成入口**：状态块内「立即生成今日日报」按钮 → 就地展开板块清单（**勾选 = 参与本次生成，默认全选**，可取消勾选不参与的板块）→ 确认生成。生成期间按钮进入进度态（复用既有 WS `daily_report_progress` / `daily_report_done`），完成后内联显示结果并刷新雷达数据。
- **队列未清空时禁用生成按钮并给出缺料说明**（防约束 24 死区：手动生成不等队列，今天才打标完的文章不会进入当天日报）。
- **后端扩展 `POST /api/daily-reports/generate`**：新增可选 `board_ids: number[]`（多板块集合），与既有 `board_id`（单板块）并存且互斥；一次请求 = 一个生成门 = 一个 `job_id` = 一次 WS 终态 = 一次通知。不传 `board_id`/`board_ids` 保持"生成全部当日有内容的活跃板块"原语义。
- **后端暴露日报兜底时刻**：`/api/poll` 的 `schedulers[daily_report]` 增加 `schedule_deadline` 字段（值取 `ai_settings.daily_report_deadline`），与既有 `schedule_time`（`daily_report_time`）并列。

非破坏性变更：`board_id` 参数、默认全量语义、生成门互斥范围（约束：同一时刻至多一条管线）、WS 广播格式、终态通知语义全部保持不变。

## Capabilities

### New Capabilities

- `radar-pipeline-status`：雷达总览页（L0 `/tags`）右栏底部常驻管线状态块的契约——双队列事实（标签队列 / Embedding 队列）、日报生成窗口、最新日报落点、当日日报状态，以及带板块作用域的手动日报生成入口。

### Modified Capabilities

- `daily-report-system`：日报生成 API 的板块作用域参数（`board_id` 单值 → 追加 `board_ids` 集合）与调度器状态载荷（新增 `schedule_deadline`）。
- `client-poll-budget`：合并对账端点（`/api/poll`）额外下发 Embedding 队列计数——日报双队列清空判据需要它，且不得为它另建周期性请求。

## Impact

**前端**
- 新增 `front/app/features/tags/components/console/BoardRadarStatusBlock.vue`（状态块，独立组件）
- 新增 `front/app/features/tags/composables/useRadarPipelineStatus.ts`（聚合 `/api/poll` 分发状态 + overview 派生 + 生成动作）
- 新增 `front/app/utils/todayKey.ts`（本地日期 key，避开 `toISOString()` 的 UTC 截断坑）
- 改 `front/app/features/tags/components/console/BoardRadarView.vue`（右栏改「上滚下驻」两段容器）
- 改 `front/app/api/dailyReports.ts`（`generateDailyReport` 支持 `board_ids`）
- 改 `front/app/types/scheduler.ts`（`SchedulerStatus` 加 `schedule_deadline?`）

**后端**
- 改 `backend-go/internal/topicgraph/handler/daily_report_handler.go`（`triggerGenerateDailyReport` 支持 `board_ids`；新增多板块生成 goroutine）
- 改 `backend-go/internal/admin/handler/scheduler_handler.go`（`enrichStatus` 为 `daily_report` 补 `schedule_deadline`）
- 改 `backend-go/internal/platform/scheduler/base.go` 或 `admin/handler/scheduler_handler.go` 的 `SchedulerStatusResponse`（加 `ScheduleDeadline` 字段）

**零新增 HTTP 请求**：状态块全部数据来自已常驻的 `/api/poll` 对账分发（`usePollBundle`）+ 雷达页已有的 `/semantic-boards/overview` 载荷；手动生成复用既有 `POST /api/daily-reports/generate` 与 WS 事件。

**数据与迁移**：无 schema 变更、无数据迁移。既有 `ai_settings.daily_report_deadline` 行若缺失则回落默认值 23:30（沿用 `aisettings.LoadDailyReportDeadlineConfig` 既有回落逻辑）。

**部署后用户可见行为**：雷达页右下角多一块状态信息；队列有积压时无法手动生成日报（这是刻意的门槛，非故障）；已生成的当日日报在状态块显示"今日已出"。无数据迁移、无需重生成、无需用户配置。
