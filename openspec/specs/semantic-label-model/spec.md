## Purpose

semantic_labels 统一数据模型及关联表定义，用于存储辅助标签和 SemanticBoard。

## Requirements

### Requirement: semantic_labels 统一数据模型
系统 SHALL 使用 `semantic_labels` 表统一存储辅助标签、SemanticBoard 和组合标签。表 SHALL 包含以下字段：id, label, slug, embedding (vector, storage embedding), merge_embedding (vector, label-only merge embedding), label_type ("auxiliary"|"board"|"composite"), aliases (jsonb), ref_count, description, display_order, source, status, protected, created_at, updated_at。SemanticBoard SHALL 是全局共享的长期语义板块，不按 tag category 或 feed category 分表。组合标签的行为契约（embedding 生成、去重 canonical 化、治理操作）见 `composite-label` capability。

#### Scenario: 辅助标签写入
- **WHEN** 新辅助标签 "量子计算" 入库（L3 新建）
- **THEN** 创建 semantic_labels 记录，label_type="auxiliary", source="llm_extract", status="active"，merge_embedding 由 label 生成，embedding 由 label + description 生成

#### Scenario: 板块创建
- **WHEN** LLM 从辅助标签簇中生成新板块 "AI与机器学习"
- **THEN** 创建 semantic_labels 记录，label_type="board", source="llm_suggest", status="active", description 为 LLM 生成的描述

#### Scenario: 组合标签写入
- **WHEN** compose 建议确认创建组合标签 "美债收益率"
- **THEN** 创建 semantic_labels 记录，label_type="composite", source="upgrade_suggest", status="active"，embedding 由 LLM 对组合短语生成，merge_embedding 不使用

#### Scenario: SemanticBoard 全局共享
- **WHEN** 不同 feed category 下的 tag 都匹配到 SemanticBoard "AI与机器学习"
- **THEN** 系统 SHALL 复用同一条 label_type="board" 的 semantic_labels 记录

### Requirement: topic_tag_semantic_labels 关联表
系统 SHALL 使用 `topic_tag_semantic_labels` 关联表记录 tag 和辅助标签的多对多关系。表 SHALL 包含 topic_tag_id 和 semantic_label_id 字段，semantic_label_id SHALL 指向 label_type="auxiliary" 的 semantic_labels。

#### Scenario: Tag 关联辅助标签
- **WHEN** tag "happyhorse" 的辅助标签 "AI" 入库后关联到 semantic_label #42
- **THEN** 创建关联记录 (topic_tag_id=tag.id, semantic_label_id=42)

### Requirement: board_composition 构成关系
系统 SHALL 使用 `board_composition` 关联表记录每个 board 由哪些辅助标签组成。表 SHALL 包含 board_id 和 auxiliary_label_id 字段。

#### Scenario: Board 从辅助标签簇生成
- **WHEN** LLM 从簇 [AI, 大语言模型, GPT] 生成新 board "AI与机器学习"
- **THEN** 为该 board 创建 3 条 board_composition 记录，分别指向这 3 个辅助标签

### Requirement: topic_tag_board_labels 持久化 Tag-Board 归属
系统 SHALL 使用 `topic_tag_board_labels` 关联表记录 tag 和 SemanticBoard 的多对多匹配结果。表 SHALL 包含 topic_tag_id, semantic_board_id, score, match_reason, created_at, updated_at 字段，semantic_board_id SHALL 指向 label_type="board" 的 semantic_labels。

#### Scenario: Tag 关联多个 SemanticBoard
- **WHEN** tag "霍尔木兹海峡" 同时匹配 SemanticBoard "地缘政治" 和 "能源安全"
- **THEN** 系统 SHALL 创建两条 topic_tag_board_labels 记录，分别记录匹配分和匹配原因

#### Scenario: 回填重算覆盖旧归属
- **WHEN** 回填任务重算 tag #10 的 board 归属
- **THEN** 系统 SHALL 用新匹配结果替换 tag #10 在 topic_tag_board_labels 中的旧归属

### Requirement: ref_count 自动维护
系统 SHALL 在 tag 关联或取消关联辅助标签时，自动增减对应辅助标签 semantic_label 的 ref_count。系统 SHALL 在 topic_tag 被删除（含 CleanupOrphanedTags 和 HardMergeTags）时，重新计算受影响辅助标签的 ref_count，确保 ref_count 始终反映实际的 topic_tag_semantic_labels 关联数量。SemanticBoard 的 tag_count SHALL 从 topic_tag_board_labels 聚合计算，不复用 ref_count。

#### Scenario: 新 tag 关联辅助标签
- **WHEN** tag 关联到 semantic_label "AI"（当前 ref_count=10）
- **THEN** ref_count 更新为 11

#### Scenario: topic_tag 被删除后 ref_count 重算
- **WHEN** orphan topic_tag #42（关联 aux label #7 和 #8）被 CleanupOrphanedTags 硬删除
- **THEN** aux label #7 和 #8 的 ref_count SHALL 重新计算为 topic_tag_semantic_labels 中实际剩余的行数

#### Scenario: HardMerge source tag 删除后 ref_count 重算
- **WHEN** HardMergeTags 删除 source tag（关联 aux label #3），将其文章迁移到 target tag
- **THEN** aux label #3 的 ref_count SHALL 重新计算为当前 topic_tag_semantic_labels 中实际关联数

#### Scenario: 存量 ref_count 校准
- **WHEN** 执行一次性存量校准脚本
- **THEN** 所有 label_type='auxiliary' 的 semantic_labels.ref_count SHALL 更新为 topic_tag_semantic_labels 中对应的实际计数

### Requirement: 辅助标签双 embedding 字段用途隔离
系统 SHALL 使用 merge_embedding 执行辅助标签 L2 merge 判断，使用 embedding 执行 SemanticBoard 推荐、Tag-Board 匹配、升级聚类和回填。系统 SHALL NOT 使用 storage embedding 做 L2 merge 判断，也 SHALL NOT 使用 merge_embedding 做 board 匹配。

#### Scenario: L2 merge 使用 merge_embedding
- **WHEN** 新辅助标签 "AI绘图" 与已有辅助标签比较是否 merge
- **THEN** 系统 SHALL 使用双方的 merge_embedding 计算 cosine similarity

#### Scenario: Board 匹配使用 storage embedding
- **WHEN** 系统计算 tag 辅助标签与 SemanticBoard composition 的间接匹配
- **THEN** 系统 SHALL 使用 semantic_labels.embedding（storage embedding）计算相似度

### Requirement: 删除旧概念和层级字段
系统 SHALL 删除旧 `board_concepts` 表、`topic_tags.concept_id` 字段，以及层级体系相关表/字段。系统 SHALL 不提供旧数据自动迁移。

#### Scenario: 用户手动清空旧数据后重建
- **WHEN** 新模型迁移完成且用户已手动删除旧数据
- **THEN** 系统 SHALL 仅依赖 semantic_labels、topic_tag_semantic_labels、topic_tag_board_labels、board_composition 进行板块匹配

### Requirement: disabled 标签不保留向量

`semantic_labels` 中 status='disabled' 的行 MUST 将 embedding 与 merge_embedding 置为 NULL（行本体与 aliases 保留）；重新启用时由 llm_extract 重算向量。禁用流程的代码路径 MUST 同步置 NULL，防止新增 disabled 行继续积累向量。

#### Scenario: 禁用标签时向量置 NULL
- **WHEN** 通过 API 或流程将一个 semantic_label 置为 disabled
- **THEN** 该行的 embedding 与 merge_embedding 被置 NULL，向量存储立即释放

#### Scenario: 存量 disabled 行向量清理
- **WHEN** 执行一次性存量清理（分批 UPDATE）
- **THEN** 所有 status='disabled' 且向量非 NULL 的行向量被置 NULL，行数据与 aliases 不受影响

#### Scenario: 重新启用标签
- **WHEN** 一个 disabled 标签被重新启用
- **THEN** 其向量通过 llm_extract 流程重新生成，语义功能恢复

### Requirement: composite_components 组件引用表
系统 SHALL 使用 `composite_components` 关联表存储组合标签的有序组件引用，表 SHALL 包含 composite_id（指向 label_type="composite" 的 semantic_labels）、component_label_id（指向 label_type="auxiliary" 的 semantic_labels）、position（组件顺序）。组件的删除级联与约束细节由 composite-label capability 定义。

#### Scenario: 组合标签组件写入
- **WHEN** 组合标签 "美债收益率" 创建（组件：美国国债、收益率）
- **THEN** 创建 2 条 composite_components 记录，position 分别为 1、2

#### Scenario: 组合标签删除时组件级联
- **WHEN** 组合标签行被删除
- **THEN** 其 composite_components 记录 SHALL 级联删除
