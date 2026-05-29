## Why

max_sim 规则仅依赖辅助标签之间的 pairwise cosine 相似度，缺乏"标签整体语义方向"与"板块整体语义方向"的校验。导致"日经225指数"因与"标普500指数"辅助标签相似度 0.80 而被匹配到"美国政治与经济动态"板块——辅助标签层面相似（都是股指），但话题方向不符。此外，全部 10 个现有板块由 LLM 升级建议创建，embedding 均为 NULL，导致方向校验无数据可用；且前端缺少板块编辑功能，无法修正板块信息或刷新 embedding。

## What Changes

- **方向性校验**：max_sim 匹配成功后，计算 tag identity embedding 与 board embedding 的 cosine 相似度；低于阈值时标记 `direction_mismatch=true`，仍记录但不计入日报、默认前端隐藏
- **板块 embedding 生成**：LLM 升级建议创建板块时补调 embedder 生成 embedding；为现有 NULL embedding 板块一次性 backfill；description 变更时刷新 embedding
- **板块编辑 UI**：TagsPage 新增板块编辑功能（修改 label、description），调用已有 updateBoard API

## Capabilities

### New Capabilities
- `board-direction-check`: max_sim 方向性校验——计算 tag identity embedding × board embedding 的 cosine，低于阈值标记 direction_mismatch
- `board-editing-ui`: 前端板块编辑功能——编辑对话框、label/description 修改、embedding 刷新

### Modified Capabilities
- `board-upgrade`: LLM 升级建议 create_new 时生成 board embedding（当前缺失）
- `board-management-api`: updateBoard 当 description 变更时刷新 board embedding
- `tag-to-board-matching`: evaluateSemanticBoardMatches 新增 tagEmbedding + boardEmbeddings 参数，max_sim 成功后执行方向校验
- `daily-report-match-fallback`: 日报收集排除 direction_mismatch=true 的标签
- `match-detail-ondemand`: getTagMatchDetail 返回 direction_sim 值
- `match-score-visualization`: TagsPage 默认隐藏 direction_mismatch 标签，提供显示开关

## Impact

- **后端**: `semantic_board_upgrade.go`（embedding 生成）、`semantic_board_matching.go`（方向校验逻辑）、`semantic_board_handler.go`（API 返回字段、embedding 刷新、backfill 接口）、`daily_report/generator.go`（排除过滤）
- **前端**: `TagsPage.vue`（板块编辑 UI、方向不符过滤）、`MatchDetailPanel.vue`（方向校验展示）、`semanticBoards.ts`（类型更新）
- **数据库**: `topic_tag_board_labels` 新增 `direction_mismatch` 列；现有板块 embedding backfill
