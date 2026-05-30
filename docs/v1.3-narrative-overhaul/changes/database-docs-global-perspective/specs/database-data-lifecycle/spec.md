## ADDED Requirements

### Requirement: Four core lifecycle chains documented
`DATA_LIFECYCLE.md` SHALL document 4 core data lifecycle chains: Article Lifecycle, Topic Tag Lifecycle, Reading Feedback Lifecycle, Narrative Generation Lifecycle.

#### Scenario: Reader can trace article data from ingestion to narrative
- **WHEN** reader follows the "Article Lifecycle" chain
- **THEN** they see status field transitions from `articles` through `firecrawl_jobs`, `tag_jobs`, `topic_tags`, `embedding_queues`, `narrative_boards`, to `narrative_summaries`

### Requirement: Each chain uses status field flow granularity
Each lifecycle chain SHALL describe data state transitions using status field values and table write points, approximately 30-50 lines per chain, listing which tables are written and which status fields change at each step.

#### Scenario: Reader sees firecrawl_status transitions
- **WHEN** reader reads the article lifecycle Firecrawl step
- **THEN** they see `articles.firecrawl_status: pending → processing → completed` and the corresponding `firecrawl_jobs` queue record lifecycle

### Requirement: Reserved features in separate section
`DATA_LIFECYCLE.md` SHALL include a separate section titled "预留功能" for lifecycle chains that have zero data rows and no active Go code, explicitly labeled as "当前未启用".

#### Scenario: Reader identifies unused features
- **WHEN** reader views the reserved features section
- **THEN** they see "AI 批量摘要" and "Digest 推送" chains clearly marked as reserved, with the involved tables listed

### Requirement: Configuration requirements included
`DATA_LIFECYCLE.md` SHALL include the feature enablement conditions migrated from `DATABASE_FIELDS.md`, describing which settings and flags control each lifecycle chain's activation.

#### Scenario: Reader understands Firecrawl activation conditions
- **WHEN** reader reads the article lifecycle Firecrawl step
- **THEN** they see that it requires `feed.firecrawl_enabled = true` and global Firecrawl API configuration

### Requirement: Cross-references to data-flow.md
`DATA_LIFECYCLE.md` SHALL establish a clear boundary with `architecture/data-flow.md`: this file covers data state transitions (table writes, status field changes), while `data-flow.md` covers code execution flows (function calls, API requests, store interactions).

#### Scenario: Reader navigates between perspectives
- **WHEN** reader wants to understand the implementation behind a lifecycle step
- **THEN** they find a cross-reference to the corresponding section in `data-flow.md`
