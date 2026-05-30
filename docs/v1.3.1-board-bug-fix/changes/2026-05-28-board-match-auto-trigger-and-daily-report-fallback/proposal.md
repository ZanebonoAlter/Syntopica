## Why

board 匹配管道（`MatchTopicTag`）只在手动 backfill 时触发。文章打标签流程创建事件标签、附加辅助标签、生成 embedding 后，不会自动执行 board 匹配，导致 `topic_tag_board_labels` 缺失。日报的 `collectBoardTags` 通过 `topic_tag_board_labels` JOIN 收集 tag，因此大量已打标签的文章对应的 tag 没有板块归属，日报只生成极少量内容（如伊朗局势 5.27 有 33 个相关事件标签但日报只收了 1 个）。

同时，`max_sim` 规则中 `minHits = min(config.DirectMaxSimMinHits, N)` 的降级逻辑在标签只有 1 个辅助标签时将阈值从 2 降为 1，但这些降级匹配与正常匹配在前端同等展示，用户无法区分"满足完整阈值"和"因标签不足被降级"。

## What Changes

- **embedding 完成后自动触发 board 匹配**：`EmbeddingQueueService.processNext` 在 embedding 生成完成后，对 event tag 自动调用 `MatchTopicTag`，将匹配结果写入 `topic_tag_board_labels`
- **日报收集增加兜底补算**：`collectBoardTags` 发现 tag 有辅助标签但无 `topic_tag_board_labels` 记录时，现场调 `MatchTopicTag` 补算并写入，确保日报不因上游管道延迟而遗漏 tag
- **max_sim 降级匹配分组展示**：`topic_tag_board_labels` 新增 `downgraded` 布尔字段，匹配时记录 minHits 是否被降级；前端 MatchDetailPanel 和 tag chip 列表对降级匹配降低视觉权重

## Capabilities

### New Capabilities
- `board-match-auto-trigger`: event tag embedding 完成后自动触发 board 匹配，无需手动 backfill
- `daily-report-match-fallback`: 日报收集 tag 时对无板块归属的 tag 现场补算匹配

### Modified Capabilities
- `tag-to-board-matching`: 匹配结果记录 `downgraded` 标记，标识 minHits 降级匹配
- `match-detail-ondemand`: API 返回 `downgraded` 字段，前端 MatchDetailPanel 区分展示降级匹配
- `match-score-visualization`: tag chip 列表对降级匹配降低视觉权重

## Impact

- **后端**：`embedding_queue.go` 的 `processNext` 增加 embedding 完成后调 `MatchTopicTag` 逻辑；`generator.go` 的 `collectBoardTags` 增加兜底补算路径；`semantic_board_matching.go` 的 `replaceTopicTagBoardLabels` 写入时新增 `downgraded` 字段；`semantic_board_handler.go` 的 `getTagMatchDetail` 返回 `downgraded` 字段
- **数据库**：`topic_tag_board_labels` 表新增 `downgraded` 布尔列，默认 false
- **前端**：`MatchDetailPanel.vue` 匹配流程步骤标注降级说明；`TagsPage.vue` 的 tag chip 对降级匹配降低视觉权重
- **API 兼容性**：`topic_tag_board_labels` 新增非破坏性列；match-detail API 响应新增 `downgraded` 字段
