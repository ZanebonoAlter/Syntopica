## ADDED Requirements

### Requirement: Embedding-based tag-to-board matching
The system SHALL match small abstract trees (node count < N, default N=6) and unclassified event tags to board concepts using cosine similarity of their embeddings against `board_concepts.embedding`.

#### Scenario: Tag matches a board concept above threshold
- **WHEN** event tag "Claude Code静默截断故障" has embedding, and board_concept "AI工具实践" has embedding with cosine_similarity=0.85
- **THEN** the tag is assigned to board_concept "AI工具实践"

#### Scenario: Tag falls below threshold for all concepts
- **WHEN** event tag "某不知名标签" has embedding with max cosine_similarity=0.35 against all concepts
- **THEN** the tag is placed in the "unclassified" bucket

#### Scenario: Small abstract tree matched as a unit
- **WHEN** a small abstract tree with root label="LangGraph教程" and 3 child event tags is evaluated
- **THEN** the root tag's label+description embedding is used for matching; if matched, all child tags follow to the same board concept

### Requirement: Configurable matching threshold
The system SHALL read the embedding matching threshold from `ai_settings` table with key `narrative_board_embedding_threshold`. The default value SHALL be 0.7.

#### Scenario: Default threshold used
- **WHEN** ai_settings has no entry for narrative_board_embedding_threshold
- **THEN** the threshold defaults to 0.7

#### Scenario: Custom threshold applied
- **WHEN** ai_settings has narrative_board_embedding_threshold=0.6
- **THEN** tags with cosine_similarity ≥ 0.6 are matched

### Requirement: Unclassified bucket
Tags that fail to match any board concept above the threshold SHALL be collected into an "unclassified" bucket. If the bucket size exceeds 5 items, the system SHALL trigger an LLM suggestion for new board concepts.

#### Scenario: Unclassified bucket triggers suggestion
- **WHEN** after matching, 8 tags remain unclassified
- **THEN** LLM is invoked to suggest new board concepts based on the unclassified tags' labels and descriptions

#### Scenario: Small unclassified bucket
- **WHEN** after matching, 2 tags remain unclassified
- **THEN** no LLM suggestion is triggered; tags remain in the unclassified state visible in UI

### Requirement: Matching precedence
When both a small abstract tree and its descendant event tags could match different concepts, the system SHALL use the abstract tree's root label embedding for matching and assign all descendants to the same concept.

#### Scenario: Tree takes precedence over individual tags
- **WHEN** abstract tree "AI 安全" has 3 child event tags, and the tree-level embedding matches concept "安全与合规"
- **THEN** all 3 child event tags are assigned to "安全与合规" regardless of individual tag embeddings
