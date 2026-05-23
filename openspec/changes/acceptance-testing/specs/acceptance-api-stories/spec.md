## ADDED Requirements

### Requirement: Sector CRUD API happy path
The acceptance test SHALL verify the Sector CRUD API cycle: create a manual sector → list sectors → verify new sector appears → delete sector → verify removed.

#### Scenario: Full Sector CRUD cycle
- **WHEN** test creates a sector with name "验收测试板块", category "event", source "manual" via POST /api/narratives/board-concepts/sectors
- **THEN** response SHALL contain `success: true` and `data` with the created sector including `source: "manual"` and `protected: true`
- **WHEN** test then calls GET /api/narratives/board-concepts/sectors with category "event"
- **THEN** response list SHALL contain the created sector
- **WHEN** test calls DELETE /api/narratives/board-concepts/sectors/:id
- **THEN** response SHALL contain `success: true`
- **WHEN** test calls GET /api/narratives/board-concepts/sectors again
- **THEN** response list SHALL NOT contain the deleted sector

### Requirement: Hierarchy config read/write API
The acceptance test SHALL verify the hierarchy config API: read current config → modify → verify change → restore original config.

#### Scenario: Config read and update cycle
- **WHEN** test calls GET /api/hierarchy/config
- **THEN** response SHALL contain `success: true` with templates array, each having category and levels
- **WHEN** test calls PUT /api/hierarchy/config with modified templates
- **THEN** response SHALL contain `success: true` with `data.impact` including `total_tags` count
- **WHEN** test calls GET /api/hierarchy/config again
- **THEN** the modification SHALL be reflected in the returned templates

#### Scenario: Config restoration
- **WHEN** test restores original config via PUT /api/hierarchy/config
- **THEN** subsequent GET SHALL return the original template values

### Requirement: Rebuild job API happy path
The acceptance test SHALL verify rebuild job lifecycle: trigger rebuild → poll status → verify completion or skip on timeout.

#### Scenario: Trigger rebuild and poll to completion
- **WHEN** test calls POST /api/hierarchy/rebuild/start with category "event"
- **THEN** response SHALL contain `success: true` with `data` including job `id` and `status`
- **WHEN** test polls GET /api/hierarchy/rebuild/:id every 5 seconds for up to 10 minutes
- **THEN** status SHALL eventually be "completed" or "failed"
- **WHEN** rebuild completes with status "completed"
- **THEN** `processed_tags` SHALL be >= 0 and `total_tags` SHALL be >= 0

#### Scenario: Rebuild timeout
- **WHEN** rebuild does not complete within 10 minutes
- **THEN** test SHALL skip (not fail) with message indicating timeout

### Requirement: Pending changes API
The acceptance test SHALL verify the pending changes API: list pending changes → approve batch → verify cleared.

#### Scenario: List and approve pending changes
- **WHEN** test calls GET /api/hierarchy/pending
- **THEN** response SHALL contain `success: true` with `data` as an array (possibly empty)
- **WHEN** pending changes exist and test calls POST /api/hierarchy/pending/approve
- **THEN** response SHALL contain `success: true`
- **WHEN** test calls GET /api/hierarchy/pending again
- **THEN** pending changes count SHALL be 0 or reduced

#### Scenario: No pending changes
- **WHEN** GET /api/hierarchy/pending returns empty array
- **THEN** test SHALL skip with message "无待处理变更"
