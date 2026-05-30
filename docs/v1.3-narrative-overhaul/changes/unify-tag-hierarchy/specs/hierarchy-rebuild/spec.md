## ADDED Requirements

### Requirement: rebuild_jobs persistence
The system SHALL store rebuild jobs in a `rebuild_jobs` table with fields: `id` (SERIAL PK), `category` (TEXT NOT NULL), `trigger` (TEXT NOT NULL: 'template_change'/'sector_regen'/'manual'), `status` (TEXT NOT NULL: 'pending'/'running'/'paused'/'completed'/'failed'), `total_tags` (INT DEFAULT 0), `processed_tags` (INT DEFAULT 0), `failed_tags` (INT DEFAULT 0), `estimated_end` (TIMESTAMP), `started_at` (TIMESTAMP), `completed_at` (TIMESTAMP), `last_tag_id` (INT DEFAULT 0), `config_snapshot` (JSONB), `error_detail` (TEXT), `created_at` (TIMESTAMP DEFAULT NOW()).

#### Scenario: Job created on template change
- **WHEN** user confirms template change for category "event"
- **THEN** a rebuild_job is created with trigger='template_change', category='event', status='pending', total_tags set to leaf Tag count

### Requirement: Rebuild execution with batch processing
The system SHALL process rebuild jobs by selecting leaf Tags in batches of 20 (configurable) ordered by ID, starting from `last_tag_id + 1`. After each batch completes, the system SHALL update `processed_tags`, `last_tag_id`, and recalculate `estimated_end`. Between batches, the system SHALL sleep for 1 second (configurable) for rate limiting.

#### Scenario: Batch processes Tags
- **WHEN** a running rebuild_job has total_tags=100, last_tag_id=40, processed_tags=40
- **THEN** the next batch SHALL select 20 Tags with id > 40 from the category, call PlaceTagInHierarchy for each, then update processed_tags=60 and last_tag_id to the batch max

#### Scenario: Rate limiting between batches
- **WHEN** a batch of 20 Tags completes processing
- **THEN** the system SHALL sleep 1 second before processing the next batch

### Requirement: Rebuild checkpoint resume
When a rebuild job resumes after interruption (server restart, pause), the system SHALL continue from `last_tag_id` and `processed_tags` without reprocessing completed Tags. A paused job SHALL NOT block new jobs for other categories.

#### Scenario: Resume after server restart
- **WHEN** server restarts and rebuild_job for "event" has status='running', last_tag_id=80, processed_tags=80
- **THEN** on startup the system SHALL detect the incomplete job, set status='paused', and allow manual resume from tag ID 81

#### Scenario: Only one active rebuild per category
- **WHEN** a rebuild_job is already running for category "event"
- **THEN** attempting to create another rebuild for "event" SHALL be rejected with an error

### Requirement: Rebuild time estimation
When a rebuild job is created, the system SHALL estimate completion time using: `total_tags × avg_placement_time`, where `avg_placement_time` is calculated from the last 100 successful PlaceTagInHierarchy calls recorded in `ai_call_logs`.

#### Scenario: Time estimate displayed
- **WHEN** user confirms template change affecting 247 Tags and avg_placement_time is 0.4s
- **THEN** the system SHALL display "预计重建耗时 1-2 分钟"

#### Scenario: No history fallback
- **WHEN** no ai_call_logs history exists for PlaceTagInHierarchy
- **THEN** the system SHALL use a default estimate of 0.5s per Tag

### Requirement: Template change rebuild flow
When user saves a modified HierarchyTemplate, the system SHALL: (1) save the new template to hierarchy_config, (2) DELETE all topic_tag_relations with relation_type='abstract' where parent or child belongs to an abstract tag of that category, (3) DELETE all topic_tags with source='abstract' and matching category, (4) DELETE corresponding topic_tag_embeddings, (5) create a rebuild_job. Leaf Tags (source='llm'/'heuristic') SHALL be preserved with their concept_id intact.

#### Scenario: Template change preserves leaf Tags
- **WHEN** user changes event template from 3 levels to 4 levels
- **THEN** all topic_tags with source='llm' SHALL remain with status='active' and concept_id unchanged; all topic_tags with source='abstract' SHALL be deleted

#### Scenario: Template change shows impact before execution
- **WHEN** user modifies event template levels and clicks save
- **THEN** the system SHALL display the count of affected leaf Tags and estimated rebuild time before requiring confirmation

### Requirement: Rebuild progress WebSocket push
During rebuild execution, the system SHALL push progress updates via WebSocket: `{ type: 'rebuild_progress', job_id, category, processed, total, estimated_remaining_seconds }`. On completion, push `{ type: 'rebuild_complete', job_id, category, total_processed, failed_count }`.

#### Scenario: Progress pushed during rebuild
- **WHEN** rebuild_job processes a batch of 20 Tags
- **THEN** a WebSocket message with updated processed count SHALL be pushed to connected clients
