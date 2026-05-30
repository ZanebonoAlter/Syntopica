## ADDED Requirements

### Requirement: Event clustering phase in cleanup cycle
The `tag_hierarchy_cleanup` scheduler SHALL include a Phase 2.5 (between flat merge and hierarchy pruning) that calls `ClusterUnclassifiedTags` for category "event" only. This phase SHALL respect the existing cleanup budget and skip if timed out.

#### Scenario: Event clustering phase executes
- **WHEN** the cleanup cycle runs and Phase 2 (flat merge) completes within budget
- **THEN** Phase 2.5 SHALL call `ClusterUnclassifiedTags(ctx, "event")` and log the result

#### Scenario: Budget exhausted before event clustering
- **WHEN** the cleanup budget is timed out after Phase 2
- **THEN** Phase 2.5 SHALL be skipped and logged as "budget timed out"

#### Scenario: Event clustering produces abstracts
- **WHEN** event clustering finds clusters that LLM judges as related
- **THEN** the resulting abstract tags SHALL be recorded in the cleanup summary
