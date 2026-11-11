<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, semantic-board -->

## Why

日报与雷达上的「来源数 / 热度」会虚高，两种成因都已在真库取证：

1. **补档生成历史日报时，关键字物化轨扫的是「今天」而不是报告日**：`MaterializeKeywordWatches` 没有接收报告日期，内部固定调用 `scanBoardDayArticles(boardID, time.Time{})`，而该函数在零值时退化为 `time.Now()`。2026-11-03 21:13–21:21 补档的 10-31 / 11-01 / 11-02 三份报告各混入同一批 7 篇 `pub_date=2026-11-03` 的文章（全部来自 `watch_id=7`「DeepSeek Harness动态与Harness实践」，`lane_tier=watch_keyword`），造成**同一篇文章连续出现在多天日报里**；全库 `watch_keyword` 轨已污染 21 份报告 / 50 篇非当天文章（2026-09-12 起，样例 thread `16222/16370/16487` 的 `created_at` 均在 11-03 补档时段）。
2. **同一篇文章的多个 feed 副本被当成多个独立来源**：`articles` 唯一键是 `(feed_id, link)`，同一 URL 被两个 feed 抓取即存两行（示例：`170304`@feed11 与 `170503`@feed3，link 同为 `cnblogs.com/token-ai/p/23600509`，标题与 `pub_date` 完全一致）。`story_signals` 每行一个 `(story, article)` 且 `participant_key = feed:<id>`，于是 1 篇报道被算成 2 个参与者，`story_heat` 从 0.7 虚高到 1.4（截图「2 个来源 · 热度 1.4」即此）。全库同 link 重复副本 505 组 / 1038 行，已全部计入 signals，影响 32 条事件线。

两者叠加才是用户观察到的「每日事件反复选同一篇、热度虚高」全貌：A 让同一篇文章进多天报告（每天各自新建事件线、各算一遍热度），B 让单篇报道的来源数翻倍。

## What Changes

- **A. 关键字物化轨的日期穿透**：`MaterializeKeywordWatches` 新增报告日期入参并透传到 `scanBoardDayArticles`；补档 / 重建历史报告时扫描窗口跟随 `period_date`，不再退化为 `time.Now()`。生产调用点只有 `daily_report_orchestrator.go` 一处（sentence 轨早已正确传 `date`），同步补齐其测试调用点。
- **B. 参与者身份按内容身份去重（口径变更）**：同一篇内容（规范化 link 相同：去 `#fragment` 与 `?query`）的多个 article 副本，在热度证据层只贡献 **1 个参与者**；`participant_count`、`story_heat`、`lane_heat`、「N 个来源」文案随之按独立内容计数。实现落在 `story_signals` 的写入与参与者计数，`(story_id, article_id)` 主键与热度公式本身不变（仍 48h/24h 半衰期）。
- **B 的边界**：`articles` 入库层**不变**（`(feed_id, link)` 唯一保留，feed 归属与打标复用语义不受影响）；`related_article_ids` 生成口径**不变**（仍是 tag→文章并集），story 层只读消费；同标题不同 URL 的转载**不合并**（本期只认规范化链接）。
- **存量修复**：按新口径重算受影响事件线的 `story_signals` / `participant_count` / 热度（幂等、可重入），**不重建任何日报正文**（section / thread 保持不变），旧报告不回填身份列。
- 文档同步：`docs/reference/flow/daily-report.md`（热度与物化轨约束节）、`docs/reference/standard/backend/` 相关口径说明；`story-heat-evidence` 原「转载各记一个参与者」的已知简化条目随之作废。

## Capabilities

### New Capabilities

（无新增 capability。）

### Modified Capabilities

- `story-heat-evidence`：改写「参与者身份与去重」——参与者身份由 `feed:<id>` 扩展为「内容身份优先、feed 兜底」；同一规范化链接的多份副本只计 1 个参与者；原「同一篇 article 被多个 feed 转载时各记一个参与者」的已知简化条目删除。
- `watch-materialized-topic`：「关键字轨物化生成」中的「当天发布窗口」显式定义为**报告周期日（`period_date`）**，补档 / 重建历史报告时 SHALL 使用该日期而非执行时刻；日期不得由调用方缺省。
- `daily-report-system`：「缺档日报自动补档」补充一致性要求——补档重建出的报告在内容口径上 SHALL 与「当天按时生成」等价（同一 `period_date` 得到同一发布窗口），不得因执行时刻不同而混入非当天的文章。

## Impact

- **代码（A）**：`backend-go/internal/topicgraph/service/watch_materialize_keyword.go`（签名与 `scanBoardDayArticles` 调用）、`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（生产调用点）、`watch_materialize_integration_test.go`（调用点）。
- **代码（B）**：`backend-go/internal/topicgraph/repository/story_signals_repository.go`（signal 写入与 `RecomputeSignalsForStory` / `recomputeStoryParticipantsWith`）、`story_heat.go`（`loadBoardHeatEvidence` 与参与者去重）、`report_story_highlights.go`、`board_radar_stories.go`、`lane_snapshot_repository.go`（凡按 `participant_key` 去重/计数的消费点）。
- **数据**：`story_signals` 新增内容身份判定所需信息（优先在写入时归一，必要时加列/加索引，迁移需可重入）；存量按事件线重算，**不删除任何日报 thread/section**。
- **API / 前端**：响应字段不变（`participants` / `heat` 数值语义变化）；「N 个来源」显示由重复副本导致的虚高自动回落，无需前端改动。
- **文档**：`docs/reference/flow/daily-report.md`、`docs/reference/flow/semantic-board.md`（雷达消费口径）、`docs/reference/database/tables/`（若加列）。
- **不影响**：RSS 入库去重（`rss-article-dedup`）、跨 feed 副本打标复用、日报聚类/分桶/归属逻辑、事件线身份判定（`subject_key` 三层匹配）。
