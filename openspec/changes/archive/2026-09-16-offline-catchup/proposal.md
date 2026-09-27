<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: reading, scheduler, daily-report -->

## Why

Syntopica 转向常驻拓扑（树莓派 7×24 采集 + PC 按需启动 AI，前置 change `ai-health-heartbeat-reprobe` 已解决暂停/自动恢复）。恢复后仍存在两处"停机回来丢东西"：① 归档时无条件删除 `article_topic_tags` 边，而处理队列不筛 archived——迟到打标挂回的边与"归档=退出分析面"互相打架，且边的删除时序完全靠运气（现状：1.5 千篇归档带边 vs 1.4 万篇无边，同一批文章两种状态）；② 日报 21:00 只生成当天，停机错过的日期永不自动补，需人工逐日重建（AI 恢复当晚积压未清时手动补还会补出不全的报告）。两条链路合起来，间歇使用者"回来后什么都不丢、什么都不用手动补"的最后两块缺口。

## What Changes

- **归档不再删除标签边**：`CleanupOldArticles` 移除删边与孤儿清理段，归档变为纯生命周期标志（`reading_behaviors` 删除、`search_vector` 置 NULL、正文保留等其余行为不变）。标签聚合消费方本就只按时间窗（cotag 30 天、日报按天）控制范围、不筛归档位，删边对它们是多余动作。
- **标签边时间窗回收（β'）**：边按 `created_at` 保留 `tag_edge_retention_days` 天（默认 7，`ai_settings` 可配，非法值回退默认），超窗物理删除并复用现成 `CleanupOrphanedTags` 收孤儿；回收逻辑收编进既有 `aux_label_cleanup` 维护 job（不新增 scheduler 注册）。稳态边表 ≈ 日入库 × 边密度 × 7 天 ≈ 1.5 万行，与现状持平。
- **缺档日报自动补档**：`DailyReportJob` 在生成完当天报告后，扫描窗口内（同 `tag_edge_retention_days`，语义绑定：边在=候选可信）缺失的 (board, date) 组合并自动重建；前置条件为 tag 处理队列已清空，未清则本轮跳过、次日再试（缺档仍在窗口内，不丢失）。
- **补档窗口守卫**：`POST /api/daily-reports/generate` 与调度器 `TriggerNowWithDate` 对早于窗口下界的日期拒绝执行（边已回收、候选必然不全，防止空报告覆盖既有好报告）。
- 明确不改动：日报生成算法本身、`reading_behaviors`/`search_vector` 的归档处理、队列的 archived 不筛语义（正是补档的基石）。

## Capabilities

### New Capabilities

（无——均为既有 capability 的行为扩展）

### Modified Capabilities

- `article-retention`: 「归档清除衍生数据、内容字段全保留」requirement 修订——归档不再删 `article_topic_tags` 边（行为/搜索向量/正文语义不变）；新增 requirement「标签边时间窗回收」（默认 7 天、可配、孤儿回收、迟到打标挂边不再受归档时序影响）。
- `daily-report-system`: 新增 requirement「缺档日报自动补档」（窗口内自动重建 + 队列空前置 + 顺延语义）与「日报重建窗口守卫」（超窗日期拒绝重建，覆盖 generate API 与指定日期触发两条入口）。

## Impact

- **代码**：`internal/reader/service/feed_service.go`（CleanupOldArticles 删边段移除）；`internal/tagmanagement`（新增边 GC 服务函数 + 配置读取，照 `persistent_topic_*` cfg 模式）；`internal/admin/scheduler/job_aux_label_cleanup.go`（串接边 GC）、`job_daily_report.go`（补档扫描）；`internal/topicgraph/handler/daily_report_handler.go`（超窗守卫）。
- **行为**：归档文章的标签边保留 7 天供补档/聚合消费，7 天后随孤儿 tag 一并回收；停机 ≤7 天场景下，恢复 → 队列清空后的下一个 21:00 自动补齐全部缺档日报，零手动。
- **配置**：新增 `ai_settings` 键 `tag_edge_retention_days`（默认 "7"；v1 无专门设置 UI，走既有 ai_settings 写入途径；非法/缺失回退默认）。
- **部署提示**：与心跳 change 叠加后即构成常驻拓扑的完整"停机恢复"链路；无 schema 迁移。
