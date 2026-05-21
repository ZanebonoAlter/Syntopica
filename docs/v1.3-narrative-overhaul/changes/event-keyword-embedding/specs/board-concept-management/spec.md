## MODIFIED Requirements

### Requirement: Board concept LLM cold-start suggestion
On first deployment or user request, the system SHALL bootstrap concepts per category via pgvector clustering of tag embeddings. Each cluster with >= 5 tags SHALL be named via LLM and created as a pending BoardConcept for user confirmation. If no cluster reaches the minimum size, the system SHALL create one default BoardConcept per category with status='active' as a fallback. The default concept name SHALL be the category's Chinese label.

#### Scenario: LLM scans abstract tags
- **WHEN** bootstrap is triggered for a category
- **THEN** all active tags with semantic embeddings in that category SHALL be loaded for clustering

#### Scenario: Suggestions returned as JSON list
- **WHEN** bootstrap completes
- **THEN** the response SHALL contain created concepts (pending) or a default concept (active) if no clusters qualified

#### Scenario: Bootstrap triggers via API
- **WHEN** POST /api/hierarchy/concepts/bootstrap is called with category="event"
- **THEN** bootstrap runs for the event category and returns created concepts

#### Scenario: Default concept created on empty bootstrap
- **WHEN** bootstrap runs and no cluster reaches 5 tags
- **THEN** one default concept with status='active' SHALL be created; no user confirmation required
