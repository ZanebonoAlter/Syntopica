## MODIFIED Requirements

### Requirement: Abstract tag creation requires minimum information gain
Source A（findOrCreateTag）SHALL NOT 创建 abstract 标签。HasAbstract() 路径、createChildOfAbstract 函数、abstract co-tag 扩展 SHALL 被删除。Source A 只保留 merge 和 create new tag 能力。

#### Scenario: Source A 不再创建 abstract
- **WHEN** LLM 在 findOrCreateTag 中返回 HasAbstract()=true
- **THEN** 系统忽略 abstract 建议，只处理 HasMerge() 部分

### Requirement: Degenerate abstract trees are flattened
ReviewHierarchyTrees（Source C）SHALL 只做 merge/move/复用已有 abstract，SHALL NOT 创建新的 abstract 标签。

#### Scenario: Source C 复用已有 abstract
- **WHEN** ReviewHierarchyTrees 的 LLM 建议使用已有 abstract
- **THEN** 系统执行 attachChildrenToReviewAbstract 复用该 abstract

#### Scenario: Source C 忽略新建 abstract 建议
- **WHEN** ReviewHierarchyTrees 的 LLM 建议创建新 abstract
- **THEN** 系统忽略该建议，不调用 createAbstractTagDirectly

### Requirement: Existing whitespace-variant duplicate tags are cleaned up
此需求保持不变，但 cleanup scheduler 中的 depth 检查 SHALL 改用 getMaxDepthForCategory(category) 替代 maxHierarchyDepth 常量。

#### Scenario: Depth 检查使用 per-template 上界
- **WHEN** CleanupTemplateViolations 检查 person 标签深度
- **THEN** 使用 getMaxDepthForCategory('person')=1 而非 maxHierarchyDepth=4

## ADDED Requirements

### Requirement: Concept-aware dedup
dedupAtDepth SHALL 优先合并同 concept 内的重复 abstract，跨 concept 不执行 dedup。

#### Scenario: 同 concept 内 dedup
- **WHEN** abstract A 和 abstract B 都属于 concept C 且 cosine similarity ≥ 0.95
- **THEN** 执行 MergeTags

#### Scenario: 跨 concept 不 dedup
- **WHEN** abstract A 属于 concept C1，abstract B 属于 concept C2
- **THEN** 不执行 dedup，即使 cosine similarity ≥ 0.95
