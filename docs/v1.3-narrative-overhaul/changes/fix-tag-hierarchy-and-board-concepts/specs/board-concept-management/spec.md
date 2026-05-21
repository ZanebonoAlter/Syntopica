## MODIFIED Requirements

### Requirement: Board concept persistence
The system SHALL store board concepts in a `board_concepts` table with fields: `id`, `name`, `description`, `embedding` (pgvector), `category`, `scope_type`, `scope_category_id`, `is_system`, `status`, `display_order`, `created_at`, `updated_at`. The `status` field SHALL have values `active`, `pending`, `inactive`, or `merged`.

#### Scenario: Board concept creation
- **WHEN** a board concept is created with name="AI工具实践" and category="event"
- **THEN** a row is inserted into `board_concepts` with status="active", is_system=false

#### Scenario: Inactive board concepts excluded from matching
- **WHEN** tag-to-board matching runs
- **THEN** only board_concepts with status="active" are considered

### Requirement: Board concept LLM cold-start suggestion
On user request via the suggest API, the system SHALL invoke LLM to scan active tags without concept assignments and suggest a list of board concepts. Each suggestion SHALL include name and description. The LLM SHALL NOT create database records; the frontend SHALL call the create endpoint to persist accepted suggestions.

#### Scenario: LLM scans unassigned tags
- **WHEN** LLM is asked to suggest board concepts via `POST /hierarchy/concepts/suggest`
- **THEN** it receives up to 50 active tag names and descriptions for unassigned tags as input

#### Scenario: Suggestions returned as JSON list
- **WHEN** LLM responds
- **THEN** the response is parsed as a JSON array of {name, description} objects

### Requirement: Board concept user CRUD
The system SHALL expose API endpoints on `/hierarchy/concepts` for users to list, create, update, and deactivate board concepts.

#### Scenario: List active board concepts
- **WHEN** GET /hierarchy/concepts is called with `?category=event`
- **THEN** all board_concepts with status="active" and matching category are returned, ordered by display_order

#### Scenario: List all board concepts including inactive
- **WHEN** GET /hierarchy/concepts is called with `?category=event&all=true`
- **THEN** all board_concepts matching the category are returned regardless of status

#### Scenario: Create a new board concept
- **WHEN** POST /hierarchy/concepts with {name, description, category} is called
- **THEN** a new board_concept is created with status="active", its embedding is generated, and the record is returned

#### Scenario: Update a board concept
- **WHEN** PUT /hierarchy/concepts/:id with {name, description} is called
- **THEN** the board_concept's name and description are updated, embedding is regenerated

#### Scenario: Deactivate a board concept
- **WHEN** DELETE /hierarchy/concepts/:id is called
- **THEN** the board_concept's status is set to "inactive"; no rows are physically deleted

### Requirement: Board concept embedding generation
When a board concept is created or its name/description is updated, the system SHALL generate a pgvector embedding by calling the embedding service with the concept's name and description as input text.

#### Scenario: Embedding generated on creation
- **WHEN** a board concept with name="AI工具实践" and description="..." is created
- **THEN** the embedding column is populated via the embedding API

#### Scenario: Embedding regenerated on update
- **WHEN** a board concept's description is updated
- **THEN** the embedding is regenerated and updated in the database
