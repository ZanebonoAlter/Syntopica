## MODIFIED Requirements

### Requirement: Abstract tree deterministically maps to a Board
The system SHALL classify each active abstract tree by its node count against a configurable threshold N (default=6). Trees with node count ≥ N SHALL each create a standalone "daily hotspot" NarrativeBoard (see `daily-hotspot-board` spec). Trees with node count < N SHALL be routed to embedding-based Board Concept matching (see `tag-to-board-matching` spec).

#### Scenario: Large tree creates hotspot board
- **WHEN** on 2026-05-01, abstract tree "AI 行业动态" has 20 child event tags active
- **THEN** a NarrativeBoard is created with name="AI 行业动态", is_system=true, without a board_concept_id

#### Scenario: Small tree routed to matching
- **WHEN** on 2026-05-01, abstract tree "LangGraph 教程" has 3 child event tags active
- **THEN** no Board is created from this tree alone; instead the root tag's embedding is matched against board_concepts

### Requirement: Misc events partitioned into Board Concepts
The system SHALL collect event tags not belonging to any abstract tree and match them to Board Concepts via embedding cosine similarity. Tags that match a concept above the configurable threshold SHALL be assigned to that concept's daily NarrativeBoard. Tags below the threshold SHALL be placed in the "unclassified" bucket.

#### Scenario: Misc events matched to concept
- **WHEN** 3 unclassified event tags are matched to board_concept "AI 工具实践" above threshold
- **THEN** a single NarrativeBoard linked to that concept is created containing all 3 events

#### Scenario: Misc events fall below threshold
- **WHEN** 2 unclassified event tags have no concept match above threshold
- **THEN** the tags are collected in the unclassified bucket and shown in UI

#### Scenario: Zero unclassified events
- **WHEN** all event tags belong to abstract trees
- **THEN** no unclassified bucket is created

### Requirement: Board narrative generation bound to concept
For each Board (hotspot or concept-matched), the system SHALL invoke LLM to generate narratives. The Board's `name` and `description` from the concept (or abstract tag for hotspots) SHALL be provided as context to LLM.

#### Scenario: Concept Board narrative generation
- **WHEN** Board linked to concept "AI 工具实践" has 6 matched event tags
- **THEN** LLM receives concept name and description as context, generating 1-N narratives

#### Scenario: Hotspot Board narrative generation
- **WHEN** Board "AI 行业动态" (hotspot) has 20 event tags
- **THEN** LLM receives the abstract tag's label and description as context, plus the full tree structure

### Requirement: Prev board matching for concept boards
Concept-linked Boards SHALL match yesterday's Boards by `board_concept_id`. Hotspot Boards SHALL match by `abstract_tag_id` (existing logic preserved).

#### Scenario: Same concept on consecutive days
- **WHEN** today has Board with board_concept_id=5, and yesterday has Board with board_concept_id=5 and id=30
- **THEN** today's Board.prev_board_ids includes 30

#### Scenario: First day for a concept
- **WHEN** today is the first day a concept has matched tags
- **THEN** Board.prev_board_ids is empty

### Requirement: All narratives belong to a Board
Every NarrativeSummary generated SHALL have a non-null `board_id`. There SHALL be no narratives created without Board affiliation. The `narrative_boards` table SHALL include `board_concept_id` (nullable, populated for concept-matched boards).

#### Scenario: Hotspot narrative saved with board_id
- **WHEN** LLM generates a narrative for hotspot Board "AI 行业动态" (id=10)
- **THEN** the NarrativeSummary record has board_id=10, and the Board's board_concept_id is NULL

#### Scenario: Concept narrative saved with board_id
- **WHEN** LLM generates a narrative for concept Board (id=20, board_concept_id=5)
- **THEN** the NarrativeSummary record has board_id=20, and the Board's board_concept_id=5

## REMOVED Requirements

### Requirement: Remove abstract card narratives
**Reason**: This requirement was already implemented in the prior version and is preserved in the new architecture. The abstract card concept was replaced by Board metadata.

**Migration**: No migration needed — this change was already applied.

### Requirement: Misc events partitioned by LLM (old)
**Reason**: The old misc event partitioning via LLM (used when >3 unclassified events existed) is replaced by embedding-based matching to Board Concepts.

**Migration**: The `createMiscBoardsFromEvents` function in `board_creation.go` is no longer called. Unclassified events are routed through `tag-to-board-matching` instead.

### Requirement: Global board merge
**Reason**: The global merge process (`MergeGlobalBoards` in `board_merge.go`) that combined category-level boards into global boards is replaced by embedding matching. Board Concepts serve as the unification layer across categories.

**Migration**: The `board_merge.go` file is no longer invoked from `GenerateAndSave`. Global boards are now concept boards with `scope_type='global'`.
