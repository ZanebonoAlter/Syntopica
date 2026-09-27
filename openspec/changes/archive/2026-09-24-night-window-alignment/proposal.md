<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: scheduler, content-enrichment, daily-report, topic-graph -->

## Why

用户晚间的 ~4 小时是唯一 AI 分析窗口（本地 LLM 盒白天关机、云额度有限不做硬顶、embedding 无云替代），全天 ~800 篇入库全部压进这个窗口，贴线运行（实测吞吐 ~190-200 篇/h，正常日子 23:30 前勉强清完，稍有波动即滚账）。而当前管线有三个机制错位在放大窗口的稀缺：

1. **firecrawl 白天被冤杀**：抓正文是纯 Pi 算力（readability 进程内 + 树莓派渲染，零 LLM），却因被归为「分析类」受 `analysis_paused`/健康门管制，白天零抓取（实测当天 `firecrawl_crawled_at` 只落在 20:00 之后），全挤到晚上跟打标抢窗口。
2. **打标队列 FIFO 与阅读价值倒挂**：实测 20:00-21:00 消费的恰好是凌晨 0-3 点最老的任务；用户晚上最想读的当日新文章排在队尾，等轮到它们，日报早已生成。
3. **日报一次性截止产生死区**：日报按 `pub_date ∈ [当天, 次日)` 选文、固定 21:00 一次性生成、只补缺不重算——21 点后才完成打标的文章（实测约占当天 30-50%）**永久缺席任何日报**。

## What Changes

- **firecrawl 移出暂停/健康门管制**：`job_firecrawl` 摘除 `PauseAware` 包裹，白天照常抓正文。抓取完成只落 firecrawl 状态位 + 下游 `tag_jobs` 照常入队（worker 仍受暂停约束不消费，**不硬调 LLM**，维持「健康门没过就是停」的既有语义）；`content_completion` / `daily_report` / tag worker 等真正的 LLM 消费方管制不变。
- **打标队列新鲜度优先**：`tag_jobs` 消费顺序从「先来后到」改为「新任务优先」（priority 字段仍可插队），保证有限夜间窗口优先消化仍有阅读/报告价值的当日文章。
- **日报生成队列感知（单版完整制）**：生成时机从固定墙钟 21:00 改为「不早于配置墙钟时刻，且 tag 队列（pending+leased）清空才生成，最晚兜底时刻（默认 23:30）强制生成」——一天只生成一次，出即完整。数据可行性已核实：超 feed 上限是归档不是删除（正文/标签边保留，标签边 7 天 GC 窗口内重算/生成语义安全，与 offline-catchup 补档口径一致）。
- **Ops 建议（非代码，随归档验证）**：观测夜间 `topic_tagging` 路由并发余量，评估 `max_concurrency` 3→5（纯 `ai_routes` 配置，UI 已支持）。

## Capabilities

### New Capabilities

- `tag-queue-scheduling`: 打标队列消费调度——新鲜度优先的 lease 顺序（陈年积压不另做 TTL 淘汰：既有 7 天归档 GC 已兜底，2026-09-24 验收期用户决策取消）。

### Modified Capabilities

- `analysis-pause-control`: 「暂停生效范围——分析类」要求变更——firecrawl（纯抓取、零 LLM）移出暂停与健康门管制，归入「入库与维护类不受暂停影响」一侧；下游 LLM 消费方管制不变。
- `scheduler-accuracy`: 「DailyReport 在可配置墙钟时刻执行」要求变更——墙钟时刻语义从「固定触发点」改为「最早可生成时刻」，新增 tag 队列清空触发与最晚兜底时刻强制生成。

## Impact

- **后端**：`backend-go/internal/admin/scheduler/job_firecrawl.go`（摘 PauseAware）、`runtime.go`（注册处）、tag worker lease 查询（`internal/tagmanagement/`，排序调整）、`daily_report` 触发链（`TriggerNowWithDate` 包装与 scheduler-accuracy 墙钟逻辑改造）。
- **数据**：`tag_jobs` 增加淘汰所需字段或复用现有（`priority`/`available_at`/状态机扩展 skipped 终态），预期无破坏性 schema 变更；日报补档/重建守卫逻辑复用既有队列空判断。
- **文档**：`flow/scheduler.md` 约束 7（暂停生效范围）、`flow/content-enrichment.md`（firecrawl 调度行为）、`flow/daily-report.md`（生成时机）随实现同步。
- **前端**：无接口结构变更（ui-impact: none）——日报晚到属数据时机变化，沿用现有空态；调度器状态卡上 firecrawl 在暂停态运行属既有展示能力。
- **运维**：无新增部署依赖；兜底时刻的默认值可后续按观测调整。
