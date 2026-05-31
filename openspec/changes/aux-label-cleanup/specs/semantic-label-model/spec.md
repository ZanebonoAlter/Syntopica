## MODIFIED Requirements

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
