## MODIFIED Requirements

### Requirement: Embedding-based tag-to-board matching
The system SHALL match eligible Tags and small abstract trees to active board concepts using cosine similarity of their embeddings against `board_concepts.embedding`. When a Tag matches a concept above the configured threshold, its `concept_id` SHALL be assigned to that concept. When no active concept exists, the system SHALL return a `no_active_sector` blocker instead of treating the Tag as permanently unclassified.

#### Scenario: Tag matches a board concept above threshold
- **WHEN** event tag "Claude Code静默截断故障" has embedding, and board_concept "AI工具实践" has embedding with cosine_similarity=0.85
- **THEN** the tag is assigned to board_concept "AI工具实践"

#### Scenario: Tag falls below threshold for all concepts
- **WHEN** event tag "某不知名标签" has embedding with max cosine_similarity=0.35 against all concepts
- **THEN** the tag remains unplaced with blocker reason `no_matching_sector`

#### Scenario: No active Sector exists
- **WHEN** a category has active Tags but zero active board concepts
- **THEN** matching SHALL return blocker reason `no_active_sector` and the orchestration flow SHALL be allowed to trigger Sector bootstrap

#### Scenario: Small abstract tree matched as a unit
- **WHEN** a small abstract tree with root label="LangGraph教程" and 3 child event tags is evaluated
- **THEN** the root tag's label+description embedding is used for matching; if matched, all child tags follow to the same board concept

### Requirement: Unclassified bucket
Tags that fail to match any board concept above the threshold SHALL be collected into an unplaced bucket visible in `/tags`. If the bucket size exceeds the configured auto Sector threshold, the hierarchy orchestration flow SHALL trigger LLM suggestion or auto Sector generation for new board concepts.

#### Scenario: Unclassified bucket triggers Sector bootstrap
- **WHEN** after matching, 20 Tags remain unplaced and auto_sector_threshold is 15
- **THEN** the orchestration flow SHALL invoke auto Sector generation based on the unplaced Tags' labels and descriptions

#### Scenario: Small unclassified bucket remains visible
- **WHEN** after matching, 2 Tags remain unplaced
- **THEN** no automatic Sector generation SHALL run, and the Tags SHALL remain visible in the `/tags` unplaced section with blocker counts
