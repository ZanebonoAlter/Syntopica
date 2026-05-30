## MODIFIED Requirements

### Requirement: Board concept persistence
The system SHALL store board concepts in a `board_concepts` table with fields: `id`, `name`, `description`, `embedding` (pgvector), `scope_type`, `scope_category_id`, `is_system`, `is_active`, `display_order`, `source` (TEXT: 'auto'/'llm'/'manual', default 'auto'), `protected` (BOOLEAN, default false), `declining` (BOOLEAN, default false), `peak_tag_count` (INT, default 0), `created_at`, `updated_at`.

#### Scenario: Board concept creation
- **WHEN** a board concept is created with name="AI工具实践" and scope_type="global" via manual mode
- **THEN** a row is inserted into `board_concepts` with is_active=true, is_system=false, source='manual', protected=true

#### Scenario: Inactive board concepts excluded from matching
- **WHEN** tag-to-board matching runs
- **THEN** only board_concepts with is_active=true are considered

#### Scenario: Auto-created concept fields
- **WHEN** a board concept is created via auto mode
- **THEN** source='auto', protected=false, declining=false

### Requirement: Board concept LLM cold-start suggestion
On first deployment or user request, the system SHALL invoke LLM to scan all active abstract tags and suggest a list of board concepts. Each suggestion SHALL include name and description. JSON schema SHALL be passed via LLM structured output parameter, not in prompt text.

#### Scenario: LLM scans abstract tags
- **WHEN** LLM is asked to suggest board concepts
- **THEN** it receives all active abstract tag names and descriptions as input, and the response format is enforced via structured output

#### Scenario: Suggestions returned as structured output
- **WHEN** LLM responds
- **THEN** the response is parsed as a JSON array of {name, description} objects guaranteed by the API parameter, not by prompt instruction

### Requirement: Board concept user CRUD
The system SHALL expose API endpoints for users to list, create, update, and deactivate board concepts. Manual mode creation SHALL set protected=true. Deletion of protected Sectors SHALL require explicit confirmation.

#### Scenario: List all active board concepts with metadata
- **WHEN** GET /api/narratives/board-concepts is called
- **THEN** all board_concepts with is_active=true are returned, ordered by display_order, including source, protected, declining, and associated Tag count

#### Scenario: Create a new board concept via manual mode
- **WHEN** POST /api/narratives/board-concepts with {name, description, source: 'manual'} is called
- **THEN** a new board_concept is created with protected=true, source='manual', its embedding is generated, and the record is returned

#### Scenario: Deactivate a board concept
- **WHEN** DELETE /api/narratives/board-concepts/:id is called
- **THEN** the board_concept's is_active is set to false; associated Tags' concept_id is set to NULL

#### Scenario: Delete protected Sector requires confirmation
- **WHEN** DELETE /api/narratives/board-concepts/:id is called for a protected Sector
- **THEN** the API SHALL require a `confirm=true` query parameter; without it, SHALL return 409 Conflict

### Requirement: Board concept embedding generation
When a board concept is created or its name/description is updated, the system SHALL generate a pgvector embedding by calling the embedding service with the concept's name and description as input text.

#### Scenario: Embedding generated on creation
- **WHEN** a board concept with name="AI工具实践" and description="..." is created
- **THEN** the embedding column is populated with a 1536-dimension vector via the embedding API

#### Scenario: Embedding regenerated on update
- **WHEN** a board concept's description is updated
- **THEN** the embedding is regenerated and updated in the database
