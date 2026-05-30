## MODIFIED Requirements

### Requirement: Abstract tag creation requires minimum information gain
Before creating a new abstract Node, the system SHALL verify that: (1) at least 2 distinct child Tags are proposed, (2) no pair of proposed child Tags shares more than 70% of their associated articles (Jaccard similarity), and (3) the resulting tree SHALL have a leaf-to-depth ratio of at least 1.5. This requirement SHALL apply to `PlaceTagInHierarchy` as the only Node creation entry point. If any check fails, Node creation SHALL be rejected, logged, and returned as a structured placement blocker.

#### Scenario: Too few children rejected
- **WHEN** `PlaceTagInHierarchy` proposes creating a Node with only 1 candidate child
- **THEN** the system SHALL reject creation, log a warning, and return blocker reason `insufficient_siblings`

#### Scenario: High article overlap rejected
- **WHEN** two candidate child Tags share 80% of their associated articles
- **THEN** the system SHALL reject Node creation, log a warning suggesting merge instead, and return blocker reason `low_information_gain`

#### Scenario: Acceptable Node passes all checks
- **WHEN** `PlaceTagInHierarchy` proposes a Node with 3 children whose maximum pairwise Jaccard is 0.3 and the resulting tree leaf-to-depth ratio is 2.0
- **THEN** the system SHALL create the Node, link the triggering Tag under it, and return placement action `created_node`

## ADDED Requirements

### Requirement: PlaceTagInHierarchy closes parent creation
When a leaf Tag has semantic embedding, matches an active Sector, and targets a valid template leaf level, `PlaceTagInHierarchy` SHALL either link the Tag to an existing parent Node, create a valid parent Node, or return a structured blocker reason. It SHALL NOT silently return `unplaced` when all prerequisites for Node creation are present.

#### Scenario: No parent creates Node
- **WHEN** Tag "GPT-5发布" matches Sector "AI产品" and no existing parent Node is suitable, but information gain checks pass
- **THEN** `PlaceTagInHierarchy` SHALL create an abstract Node in that Sector and link the Tag under it

#### Scenario: No parent returns blocker
- **WHEN** Tag "冷门事件" matches a Sector but lacks enough sibling or anchor context to create a meaningful Node
- **THEN** `PlaceTagInHierarchy` SHALL return a blocker reason instead of a generic `unplaced` action

### Requirement: Anchor signals are consumable by placement
If clustering produces anchor signals, the system SHALL persist or otherwise pass those signals to `PlaceTagInHierarchy` so they can influence parent selection or Node creation. Anchor signals SHALL have a clear lifecycle and SHALL NOT exist only as scheduler log output.

#### Scenario: Placement consumes anchor signal
- **WHEN** Tag X belongs to a current anchor signal with Tags Y and Z
- **THEN** `PlaceTagInHierarchy` SHALL include Y and Z as anchor context when selecting or creating a parent Node for X

#### Scenario: Expired anchor signal ignored
- **WHEN** an anchor signal is expired or no longer references active Tags
- **THEN** placement SHALL ignore it and cleanup SHALL remove or skip it
