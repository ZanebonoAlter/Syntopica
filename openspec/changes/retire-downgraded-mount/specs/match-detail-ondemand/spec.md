# match-detail-ondemand — retire-downgraded-mount delta

## MODIFIED Requirements

### Requirement: 匹配详情按需实时计算
系统 SHALL 提供 `GET /api/semantic-boards/:id/match-detail/:tagId` 端点，用户点击 tag chip 时按需实时计算并返回辅助标签逐对匹配明细。不修改匹配流程、不修改表结构、不存储中间数据。

**【2026-11-12 本 change 起生效】响应 SHALL NOT 再包含 `downgraded` 字段**（降级标记已随 retire-downgraded-mount 退役）。`effective_min_hits` 保留：它仍是本次计算实际使用的 minHits 值（= min(direct_max_sim_min_hits, N)），供前端展示判定用门槛，本身不再是「降级」信号。

#### Scenario: 非 direct_hit 场景返回完整 pairs
- **WHEN** tag 以 hit_rate / max_sim / weighted 方式匹配 board
- **THEN** API 返回 SHALL 包含 `direct_hit_auxiliaries` 为空，`pairs` 展示所有 tag 辅助标签与 board 最相似辅助标签的余弦相似度

#### Scenario: direct_hit 场景同时返回精确匹配和完整 pairs
- **WHEN** tag 以 direct_hit 方式匹配 board
- **THEN** API 返回 SHALL 包含 `direct_hit_auxiliaries`（精确匹配列表）和完整的 `pairs`（所有 tag 辅助标签与 board 最相似辅助标签的余弦相似度），以及 `hits` / `hit_rate` / `max_similarity` 聚合指标

#### Scenario: max_sim 降级匹配返回 downgraded 标记
- **WHEN** tag 以 max_sim 方式匹配 board，且 `effective_min_hits < direct_max_sim_min_hits`（即 minHits 被降级）
- **THEN** API 返回 SHALL 包含 `effective_min_hits`（实际使用的 minHits 值），且 SHALL NOT 包含任何 `downgraded` 字段

#### Scenario: 正常匹配返回 downgraded=false
- **WHEN** tag 以任何方式匹配 board
- **THEN** API 返回 SHALL NOT 包含 `downgraded` 字段；`effective_min_hits` 等于 min(direct_max_sim_min_hits, N)
