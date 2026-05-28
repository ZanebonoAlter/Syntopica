# Tag Hierarchy Quality

## Purpose

TBD

## Requirements

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
Before creating a new abstract tag, the system SHALL verify that:
1. At least 2 distinct child tags are proposed
2. No pair of proposed child tags shares more than 70% of their associated articles (Jaccard similarity)
3. The resulting tree SHALL have a leaf-to-depth ratio of at least 1.5

If any of these checks fail, the abstract tag creation SHALL be rejected and logged.

#### Scenario: Too few children rejected
- **WHEN** LLM proposes an abstract tag with only 1 candidate child
- **THEN** the system SHALL reject creation and log a warning

#### Scenario: High article overlap rejected
- **WHEN** two candidate child tags share 80% of their associated articles
- **THEN** the system SHALL reject abstract creation and log a warning suggesting merge instead

#### Scenario: Acceptable abstract passes all checks
- **WHEN** LLM proposes an abstract with 3 children whose maximum pairwise Jaccard is 0.3 and the resulting tree leaf-to-depth ratio is 2.0
- **THEN** the system SHALL proceed with abstract tag creation

### Requirement: Existing whitespace-variant duplicate tags are cleaned up
A scheduled cleanup task SHALL detect active tags whose slugs would be identical after whitespace normalization (`\s+` → single space) and merge them, migrating article associations, embeddings, and hierarchy relations to the surviving tag.

#### Scenario: Whitespace variant pair merged
- **WHEN** cleanup runs and finds two active tags "DeepSeek首轮融资" (slug: `deepseek首轮融资`) and "DeepSeek 首轮融资" (slug: `deepseek 首轮融资`) that are the same after normalization
- **THEN** the tag with fewer article associations SHALL be merged into the other (marking it `status=merged`, `merged_into_id` set), all article_topic_tags SHALL be migrated, and duplicate article-tag pairs SHALL be skipped

### Requirement: Degenerate abstract trees are flattened
A scheduled cleanup task SHALL detect abstract tag chains where the leaf-to-depth ratio falls below 1.5 and flatten them by removing intermediate abstract nodes, relinking children to the nearest ancestor that provides meaningful grouping.

#### Scenario: Four-level chain with 5 leaves flattened
- **WHEN** cleanup encounters A→B→C→D with D's children being 5 leaf tags
- **THEN** intermediate nodes B and C SHALL be deactivated and their children linked directly to A

### Requirement: Multi-parent assignments are prevented
When linking a child tag to a new abstract parent via `linkAbstractParentChild()`, if the child already has an active abstract parent and the new parent's article set overlaps with the existing parent's article set (Jaccard > 0.3), the second parent assignment SHALL be rejected.

#### Scenario: Second parent rejected due to overlap
- **WHEN** tag X has parent A covering articles {1,2,3} and a new parent B covering articles {1,2,4} is proposed (Jaccard = 0.5)
- **THEN** the system SHALL reject assigning B as parent and log the conflict

#### Scenario: Second parent accepted for non-overlapping sets
- **WHEN** tag X has parent A covering articles {1,2} and a new parent B covering articles {3,4,5} is proposed (Jaccard = 0)
- **THEN** the system SHALL accept B as an additional parent
