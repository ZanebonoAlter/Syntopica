## Purpose

方向性校验：对匹配结果（direct_hit 除外）计算 tag identity embedding 与 board embedding 的 cosine 相似度，低于阈值时标记 direction_mismatch=true。direct_hit 因精确重叠已足够可靠，跳过方向校验。

## Requirements

### Requirement: Direction check for non-direct_hit matches

evaluateSemanticBoardMatches 在 switch 匹配后，对 `matchReason != ""` 且 `matchReason != "direct_hit"` 的结果统一执行方向校验。计算 tag identity embedding 与 board embedding 的 cosine 相似度，低于 DirectionSimThreshold（默认 0.5）时标记 direction_mismatch=true。不影响 score，仅标记。

#### Scenario: hit_rate match passes direction check
- **WHEN** tag matches board via hit_rate rule AND cosine(tag_identity_embedding, board_embedding) >= 0.5
- **THEN** direction_mismatch=false, match recorded normally

#### Scenario: hit_rate match fails direction check
- **WHEN** tag matches board via hit_rate rule AND cosine(tag_identity_embedding, board_embedding) < 0.5
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: weighted match fails direction check
- **WHEN** tag matches board via weighted rule AND cosine(tag_identity_embedding, board_embedding) < 0.5
- **THEN** direction_mismatch=true, match still recorded with original score

#### Scenario: tag or board embedding unavailable
- **WHEN** tag identity embedding is NULL OR board embedding is NULL
- **THEN** direction check is skipped, direction_mismatch=false

#### Scenario: direct_hit match
- **WHEN** tag matches board via direct_hit rule
- **THEN** direction check is NOT performed, direction_mismatch=false

### Requirement: Direction check data loading

MatchTopicTag 加载 tag identity embedding（`topic_tag_embeddings` WHERE embedding_type='identity'）和所有活跃 board embedding（`semantic_labels` WHERE label_type='board'），传入 evaluateSemanticBoardMatches。embedding 缺失时不阻塞匹配流程。

#### Scenario: tag has no identity embedding
- **WHEN** tag has no identity embedding in topic_tag_embeddings
- **THEN** tagEmbedding passed as nil, direction check skipped for all max_sim matches

#### Scenario: board has no embedding
- **WHEN** a board's semantic_labels.embedding is NULL
- **THEN** that board is excluded from boardEmbeddings map, direction check skipped for that board

### Requirement: DirectionSimThreshold configurable

DirectionSimThreshold 通过 `ai_settings` 表配置，key 为 `semantic_board_match_direction_sim_threshold`，默认 0.5。与现有 12 个匹配参数一致的管理方式。

#### Scenario: custom threshold
- **WHEN** ai_settings contains `semantic_board_match_direction_sim_threshold` with value 0.6
- **THEN** DirectionSimThreshold=0.6 used for direction check

#### Scenario: no custom threshold
- **WHEN** ai_settings does not contain the key
- **THEN** default 0.5 used

### Requirement: direction_mismatch persisted

topic_tag_board_labels 新增 `direction_mismatch BOOLEAN NOT NULL DEFAULT false` 列。replaceTopicTagBoardLabels 写入时包含此字段。

#### Scenario: persist direction mismatch
- **WHEN** evaluateSemanticBoardMatches returns a match with DirectionMismatch=true
- **THEN** topic_tag_board_labels.direction_mismatch=true for that record

### Requirement: Daily report excludes direction_mismatch tags

collectBoardTags 查询排除 direction_mismatch=true 的标签，包括主查询和 fallback 补算。

#### Scenario: direction_mismatch tag excluded from daily report
- **WHEN** tag has direction_mismatch=true in topic_tag_board_labels for target board
- **THEN** tag is NOT included in daily report for that board

### Requirement: Board articles API direction_mismatch filtering

getBoardArticles 的 filtered_tags 查询默认排除 direction_mismatch=true 的标签。通过 query param `?show_direction_mismatch=true` 可包含。

#### Scenario: default request
- **WHEN** getBoardArticles called without show_direction_mismatch param
- **THEN** filtered_tags excludes direction_mismatch=true tags

#### Scenario: explicit include
- **WHEN** getBoardArticles called with show_direction_mismatch=true
- **THEN** filtered_tags includes all tags including direction_mismatch=true

### Requirement: Match detail API returns direction_sim

getTagMatchDetail 响应新增 `direction_sim` 字段（float64），为 tag identity embedding 与 board embedding 的 cosine 值。无 embedding 时返回 null。

#### Scenario: direction_sim available
- **WHEN** both tag identity embedding and board embedding exist
- **THEN** response includes direction_sim value

#### Scenario: direction_sim unavailable
- **WHEN** tag or board embedding is NULL
- **THEN** response direction_sim is null/omitted
