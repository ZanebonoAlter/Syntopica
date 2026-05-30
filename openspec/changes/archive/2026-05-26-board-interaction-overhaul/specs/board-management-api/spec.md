## ADDED Requirements

### Requirement: 升级建议 DTO 包含标签名称和板块名称
升级建议 API 的响应 SHALL 在每条建议中包含 `auxiliary_labels`（数组，每个元素含 id 和 label）和 `target_board_label`（字符串，当 decision=merge_into_existing 时）。原有 `auxiliary_label_ids` 和 `target_board_id` 字段 SHALL 保留用于执行操作。

#### Scenario: create_new 建议包含辅助标签名称
- **WHEN** LLM 返回 create_new 建议，auxiliary_label_ids=[1, 5, 12]
- **THEN** 响应 SHALL 包含 auxiliary_labels=[{id:1, label:"AI"}, {id:5, label:"半导体"}, {id:12, label:"大模型"}]

#### Scenario: merge_into_existing 建议包含板块名称
- **WHEN** LLM 返回 merge_into_existing 建议，target_board_id=3
- **THEN** 响应 SHALL 包含 target_board_label="AI与机器学习"

#### Scenario: skip 建议不包含额外名称
- **WHEN** LLM 返回 skip 建议
- **THEN** auxiliary_labels 和 target_board_label SHALL 为空/省略

## MODIFIED Requirements

### Requirement: 匹配参数配置 API
系统 SHALL 提供读取和修改匹配参数的 API，参数存储在 ai_settings 表中。

#### Scenario: 读取参数
- **WHEN** 用户请求 GET /api/semantic-boards/matching-config
- **THEN** 系统 SHALL 返回当前 semantic_board_match_sim_threshold, semantic_board_match_direct_hit_rate, semantic_board_match_direct_max_sim, semantic_board_match_weight_sim, semantic_board_match_weight_density, semantic_board_match_weighted_threshold, semantic_board_match_max_boards, semantic_board_match_direct_max_sim_min_hits（默认 2）, semantic_board_match_direct_max_sim_min_hit_rate（默认 0.3）

#### Scenario: 修改参数
- **WHEN** 用户通过 PUT /api/semantic-boards/matching-config 修改 semantic_board_match_sim_threshold 为 0.7
- **THEN** 系统 SHALL 更新 ai_settings 中的对应配置，后续匹配使用新值
