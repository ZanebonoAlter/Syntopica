# Design — night-window-alignment

## Context

见 proposal.md §Why。关键现状事实（探索阶段核实，实据在 `.pi/harness` 探索会话与 pin）：

- 本地 LLM 盒（qwythos + qwen3-embedding）只在晚间 ~20:00 后可用，夜间窗口 ~4h 是唯一分析产能；实测吞吐 ~190-200 tag job/h，日入库 ~800 篇，贴线运行。
- `job_firecrawl` 在 `runtime.go` 注册时被 `scheduler.PauseAware(...)` 包裹，与 content_completion 等一起受 `analysispause.IsPaused()`（用户暂停 || 模型不健康）管制——白天健康门常态关闭导致 Pi 空转（当日 `firecrawl_crawled_at` 全落在 20:00 后）。
- tag worker（`internal/tagmanagement` 的 TagQueue）lease 按 FIFO（旧→新），与阅读价值倒挂。
- `daily_report` 由墙钟触发（`AISettings.daily_report_time`，默认 21:00，`TriggerNowWithDate` 包装）；生成后当日不再重算（offline-catchup 只补缺档）。
- 数据生命周期：超 feed 活跃上限是**归档**（`archived=true`，正文与标签边保留）非删除；归档满 `tag_edge_retention_days`（默认 7 天）标签边才被 GC。日报选文按 `articles.pub_date` 日窗口且不过滤 archived。

## Goals / Non-Goals

**Goals**
- 白天把零 LLM 的正文抓取全部前置完成，夜间窗口纯留 LLM 工作。
- 有限夜间窗口内优先消化仍有阅读/报告价值的最新文章。
- 日报单版完整制：出即覆盖当日已打标全量，最晚 23:30 兜底。

**Non-Goals**
- 不引入云 provider 做白天兜底（额度有限 + embedding 无云替代，用户明确否决）。
- 不做本地盒 Wake-on-LAN/定时开关机（用户明确否决）。
- 不做日报双版制/次日重算（用户选择单版延迟；7 天边窗口内的重算能力留给未来 change）。
- 不改 `content_completion` 的暂停管制（它是真 LLM 工作）。
- 不动 firecrawl worker 数/限速（3 worker + 500ms 礼貌限速维持）。

## Decisions

### D1：firecrawl 摘门采用「移出 PauseAware 包裹」而非「健康门内豁免」
`runtime.go` 注册处去掉 `scheduler.PauseAware` 包装即可，与 auto_refresh 同列。备选是在 `PauseAware`/`IsPaused` 里加白名单——但那会让暂停语义出现第二套判定路径，健康门 spec（ai-model-health / analysis-pause-control）的本意是「不健康就不做 LLM 工作」，白名单方案在门内开洞更难推理。移出包裹后，firecrawl 的「分析暂停中仍运行」直接体现在注册表，scheduler 状态卡沿用既有展示。
下游不变式：firecrawl 完成/终态失败的回调里对 tag 的 enqueue 逻辑零改动——enqueue 不是 LLM 调用，worker 暂停天然不消费（约束 7 既有语义），满足 proposal 的「抓完不硬分析」。

### D2：lease 排序改为 `priority DESC, created_at DESC`
tag worker 的 lease 查询排序变更。用 `created_at DESC` 而非「文章 pub_date 新→旧」：入队时间与到达时间近似单调同步（auto_refresh 60s 周期），避免 join articles 的成本与 pub_date 缺失的边界。**实现期修正（验收发现）**：设计初稿的 `available_at ASC` 排序键不可用——Enqueue 时 `available_at = created_at`，对 due 任务按它升序等于旧任务先出，字面三键会复活 FIFO（PG 测试实证）；退避语义本就由 `WHERE available_at <= now` 完整保证（B3），故最终排序为 `priority DESC, created_at DESC`，代码注释留痕。embedding 队列 worker 不改（其任务粒度是 tag/section 嵌入，不与阅读新鲜度直接挂钩，保持现状最小改动）。
备选否决：按 pub_date 排序需要 join + NULL 处理，收益（pub_date 与 created_at 偶发偏移）不抵复杂度。

### D3：TTL 淘汰——已取消（2026-09-24 验收期用户决策）
原方案为 PauseAware 周期扫描 job（`tag_job_ttl_sweep`，3600s）+ `skipped` 终态，代码实现完成后用户决策取消：**既有 7 天归档 GC（`tag_edge_retention_days`，归档文章标签边 GC 窗口）已兜底，再加一套业务侧 TTL 给业务负担太重**。已实现代码（sweep job、JobStatusSkipped、SkipExpiredPending、`tag_job_ttl_hours` 配置、统计/清理口径）全部摘除，制品与文档同步回退。陈年 pending 任务不做业务侧退场：随文章归档自然失去下游价值。

### D4：日报队列感知触发做成「墙钟到点 → 复查循环 → 兜底」，而非定时器重排
`daily_report` 的墙钟机制（scheduler-accuracy 既有 HH:MM 配置、跨重启稳定）保留为「最早可生成时刻」；到点后 JobFunc 进入等待循环：每 60s 查 `tag_jobs` 与 `embedding_queues` 的 pending+leased 计数，双零即生成；到达 `daily_report_deadline`（新 `AISettings` key，默认 23:30，校验不早于墙钟）强制生成。JobFunc 单次执行时长上限 = deadline − time（≤2.5h），远小于 scheduler 30s tick 的隐含预期，故采用「到点触发一次长执行」而非「每 tick 重查」——后者会让 scheduler_tasks 的执行统计语义（total_executions/时长）失真。
重启行为：墙钟已过、兜底未到、当日未生成 → 重启后调度器按 next_execution 逻辑重新进入等待（等待循环内状态不持久化，重启代价 = 重新等待，幂等）。兜底后启动 → 顺延次日 + 既有缺档补档兜住（spec 场景已覆盖）。
队列条件取双队列（tag + embedding）：lane 分桶依赖 tag embedding 对质心的距离，embedding 未完成时生成会退化为 unmatched 桶；双零条件比 offline-catchup 的单 tag 队列口径更完整，代价是生成时刻略晚（embedding 队列通常在 tag 清空后数分钟内清空）。

**实现期补充决策（验收采纳）**：① 当日报告已存在（如用户当日手动生成过）→ `already_exists` 早退且本轮补档扫描一并跳过，缺口顺延下一定时轮（保守实现，spec 只约束不重复生成）；② 暂停期 PauseAware skip 后 `NextDailyReportTime` 返回 now+60s，暂停期间按分钟级轻量空转重试（不热循环也不丢当日），代价是该窗口内 scheduler_tasks 的 total_execuctions 按分钟累积（可接受，属统计口径非行为）。

### D5：兜底时刻的配置落 `AISettings`，默认值硬编码回退
`daily_report_deadline`（默认 23:30）走既有 `AISettings` key/value 通道（与 `daily_report_time` 同族），缺失/非法回退默认 + warn。不做前端配置界面（ui-impact: none；需要时后续 change 补）。（原 `tag_job_ttl_hours` 随 D3 取消一并摘除）

## Risks / Trade-offs

- [firecrawl 白天全量抓取放大目标站压力] → 既有 3 worker + 每 worker 500ms 礼貌限速不变；抓取量不变（只是时段前移），不新增压力。
- [新鲜度优先让旧任务饥饿，可能一直排不上] → 有意为之（旧任务价值随 7 天归档 GC 自然衰减）；若观测到高优旧任务被饿，priority 字段是现成的人工插队通道。
- [日报延迟到 23:30 后用户已睡/已离线] → 单版完整制是用户显式选择；WS/前端无推送依赖，报告生成后次日可看；兜底时刻可配置（D5）。
- [陈年 pending 无业务侧退场通道] → D3 取消后的接受项：旧任务几乎不被 lease（新鲜度优先），不挤占窗口；随文章 7 天归档 GC 自然失去下游价值。
- [等待循环型 JobFunc 与 `TriggerNow` 409 重入保护的交互] → 手动 trigger 在等待期内会命中 isExecuting 409（accepted=false），符合既有语义（同 job 不并发）；tasks 中加用例固化。
- [重启落在等待循环中导致当日生成顺延到兜底] → 幂等可接受（生成语义不破坏，只是晚）；spec「重启不丢失调度」场景已约束不得退化为 24h。

## Migration Plan

1. 部署顺序：后端单次发布即可（无破坏性 DDL；`tag_jobs.status` 新字符串值、`AISettings` 新 key 惰性创建）。
2. 存量数据处理：无需迁移——现存 pending 任务自然按新排序消费，不做 TTL 淘汰（D3 取消）。
3. 回滚：revert 发布即可。

## Open Questions

- 兜底 23:30 的默认值是否合适，留待上线后按观测调整（配置已参数化，不改结构）。
- embedding 队列加入日报等待条件后，实际生成时刻的推迟幅度（预估 ≤ 数分钟），上线后观测确认是否需要放宽为单队列口径。
