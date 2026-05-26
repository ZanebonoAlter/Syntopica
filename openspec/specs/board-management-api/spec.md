## Purpose

SemanticBoard 和辅助标签相关的所有 API 端点，包括 CRUD、升级建议、回填、匹配参数配置、辅助标签推荐和 composition 管理。

## Requirements

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

#### Scenario: 查看升级候选
- **WHEN** 用户请求升级候选列表
- **THEN** 系统 SHALL 返回 ref_count ≥ 5 的未升级辅助标签及其预聚类结果

#### Scenario: 触发 LLM 升级建议
- **WHEN** 用户通过 API 触发升级建议
- **THEN** 系统 SHALL 执行聚类 + LLM 判断流程，返回建议结果供用户确认/拒绝

#### Scenario: 确认执行升级建议
- **WHEN** 用户通过 API 确认 create_new 或 merge_into_existing 建议
- **THEN** 系统 SHALL 写入 SemanticBoard 和 board_composition，并返回执行结果

### Requirement: 升级建议面板支持逐项处理和重新生成
前端 SHALL 在升级建议面板中支持逐项处理 LLM 建议。用户确认某个 create_new 或 merge_into_existing 建议成功后，面板 SHALL 保持打开，并将该建议从待处理列表移除或标记为已处理，不得自动关闭整轮建议面板。前端 SHALL 提供重新生成升级建议的操作入口，允许用户在已有建议列表存在时重新调用升级建议 API 并替换当前建议列表。

#### Scenario: 确认单个建议后继续处理剩余建议
- **WHEN** 面板中存在多个升级建议，用户确认其中一个 create_new 或 merge_into_existing 建议且 API 返回成功
- **THEN** 面板 SHALL 继续保持打开，并保留剩余未处理建议供用户继续确认

#### Scenario: 重新生成升级建议
- **WHEN** 面板中已经存在升级建议，用户点击重新生成
- **THEN** 前端 SHALL 重新调用 POST /api/semantic-boards/upgrade-suggest，并用新的建议结果替换当前建议列表

#### Scenario: 处理完成后提示回填
- **WHEN** 用户确认执行至少一个升级建议
- **THEN** 前端 SHOULD 提示用户可手动触发匹配回填，使历史 tag-board 归属按最新 board composition 生效

### Requirement: 回填触发 API
系统 SHALL 提供手动触发回填的 API 端点，支持 all、unassigned、board 三种模式，并提供进度查询。

#### Scenario: 触发回填
- **WHEN** 用户调用 POST /api/semantic-boards/backfill
- **THEN** 系统 SHALL 将待回填的 tag 入队，返回任务 ID

#### Scenario: 查询回填进度
- **WHEN** 用户请求回填任务状态
- **THEN** 系统 SHALL 返回 total、processed、failed、status

### Requirement: 匹配参数配置 API
系统 SHALL 提供读取和修改匹配参数的 API，参数存储在 ai_settings 表中。

#### Scenario: 读取参数
- **WHEN** 用户请求 GET /api/semantic-boards/matching-config
- **THEN** 系统 SHALL 返回当前 semantic_board_match_sim_threshold, semantic_board_match_direct_hit_rate, semantic_board_match_direct_max_sim, semantic_board_match_weight_sim, semantic_board_match_weight_density, semantic_board_match_weighted_threshold, semantic_board_match_max_boards, semantic_board_match_direct_max_sim_min_hits（默认 2）, semantic_board_match_direct_max_sim_min_hit_rate（默认 0.3）, semantic_board_match_min_effective_sample（默认 3）, semantic_board_match_hit_rate_sim_blend（默认 0.7）

#### Scenario: 修改参数
- **WHEN** 用户通过 PUT /api/semantic-boards/matching-config 修改 semantic_board_match_sim_threshold 为 0.7
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

### Requirement: 辅助标签推荐 API
系统 SHALL 提供基于 embedding 相似度的辅助标签推荐 API，用于手动创建或编辑 SemanticBoard 时人工填充 board_composition。API SHALL 接受 label（必选）和 description（可选），生成 board label + description embedding 后与全量 active 辅助标签的 storage embedding（semantic_labels.embedding）计算余弦相似度，按相似度从高到低排序返回，不设阈值。推荐结果 SHALL 仅供用户选择，不自动写入 board_composition，也不参与自动 tag-board 匹配规则。API SHALL 支持分页（page/page_size）和搜索过滤（排除已在 board_composition 中的标签）。

#### Scenario: 创建时推荐辅助标签
- **WHEN** 用户调用 GET /api/semantic-boards/suggest-auxiliaries?label=量子计算
- **THEN** 系统 SHALL 生成 "量子计算" 的 board 查询 embedding，与所有 active 辅助标签的 storage embedding 计算相似度，按相似度降序返回分页列表，包含 id、label、ref_count、aliases、similarity

#### Scenario: 编辑时推荐辅助标签
- **WHEN** 用户调用 GET /api/semantic-boards/:id/suggest-auxiliaries
- **THEN** 系统 SHALL 使用已有 board 的 label+description 生成 embedding，返回推荐列表，排除已在 board_composition 中的辅助标签

#### Scenario: 搜索过滤
- **WHEN** 用户调用 GET /api/semantic-boards/suggest-auxiliaries?label=AI&search=图像
- **THEN** 系统 SHALL 先按搜索词过滤辅助标签（label/slug 模糊匹配），再计算相似度排序

### Requirement: Board composition 添加 API
系统 SHALL 提供向 SemanticBoard 的 board_composition 中添加辅助标签的 API。

#### Scenario: 添加单个辅助标签
- **WHEN** 用户调用 POST /api/semantic-boards/:id/composition，body 含 auxiliary_label_id
- **THEN** 系统 SHALL 验证辅助标签存在且 active，写入 board_composition 记录（幂等），且不自动回填历史 tag-board 归属

#### Scenario: 添加后需要用户手动回填
- **WHEN** 用户向 SemanticBoard 添加辅助标签
- **THEN** 系统 SHALL NOT 自动启动回填；前端 SHOULD 提示用户可手动触发 board 模式回填使历史 tag-board 归属生效
