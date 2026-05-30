## ADDED Requirements

### Requirement: Phase 1 — Zombie Tag cleanup
The system SHALL DELETE topic_tags that have no article associations (zero rows in article_topic_tags), no hierarchy relations (neither parent nor child in topic_tag_relations), and are older than 7 days. The system SHALL also DELETE corresponding topic_tag_embeddings.

#### Scenario: Zombie Tag deleted
- **WHEN** Tag "冷门术语" has 0 article associations, 0 relations, created 10 days ago
- **THEN** the system SHALL DELETE the Tag and its embeddings

#### Scenario: Recent Tag preserved
- **WHEN** Tag "新标签" has 0 article associations, 0 relations, created 2 days ago
- **THEN** the system SHALL NOT delete it

### Requirement: Phase 2 — Low quality Tag cleanup
The system SHALL DELETE topic_tags with quality_score < 0.15 AND article_count = 1 AND source IN ('llm', 'heuristic'). The system SHALL migrate article_topic_tags references, topic_tag_relations, and topic_tag_embeddings before deletion.

#### Scenario: Single-article low-quality Tag deleted
- **WHEN** Tag "某公司某日动态" has quality_score=0.08, article_count=1, source='llm'
- **THEN** the system SHALL DELETE the Tag and its article_topic_tags row

#### Scenario: Low-quality but multi-article Tag preserved
- **WHEN** Tag "某热门话题" has quality_score=0.10, article_count=5
- **THEN** the system SHALL NOT delete it

### Requirement: Phase 3 — Empty Node cleanup
The system SHALL DELETE topic_tags with source='abstract' that have zero child relations in topic_tag_relations (no child with relation_type='abstract'). The system SHALL also DELETE their topic_tag_embeddings. This is a hard DELETE, not a status change.

#### Scenario: Empty Node deleted
- **WHEN** Node "空分类" (source='abstract') has 0 children in topic_tag_relations
- **THEN** the system SHALL DELETE the Node and its embeddings

#### Scenario: Node with children preserved
- **WHEN** Node "AI产品发布" (source='abstract') has 3 children
- **THEN** the system SHALL NOT delete it

### Requirement: Phase 4 — Same-Level dedup with source DELETE
For each Sector, the system SHALL find Node pairs at the same Level within the Sector whose embedding cosine similarity exceeds 0.90. The system SHALL merge the lower-quality Node into the higher-quality one: migrate all article_topic_tags, topic_tag_relations (children), and topic_tag_embeddings to the surviving Node, then DELETE the source Node.

#### Scenario: Similar Nodes merged
- **WHEN** Node A (quality_score=0.8) and Node B (quality_score=0.5) are both at Level 1 in Sector "AI产品" with similarity 0.93
- **THEN** Node B's children SHALL be relinked to Node A, Node B's article_topic_tags SHALL be migrated to Node A, Node B SHALL be DELETED

#### Scenario: Cross-Sector Nodes not merged
- **WHEN** Node A is in Sector "AI产品" and Node B is in Sector "AI商业" with similarity 0.92
- **THEN** the system SHALL NOT merge them

### Requirement: Phase 5 — Template compliance check
The system SHALL verify all active hierarchy relations against the current HierarchyTemplate. Violations include: depth exceeding max_level, leaf Tags at non-leaf levels, abstract Tags at leaf levels, max_children exceeded. Violations SHALL be recorded as `hierarchy_pending_changes` entries (change_type, tag_id, current_parent_id, reason) with status='pending'.

#### Scenario: Depth violation detected
- **WHEN** Tag at depth 4 exists but template max_level=3 for its category
- **THEN** a hierarchy_pending_change SHALL be created with change_type='depth_exceeded' and reason describing the violation

#### Scenario: Leaf at non-leaf level
- **WHEN** a Tag with source='llm' exists at Level 1 but template defines Level 1 as non-leaf
- **THEN** a hierarchy_pending_change SHALL be created with change_type='level_mismatch'

### Requirement: Phase 6 — Sector health check
The system SHALL check Sector health based on source: auto-created Sectors with zero Tags → DELETE; LLM-created Sectors with ≥50% Tag count decline from peak → mark declining=true (do not delete); manual Sectors → no automated action.

#### Scenario: Auto Sector with Tags preserved
- **WHEN** auto Sector "AI开源" has 5 Tags
- **THEN** the system SHALL NOT delete it

### Requirement: Phase 7 — Template-constrained clustering
The system SHALL cluster unplaced Tags within each category using embedding similarity, but SHALL NOT directly create Nodes or modify topic_tag_relations. Clustering results SHALL be stored as anchor signals (which Tags should be grouped together) and fed into PlaceTagInHierarchy as input.

#### Scenario: Clustering produces anchor signals
- **WHEN** 10 unplaced event Tags are clustered into 3 groups
- **THEN** each group SHALL be recorded as an anchor signal with member Tag IDs, but no Node SHALL be created

#### Scenario: Anchor signals consumed by PlaceTagInHierarchy
- **WHEN** PlaceTagInHierarchy processes a Tag that belongs to a cluster anchor signal
- **THEN** it SHALL use the other cluster members as additional anchor context for parent resolution

### Requirement: Cleanup execution order
The cleanup scheduler SHALL execute phases in order: Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5 → Phase 6 → Phase 7. Each phase SHALL log its results (count of affected entities). The scheduler SHALL respect a time budget and skip remaining phases if budget is exhausted.

#### Scenario: All phases run in order
- **WHEN** cleanup scheduler starts with sufficient time budget
- **THEN** Phase 1 runs before Phase 2, Phase 2 before Phase 3, etc., and each logs its affected count

#### Scenario: Budget exhausted mid-run
- **WHEN** cleanup scheduler budget is exhausted during Phase 4
- **THEN** Phases 5, 6, 7 SHALL be skipped and a warning SHALL be logged
