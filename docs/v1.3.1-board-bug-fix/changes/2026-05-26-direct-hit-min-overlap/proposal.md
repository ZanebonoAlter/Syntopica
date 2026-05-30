## Why

`direct_hit` 匹配规则中，只要 tag 的辅助标签与 board 的辅助标签有 **1 个 ID 交集**就直接 score=1.0 命中，完全跳过其他辅助标签是否匹配的检查。这导致包含高频通用辅助标签（如「特朗普」ref_count=141）的事件，即使只有一个弱相关辅助标签交集，也会被强行归入不相关的板块。典型案例：事件「日菲加强安保合作旨在牵制中国」因 LLM 提取了「特朗普」作为辅助标签，与「美国政治与经济动态」板块产生 1 个交集，直接命中（score=1.0），而其他 3 个辅助标签（日菲安全合作、日下涉、马科斯）与该板块毫无关系。

## What Changes

- **direct_hit 规则增加最小交集数要求**：新增可配置参数 `direct_hit_min_overlap`（默认 2），要求 tag 与 board 的辅助标签交集数 ≥ 该阈值才算 direct_hit。交集不足时退回到相似度匹配流程（hit_rate/max_sim/weighted），让其他辅助标签参与评分。
- **LLM 辅助标签提取 prompt 增加相关性约束**：在 event/person 提取 prompt 中明确要求辅助标签必须与事件核心主题强相关，仅为背景提及的人物/国家不应作为辅助标签，减少源头噪音。
- **匹配详情 API 展示 direct_hit 场景下的所有辅助标签对**：当前 `getTagMatchDetail` 在 direct_hit 时只返回精确匹配的辅助标签，不展示未命中标签的相似度。改为同时计算并返回所有辅助标签的逐对匹配（包括命中和未命中），让用户理解「为什么只有 1 个交集就命中了，其他标签如何」。

## Capabilities

### New Capabilities
（无新增能力）

### Modified Capabilities
- `tag-to-board-matching`: direct_hit 规则增加 `direct_hit_min_overlap` 最小交集数要求，交集不足时退回相似度匹配；新增配置参数加载
- `auxiliary-label-extraction`: event/person 提取 prompt 增加辅助标签相关性约束，拒绝仅为背景提及的实体
- `match-detail-ondemand`: direct_hit 场景下同时返回所有辅助标签的相似度匹配对（包含命中和未命中），而非仅返回精确匹配列表

## Impact

- **后端**：`semantic_board_matching.go` 修改 `hasDirectSemanticBoardHit` 及 `evaluateSemanticBoardMatches` 逻辑；`loadConfig` 新增 `direct_hit_min_overlap` 参数；`semantic_board_handler.go` 修改 `getTagMatchDetail` 使 direct_hit 场景也计算并返回完整 pairs；`extractor_enhanced.go` 修改 `buildEventPersonPrompt` 的辅助标签约束
- **前端**：`MatchDetailPanel.vue` 需处理 direct_hit 场景下新增的 pairs 数据展示
- **API 兼容性**：`matchDetailResponse` 在 direct_hit 场景下新增 `pairs`/`hits`/`hit_rate`/`max_similarity` 字段（之前为空数组/零值），非破坏性新增
- **配置**：`ai_settings` 表可选新增 `semantic_board_match_direct_hit_min_overlap` 键
