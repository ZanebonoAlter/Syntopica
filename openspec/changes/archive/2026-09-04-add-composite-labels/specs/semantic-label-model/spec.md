## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: composite_components 组件引用表
系统 SHALL 使用 `composite_components` 关联表存储组合标签的有序组件引用，表 SHALL 包含 composite_id（指向 label_type="composite" 的 semantic_labels）、component_label_id（指向 label_type="auxiliary" 的 semantic_labels）、position（组件顺序）。组件的删除级联与约束细节由 composite-label capability 定义。

#### Scenario: 组合标签组件写入
- **WHEN** 组合标签 "美债收益率" 创建（组件：美国国债、收益率）
- **THEN** 创建 2 条 composite_components 记录，position 分别为 1、2

#### Scenario: 组合标签删除时组件级联
- **WHEN** 组合标签行被删除
- **THEN** 其 composite_components 记录 SHALL 级联删除
