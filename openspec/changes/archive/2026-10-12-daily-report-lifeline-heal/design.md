# Design: daily-report-lifeline-heal

## Context

见 proposal.md（Why：月初定时触发被 `model_unhealthy` 跳过即丢整月；年报年中空洞要等次年）。

现状代码事实（探索阶段实查）：

- 日报 job 入口 `backend-go/internal/admin/scheduler/job_daily_report.go` 的 `DailyReportJob(targetDate)`：主流程生成当日报告 → `backfillMissingReports`（缺日回填，顺延不丢）→ tail steps（dangling refs 检查、board health checkup）。
- 月/年报归档服务 `backend-go/internal/dataenrichment/service/lifeline_context.go`：`HealMissing(ctx, granularity, now, lister)` 已实现双向回填——扫「有 section 素材但无 `topic_lifeline_context` 行」的 (topic, period) 逐个补齐，上限当前 period，不重写已存在行。月/年定时 job（`scheduler_jobs.go`）即「HealMissing → 全量 Refresh 当期 → prune」。
- service/lister 获取：`dataenrichment.GetLifelineService()`（wire.go 已导出）；`topicLister` 为包内私有变量，`NewDBTopicLister(db)` 可重建，或补一个 getter。
- `HealMissing` 内部对单 topic 失败的处理：逐 topic 逐 period 补，错误返回中断整轮（与 job 层「单 topic 失败 failed++ continue」不同——见决策②）。
- 数据规模实测：active 话题 59 个；月度素材 p50=1 行/p90=7 行；年素材 p50≈2.8k/p90≈7.9k/max≈17.7k 字符；历史缺口估算一两百次 LLM 调用（本地 qwen3.5-4b Q8，一次数秒，全量首轮回填预计 10-30 分钟）。

## Goals / Non-Goals

**Goals:**

- 月/年报断供窗口从「一个月/一年」缩到「一天」：每日日报成功后异步补齐 month/year 缺口。
- 复用 `HealMissing`，不新写生成算法、不动表结构、不动模型路由。
- 失败语义与 `backfillMissingReports` 对齐：顺延不丢、不阻塞日报、单点失败不扩散。

**Non-Goals:**

- 不启用 week 粒度每日补齐（weekly job 维持禁用，存量 week 行不动）。
- 不做已存在归档行的「当期增量刷新」（年报语义 = 归档定稿 + 缺行补齐；HealMissing 只补缺行不重写旧行——见 Risks 的取舍说明）。
- 不动模型路由（4B 沿用；分级路由留待独立 change）。
- 不删/不改 `lifeline_monthly` / `lifeline_yearly` 定时 job（双保险）。
- 不做补齐进度的前端展示或通知。

## Decisions

### ① 挂载点：日报 job 尾部独立 goroutine（异步 tail step）

**选择**：在 `DailyReportJob` 的 tail steps 之后追加 `go healLifelineGaps(...)`，带自己的 recover/超时，不 join 日报 job 的执行路径。

**理由**：日报 job 已经很重（主生成 + 回填 + 两个 tail step），补齐首轮可能跑 10-30 分钟，同步执行会把 scheduler_tasks 里 daily_report 的执行时长/状态与补齐耦合；异步则日报 job 状态只反映日报本身。模型健康隐含判断免费获得——日报成功生成说明 LLM 通路可用，无需重复探测。

**备选**：a) 同步 tail step（拒绝：耦合执行时长）；b) 独立 22:00 定时 job（拒绝：又回到「定时触发撞不健康窗口」的老问题，且与「日报成功」的因果关系断裂——日报没生成成功时补齐意义存疑）；c) 挂 `daily_report` job 的 JobResult 成功回调（拒绝：JobFunc 返回即 scheduler 视为完成，回调语义与 goroutine 等价但更隐晦）。

**执行窗口约束**：goroutine 内不设全局超时上限（回填量不定），但单次 LLM 调用沿用 airouter 现有超时；进程优雅停机时若回填未完成，靠「顺延不丢」语义次日重试（不引入持久化任务队列——单用户系统，成本不值）。

### ② 失败语义：HealMissing 细粒度容错（单 (topic, period) 失败不中断）

**选择**：不直接在 tail step 里裸调 `HealMissing`——其内部单 topic 错误会 `return err` 中断整轮（month 粒度某 topic 读 sections 失败会导致后面所有 topic 不补）。新增一个包装函数（或给 `HealMissing` 加容错模式）：逐 (topic, period) 补齐，失败计数继续，返回 (healed, failed) 汇总。

**理由**：日报 tail 的补齐轮应该尽量多补——「补 55 个丢 4 个，明日只重试 4 个」远好于「第 3 个失败后全部顺延」。与 job 层 `failed++ continue` 哲学一致。

**备选**：裸调 `HealMissing` 两次（month/year），失败靠次日重试（拒绝：单点失败放大重试窗口；且月/年两轮共享同一批 topic 读取，一处 DB 抖动两轮全断）。

### ③ 触发粒度与频控：每日一次、month+year 两粒度、无额外频控

**选择**：日报成功后固定跑 `heal(month)` + `heal(year)` 两轮，不做「距上次补齐 <N 小时则跳过」类频控。

**理由**：稳态下 HealMissing 扫描零缺口即返回（纯 DB 查询，无 LLM 调用），频控没有收益只有状态复杂度。月切/年切日的全量补齐（59 次 LLM 调用）本来就是期望行为。

**备选**：加最小间隔频控（拒绝：解决不存在的问题）；只补当期不回填历史（拒绝：HealMissing 的双向回填正是要治 2026-09/10 的历史缺口，收窄反而要新写逻辑）。

### ④ 首轮大回填：一次性跑完，不分批不设预算

**选择**：上线后首轮直接补完所有历史缺口（约一两百次本地 LLM 调用），不做每日限量。

**理由**：本地 4B 无 API 成本，一次跑 10-30 分钟可接受；「分批限流」引入跨天状态（已补到哪）与顺延语义交互复杂。若回填中进程重启，幂等性由「只补缺行」保证，重启后从缺的地方继续。

**备选**：每日限 N 个 topic（拒绝如上）；上线前手动 trigger 一次 monthly/yearly job 预热（拒绝：手动步骤不可靠，tail step 首晚自动完成同样效果）。

### ⑤ 可观测性：log + JobResult 透传，不加通知

**选择**：补齐轮结果（healed/failed 计数、粒度、耗时）写 `logging.Infof`；日报 job 的 JobResult.Data 增加 `lifeline_heal` 字段（异步启动即记 `triggered: true`，不等待结果）。scheduler_tasks 的 daily_report 行 last_execution_result 可见触发痕迹。

**理由**：单用户系统，日志 + result 字段足够排查；通知（notification-center）对本 change 是过度设计。

## Risks / Trade-offs

- [首轮回填与当晚其他 LLM 负载并发] → 本地 4B 单实例排队执行（llama.cpp/ollama 侧天然串行），最坏情况拉长当晚补齐耗时；不与日报争用（日报已先行完成）。接受。
- [异步 goroutine 与进程重启] → 回填中断无完成标记，靠「只补缺行」幂等 + 次日重试覆盖；不做持久化任务状态（单用户、低价值）。接受。
- [日报成功 ≠ 模型全程健康（回填中再次不健康）] → 单 (topic, period) 失败容错（决策②）+ 次日重试，缺口收敛速度退化为「每天补一部分」。接受。
- [HealMissing 容错改造影响既有 monthly/yearly job] → 容错包装作为新函数/新模式实现，既有 job 调用路径行为不变（它们自身的 failed++ continue 在 job 层）；用既有 scheduler_jobs 测试守护回归。
- [年报「定稿不刷新」的语义取舍] → 已存在的 2026 year 行（内容停在 09-04）不会被每日补齐更新，只有缺行才补。若用户期望「年报随素材演进刷新」，需另开 change（当期增量刷新）。proposal 已声明 Non-Goal。
- [active 话题数增长导致月切日补齐量线性涨] → 当前 59 active × 1 调用/月切，可忽略；话题数涨 10 倍再考虑预算（届时 pool 也该扩了）。

## Migration Plan

1. 纯代码变更 + `flow/scheduler.md`、`flow/data-enrichment.md` 文档同步，无 DB 迁移。
2. 部署即生效：当晚日报成功后首轮自动回填历史缺口（无需手动 trigger）。
3. 回滚：revert 代码即可——已补齐的归档行是合法数据，无需清理；定时 job 双保险不受影响。

## Open Questions

（无——探索阶段的两个待定细节已在决策①④中定案：异步 goroutine、一次性回填。）
