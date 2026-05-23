## ADDED Requirements

### Requirement: ER diagram covers all 38 tables
`ER_DIAGRAM.md` SHALL document all 38 tables in the `public` schema of the `rss_reader` database, grouped into 6 business domains: Core, Topic Tags, AI Summaries, Narrative, Hierarchy, AI Infrastructure.

#### Scenario: Reader can see every table's domain归属
- **WHEN** reader opens `ER_DIAGRAM.md`
- **THEN** every table from `pg_stat_user_tables` appears in exactly one domain section

### Requirement: Global overview uses ASCII grouped diagram
`ER_DIAGRAM.md` SHALL include an ASCII diagram showing domain-level relationships (not individual tables), with arrows indicating cross-domain FK dependencies.

#### Scenario: Reader sees domain-level dependencies at a glance
- **WHEN** reader views the global overview section
- **THEN** they see ~6 domain boxes with labeled arrows showing which domains reference which

### Requirement: Per-domain ER uses Mermaid erDiagram
Each domain section SHALL include a Mermaid `erDiagram` showing 5-10 entities with their FK relationships, cardinality labels, and key columns.

#### Scenario: Reader can inspect a single domain's entity relationships
- **WHEN** reader views the "Topic Tags" domain section
- **THEN** they see a Mermaid erDiagram with `topic_tags`, `topic_tag_embeddings`, `topic_tag_relations`, `article_topic_tags`, `embedding_queues`, and related queue tables, with FK lines connecting them

### Requirement: FK reference matrix lists all constraints
`ER_DIAGRAM.md` SHALL include a table listing all 35 FK constraints with columns: source table, FK column, target table, target column, constraint name.

#### Scenario: Reader can look up any FK relationship
- **WHEN** reader searches the FK reference matrix for `topic_tags`
- **THEN** they find all 12 incoming FK references across 10 tables

### Requirement: Relationship patterns documented
`ER_DIAGRAM.md` SHALL include a section describing relationship patterns: many-to-many bridge tables (`article_topic_tags`), self-referential FKs (`topic_tags.merged_into_id`), denormalized references (`ai_call_logs`), and JSON-stored ID lists (`narrative_boards.event_tag_ids`).

#### Scenario: Reader understands non-FK relationship patterns
- **WHEN** reader encounters `narrative_boards.event_tag_ids` in a diagram
- **THEN** the relationship patterns section explains this is a denormalized JSON array referencing `topic_tags.id` without a FK constraint
