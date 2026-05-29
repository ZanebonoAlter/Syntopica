## MODIFIED Requirements

### Requirement: updateBoard refreshes embedding on description change

PUT /api/semantic-boards/:id 当 description 变更时，重新生成 board embedding（输入 label + description）。当前仅在 label 变更时生成 embedding。

#### Scenario: description changed
- **WHEN** updateBoard called with new description value
- **THEN** board embedding regenerated from `new_label + ". " + description`, saved to semantic_labels.embedding

#### Scenario: label changed
- **WHEN** updateBoard called with new label value
- **THEN** board embedding regenerated from `new_label + ". " + description` (统一输入格式)

#### Scenario: neither changed
- **WHEN** updateBoard called without label or description changes
- **THEN** embedding NOT regenerated

### Requirement: Board embedding backfill API

POST /api/semantic-boards/backfill-embeddings 为所有 embedding IS NULL 且 label_type='board' 的板块生成 embedding（输入 `label + ". " + description`，description 为空时仅用 label）。返回 backfill 数量。

#### Scenario: backfill null embeddings
- **WHEN** backfill-embeddings called
- **THEN** all boards with NULL embedding get embedding generated from `label + ". " + description`, count returned

### Requirement: Board rematch API

POST /api/semantic-boards/rematch-all 查询所有在 `topic_tag_board_labels` 中有记录的 tag，逐个调用 `MatchTopicTag` 重新匹配。用于 backfill embedding 后刷新 direction_mismatch 标记。

#### Scenario: rematch after backfill
- **WHEN** rematch-all called
- **THEN** all tags with existing board labels are re-matched, topic_tag_board_labels records updated with current direction_mismatch values

#### Scenario: partial failure
- **WHEN** individual tag rematch fails
- **THEN** log error, continue with remaining tags, return success/failure counts

### Requirement: getBoardArticles supports direction_mismatch filtering

GET /api/semantic-boards/:id/articles 新增 query param `show_direction_mismatch`。默认 `false`，filtered_tags 排除 direction_mismatch=true 的标签。`true` 时包含全部。

#### Scenario: default (hide direction mismatch)
- **WHEN** request without show_direction_mismatch param
- **THEN** filtered_tags SQL adds `AND NOT COALESCE(tbl.direction_mismatch, false)` condition

#### Scenario: show direction mismatch
- **WHEN** request with show_direction_mismatch=true
- **THEN** filtered_tags returns all tags regardless of direction_mismatch

### Requirement: Matching config API includes direction_sim_threshold

GET /api/semantic-boards/matching-config 返回新增 `direction_sim_threshold` 参数。PUT 可更新此参数。

#### Scenario: read config
- **THEN** response includes direction_sim_threshold (default 0.5)

#### Scenario: update config
- **WHEN** PUT with direction_sim_threshold=0.6
- **THEN** value saved to ai_settings, subsequent matching uses 0.6
