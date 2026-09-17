
## 通知中心 change 规划裁决与技术落点（apply 用）

**add-notification-center 规划期裁决记录（apply 阶段直接照此实现，无需再问用户）**

通知白名单=只日报终态（2026-xx 对话裁决，推翻了早期"终态+任务开始"）：①完成通知挂 `daily_report_handler.go` broadcastDone 收尾处；②失败=当次任务收尾处聚合判定失败版面数，发一条汇总（handler 里失败目前只有 board 级 broadcastProgress(...,"failed",...)，无任务级失败事件，需在收尾聚合）。抓取/自动刷新/单篇打标不进通知；打标排空汇总=候选不实现；任务开始通知不做。

队列行保留（并入本 change，挂 log-cleanup capability）：completed 留 1 天、failed 留 30 天（保 TagQueuePanel retry）；embedding_queues 30d→1d 属 MODIFIED（openspec/specs/log-cleanup/spec.md 有对应 Requirement，tests: job_log_cleanup_test.go TestLogCleanupJobRetention 断言 30 天——需按「继承与调整」表处置）。存实测：tag_jobs completed 19,153（6-14 起）、embedding_queues completed 72,482（~2400/天）、firecrawl_jobs completed 2,648+failed 493。清理扩展 job_log_cleanup.go（已有：ai_call_logs 7d/otel 7d/embedding_cache 14d/embedding_queues completed 30d），复用 24h 调度不建新 job。

技术事实：notifications 表字段 type/title/summary/link_type/link_id/is_read/created_at；上限 500 写入路径淘汰最旧已读；未读数=启动拉一次+WS notification 事件驱动+断线重连对账；打开面板即清角标（已读落库=已浏览）；status API 保留 total 字段，新增 completed_today，前端展示口径改活跃量 pending+leased。

UI 已批：ui-design.md 八节 + ui-prototype/index.html，ui-approval: **approved**（用户对话确认）。布局：面板 380px 锚定 popover（自由宽度理由已记）、芯片 header-btn 同高、目标视口 1440×900/1920×1080。

<!-- pinned 2026-09-17T04:26:39Z -->
