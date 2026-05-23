## Purpose

Frontend UI for the narrative board view — the primary navigation and interaction surface for exploring narrative summaries organized by Board structures.

## Requirements

### Requirement: Single board-only navigation mode
The NarrativePanel SHALL only render Board-based navigation. There SHALL be no legacy timeline mode toggle. The component SHALL only use `NarrativeBoardCanvas` for rendering.

#### Scenario: No timeline mode toggle visible
- **WHEN** user opens the narrative panel
- **THEN** only a "版块" label is shown in the header; no "时间线" toggle button exists

#### Scenario: Board canvas always renders
- **WHEN** board timeline data is loaded
- **THEN** `NarrativeBoardCanvas` is rendered with the board timeline days

### Requirement: Unified scope switching
The NarrativePanel SHALL use a single `scopeMode` state (`'global'` | `'category'`) with one `selectedCategoryId`. Switching scope SHALL trigger a single API call to `getBoardTimeline`.

#### Scenario: Switching from global to category view
- **WHEN** user clicks "分类版块"
- **THEN** category list is loaded from `/api/narratives/scopes` and displayed

#### Scenario: Selecting a category
- **WHEN** user clicks category "科技"
- **THEN** `getBoardTimeline(date, 7, 'feed_category', techCategoryId)` is called, Boards for that category render

#### Scenario: Back to global
- **WHEN** user clicks "返回" in category detail view
- **THEN** `getBoardTimeline(date, 7)` is called without scope filter, all global Boards render

### Requirement: Three-level navigation consistency
The system SHALL present a clear three-level hierarchy: scope selector (global/category) → Board list → narratives within a Board. Expanding a Board SHALL reveal its child narratives.

#### Scenario: Board expansion
- **WHEN** user clicks a collapsed Board "AI 监管"
- **THEN** the Board expands to show its child narratives, and clicking again collapses them

#### Scenario: Narrative selection
- **WHEN** user clicks a narrative card inside an expanded Board
- **THEN** `NarrativeDetailCard` appears showing title, summary, status, related tags, and generation info

### Requirement: Narrative tag click stays in context
Clicking a tag in `NarrativeDetailCard` SHALL NOT switch to the graph tab. It SHALL emit `select-tag` for the parent to handle (e.g., opening a tag detail tooltip or loading related articles within the narrative context).

#### Scenario: Clicking an abstract tag in narrative detail
- **WHEN** user clicks abstract tag "AI 监管" in narrative detail card
- **THEN** if a Board with that abstract_tag_id exists on the same date, it is scrolled into view and expanded

#### Scenario: Clicking a non-abstract tag in narrative detail
- **WHEN** user clicks event tag "EU AI Act 落地" in narrative detail card
- **THEN** the `select-tag` event is emitted; parent page may open tag detail without switching tabs

### Requirement: Removed legacy timeline components
The files `NarrativeCanvas.client.vue` and its associated unused state/logic SHALL be deleted.

#### Scenario: NarrativeCanvas no longer imported
- **WHEN** building the project
- **THEN** no import error for `NarrativeCanvas.client.vue` occurs because the import is removed

### Requirement: Simplified state model
The NarrativePanel SHALL maintain only: `scopeMode`, `selectedCategoryId`, `boardTimelineDays`, `selectedId`, `hoveredId`, `expandedBoardIds`.

#### Scenario: State initialization
- **WHEN** NarrativePanel mounts with prop `date="2026-05-01"`
- **THEN** only these refs are defined: scopeMode, selectedCategoryId, boardTimelineDays, selectedId, hoveredId, expandedBoardIds. No scopeMode for legacy timeline, no categoryTimelineDays, no timelineDays

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
