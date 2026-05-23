## ADDED Requirements

### Requirement: Index page provides navigation
`_index.md` SHALL serve as the entry point for `docs/reference/database/`, listing all sibling files with a one-line description and link.

#### Scenario: Reader enters the database directory
- **WHEN** reader opens `docs/reference/database/_index.md`
- **THEN** they see links to `DATABASE_FIELDS.md`, `ER_DIAGRAM.md`, and `DATA_LIFECYCLE.md` with descriptions

### Requirement: Index page includes database overview
`_index.md` SHALL include a compact overview section with key numbers: total tables (38), FK constraints (35), business domains (6), and the most-referenced table (`topic_tags` with 12 incoming FKs).

#### Scenario: Reader builds mental model before diving in
- **WHEN** reader reads the overview section
- **THEN** they understand the scale and structure of the database at a glance
