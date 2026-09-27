# test-cases: add-notification-center

> 用例先行制品（complex 档）。Scenario→测试文件机器对账以 tasks.md「Scenario → 测试文件映射」表为准，本文件主链路表指向同一批落点，不另起炉灶。

## 故事 S1: 日报终态通知——从生成完成到已读的完整旅程（锚 Requirement: 通知白名单 / 持久化与 WS 推送 / 查询与已读 API / 条数上限淘汰）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 日报任务全部版面生成完成（触发 daily_report_done 收尾） | 定时日报完成产生完成通知 | notifications 表新增 1 条 type=success，title 含生成日期，summary 含版面数与保存条目数 | handler（testcontainer PG） | backend-go/internal/topicgraph/handler/daily_report_notification_test.go |
| 2 | 收尾同时经 WS 广播 `notification` 事件（载荷含完整通知对象） | 在线用户实时收到 | WS Hub 收到 1 条 notification 事件（服务层广播计数断言） | service（testcontainer PG） | backend-go/internal/platform/notification/notification_service_test.go |
| 3 | 页面开着的前端收到事件 | 在线用户实时收到 | 未读角标 +1（composable 计数断言） | 组件/composable | front/app/composables/useNotifications.test.ts |
| 4 | 6 个版面 1 个失败，任务收尾 | 部分版面失败产生一条失败汇总 | 仅 1 条 type=error 通知（summary 含成功/失败口径，如"5 成功 1 失败"），**不**逐版面产生 | handler | backend-go/internal/topicgraph/handler/daily_report_notification_test.go |
| 5 | 用户打开页面拉未读数 | 未读数查询 | 返回未读计数（含离线期间产生的通知——落库即触达） | handler | backend-go/internal/admin/handler/notification_handler_test.go |
| 6 | 点击铃铛展开面板 | （ui-design 交互契约） | 面板渲染、角标清零（打开即算浏览）、未读条目强调保留 | 组件 | front/app/components/ui/NotificationPanel.test.ts |
| 7 | 点击「全部标为已读」 | 全部标已读 | 所有 is_read=false 置 true，未读数归零 | handler + 组件 | backend-go/internal/admin/handler/notification_handler_test.go |
| 8 | 通知表已达 500 行后再写入 | 写入超限淘汰最旧 | 最旧已读行被删，总行数 ≤500 且新行在表中 | repository（testcontainer PG） | backend-go/internal/platform/notification/notification_repository_test.go |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V1 | 输入：summary 含特殊字符/emoji/超长（如失败版面名带引号） | 原样入库原样展示，不破坏 JSON 广播 | repository（testcontainer PG） | notification_repository_test.go |
| V2 | 输入：title/summary 空串（构造异常路径） | 通知仍落库（字段可空由服务层兜底文案），不因空串失败 | repository | notification_repository_test.go |
| V3 | 前置：空集（通知表 0 行）拉列表/未读数 | 返回空列表 + 0，不报错 | handler | notification_handler_test.go |
| V4 | 前置：单元素（仅 1 条通知）分页 | 返回该条，total=1 | handler | notification_handler_test.go |
| V5 | 前置：越界分页（offset 超过总数） | 返回空列表不报错 | handler | notification_handler_test.go |
| V6 | 前置：越界引用（对不存在的通知 id 标已读） | 404 或 no-op，不 panic、不误标他行 | handler | notification_handler_test.go |
| V7 | 时间窗口：淘汰边界恰好 500/501 行 | 见白盒 A 边界值 | repository | notification_repository_test.go |
| V8 | 幂等：重复标已读同一条两次 | 第二次 no-op 不报错，计数不再减 | handler | notification_handler_test.go |
| V9 | 幂等：全部标已读连调两次 | 第二次影响 0 行，不报错 | handler | notification_handler_test.go |
| V10 | 幂等：并发双写触发淘汰 | 单用户低频，接受 last-write 语义（白盒 A 划除留痕，不设计行锁） | — | 不适用：留痕见白盒 A |
| V11 | 可用性：面板空态（无任何通知） | 「暂无通知」空态文案，非白屏/非报错 | 组件 | NotificationPanel.test.ts |
| V12 | 可用性：错误态（列表 API 失败） | 面板内错误态 + 重试按钮；角标不显示不误报 | 组件 | NotificationPanel.test.ts |
| V13 | 可用性：加载态（首次拉取） | 面板内骨架行 | 组件 | NotificationPanel.test.ts |
| V14 | 可用性：清空全部误触防护 | 清空按钮先弹 confirm（AppDialog sm），未确认不删 | 组件 | NotificationPanel.test.ts |
| V15 | 可用性：超长 summary 在条目内截断 | CSS line-clamp 截断，不撑破 380px 面板 | 组件 | NotificationPanel.test.ts |
| V16 | 可用性：重复提交（快速双击「全部标为已读」） | 后端幂等兜底（V9），界面无重复请求副作用 | 组件 | NotificationPanel.test.ts |

### 效果核对（问句④）

- 触发原因：WS 事件到达时序与角标实时性依赖真实浏览器 WS 生命周期，组件测试断言不到
- 核对方法：人工——V.7 触发一次日报生成（TriggerNow），观察角标 +1 → 面板出现通知 → 标已读后角标消失
- 量化结果：人工留痕（tasks.md V.7）
- 结论：达标交付 ｜ 瓶颈在上游 ｜ 需调整预期（验收时填写）

## 故事 S2: 批量打标进度芯片——从入队到排空的常驻旅程（锚 Requirement: 进度芯片常驻展示 / 空闲自动隐藏 / 失败态 / 计数展示语义）

### 主链路（节拍串联）

| 步 | 动作 | 来源 Scenario | 期望 | 层 | 落点（测试/人工） |
| --- | --- | --- | --- | --- | --- |
| 1 | 40 篇文章批量入队打标，用户停留在任意页面 | 批量打标进行中展示进度 | 芯片可见，显示"分析中 n/total"与迷你进度条 | 组件 | front/app/features/shell/components/TagQueueProgressChip.test.ts |
| 2 | 收到 tag_completed 事件 | WS 事件驱动计数刷新 | 计数即时 +1（composable 事件驱动断言，无轮询等待） | composable | front/app/composables/useTagQueueProgress.test.ts |
| 3 | 最后一个任务 completed 且无新增入队 | 队列排空后芯片消失 | pending+leased==0 → 芯片不渲染 | 组件 | TagQueueProgressChip.test.ts |
| 4 | 空闲状态下新文章入队 | 新任务入队芯片重现 | 芯片重现并展示新计数 | 组件 | TagQueueProgressChip.test.ts |
| 5 | 队列出现 2 个 failed 且仍有活跃任务 | 失败态视觉区分 | 芯片红色失败态，进度条填充满 | 组件 | TagQueueProgressChip.test.ts |
| 6 | 点击芯片 | 点击芯片跳转 | 路由跳 /settings 队列区 | 人工：opencli 主链路断言（tasks V.5） | 人工 |
| 7 | 面板状态区查看计数 | 面板计数语义 | 活跃量=pending+leased 主展示、"今日完成"=今日 completed 计数，不展示累计 total | 组件 | front/app/features/settings/components/TagQueuePanel.test.ts |

### 变体走查

| # | 变体（组/条目） | 期望答案 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| V21 | 前置：空集（队列空、无 failed） | 芯片不渲染；TagQueuePanel 计数全 0 | 组件 | TagQueueProgressChip.test.ts |
| V22 | 前置：单元素（1 条 pending） | 芯片显示"分析中 0/1" | 组件 | TagQueueProgressChip.test.ts |
| V23 | 前置：status API 失败（WS 也断） | 芯片隐藏不报错，静默降级（恢复由重连/下一轮拉取驱动） | composable | useTagQueueProgress.test.ts |
| V24 | 幂等：同一 tag_completed 事件重复投递（WS 重连重放） | 计数不重复累加（以 API status 对账修正为准） | composable | useTagQueueProgress.test.ts |
| V25 | 可用性：超长计数（total 4 位数） | 芯片文案不换行不截断（设计已定自适应宽） | 组件 | TagQueueProgressChip.test.ts |
| V26 | 可用性：重复点击芯片 | 仅一次路由跳转，无副作用 | 人工：opencli（V.5） | 人工 |

### 效果核对（问句④）

- 触发原因：事件驱动计数的实时推进依赖真实 WS 流量与打标任务真实并发
- 核对方法：人工——V.7 触发批量打标（retag-today），观察芯片计数推进与排空消失
- 量化结果：人工留痕（tasks.md V.7）
- 结论：（验收时填写）

## 效果核对（候选需求负向）

- 触发原因：「队列排空不产生通知」是 SHALL NOT 负向断言，且属候选需求的防回归锚
- 核对方法：后端负向测试——队列排空事件序列后断言通知表无新增行
- 落点：backend-go/internal/tagmanagement/service/core/tag_queue_notification_test.go
- 结论：不适用量化（负向行为无"效果"核对，仅断言无副作用）

## 继承与调整（⓪：log-cleanup delta 含 MODIFIED Requirement）

`bash scripts/test-assets.sh log-cleanup` 反查结果：主 specs 9 Scenarios / archive（2026-08-22-analysis-remediation，无 test-cases） / 无历史映射表。旧测试资产仅 1 个：

| 旧 Scenario | 处置 | 旧测试文件 | 动作 |
| --- | --- | --- | --- |
| 定时清理移除过期 completed 行（embedding_queues 30 天版） | 改语义（30 天→1 天） | backend-go/internal/admin/scheduler/job_log_cleanup_test.go `TestLogCleanupJobRetention` | 改断言：seed 的 old 行从 -31d 调整为跨 1 天边界两端（-25h 删 / -23h 留），并补 tag_jobs/firecrawl_jobs 分支断言 |
| 无过期行时正常空跑（embedding） | 继承 | 同上 | 照跑（回归网） |
| 其余 6 个 log-cleanup Scenario（log 表/手动触发/索引/状态） | 继承（本次不触及） | 同包既有覆盖 | 照跑（回归网） |

## 白盒附加（complex 档：淘汰状态机 / 汇总判定协议 / 清理边界）

### A. 通知 500 上限淘汰状态机

#### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| A1 | 行数 < 500 | 499 行 + 写入 1 条 | 直插，不触发淘汰 | TestNotificationEviction |
| A2 | 行数 = 500 且存在已读行 | 500 行（含已读）+ 写入 1 条 | 删除最旧已读行（可多条直至 ≤500），新行在表中 | TestNotificationEviction |
| A3 | 行数 = 500 且全部未读 | 500 条未读 + 写入 1 条 | 删除最旧未读行，总行数 ≤500 | TestNotificationEvictionAllUnread |
| A4 | 并发双写 | 两路同时写入 | 接受 last-write 语义（单用户低频），最终行数 ≤500 即可，不设计行锁 | 不适用（划除留痕：并发仅当声称线程安全才测；本实现明确不声称，边界防御由约束性 DELETE 保证） |

#### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| 总行数 | 恰好 500 | 再写入触发淘汰 | TestNotificationEviction |
| 总行数 | 501（淘汰后） | ≤500 且新行存在 | TestNotificationEviction |
| 连续写入 | 连续 3 次写入（502→503） | 每次同步淘汰，稳定 ≤500 | TestNotificationEviction |
| 已读/未读分布 | 最旧行=未读、次旧=已读 | 淘汰次旧已读，未读行保留 | TestNotificationEvictionAllUnread |

断言判据（主线程定）：每次淘汰后总行数 ≤500 且新写入行在表中。

### B. 日报失败汇总判定

#### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| B1 | 失败版面数 = 0 | 全部版面 completed | 发 1 条完成通知（success） | TestDailyReportNotification |
| B2 | 失败版面数 ≥1 且 < 总数 | 6 版面 1 失败 | 发 1 条失败汇总（error，summary 含成功/失败口径），不逐版面 | TestDailyReportNotification |
| B3 | 失败版面数 = 总数 | 全部失败 | 仅 1 条失败汇总 | TestDailyReportNotification |

#### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| 版面总数 | 1（单版面项目） | 失败 1 = 全部失败 → 1 条失败汇总 | TestDailyReportNotification |

断言判据（主线程定）：每次任务运行**至多 1 条**失败类通知；完成与失败汇总互斥（失败时发失败汇总，不发完成通知）。

### C. log_cleanup 保留边界

#### 边界值清单

| 变量 | 边界值 | 期望 | 测试用例名 |
| --- | --- | --- | --- |
| completed 行年龄 | >24h（如 -25h） | 删除 | TestLogCleanupJobRetention（扩展版） |
| completed 行年龄 | ≤24h（如 -23h） | 保留 | 同上 |
| failed 行年龄 | >30d（如 -31d） | 删除 | 同上 |
| failed 行年龄 | ≤30d（如 -29d） | 保留（TagQueuePanel 重试可用） | 同上 |
| embedding_queues completed | >24h 删 / ≤24h 留 | 与 tag_jobs 同口径 | 同上 |
| 表为空/无过期行 | 空跑 | 删除 0 行，不报错（幂等） | TestLogCleanupJobRetention |

断言判据（主线程定）：边界两端各至少一个 case；重跑一次删除 0 行（幂等不报错）。

#### 不适用划除（留痕）

- 并发清理（手动 trigger 与定时同时执行）：不适用——既有 scheduler 已有 409 already_running 防护（主 specs「Manual trigger while already running」Scenario 继承），本次不新增并发面。

### D. 未读/已读语义

#### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| D1 | 打开面板（含未读） | 未读数 N>0 | 角标清零（打开即算浏览），条目未读强调保留至本次浏览会话 | NotificationPanel.test.ts |
| D2 | 重复打开面板 | 第二次打开 | 幂等：不重复计数、不报错 | NotificationPanel.test.ts |
| D3 | 单条标已读（与 D1 独立） | hover 单条操作 | 该条 is_read=true，未读数 -1 | notification_handler_test.go |
| D4 | 重复标已读同一条 | 第二次调用 | no-op 不报错（不变式） | notification_handler_test.go |

### E. 芯片空闲判定

#### 分支表

| # | 条件/分支 | 输入 | 期望 | 测试用例名 |
| --- | --- | --- | --- | --- |
| E1 | pending+leased == 0 | 排空后 status | 芯片不渲染 | TagQueueProgressChip.test.ts |
| E2 | pending+leased > 0 | 有活跃任务 | 芯片可见 | TagQueueProgressChip.test.ts |
| E3 | WS 断线 | 断线期间无事件推送 | 以 API status 对账为准：不因断线误显/误隐；重连后重拉修正 | useTagQueueProgress.test.ts |
| E4 | failed > 0 且活跃 == 0 | 只剩失败无活跃 | 芯片保持失败态可见（可点击去处理），不隐藏 | TagQueueProgressChip.test.ts |

断言判据（主线程定）：可见性唯一判据 = pending+leased>0 或 failed>0；WS 事件只影响计数，不影响可见性判定来源（可见性以最近一次 status 对账为准）。

### 不适用划除（留痕）

- 输入变体组（S2 故事）：纯分隔符/大小写变体——不适用（芯片输入为后端数值计数，无字符串解析面）。
- 时间窗口变体组（S1 通知故事）：「跨窗口/时区归一化」——不适用（通知无窗口逻辑；日报日期归属由 daily-report 既有链路负责，本 change 只透传）。

## 层选择对账（问句③）

| 层 | 用例 | 载体 |
| --- | --- | --- |
| testcontainer PG（禁 SQLite） | 淘汰状态机 A、清理边界 C、通知落库 V1/V2 | notification_repository_test.go / job_log_cleanup_test.go（testutil.SetupTestDB） |
| handler 测试 | S1 步 1/4/5/7、V3-V6/V8-V9、B 分支、D3/D4 | daily_report_notification_test.go / notification_handler_test.go |
| service 单测 | WS 广播载荷、白名单负向 | notification_service_test.go / tag_queue_notification_test.go |
| Vitest 组件测试 | S1 步 3/6、S2 步 1/3/4/5/7、V11-V16/V21-V25、D1/D2、E1/E2/E4 | NotificationPanel.test.ts / TagQueueProgressChip.test.ts / TagQueuePanel.test.ts |
| composable 单测 | S1 步 3、S2 步 2、V24、V23、E3 | useNotifications.test.ts / useTagQueueProgress.test.ts |
| opencli / 人工 | S2 步 6、完整故事（tasks V.5/V.7） | opencli 主链路断言 / 人工留痕 |
