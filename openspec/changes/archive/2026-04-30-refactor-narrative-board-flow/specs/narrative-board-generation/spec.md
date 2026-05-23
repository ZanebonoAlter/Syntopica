## ADDED Requirements

### Requirement: Abstract tree deterministically maps to a Board
The system SHALL create exactly one NarrativeBoard for each abstract tag tree that has active event tags on the target date. The Board's `name` SHALL equal the abstract tag's `label`, and `abstract_tag_id` SHALL reference the root abstract tag's ID.

#### Scenario: Single abstract tree with active events
- **WHEN** on 2026-05-01, category "科技" has abstract tree "AI 监管" (root id=101) with 3 child event tags active
- **THEN** exactly one NarrativeBoard is created with name="AI 监管", abstract_tag_id=101, event_tag_ids=[201,202,203]

#### Scenario: Multiple abstract trees in one category
- **WHEN** category "科技" has 3 active abstract trees on the target date
- **THEN** 3 Boards are created, each with distinct abstract_tag_id

#### Scenario: Abstract tree with zero active events
- **WHEN** abstract tree "AI 监管" has no child event tags with articles on the target date
- **THEN** no Board is created for that tree

### Requirement: Misc events partitioned into Boards
The system SHALL collect event tags not belonging to any abstract tree and group them into misc Boards. If the count is ≤3, one misc Board named "其他动态" is created without LLM invocation. If >3, LLM partitions them into N named Boards.

#### Scenario: Three or fewer misc events
- **WHEN** 2 unclassified event tags remain after abstract tree mapping
- **THEN** a single misc Board "其他动态" is created containing both events, with abstract_tag_id=NULL, without any LLM call

#### Scenario: More than three misc events
- **WHEN** 12 unclassified event tags remain
- **THEN** LLM is called once to partition them into Boards, each with distinct name and event_tag_ids, all with abstract_tag_id=NULL

#### Scenario: Zero misc events
- **WHEN** all event tags belong to abstract trees
- **THEN** no misc Board is created

### Requirement: Board narrative generation
For each Board (abstract tree or misc), the system SHALL invoke LLM to generate narratives. Input includes the Board's event tags, abstract tree structure (if applicable), and yesterday's related narratives for continuation context.

#### Scenario: Abstract tree Board narrative generation
- **WHEN** Board "AI 监管" has 4 child event tags and 2 keyword/person tags from its tree
- **THEN** LLM receives the full tree structure as context and generates 1-N narratives within that Board

#### Scenario: Misc Board narrative generation
- **WHEN** misc Board "其他·半导体供应链" has 5 unnamed event tags
- **THEN** LLM receives only event tags and generates 1-N narratives within that Board

### Requirement: Prev board matching by abstract tag ID
The system SHALL deterministically match today's Boards to yesterday's Boards using `abstract_tag_id`. If a Board's `abstract_tag_id` matches a yesterday Board's `abstract_tag_id`, the yesterday Board's ID is set as `prev_board_id`.

#### Scenario: Same abstract tag on consecutive days
- **WHEN** today has Board with abstract_tag_id=101, and yesterday has Board with abstract_tag_id=101 and id=50
- **THEN** today's Board.prev_board_ids includes 50

#### Scenario: New abstract tag on first day
- **WHEN** today is the first day an abstract tag with id=101 appears
- **THEN** Board.prev_board_ids is empty

#### Scenario: Misc Board prev matching
- **WHEN** today's misc Board "其他·半导体供应链" matches yesterday's misc Board of the same name
- **THEN** yesterday's misc Board ID is set as prev_board_id

### Requirement: All narratives belong to a Board
Every NarrativeSummary generated SHALL have a non-null `board_id`. There SHALL be no narratives created without Board affiliation.

#### Scenario: Narrative saved with board_id
- **WHEN** LLM generates a narrative for Board "AI 监管" (id=10)
- **THEN** the NarrativeSummary record has board_id=10

### Requirement: Remove abstract card narratives
The system SHALL NOT generate `source="abstract"` narratives. Board metadata (name, description) replaces the abstract card concept.

#### Scenario: No abstract cards generated
- **WHEN** Boards are created for an abstract tree
- **THEN** no separate NarrativeSummary with source="abstract" is inserted
