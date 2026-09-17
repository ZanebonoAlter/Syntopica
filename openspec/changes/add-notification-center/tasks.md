<!-- doc-impact: flow, api, database -->

## 1. 后端：通知持久化与日报接入点

- [x] 1.1 新建 notification 模块（model/repository/service/handler + notifications 表 AutoMigrate，字段：type/title/summary/link_type/link_id/is_read/created_at），验证：`go build ./...` 通过
- [x] 1.2 通知写入服务：写入 + WS `notification` 事件广播 + 500 条上限淘汰（淘汰最旧已读行，写入路径内完成），失败仅记日志不阻塞调用方（fail-open），验证：repository 单测（testcontainer PG）覆盖写入/淘汰/未读保护
- [x] 1.3 日报链路终态接入：`daily_report_handler.go` 收尾处判定——全部完成写完成通知（含日期/版面数/条目数）；存在失败版面写一条失败汇总（含失败版面数），不逐版面写，验证：handler 单测覆盖三场景（全成功/部分失败=一条/全失败=一条）
- [x] 1.4 通知 API 注册：未读数、分页列表（支持 unread 过滤）、单条标已读、全部标已读、清空全部（单用户无鉴权），验证：handler 测试覆盖各端点 + 空表/越界分页变体
- [x] 1.5 影响包测试：`go test ./internal/<notification模块路径> ./internal/topicgraph` 全绿

## 2. 后端：队列表清理扩展（log_cleanup）

- [x] 2.1 `job_log_cleanup.go` 扩展：tag_jobs / firecrawl_jobs completed 行 >1 天删除、failed 行 >30 天删除；embedding_queues completed 保留期 30 天→1 天，验证：`bash scripts/test-assets.sh log-cleanup` 反查旧资产，TestLogCleanupJobRetention 按继承与调整表更新
- [x] 2.2 清理结果 JobResult.Data 增加对应 deleted 计数字段，验证：单测断言新字段（testcontainer PG，建表对齐生产 schema + created_at 索引）
- [x] 2.3 影响包测试：`go test ./internal/admin/scheduler ./internal/admin/repository` 全绿（-short）

## 3. 前端：通知中心

- [x] 3.1 `app/api/notifications.ts` client + `useNotifications` composable（未读数对账 + WS `notification` 事件驱动 + 标已读/清空），验证：composable 单测（事件驱动计数/断线重连对账）
- [x] 3.2 NotificationBell + NotificationPanel + NotificationItem 组件（Teleport 锚定面板、状态矩阵：loading/empty/error/success、打开面板即清零角标、全部标已读、清空 confirm 复用 AppDialog sm），验证：组件单测含机械锚（Teleport 挂载锚 + 浮层样式锚）
- [x] 3.3 AppHeaderView header-right 插入铃铛（「全部已读」与分隔线之间），验证：`pnpm lint` + 组件测试绿
- [x] 3.4 `utils/eventTypes.ts` 增加 `notification` 事件常量，验证：grep 一致性

## 4. 前端：进度芯片与计数语义

- [x] 4.1 `useTagQueueProgress` composable（status 拉取对账 + tag_completed/tag_failed 事件驱动 + 空闲隐藏判定），验证：composable 单测（空闲隐藏/失败态/WS 断线兜底）
- [x] 4.2 TagQueueProgressChip 组件（进行中/失败红态/空闲不渲染，点击跳 /settings 队列区），验证：组件单测含空闲不渲染断言
- [x] 4.3 后端 status API 补 `completed_today` 字段；TagQueuePanel 状态区改活跃量主展示 + "今日完成"（total 累计口径停用），验证：handler 测试 + 面板组件测试
- [x] 4.4 AppHeaderView 插入芯片（铃铛左侧），验证：`pnpm lint` + `pnpm exec nuxi typecheck` 绿

## 5. 文档

（并入文末「文档」节统一编号）

## 6. 测试

- [x] T.1 test-cases.md 主链路故事表：日报完成→通知落库→WS 推送→角标→面板→标已读（步/动作/来源 Scenario/期望/层/落点），验证：文件存在且节拍全覆盖
- [x] T.2 变体走查（五组清单）+ 白盒附加（通知淘汰边界：满 500/淘汰最旧已读/全未读；清理幂等重跑；部分失败汇总判定）落 test-cases.md，验证：每变体有明确答案或不适用划除留痕
- [x] T.3 后端影响包测试（-short）：`go test ./internal/<notification> ./internal/admin/scheduler ./internal/topicgraph`，期望全绿

## 7. 文档

- [x] D.1 flow 变更溯源：daily-report.md / scheduler.md「变更溯源」节补链接，验证：文档节存在链接
- [x] D.2 API 参考：`docs/reference/api/tag-ops.md` 补 notifications API 与 status `completed_today` 字段说明，验证：grep 命中
- [x] D.3 `docs/reference/database/` 补 notifications 表结构与三队列表保留策略说明，验证：文档存在且与实现一致
- [x] D.4 `bash scripts/doc-impact.sh verify add-notification-center` 对账通过（声明域：daily-report, scheduler）

## 8. 验证

- [x] V.1 `cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`，期望零报错
- [x] V.2 `cd backend-go && go test ./internal/admin/scheduler ./internal/admin/repository ./internal/topicgraph ./internal/<notification模块>`（影响包，-short），期望全绿
- [x] V.3 `cd front && pnpm lint && pnpm exec nuxi typecheck && pnpm test:unit`，期望全绿
- [x] V.4 `cd front && pnpm build`，期望构建成功
- [x] V.5 opencli 主链路断言：铃铛点击→面板展开→角标清零→全部标已读→角标消失；芯片任务态可见→点击跳 /settings 队列区，期望断言全过
- [x] V.6 双视口视觉检查（1440×900 / 1920×1080）：面板锚定无重叠/无溢出、芯片不换行，截图留档并回写 ui-design.md Acceptance 差异说明
- [ ] V.7 人工：触发一次日报生成（或 TriggerNow），验证完成通知出现、失败场景（若有）仅一条汇总；验证次日 log_cleanup 后队列表 completed 行只剩当日

### Scenario → 测试文件映射

| Scenario | 测试文件 |
| --- | --- |
| 定时日报完成产生完成通知 | backend-go/internal/topicgraph/handler/daily_report_notification_test.go |
| 部分版面失败产生一条失败汇总 | backend-go/internal/topicgraph/handler/daily_report_notification_test.go |
| 全部失败同样只有一条 | backend-go/internal/topicgraph/handler/daily_report_notification_test.go |
| 非白名单事件不产生通知 | backend-go/internal/platform/notification/notification_service_test.go |
| 在线用户实时收到 | backend-go/internal/platform/notification/notification_service_test.go |
| 离线事件下次打开可见 | 人工：API 启动后拉未读数含离线期间通知（V.7） |
| 失败通知携带可跳转目标 | backend-go/internal/topicgraph/handler/daily_report_notification_test.go |
| 未读数查询 | backend-go/internal/admin/handler/notification_handler_test.go |
| 全部标已读 | backend-go/internal/admin/handler/notification_handler_test.go |
| 写入超限淘汰最旧 | backend-go/internal/platform/notification/notification_repository_test.go |
| 未读行不被优先淘汰 | backend-go/internal/platform/notification/notification_repository_test.go |
| 队列排空不产生通知 | backend-go/internal/tagmanagement/service/core/tag_queue_notification_test.go |
| 批量打标进行中展示进度 | front/app/features/shell/components/TagQueueProgressChip.test.ts |
| WS 事件驱动计数刷新 | front/app/composables/useTagQueueProgress.test.ts |
| 队列排空后芯片消失 | front/app/features/shell/components/TagQueueProgressChip.test.ts |
| 活跃归零但仍有失败时保持可见 | front/app/features/shell/components/TagQueueProgressChip.test.ts |
| 新任务入队芯片重现 | front/app/features/shell/components/TagQueueProgressChip.test.ts |
| 失败态视觉区分 | front/app/features/shell/components/TagQueueProgressChip.test.ts |
| 点击芯片跳转 | 人工：opencli 主链路断言（V.5） |
| 面板计数语义 | front/app/features/settings/components/TagQueuePanel.test.ts |
| 定时清理移除过期 completed 行 | backend-go/internal/admin/scheduler/job_log_cleanup_test.go |
| failed 行保留 30 天 | backend-go/internal/admin/scheduler/job_log_cleanup_test.go |
| 存量堆积一次性自动淘汰 | backend-go/internal/admin/scheduler/job_log_cleanup_test.go |
| 无过期行时正常空跑 | backend-go/internal/admin/scheduler/job_log_cleanup_test.go |
| 定时清理移除过期 embedding completed 行（修改后） | backend-go/internal/admin/scheduler/job_log_cleanup_test.go |
