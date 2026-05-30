## ADDED Requirements

### Requirement: Template config preview has no side effects
The system SHALL allow users to preview the impact of a hierarchy template change without saving the template, deleting Nodes, creating PendingChanges, or starting rebuild jobs.

#### Scenario: Preview template change
- **WHEN** user edits the event template and requests impact preview
- **THEN** the system SHALL return affected Tag count, estimated rebuild duration, and violation summary without modifying hierarchy_config, topic_tags, topic_tag_relations, or rebuild_jobs

### Requirement: Template config apply requires confirmation
The system SHALL apply a hierarchy template change only after explicit user confirmation. Applying a template change SHALL save the new template, delete old abstract Nodes and their abstract relations for that category, create a rebuild job, and start rebuild execution.

#### Scenario: Confirmed template apply starts rebuild
- **WHEN** user confirms a previewed event template change
- **THEN** the system SHALL save the template, create a rebuild job with trigger `template_change`, and start processing the job

#### Scenario: Cancelled template change has no effect
- **WHEN** user previews a template change and cancels before confirmation
- **THEN** no template, Node, relation, PendingChange, or rebuild job SHALL be modified

### Requirement: Rebuild WebSocket event contract
The system SHALL broadcast hierarchy rebuild events with `type="hierarchy_rebuild"` and `status` equal to `processing`, `completed`, or `failed`. Each event SHALL include `job_id`, `category`, `processed`, `total`, and MAY include `failed_count`, `estimated_remaining_seconds`, `current_tag`, and `error`.

#### Scenario: Processing event received by frontend
- **WHEN** a rebuild job processes a batch of Tags
- **THEN** backend SHALL broadcast `{ type: "hierarchy_rebuild", status: "processing", job_id, category, processed, total, estimated_remaining_seconds }`

#### Scenario: Completed event received by frontend
- **WHEN** a rebuild job completes
- **THEN** backend SHALL broadcast `{ type: "hierarchy_rebuild", status: "completed", job_id, category, processed, total, failed_count }`

#### Scenario: Failed event received by frontend
- **WHEN** a rebuild job fails
- **THEN** backend SHALL broadcast `{ type: "hierarchy_rebuild", status: "failed", job_id, category, processed, total, error }`

### Requirement: /tags rebuild progress display
The `/tags` page SHALL update rebuild progress from the rebuild WebSocket event contract and SHALL refresh hierarchy data after a completed rebuild for the selected category.

#### Scenario: Progress bar updates
- **WHEN** frontend receives a `hierarchy_rebuild` processing event for the selected category
- **THEN** the bottom bar SHALL show processed/total progress and estimated remaining time

#### Scenario: Completed rebuild refreshes page
- **WHEN** frontend receives a completed rebuild event for the selected category
- **THEN** the page SHALL refresh hierarchy tree, Sector counts, pending count, and closure status
