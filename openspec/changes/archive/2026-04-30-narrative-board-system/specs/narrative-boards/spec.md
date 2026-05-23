## ADDED Requirements

### Requirement: Board generation per feed category
The system SHALL generate narrative boards within each active feed category. When a category has more than 5 unclassified event tags for the target date, the system MUST call LLM Pass 0 to partition events into boards and assign abstract tags to boards. Each event tag MUST be assigned to exactly one board (prompt-enforced constraint).

#### Scenario: Category with many events generates boards
- **WHEN** a feed category has more than 5 unclassified event tags for the target date
- **THEN** the system calls LLM Pass 0 with event tags, abstract tag trees, and previous day's boards, and creates narrative_board records with scope=feed_category

#### Scenario: Category with few events skips boards
- **WHEN** a feed category has 5 or fewer unclassified event tags for the target date
- **THEN** the system skips board creation and directly generates narratives via LLM without board grouping

### Requirement: Board data model
The system SHALL persist boards in a `narrative_boards` table with fields: id, period_date, name, description, scope_type, scope_category_id, event_tag_ids (JSON), abstract_tag_ids (JSON), prev_board_ids (JSON), created_at.

#### Scenario: Board record created from LLM output
- **WHEN** LLM Pass 0 returns board definitions for a category
- **THEN** each board is saved as a narrative_boards record with all fields populated from LLM output

### Requirement: Abstract tag assignment to boards
LLM Pass 0 SHALL simultaneously assign abstract tag trees to boards. Each abstract tag MUST be assigned to at most one board per category.

#### Scenario: Abstract tags assigned alongside board partitioning
- **WHEN** LLM Pass 0 processes a category with both unclassified events and abstract tag trees
- **THEN** the LLM output includes abstract_tag_ids for each board, and each abstract tag appears in at most one board

### Requirement: Cross-day board matching via prev_board_ids
LLM Pass 0 SHALL receive previous day's boards (name, description, id) as context and annotate each new board with prev_board_ids indicating which yesterday's boards it continues from.

#### Scenario: Board continues from yesterday's matching board
- **WHEN** today's "地缘政治" board is generated and yesterday had a "地缘政治" board
- **THEN** LLM Pass 0 annotates today's board with prev_board_ids containing yesterday's board id

#### Scenario: Board splits from yesterday's board
- **WHEN** yesterday's "AI竞争" board's narratives split into today's "AI芯片" and "大模型" boards
- **THEN** both today's boards have prev_board_ids referencing yesterday's "AI竞争" board id

### Requirement: Narrative generation within boards
For each board, the system SHALL generate narratives via LLM Pass 1. The LLM receives: board's event tags with descriptions, abstract tag cards as context, and previous narratives from matched boards (via prev_board_ids). Boards within a category MAY be processed in parallel.

#### Scenario: Board with events and abstract tags generates narratives
- **WHEN** a board contains event_tag_ids and abstract_tag_ids
- **THEN** LLM Pass 1 generates narratives using event tags as primary input, abstract tag cards as context, and previous day's matching board narratives for cross-day linking

#### Scenario: Board narratives saved with board_id
- **WHEN** LLM Pass 1 returns narratives for a board
- **THEN** each narrative is saved to narrative_summaries with board_id referencing the parent board

### Requirement: Abstract tags mapped as narrative cards
Abstract tags assigned to a board SHALL be mapped directly as narrative cards (source='abstract') without additional LLM calls. The narrative card's title is the abstract tag's label, summary is the abstract tag's description, and related_tag_ids are the abstract tag's child tag IDs.

#### Scenario: Abstract tag becomes narrative card
- **WHEN** abstract tag 10 is assigned to board 5
- **THEN** a narrative_summaries record is created with board_id=5, source='abstract', title=abstract_tag.label, summary=abstract_tag.description, related_tag_ids=abstract_tag's children

### Requirement: Abstract tag card status computation
The system SHALL compute abstract tag narrative card status based on child tag article activity over the past 3 days. Status is: emerging (tag created within 1 day), continuing (child tags have ≥3 associated articles in window), ending (child tags have <3 articles and declining).

#### Scenario: Active abstract tag gets continuing status
- **WHEN** abstract tag's child tags have 5+ associated articles in the past 3 days
- **THEN** the narrative card status is set to 'continuing'

#### Scenario: Inactive abstract tag gets ending status
- **WHEN** abstract tag's child tags have fewer than 3 associated articles and article count is declining
- **THEN** the narrative card status is set to 'ending'

### Requirement: Global board merge across categories
After all per-category boards are generated, the system SHALL call LLM to identify and merge boards with the same topic across different feed categories. Merged boards get scope=global, and their narratives are combined under the global board.

#### Scenario: Same-topic boards across categories are merged
- **WHEN** "地缘政治" board exists in both "政治新闻" and "经济新闻" categories
- **THEN** LLM identifies them as the same topic, creates a global board, and reassigns both boards' narratives to the global board

#### Scenario: No matching boards across categories
- **WHEN** no boards share the same topic across categories
- **THEN** each per-category board becomes a standalone global board with scope=global

### Requirement: Cross-day board connection derivation
Board-to-board connections across days SHALL be derived from narrative parent_ids. If narrative X in board A (day N) has parent_ids pointing to narrative Y in board B (day N-1), then board A is connected to board B.

#### Scenario: Board connection from narrative lineage
- **WHEN** narrative X (board A, day 2) has parent_ids=[id_of_narrative_Y] and narrative Y belongs to board B (day 1)
- **THEN** board A is connected to board B with the connection status derived from the narratives' statuses

### Requirement: Fallback for failed narrative association
When a narrative's parent_ids reference a narrative not found in any matched board, the system SHALL retry association with a separate LLM call providing full context, up to 3 times maximum.

#### Scenario: Parent narrative not in matched board
- **WHEN** narrative X has parent_ids pointing to a narrative not in any board matched via prev_board_ids
- **THEN** the system calls LLM with full yesterday's narrative context to resolve the association, and retries up to 3 times if still unresolved

### Requirement: Remove legacy narrative generation paths
The system SHALL remove GenerateCrossCategoryNarratives, GenerateWatchedTagNarratives, and the old unclassified event direct narrative generation (old Pass 2). The new board-based flow replaces all of these.

#### Scenario: Legacy code paths removed
- **WHEN** the new board system is deployed
- **THEN** GenerateCrossCategoryNarratives, GenerateWatchedTagNarratives, and old Pass 2 code are no longer called
