## Why

前一轮 `board-direction-check-and-board-editing` 只在 `max_sim` 规则上加了方向校验，但实际数据暴露了三个更深层的问题：

1. **hit_rate 和 weighted 同样被误导**："荷兰军用直升机在南海遭中国军队拦截"通过 hit_rate（`0.7×0.90 + 0.3×0.20 = 0.69`）匹配到"美国政治与经济动态"；"欧洲斯托克50指数/英国富时100/德国DAX"通过 hit_rate/weighted 匹配到"标普500指数"。辅助标签层面语义相近（都是金融指标），但板块方向完全不对。
2. **文章展示无优先级**：板块下文章纯按时间排列，低质量匹配的新闻和 direct_hit 混在一起，用户无法快速找到最有价值的内容。
3. **日报展示过于冗长**：一个板块聚类后产生 35 个叙事，翻页体验差；板块动态（dynamics）是整段揉在一起的文本，信息密度低。

## What Changes

### P1: 方向校验扩展到所有规则
- `hit_rate` 和 `weighted` 匹配成功后也执行方向校验
- 代码改动极小：将方向校验逻辑从 `case max_sim` 提到 switch 外统一执行

### P2: 文章排序优先级
- `getBoardArticles` 返回的文章按匹配质量排序：direct_hit > hit_rate > max_sim（常规）> max_sim（降级）= weighted
- 同 tier 内按 score 降序，同 score 按时间倒序
- 多辅助标签取最高 tier

### P3: 日报精简 + 报纸布局重构
- **输入端裁剪**：`collectBoardTags` 携带 match_reason/score，质量筛选层按 tier 过滤
- **聚类数限制**：ClusterTags prompt 限制 8-15 组
- **去掉板块动态**：移除 `GenerateDynamics`（Dynamics 字段保留但不再生成）
- **多页报纸布局**：每页 4-5 个聚类，第 1 页放报头+highlights+最核心聚类，后续页每页有"本页热点"

## Capabilities

### Modified Capabilities
- `tag-to-board-matching`: 方向校验从仅 max_sim 扩展到 hit_rate + weighted
- `board-article-sorting`: getBoardArticles 按 tier + score + 时间排序
- `daily-report-generation`: 输入端质量筛选 + 聚类数限制 + 去掉 dynamics
- `daily-report-display`: 多页报纸布局替代翻页式

## Impact

- **后端**: `semantic_board_matching.go`（方向校验扩展）、`semantic_board_handler.go`（文章排序）、`daily_report/generator.go`（质量筛选 + dynamics 移除）、`daily_report/cluster.go`（prompt 调整）
- **前端**: `BoardDailyReportTimeline.vue`（报纸布局重写）、`TagsPage.vue`（排序适配）
- **数据库**: 无 schema 变更

## Dependencies

- 依赖已完成的前轮变更 `board-direction-check-and-board-editing`（direction_mismatch 字段、方向校验基础设施）
