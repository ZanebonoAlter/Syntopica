## ADDED Requirements

### Requirement: Hierarchy closure status inspection
The system SHALL provide a category-level hierarchy closure status that summarizes whether the selected category can complete the `/tags` management loop. The status SHALL include active Sector count, unplaced Tag count, pending change count, active rebuild job status, and the top blocker reason when the loop is not currently closable.

#### Scenario: Category status shows no Sector blocker
- **WHEN** the event category has active leaf Tags but zero active Sectors
- **THEN** the closure status SHALL report `blocker=no_active_sector` and include the unplaced Tag count

#### Scenario: Category status shows active rebuild
- **WHEN** a rebuild job for category `event` is running
- **THEN** the closure status SHALL report the rebuild job ID, processed count, total count, and `blocker=rebuild_running`

### Requirement: Hierarchy orchestration flow
The system SHALL expose a backend orchestration flow for a category that performs the steps required to close the hierarchy loop in order: inspect current state, bootstrap Sector if required, place eligible Tags, validate template compliance, create PendingChanges for risky fixes, and return a final status summary.

#### Scenario: Bootstrap then place unplaced Tags
- **WHEN** category `event` has 20 unplaced Tags, no active Sector, and the auto Sector threshold is 15
- **THEN** the orchestration flow SHALL trigger auto Sector generation before retrying Tag placement

#### Scenario: Existing Sector allows placement directly
- **WHEN** category `keyword` has active Sectors and unplaced Tags with embeddings
- **THEN** the orchestration flow SHALL skip Sector bootstrap and call placement for eligible unplaced Tags

#### Scenario: Placement blocker is returned
- **WHEN** a Tag cannot be placed because its embedding is missing
- **THEN** the orchestration result SHALL include that Tag under `blocked_tags` with reason `pending_embedding`

### Requirement: /tags page reflects closure state
The `/tags` page SHALL display closure state for the selected category and SHALL refresh Sector list, hierarchy tree, pending count, and rebuild progress after any action that changes hierarchy state.

#### Scenario: User enters /tags with unclosed hierarchy
- **WHEN** user navigates to `/tags` and the selected category has unplaced Tags
- **THEN** the page SHALL display the unplaced count and available actions to create Sector, run LLM suggestions, or start closure/rebuild

#### Scenario: Action refreshes all dependent panels
- **WHEN** user confirms a Sector LLM diff or approves PendingChanges
- **THEN** the page SHALL refresh Sector counts, hierarchy tree, pending count, and closure status from backend state

### Requirement: Placement blockers are user-visible
When automatic placement cannot close a Tag, the system SHALL preserve a structured blocker reason and expose aggregated blocker counts to `/tags` so the user can understand why Tags remain unplaced.

#### Scenario: Low information gain blocker
- **WHEN** `PlaceTagInHierarchy` rejects Node creation because candidate children do not satisfy information gain rules
- **THEN** the closure status SHALL include `low_information_gain` in blocker counts

#### Scenario: No matching Sector blocker
- **WHEN** `MatchTagToConcept` returns no Sector above threshold
- **THEN** the closure status SHALL include `no_matching_sector` in blocker counts
