<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, scheduler, data-enrichment -->

## Why

话题泳道的月报/年报（`topic_lifeline_context` 归档行）近期断供：`lifeline_monthly` 在 2026-10-01 03:30 触发时撞上 `aihealth` 探测不健康窗口被 `PauseAware` 跳过，wall-clock 循环直接把 next 推到 11-01——**skip 一次即丢一整月，无重试**。年报更脆弱：year 粒度的检查自愈只在每年 1 月 1 日的 `lifeline_yearly` job 内执行，年中新增/激活的话题年报要等次年才补（2026 期内容停留在 09-04 手动触发）。

每日日报生成成功本身就证明模型层健康（日报生成依赖 LLM），是月/年报缺口自愈的天然挂载点：把补齐探测挂到日报 job 尾部，断供窗口从「一个月/一年」缩到「一天」。

## What Changes

- 日报 job（`DailyReportJob`）尾部追加 **lifeline gap check tail step**（异步 goroutine，不拖长日报 job）：对 month、year 两粒度各跑一次 `HealMissing`——扫描「有 section 素材但无归档行」的 (topic, period) 并补齐（复用现有双向回填逻辑，不新写生成算法）。
- **顺延不丢**：当天模型不健康/失败则当日不补，次日日报成功后再试（与 `backfillMissingReports` 同哲学）；单话题补齐失败不阻塞其他话题。
- **稳态零 LLM 成本**：归档行齐全时不发起任何 LLM 调用；仅月切/年切日或存在断供缺口时产生调用（active 话题 × 缺失期数，本地 qwen3.5-4b 承载）。
- 首轮上线会一次性回填全部历史缺口（2026-09 期缺 ~55 行、2026-10 期全缺、年报 09-04 后空洞，估计一两百次本地 LLM 调用，一次性成本，需在设计中确认是否加预算/分批控制）。
- `lifeline_monthly` / `lifeline_yearly` 定时 job **保留为双保险不删**；`lifeline_weekly` 维持现有禁用状态不动。
- 模型路由**维持现状**（`data_enrichment_news` → 本地 qwen3.5-4b Q8）：实测月报素材 p90 <1k tokens、年报 p90 ~5.7k tokens / max ~12.6k tokens，均远低于 32k 窗口；月/年报一直由 4B 生成且质量已被接受（用户感知好的 14 天档汇总实为 digest_polish 路由的 DeepSeek flash，与月/年报不同源，不存在「沿用现有质量」的降级）。年报头部重度话题（素材 >8k 字符）若日后质量不满意，可在独立 change 中做分级路由（走已配置的 `data_enrichment_analysis` / DeepSeek），本 change 不动路由。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `data-enrichment`: 「循环 A 触发与汇总算法」Requirement 的触发契约扩展——month/year 粒度的检查自愈除各自定时 job 外，SHALL 在每日日报成功生成后异步触发一次；补齐失败顺延至次日重试，SHALL NOT 阻塞日报 job 本身。

## Impact

- **后端代码**：
  - `backend-go/internal/admin/scheduler/job_daily_report.go`：日报 job 尾部追加 lifeline gap check tail step（新 goroutine + 错误隔离）
  - `backend-go/internal/dataenrichment/service/lifeline_context.go`：`HealMissing` 复用为主，按设计结论可能追加批次预算/并发上限微调
  - `backend-go/internal/dataenrichment/wire.go`：暴露 heal 所需 service/lister 获取入口（`GetLifelineService` 已有，lister 需确认）
- **调度系统**：无新 job 注册；`lifeline_monthly` / `lifeline_yearly` / `daily_report` 三 job 配置不变
- **数据模型**：无表结构变更；`topic_lifeline_context` 写入路径复用现有 Upsert
- **模型路由**：无变更
- **前端**：无变更（月/年报消费方 `listContexts` API 行为不变，只是数据及时性恢复）
- **文档**：`docs/reference/flow/scheduler.md`（日报 job 职责）、`docs/reference/flow/data-enrichment.md`（循环 A 触发时机）需同步
