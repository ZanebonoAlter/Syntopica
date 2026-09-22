# Design — unify-feed-summary-toggles

## Context

`completion_on_refresh` 当前是死字段：唯一消费点 `ListArticlesForCompletion`（repository.go:473）零调用方；7 个开总结的 feed 全为 firecrawl feed，其自动总结只由 `article_summary_enabled` 决定（`buildArticleFromEntry` 两分支、`job_firecrawl.go` 置 incomplete 两处、`ListReadyArticles` 扫描）。gorm default:true 造成 24/24 存量全 true。前端双编辑入口（主界面 EditFeedDialog / settings FeedDetailEditor）能力互补残缺、文案两套。见 proposal.md - Why。

## Goals / Non-Goals

**Goals**
- `completion_on_refresh` 成为真正生效的"刷新后自动总结"闸门，组合语义按方案 A（主开关=能力、闸门=自动）。
- 自动总结默认关闭、存量一次性全关；手动总结能力保留。
- feed 编辑收敛到 settings 单一入口，词汇表统一。

**Non-Goals**
- 不解决排行榜 feed 跨源重复文章重复总结问题（同链接共享总结，另开 change）。
- 不改 AI 总结本身的生成逻辑/提示词/能力路由。
- 不改 content_completion 调度器的节奏与暂停控制（analysis-pause-control 契约不变）。
- 不动打标签（tagging）与全文抓取链路的触发条件。

## Decisions

### D1: 闸门采用"标记侧 + 扫描侧"双保险

标记侧（决定新文章是否进入总结流程）：
- `buildArticleFromEntry`（feed_service.go）两个分支：`ArticleSummaryEnabled` → `ArticleSummaryEnabled && CompletionOnRefresh`；
- `job_firecrawl.go` 置 `summary_status="incomplete"` 的两处（抓取成功完成处 + 抓取终态失败降级 RSS 处）同样加 `&& CompletionOnRefresh`。

扫描侧（决定调度器捞谁）：
- `ListReadyArticles` JOIN feeds 条件追加 `feeds.completion_on_refresh = true`。

为什么两侧都改：仅改标记侧，存量 26 篇 `pending/incomplete` 文章仍会被调度器继续消费；仅改扫描侧，文章状态机仍被错误置位、overview 统计持续虚高。两侧一致后语义闭环。

### D2: 迁移同时冻结存量积压状态

新增 Migration（`postgres_migrations.go` 既有模式，幂等）：
1. `UPDATE feeds SET completion_on_refresh = false WHERE completion_on_refresh = true`；
2. `UPDATE articles SET summary_status = 'complete' WHERE summary_status IN ('pending','incomplete')`（仅冻结自动路径的待处理标记；`failed` 保留以维持失败可观测性——失败文章不会再被扫描捞起，无需重置）。

gorm tag `CompletionOnRefresh` 的 `default:true` 同步改 `default:false`（AutoMigrate 调整列默认值；存量行由上面第 1 步显式归位）。

### D3: 次要路径逐一审查，死代码删除

- `ListArticlesIncomplete`（repository.go:457，条件含 `article_summary_enabled`）：查明调用方后决定是否同步加闸门或随死代码一并处理（实现任务中确认）。
- `job_blocked_article_recovery.go:51`（按 `article_summary_enabled=true` 找 feed）：恢复路径语义是"为开总结的 feed 恢复被卡文章"，加 `&& completion_on_refresh` 与主闸门对齐。
- `ListArticlesForCompletion`（repository.go:473）：删除（零调用方）。

### D4: 编辑入口收敛——删除 EditFeedDialog，深链跳转

- 主界面 FeedLayoutShell 的编辑入口改为 `navigateTo('/settings?feed=<id>&section=feeds')`；删除 `EditFeedDialog.vue` 及其引用/测试。
- SettingsSectionFeeds 初始化时读 `route.query.feed`，feeds 列表就绪后选中并滚动定位；目标不存在则忽略参数落默认视图。
- FeedDetailEditor 补齐：AI 总结 toggle、刷新后自动总结 toggle（原"内容补全"改名换文案）、最大重试次数、RSS 地址编辑（自 EditFeedDialog 迁移，保存走现有 PATCH）。
- 文案按 proposal 词汇表，直接写在 FeedDetailEditor 单一位置（收敛后无第二展示处，无需抽共享常量模块；词汇表的持久权威在本 change specs 中）。
- 前端 `completionOnRefresh` 回退值统一 `?? false`（顺带消灭 EditFeedDialog 内 `?? false`/`?? true` 不一致 bug——随组件删除自然消失，但 store 映射处统一）。

### D5: 后端 API 不动，靠字段语义收紧

CreateFeedRequest 的 Go 零值 false 天然满足新默认；UpdateFeedRequest 指针字段已支持单字段 PATCH。无路由/契约变更。

## Risks / Trade-offs

- [关自动后打标签素材变长] 打标签与话题 watch 的素材降级链为 AI 总结 → firecrawl 全文 → RSS 内容；总结缺失时 fallback 全文，token 单次消耗可能上升 → 缓解：可接受（重复总结的浪费远大于素材增量）；若实测打标签成本显著上升，后续可按 feed 精选总结开关。
- [用户忘记手动总结导致整理稿缺失] 部署后自动总结静默全关 → 缓解：完成汇报明确告知；settings 列表"AI 总结"标识让开启状态可见，想恢复逐 feed 打开即可。
- [overview/队列统计口径] 存量 pending 冻结后，补全 overview 的待处理计数应归零（D2 第 2 步保证）；实现时验证 GetOverview 不再显示僵尸待办。
- [深链参数时效] feeds 列表异步加载期间参数保留于 URL，加载失败重试后仍可定位 → 已在 ui-design 受影响状态中约定。

## Migration Plan

1. 合并部署：AutoMigrate（列默认值改 false）→ 新 Migration（feeds 归位 + articles 冻结）幂等执行。
2. 回滚策略：代码回滚后，已被置 false 的闸门需用户按 feed 手动重开（或执行反向 UPDATE）；已生成的整理稿不受影响。
3. 部署后用户可见行为与操作清单见 proposal Impact 节，完成汇报需含"部署后影响 + 需要的操作"。

## Open Questions

（无——方案 A 组合语义、存量全关、入口统一均已在探索阶段与用户确认。）
