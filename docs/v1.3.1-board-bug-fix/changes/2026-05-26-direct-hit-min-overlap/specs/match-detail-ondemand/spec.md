## MODIFIED Requirements

### Requirement: 匹配详情按需实时计算

**原需求**: direct_hit 场景下只返回 `direct_hit_auxiliaries`（精确匹配的辅助标签列表），`pairs` / `hits` / `hit_rate` / `max_similarity` 为空/零值。

**变更为**: direct_hit 场景下同时返回 `direct_hit_auxiliaries` 和完整的 `pairs` / `hits` / `hit_rate` / `max_similarity`。`pairs` 展示所有 tag 辅助标签与该 board 最相似辅助标签的余弦相似度，让用户看到"命中了哪些、没命中哪些"。

#### Scenario: direct_hit 场景展示完整匹配对
- **WHEN** tag 100154（日菲加强安保合作）与 board 3640（美国政治与经济动态）以 direct_hit 匹配，交集为 {特朗普}
- **THEN** API 返回 SHALL 包含 `direct_hit_auxiliaries: [{tag_label: "特朗普", board_label: "特朗普"}]`，同时包含 `pairs` 展示所有 4 个 tag 辅助标签与 board 最相似辅助标签的余弦相似度

#### Scenario: 非 direct_hit 场景行为不变
- **WHEN** tag 以 hit_rate / max_sim / weighted 方式匹配 board
- **THEN** API 返回行为 SHALL 与变更前完全一致（`direct_hit_auxiliaries` 为空，`pairs` 展示完整匹配对）
