## ADDED Requirements

### Requirement: Board nodes as canvas large nodes
The narrative canvas SHALL render boards as large collapsible nodes in a folded state by default. Each board node displays the board name, narrative count, and a status indicator derived from its narratives' statuses.

#### Scenario: Boards rendered as large nodes in timeline
- **WHEN** the narrative timeline loads with board data for multiple days
- **THEN** each board is rendered as a large rectangular node showing board name, narrative count, and aggregate status

#### Scenario: Empty boards are not displayed
- **WHEN** a board has zero narratives
- **THEN** the board node is not rendered on the canvas

### Requirement: Board drill-down to narrative nodes
Clicking a board node SHALL expand it to reveal narrative sub-nodes within the board. The board node transitions from a collapsed summary to an expanded container showing individual narrative cards.

#### Scenario: Click board to expand narratives
- **WHEN** user clicks a collapsed board node
- **THEN** the board expands to show its narrative cards as smaller nodes within the board's boundary, including both LLM-generated narratives and abstract tag cards

#### Scenario: Click expanded board to collapse
- **WHEN** user clicks an expanded board node's header area
- **THEN** the board collapses back to the folded summary view, hiding narrative sub-nodes

### Requirement: Narrative-level edge connections
Edges between narratives (parent_ids lineage) SHALL be rendered as lines connecting narrative nodes across days. Board-level connections are derived from these narrative edges and shown as subtle background lines between board boundaries.

#### Scenario: Narrative edges rendered across days
- **WHEN** narrative X (day 2, board A) has parent pointing to narrative Y (day 1, board B)
- **THEN** a bezier line connects narrative node X to narrative node Y, with color/style based on the narrative status

#### Scenario: Board connection lines derived from narratives
- **WHEN** narratives within board A connect to narratives within board B (previous day)
- **THEN** a subtle background line connects board A and board B boundaries, with style derived from the aggregate narrative statuses

### Requirement: Abstract tag card visual distinction
Abstract tag narrative cards SHALL have a distinct visual style (dashed border) to differentiate them from LLM-generated narrative cards within the expanded board view.

#### Scenario: Abstract tag card displayed with dashed border
- **WHEN** a board expands and contains an abstract tag narrative card (source='abstract')
- **THEN** the card is rendered with a dashed border and a label indicating it is an abstract tag

### Requirement: Board status badge
Each board node SHALL display a status badge derived by aggregating its narratives' statuses. The aggregation rule is: if any narrative is 'emerging' → emerging; if all are 'ending' → ending; if narratives come from different previous-day boards → merging; if narratives split to different next-day boards → splitting; otherwise → continuing.

#### Scenario: Board with mixed narrative statuses shows continuing
- **WHEN** a board contains narratives with statuses ['continuing', 'emerging', 'continuing']
- **THEN** the board status badge shows 'continuing' as the aggregate

#### Scenario: Board with all ending narratives shows ending
- **WHEN** all narratives within a board have status 'ending'
- **THEN** the board status badge shows 'ending'

### Requirement: Board timeline layout
The canvas SHALL arrange boards in a day-column layout, similar to the current narrative timeline. Each day column contains board nodes stacked vertically. Board width is larger than narrative card width to visually indicate hierarchy.

#### Scenario: Multi-day board timeline
- **WHEN** narrative data spans 7 days
- **THEN** the canvas renders 7 columns, each containing board nodes for that day, with cross-day connections between boards

### Requirement: Board scope filter
The narrative panel SHALL provide a scope filter to toggle between global boards (merged across categories) and per-category boards, similar to the existing scope switcher.

#### Scenario: View global boards
- **WHEN** user selects 'global' scope mode
- **THEN** the canvas displays only scope=global boards with their merged narratives

#### Scenario: View category boards
- **WHEN** user selects a specific category in scope mode
- **THEN** the canvas displays only boards for that category (scope=feed_category)
