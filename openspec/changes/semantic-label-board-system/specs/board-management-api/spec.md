## ADDED Requirements

### Requirement: 板块 CRUD API
系统 SHALL 提供 SemanticBoard（semantic_labels label_type=board）的增删改查 API，包括列表、详情、手动创建、编辑、删除。API SHALL 使用 `/api/semantic-boards` 命名空间，避免与每日叙事板 `/api/narratives/boards` 混淆。

#### Scenario: 手动创建板块
- **WHEN** 用户通过 API 创建板块 "量子计算"
- **THEN** 系统 SHALL 创建 semantic_label（label_type="board", source="manual", protected=true），生成 embedding

#### Scenario: 列出板块
- **WHEN** 用户请求板块列表
- **THEN** 系统 SHALL 返回所有 label_type="board" 且 status="active" 的 semantic_labels，包含 ref_count 和 tag_count

### Requirement: 辅助标签池查询 API
系统 SHALL 提供辅助标签池的查询 API，支持按 ref_count 排序、按 label 搜索、查看 aliases 和 merge 历史、禁用辅助标签、手动合并 alias。

#### Scenario: 查看辅助标签列表
- **WHEN** 用户请求辅助标签列表
- **THEN** 系统 SHALL 返回所有 label_type="auxiliary" 的 semantic_labels，包含 ref_count 和 aliases

#### Scenario: 禁用辅助标签
- **WHEN** 用户调用辅助标签禁用 API
- **THEN** 系统 SHALL 将该辅助标签 status 更新为 "disabled"

#### Scenario: 手动合并辅助标签 alias
- **WHEN** 用户调用辅助标签合并 API，将 source 合并到 target
- **THEN** 系统 SHALL 迁移 source 的 tag 关联，并将 source label 加入 target aliases

### Requirement: 升级建议 API
系统 SHALL 提供查看当前升级候选（ref_count ≥ 5 的辅助标签 + 聚类结果）和触发 LLM 升级建议的 API。

#### Scenario: 查看升级候选
- **WHEN** 用户请求升级候选列表
- **THEN** 系统 SHALL 返回 ref_count ≥ 5 的未升级辅助标签及其预聚类结果

#### Scenario: 触发 LLM 升级建议
- **WHEN** 用户通过 API 触发升级建议
- **THEN** 系统 SHALL 执行聚类 + LLM 判断流程，返回建议结果供用户确认/拒绝

#### Scenario: 确认执行升级建议
- **WHEN** 用户通过 API 确认 create_new 或 merge_into_existing 建议
- **THEN** 系统 SHALL 写入 SemanticBoard 和 board_composition，并返回执行结果

### Requirement: 回填触发 API
系统 SHALL 提供手动触发回填的 API 端点，支持 all、unassigned、board 三种模式，并提供进度查询。

#### Scenario: 触发回填
- **WHEN** 用户调用 POST /api/boards/backfill
- **THEN** 系统 SHALL 将待回填的 tag 入队，返回任务 ID

#### Scenario: 查询回填进度
- **WHEN** 用户请求回填任务状态
- **THEN** 系统 SHALL 返回 total、processed、failed、status

### Requirement: 匹配参数配置 API
系统 SHALL 提供读取和修改匹配参数的 API，参数存储在 ai_settings 表中。

#### Scenario: 读取参数
- **WHEN** 用户请求 GET /api/boards/matching-config
- **THEN** 系统 SHALL 返回当前 semantic_board_match_sim_threshold, semantic_board_match_direct_hit_rate, semantic_board_match_direct_max_sim, semantic_board_match_weight_sim, semantic_board_match_weight_density, semantic_board_match_weighted_threshold, semantic_board_match_max_boards

#### Scenario: 修改参数
- **WHEN** 用户通过 PUT /api/boards/matching-config 修改 semantic_board_match_sim_threshold 为 0.7
- **THEN** 系统 SHALL 更新 ai_settings 中的对应配置，后续匹配使用新值

### Requirement: 标签关联的辅助标签和板块查询
系统 SHALL 提供查询 tag 关联的辅助标签列表和所属板块列表的 API。

#### Scenario: 查看 tag 的辅助标签
- **WHEN** 用户请求 tag 的辅助标签
- **THEN** 系统 SHALL 返回该 tag 通过 topic_tag_semantic_labels 关联的所有 semantic_labels

#### Scenario: 查看 tag 的板块
- **WHEN** 用户请求 tag 的板块归属
- **THEN** 系统 SHALL 返回该 tag 通过 topic_tag_board_labels 关联的所有 label_type="board" 的 semantic_labels，按匹配分排序

### Requirement: Board composition 管理 API
系统 SHALL 提供查看和修改 SemanticBoard 构成辅助标签的 API，支持从 board_composition 移除辅助标签。

#### Scenario: 查看 board composition
- **WHEN** 用户请求 SemanticBoard 的构成标签
- **THEN** 系统 SHALL 返回该 board 的所有辅助标签、ref_count 和 aliases

#### Scenario: 移除 board 构成标签
- **WHEN** 用户从 SemanticBoard 中移除辅助标签
- **THEN** 系统 SHALL 删除对应 board_composition 记录，且不自动回填历史 tag-board 归属
