## Purpose

匹配详情按需实时计算 API，用户点击 tag chip 时返回辅助标签逐对匹配明细和匹配公式，帮助用户理解"为什么这个事件标签被分到了这个板块"。

## Requirements

### Requirement: 匹配详情按需实时计算
系统 SHALL 提供 `GET /api/semantic-boards/:id/match-detail/:tagId` 端点，用户点击 tag chip 时按需实时计算并返回辅助标签逐对匹配明细。不修改匹配流程、不修改表结构、不存储中间数据。

#### Scenario: 非 direct_hit 场景返回完整 pairs
- **WHEN** tag 以 hit_rate / max_sim / weighted 方式匹配 board
- **THEN** API 返回 SHALL 包含 `direct_hit_auxiliaries` 为空，`pairs` 展示所有 tag 辅助标签与 board 最相似辅助标签的余弦相似度

#### Scenario: direct_hit 场景同时返回精确匹配和完整 pairs
- **WHEN** tag 以 direct_hit 方式匹配 board
- **THEN** API 返回 SHALL 包含 `direct_hit_auxiliaries`（精确匹配列表）和完整的 `pairs`（所有 tag 辅助标签与 board 最相似辅助标签的余弦相似度），以及 `hits` / `hit_rate` / `max_similarity` 聚合指标

#### Scenario: max_sim 降级匹配返回 downgraded 标记
- **WHEN** tag 以 max_sim 方式匹配 board，且 `effective_min_hits < direct_max_sim_min_hits`（即 minHits 被降级）
- **THEN** API 返回 SHALL 包含 `downgraded: true`，以及 `effective_min_hits`（实际使用的 minHits 值）

#### Scenario: 正常匹配返回 downgraded=false
- **WHEN** tag 以任何方式匹配 board，且未触发 minHits 降级
- **THEN** API 返回 SHALL 包含 `downgraded: false`，`effective_min_hits` 等于 `direct_max_sim_min_hits`

### Requirement: getTagMatchDetail 返回 direction_sim

getTagMatchDetail 响应新增 `direction_sim` 字段（float64 或 null）。计算方式：在 handler 层实时计算 cosine(tag identity embedding, board embedding)，**不在** `computeMatchDetail` 内部——direction_sim 是匹配后校验结果，非匹配过程的一部分。

#### Scenario: direction_sim available
- **WHEN** tag identity embedding 和 board embedding 均存在
- **THEN** handler 加载两者 embedding，计算 cosine 值，返回 direction_sim

#### Scenario: direction_sim unavailable
- **WHEN** tag identity embedding 或 board embedding 为 NULL
- **THEN** 响应中 direction_sim 为 null

### Requirement: 返回当前匹配配置参数
系统 SHALL 在匹配详情响应中返回当前生效的匹配配置参数（从 ai_settings 读取），包括 sim_threshold、direct_hit_min_overlap、direct_hit_rate、hit_rate_sim_blend、min_effective_sample、direct_max_sim、direct_max_sim_min_hits、direct_max_sim_min_hit_rate、weight_sim、weight_density、weighted_threshold。
