## ADDED Requirements

### Requirement: semantic_labels 统一数据模型
系统 SHALL 使用 `semantic_labels` 表统一存储辅助标签和 SemanticBoard。表 SHALL 包含以下字段：id, label, slug, embedding (vector, storage embedding), merge_embedding (vector, label-only merge embedding), label_type ("auxiliary"|"board"), aliases (jsonb), ref_count, description, display_order, source, status, protected, created_at, updated_at。SemanticBoard SHALL 是全局共享的长期语义板块，不按 tag category 或 feed category 分表。

#### Scenario: 辅助标签写入
- **WHEN** 新辅助标签 "量子计算" 入库（L3 新建）
- **THEN** 创建 semantic_labels 记录，label_type="auxiliary", source="llm_extract", status="active"，merge_embedding 由 label 生成，embedding 由 label + description 生成

#### Scenario: 板块创建
- **WHEN** LLM 从辅助标签簇中生成新板块 "AI与机器学习"
- **THEN** 创建 semantic_labels 记录，label_type="board", source="llm_suggest", status="active", description 为 LLM 生成的描述

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
系统 SHALL 在 tag 关联或取消关联辅助标签时，自动增减对应辅助标签 semantic_label 的 ref_count。SemanticBoard 的 tag_count SHALL 从 topic_tag_board_labels 聚合计算，不复用 ref_count。

#### Scenario: 新 tag 关联辅助标签
- **WHEN** tag 关联到 semantic_label "AI"（当前 ref_count=10）
- **THEN** ref_count 更新为 11

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
