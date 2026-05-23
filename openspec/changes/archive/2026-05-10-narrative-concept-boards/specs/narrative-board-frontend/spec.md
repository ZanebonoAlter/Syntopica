## ADDED Requirements

### Requirement: Board concept list view
The NarrativePanel SHALL display board concepts as grouped entities distinct from daily hotspot boards. Concept boards SHALL render with their persistent name and description; hotspot boards SHALL render with their abstract tag label and an "is_system" indicator.

#### Scenario: Concept board group header
- **WHEN** board timeline data includes concept-linked boards
- **THEN** the concept name and description are shown as a group header in the canvas, with matched narratives inside

#### Scenario: Hotspot board distinct styling
- **WHEN** a board has is_system=true (hotspot)
- **THEN** the board renders with a visual indicator (e.g., "热点" badge) distinguishing it from concept boards

### Requirement: Board concept management UI
The system SHALL provide a UI for viewing and managing board concepts: listing all concepts, accepting/rejecting LLM suggestions, and deactivating unused concepts.

#### Scenario: Board concept management panel
- **WHEN** user opens board concept management
- **THEN** a list of active concepts is shown with name, description, and match count

#### Scenario: LLM suggestions for review
- **WHEN** LLM suggests new board concepts
- **THEN** suggestions are shown in a "pending review" section; user can accept or reject each

#### Scenario: Deactivate unused concept
- **WHEN** user clicks deactivate on concept "弃用板块"
- **THEN** the concept is set to is_active=false and no longer appears in matching or display

### Requirement: Unclassified tags display
The system SHALL display unclassified tags (those below the matching threshold) in the NarrativePanel as a distinct section, allowing users to manually assign them to existing concepts or create new concepts.

#### Scenario: Unclassified section visible
- **WHEN** there are unclassified tags after the daily matching run
- **THEN** an "未归类" section appears in the NarrativePanel showing the tags with their labels

#### Scenario: Manual tag-to-concept assignment
- **WHEN** user drags or selects an unclassified tag and assigns it to a concept
- **THEN** the tag is moved to that concept's board, and the board's narratives are regenerated
