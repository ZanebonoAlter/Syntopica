## MODIFIED Requirements

### Requirement: Slug generation normalizes whitespace
The `Slugify` function SHALL collapse consecutive whitespace characters into a single space before applying any other transformations, so that labels differing only in whitespace (e.g., "DeepSeek首轮融资" and "DeepSeek 首轮融资") produce the same slug.

#### Scenario: Labels with space versus no-space produce same slug
- **WHEN** `Slugify("DeepSeek首轮融资")` and `Slugify("DeepSeek 首轮融资")` are called
- **THEN** both SHALL return `deepseek 首轮融资`

#### Scenario: Multiple consecutive spaces collapse to single space
- **WHEN** `Slugify("foo   bar")` is called
- **THEN** the result SHALL be `foo bar`

#### Scenario: Leading and trailing whitespace trimmed
- **WHEN** `Slugify("  hello world  ")` is called
- **THEN** the result SHALL be `hello world` (spaces preserved between words, punctuation removed)

### Requirement: Abstract tag creation requires minimum information gain
Before creating a new abstract Node, the system SHALL verify that: (1) at least 2 distinct child Tags are proposed, (2) no pair of proposed child Tags shares more than 70% of their associated articles (Jaccard similarity), (3) the resulting tree SHALL have a leaf-to-depth ratio of at least 1.5. If any check fails, the Node creation SHALL be rejected and logged. This requirement applies to PlaceTagInHierarchy as the sole Node creation entry point.

#### Scenario: Too few children rejected
- **WHEN** PlaceTagInHierarchy proposes creating a Node with only 1 candidate child
- **THEN** the system SHALL reject creation and log a warning

#### Scenario: High article overlap rejected
- **WHEN** two candidate child Tags share 80% of their associated articles
- **THEN** the system SHALL reject Node creation and log a warning suggesting merge instead

#### Scenario: Acceptable Node passes all checks
- **WHEN** PlaceTagInHierarchy proposes a Node with 3 children whose maximum pairwise Jaccard is 0.3 and the resulting tree leaf-to-depth ratio is 2.0
- **THEN** the system SHALL proceed with Node creation

### Requirement: Tag/Node merge performs source DELETE
When Tags or Nodes are merged, the system SHALL: (1) UPDATE article_topic_tags SET topic_tag_id = target_id WHERE topic_tag_id = source_id, (2) UPDATE topic_tag_relations SET parent_id = target_id WHERE parent_id = source_id, (3) UPDATE topic_tag_relations SET child_id = target_id WHERE child_id = source_id (skip if creates cycle), (4) DELETE FROM topic_tag_embeddings WHERE topic_tag_id = source_id, (5) DELETE FROM topic_tags WHERE id = source_id. The source entity SHALL NOT be retained with status='merged'.

#### Scenario: Merge deletes source Tag
- **WHEN** Tag "DeepSeek首轮融资" is merged into Tag "DeepSeek 首轮融资"
- **THEN** all article_topic_tags referencing the source SHALL point to the target, and the source Tag row SHALL be DELETED from topic_tags

#### Scenario: Merge deletes source Node
- **WHEN** Node "AI产品" is merged into Node "AI产品与工具"
- **THEN** all children of "AI产品" SHALL be relinked to "AI产品与工具", all article_topic_tags SHALL be migrated, and "AI产品" SHALL be DELETED from topic_tags

### Requirement: Degenerate abstract trees are flattened
The system SHALL detect Node chains where the leaf-to-depth ratio falls below 1.5 and flatten them by removing intermediate Nodes (DELETE, not deactive), relinking children to the nearest ancestor that provides meaningful grouping.

#### Scenario: Four-level chain with 5 leaves flattened
- **WHEN** cleanup encounters A→B→C→D with D's children being 5 leaf Tags
- **THEN** intermediate Nodes B and C SHALL be DELETED and their children linked directly to A

### Requirement: Multi-parent assignments are prevented
When linking a child Tag to a new abstract parent via `linkTagToParent()`, if the child already has an active abstract parent and the new parent's article set overlaps with the existing parent's article set (Jaccard > 0.3), the second parent assignment SHALL be rejected.

#### Scenario: Second parent rejected due to overlap
- **WHEN** Tag X has parent A covering articles {1,2,3} and a new parent B covering articles {1,2,4} is proposed (Jaccard = 0.5)
- **THEN** the system SHALL reject assigning B as parent and log the conflict

#### Scenario: Second parent accepted for non-overlapping sets
- **WHEN** Tag X has parent A covering articles {1,2} and a new parent B covering articles {3,4,5} is proposed (Jaccard = 0)
- **THEN** the system SHALL accept B as an additional parent
