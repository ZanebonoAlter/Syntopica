> **2026-09-24 更新**：原方案中的「过期任务 TTL 淘汰」（tag_job_ttl_sweep / skipped 终态 / tag_job_ttl_hours）已在验收期用户决策取消——既有 7 天归档 GC 已兜底。下文涉及 TTL 的探索结论仅作历史留痕。


## 夜间窗口产能与队列现状（实测量化）

AI 算力拓扑与产能实测（2026-09-23 探索）：
- LLM/Embedding 全在本地台式机 10.11.12.111（llama-server :8080 qwythos / :8081 qwen3-embedding），用户下班 ~20:00 才开机，夜间窗口 ~4h 是唯一分析产能；云 mimo-v2.6-pro 额度有限且无 embedding 替代，用户否决云硬顶与 WoL。
- 全天入库 ~780-900 篇（24 feed，每小时 10~65 篇均匀到达）；夜间吞吐 ~190-200 tag job/h（836 job / 4.2h 实测），贴线运行。
- 实测证据：当日 firecrawl_crawled_at 全落在 20:00 后（白天健康门关闭 → firecrawl 全停）；20:00-21:00 消费的恰好是凌晨 0-3 点 tag_jobs（FIFO 倒挂坐实）；21:02 日报生成时打标完成 ~52%，21 点后到达/完成的文章永久缺席日报（collectBoardTags 按 articles.pub_date 日窗口选文 + 只补缺不重算 + 不过滤 archived）。
- 队列现状表：tag_jobs（pending+leased+completed）、embedding_queues、scheduler_tasks(check_interval/next_execution_time)、ai_settings(key/value jsonb)。

<!-- pinned 2026-09-23T16:05:29Z -->

## firecrawl 摘门与 tag 队列改造落点

firecrawl 摘门 + tag 队列改造落点：
- firecrawl 受门位置：backend-go/internal/app/runtime.go 注册处 scheduler.PauseAware(...) 包裹（与 content_completion/daily_report 等同列）；摘除即与 auto_refresh 同列。job 本体 internal/admin/scheduler/job_firecrawl.go（3 worker + 500ms 限速，约束 7 管制清单见 flow/scheduler.md）。
- firecrawl 完成回调 enqueue tag_jobs 逻辑在 reader 域（firecrawl_service / content-enrichment 约束 8），enqueue 非 LLM 调用，worker 暂停天然不消费（无需改）。
- tag worker：internal/tagmanagement/service/core/（tag_queue.go / embedding_queue.go / merge_reembedding_queue.go，lease 循环各自 analysispause.IsPaused() 自检）；lease 排序现状 FIFO，改 priority DESC, available_at ASC, created_at DESC（tag_jobs 有 priority/available_at/lease_expires_at/attempt_count 列）。
- 既有测试断言风险：analysis-pause-control spec「恢复后自动续跑」要求文本写死 created_at 顺序；相关测试若断言旧顺序需随契约改写（test-cases.md 继承与调整表处置）。

<!-- pinned 2026-09-23T16:05:29Z -->

## 日报触发链与数据生命周期事实

日报触发链与数据生命周期（单版完整制的安全边界）：
- 触发：daily_report 墙钟时刻 = AISettings key=daily_report_time（HH:MM 默认 21:00，TriggerNowWithDate 包装）；调度持久化 scheduler_tasks（check_interval/last/next_execution_time）；offline-catchup 补档条件 = tag 队列无 pending/leased，窗口 [today-N, today) 只补缺 (board, period_date) 不重算已有，超 tag_edge_retention_days 下界日拒绝（防空报告覆盖好报告）。
- 选文：daily_report_orchestrator.go collectBoardTags 按 articles.pub_date ∈ [day, day+1) JOIN article_topic_tags/topic_tag_board_labels，不过滤 archived；归档文章照常参与日报。
- 数据生命周期：reader/service/feed_service.go CleanupOldArticles = 归档非删除（archived=true，正文+标签边保留，删 reading_behaviors，清 search_vector）；仅 active 计数对 max_articles（全部 feed=100）；标签边 7 天 GC 仅归档文章（aux_label_cleanup）→ 重算/生成的素材安全窗 = 7 天，与补档窗口同口径。
- article 删除级联（pg_constraint confdeltype）：tag_jobs/firecrawl_jobs/article_topic_tags → CASCADE；reading_behaviors → SET NULL。
- 等待循环设计注意：daily_report JobFunc 长执行（≤2.5h）与 TriggerNow isExecuting 409 重入保护交互（手动触发在等待期返回 accepted=false，符合既有语义）。

<!-- pinned 2026-09-23T16:05:38Z -->

## night-window 五任务代码落点与测试缝

night-window-alignment 实现落点（探索确认）：
- firecrawl 注册：runtime.go L258 `Job: scheduler.PauseAware(admin.FirecrawlJob(firecrawlQueue, "scheduled"))`，摘 PauseAware 即与 auto_refresh 同列；daily_report 在 L219-229（NextRun=scheduler.NextDailyReportTime + TriggerNowWithDate wrapper + PauseAware）。
- tag lease 排序：repository/tag_job_queue.go Claim() 现为 `priority DESC, available_at ASC, id ASC`（L107-110），改 id ASC→created_at DESC。TagJob 有 created_at 索引列。
- AISettings 模式：key/value 字符串列；LoadDailyReportTimeConfig 返回 (default,nil)/缺失、(default,nil)+warn/非法、("",err)/DB错误；hhmmPattern 严格 `^([01][0-9]|2[0-3]):([0-5][0-9])$`（"1:30" 非法）。Save 验证后 upsert。
- 日报幂等判定可复用 topicgraphrepo.ReportExistsForBoardDate 的 dialect-proof 模式（±24h Pluck + NormalizeReportDate 内存比对，SQLite/PG 双兼容）；BoardDailyReport 表 board_daily_reports(semantic_board_id, period_date date)。
- BaseScheduler NextRun 墙钟循环：next=NextRun(now)→sleep→runJob()→重算；runJob 设 isExecuting（TriggerNow/TrySetExecuting 同锁 → 等待循环期间手动触发 409 天然成立）。重启后 Start() 重新计算 next。
- PauseAware(job)：IsPaused()→skip 返回 success（"skipped: <reason>"）。analysispause.SetPaused(true) 走 DB（测试需接 database.DB 或 testutil）。
- 测试模式：scheduler 包 sqlite 内存库（glebarez，MaxOpenConns(1)，db 前后 swap adminrepo.Repo/database.DB）；repository 层 testutil.SetupTestDB=testcontainer PG（-short 跳过）；aisettings/config_store_test.go 用 PG。job_daily_report_test.go 有 generateAndSaveReport 包级 seam + stub 模式。job_firecrawl_test.go 有 fakeCrawler + setupFirecrawlJobTest（AutoMigrate 含 TagJob）。
- 无既有测试断言 tag_jobs FIFO 消费顺序（3.2 处置：无需改写，新用例直接钉新序）。
- wire.go L176-193 需补 TagJobTTLSweepJob 别名；job_firecrawl.go 用 tagging.NewTagJobQueue(admin repository.Repo.DB()) 模式 enqueue。

**引用**：backend-go/internal/app/runtime.go、backend-go/internal/tagmanagement/repository/tag_job_queue.go、backend-go/internal/admin/scheduler/job_daily_report.go、backend-go/internal/admin/scheduler/base.go、backend-go/internal/platform/aisettings/config_store.go

<!-- pinned 2026-09-23T16:26:36Z -->

## 运行时验证结论：旧代码在跑，新行为均未生效（2026-09-24 13:40 数据）

**核心事实：后端进程 PID 1548127 于 2026-09-23 23:31:40 启动（go run cmd/server/main.go），而 night-window-alignment 实现完成于 9/24 凌晨 01:21（tasks.md mtime）——当前运行的是旧代码，change 的所有新行为均未实际运行。**

数据证据（PG syntopica）：
1. 【FIFO 铁证】9/24 凌晨 00-08 点创建的 247 个 tag_jobs 全部在 10:49-11:41 才被消费；09:00-10:49 消费的 176 个全是 9/23 18:23-21:01 创建的旧任务 → 纯 FIFO（旧排序），新排序（priority DESC, created_at DESC）未生效。
2. 【用户疑问解答「今天凌晨到白天没有分析」】9/24 00:00-09:00 ai_call_logs 0 行（LLM 盒关机）+ 旧代码健康门 → 零抓取零分析，这正是本 change 要解决的旧行为；09:00 用户开机后健康门通过，旧代码开始消费（09-13 点完成 848 个、LLM 调用 2.6 万行）+ firecrawl 抓取 56 篇。
3. 【9/24 白天抓取≠摘门生效】今天 09 点起的抓取是健康门通过状态下旧代码的行为，与摘门无关。9/22（19 点档 65 篇）/9/23（20 点档 98 篇）抓取全在晚间，佐证 proposal 的旧行为描述。
4. 【队列现状】tag_jobs 仅剩 completed 1476（pending/leased/skipped 全 0）——队列已清空无积压。
5. 【9/23 晚消费 583 个全是 9/23 当天创建】（当晚无跨天积压可吃，日报按旧 21:00 固定逻辑生成）。

推论：TTL sweep、日报队列感知同样未运行过。需重启后端（scripts/dev/start-dev.sh --restart）加载新代码；队列当前为空，lease 排序的运行时观察留给今晚自然窗口（repository 层 testcontainer PG 测试已覆盖排序正确性）。

<!-- pinned 2026-09-24T05:40:47Z -->
