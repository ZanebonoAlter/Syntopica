<!-- complexity: complex -->

## Context

现有推送链路：单例 WS Hub（`internal/platform/ws/hub.go`）`BroadcastRaw` 广播 JSON，7 个广播点分布在 tag_queue / job_firecrawl / job_auto_refresh / job_daily_report / daily_report_handler，无持久化、无定向。前端 `useEventStream` 单例连接按 type 分发，消费方各自订阅，无全局拦截层。已有 toast（useNotify + NotifyContainer）与 TagQueuePanel（设置页，消费 tag-queue status API）。队列三表（tag_jobs/firecrawl_jobs/embedding_queues）completed 行无清理或清理窗口过长，累计 ~9.5w 行。

可丢弃原型阶段选型结论（复杂档原型通道，已用户确认）：路线 B（后端落库+推送）优于路线 A（纯前端拦截）——A 无法覆盖"页面没开着"场景，历史/未读能力为零；B 复用 A 的全部前端结构，持久化是增量。详见 `docs/research/explore-findings.md`「WS/推送链路全景与通知中心落点」。

## Goals / Non-Goals

**Goals:**
- 日报终态通知落库 + WS 同步推送 + 前端铃铛/面板（未读角标、标已读、清空）
- header 进度芯片复用现有 status API 与 WS 事件，不动后端打标链路
- 三队列表 completed 行每日重置、failed 行保留 30 天，存量自动淘汰

**Non-Goals:**
- 打标队列排空汇总通知（候选需求，spec 已记录不实现）
- 任务开始类通知、通知多用户隔离（单用户系统）
- 新增定时清理任务（复用 log_cleanup）
- 抓取/自动刷新/单篇打标的通知化（白名单外）

## Decisions

1. **通知写入挂在日报 handler 广播点旁，不新建独立事件总线**。`daily_report_handler.go` 的 `broadcastDone` / 失败路径已聚合"当次任务"语义，在广播同一事务点落库+广播 `notification`；备选"每个业务模块各自写通知"会让白名单约束散落多处、易漂移。失败汇总在任务收尾处一次判定（失败版面计数），不逐版面写。
2. **notifications 表按"写入路径内淘汰"而非定时清理**。与队列表不同，通知是产品数据（用户信箱），写入时检查 500 上限并淘汰最旧已读行，语义即时、无延迟；log_cleanup 不掺和通知（单一职责）。
3. **未读数策略 = WS 事件增量 + 打开页面时 API 对账**。前端启动时拉一次未读数，此后由 `notification` WS 事件 +1；不轮询（单用户系统，WS 断线重连时重拉对账即可）。备选"每次面板打开才查"会让角标在面板关闭时过期。
4. **"打开面板即算浏览"清零角标，但条目未读强调保留到本次浏览会话结束**。避免"关面板就丢线索"与"强制逐条标已读"两个极端；已读状态落库语义 = 已浏览过。
5. **队列表清理扩展 log_cleanup（复用现有 24h 调度）而非新建 job**：同一执行器、同一 registry、无新 descriptor；completed 留 1 天、failed 留 30 天。embedding_queues 30d→1d 属既有 Requirement 修改（MODIFIED，见 log-cleanup delta）。
6. **队列计数展示语义改为活跃量（pending+leased）**：status API 返回结构不变（total 等字段保留，避免破坏现有消费者），TagQueuePanel/芯片只取需要的字段重新组合口径；"今日完成"由前端按 created_at 今日过滤或后端补 `completed_today` 字段（实现时取改动小者，倾向后端加字段）。
7. **前端通知组件挂 header 而非每页引入**：NotificationBell 在 AppHeaderView 插入（全局唯一挂点），useNotifications 用 useState 全局共享，与 useNotify 同模式。
8. **进度芯片数据源 = status API 轮询兜底 + WS 事件驱动**：tag_completed/tag_failed 事件已含计数所需信号（事件驱动计数），WS 断线或页面刚打开时拉一次 status 对账；不引入新推送。

## Risks / Trade-offs

- [日报失败路径无独立"任务级失败"事件，只有 board 级 failed 进度] → 收尾处聚合判定；测试覆盖"部分失败=一条"场景
- [通知写入失败不应阻塞日报生成] → 通知写入失败仅记日志（fail-open），与 WS 广播解耦
- [embedding_queues 清理 30d→1d 后，若面板/下游有依赖 30 天 completed 历史的逻辑会断] → 检索确认面板仅展示状态计数与 failed 重试，无历史聚合消费；`bash scripts/test-assets.sh log-cleanup` 反查旧测试资产并在 test-cases.md「继承与调整」表处置
- [500 条上限淘汰并发写竞争] → 单用户低频写入，简单 DELETE ... LIMIT 语义即可，无需行锁设计
- [芯片与面板同时依赖 WS 断线] → 各自的 API 对账兜底已在 Decision 3/8 覆盖
- [存量清理首次运行删除 ~9.5w 行] → 单次 DELETE 带 created_at 索引，PG 空闲期执行（log_cleanup 有 5 分钟启动延迟惯例）；失败可安全重跑（幂等）

## Migration Plan

1. 迁移：notifications 表（GORM AutoMigrate 惯例），无旧数据回填
2. 部署后首个 log_cleanup 周期自动淘汰存量 completed 行（~9.5w），用户可见队列面板计数回落
3. 回滚：前端组件回退即恢复旧行为；notifications 表残留无害；清理策略回退需改回保留期常量

## Open Questions

无（口径类问题已在对话裁决：白名单=只日报、completed 留 1 天、failed 留 30 天、条数上限 500）。
