## MODIFIED Requirements

### Requirement: Board concept persistence
The system SHALL store board concepts in a `board_concepts` table with fields: `id`, `name`, `description`, `embedding` (pgvector), `scope_type`, `scope_category_id`, `is_system`, `is_active`, `display_order`, `source` (TEXT: `auto`/`llm`/`manual`, default `auto`), `protected` (BOOLEAN, default false), `declining` (BOOLEAN, default false), `peak_tag_count` (INT, default 0), `created_at`, and `updated_at`.

#### Scenario: Board concept creation
- **WHEN** a board concept is created with name="AI工具实践" and scope_type="global" via manual mode
- **THEN** a row is inserted into `board_concepts` with is_active=true, is_system=false, source=`manual`, and protected=true

#### Scenario: Inactive board concepts excluded from matching
- **WHEN** tag-to-board matching runs
- **THEN** only board_concepts with is_active=true are considered

### Requirement: Board concept user CRUD
The system SHALL expose API endpoints for users to list, create, update, and deactivate board concepts. Manual creation SHALL set protected=true. Deletion of protected Sectors SHALL require explicit confirmation. Deleting a Sector SHALL clear associated Tags' concept_id references and return the number of affected Tags.

#### Scenario: List all active board concepts
- **WHEN** GET /api/narratives/board-concepts is called
- **THEN** all board_concepts with is_active=true are returned, ordered by display_order, including source, protected, declining, peak_tag_count, and tag_count

#### Scenario: Create a new board concept
- **WHEN** POST /api/narratives/board-concepts with {name, description, source: "manual"} is called
- **THEN** a new protected manual board_concept is created, its embedding is generated, and the record is returned

#### Scenario: Deactivate a board concept
- **WHEN** DELETE /api/narratives/board-concepts/:id is called for a non-protected Sector
- **THEN** the board_concept's is_active is set to false, associated Tags' concept_id is set to NULL, and the response includes affected_tag_count

#### Scenario: Protected delete requires confirmation
- **WHEN** DELETE /api/narratives/board-concepts/:id is called for a protected Sector without confirm=true
- **THEN** the system SHALL return 409 Conflict and SHALL NOT modify the Sector or Tags

## ADDED Requirements

### Requirement: LLM Sector execution result
When users confirm LLM Sector suggestions, the API SHALL execute each accepted add, merge, and split operation and SHALL return true backend execution results for each item, including status, affected Tag count, created Sector IDs, moved Tag count, and error message when applicable.

#### Scenario: Confirm diff returns item results
- **WHEN** user confirms one add and one merge suggestion
- **THEN** the confirm API SHALL return two result items with status `success` or `failed` and backend-computed affected counts

#### Scenario: Partial failure is visible
- **WHEN** one accepted merge fails but one add succeeds
- **THEN** the API SHALL return success for the add, failed for the merge, and the frontend SHALL display the failed item with its error

### Requirement: Sector action refreshes hierarchy closure
After creating, deleting, merging, or splitting Sectors, the system SHALL refresh affected Tag concept assignments or report that a rebuild/placement pass is required.

#### Scenario: New Sector triggers placement opportunity
- **WHEN** a new Sector is created in a category with unplaced Tags
- **THEN** the closure status SHALL indicate that placement can be retried for that category

#### Scenario: Deleted Sector exposes unplaced Tags
- **WHEN** a Sector with 12 Tags is deleted
- **THEN** those Tags SHALL have concept_id cleared and closure status SHALL report at least 12 unplaced Tags
