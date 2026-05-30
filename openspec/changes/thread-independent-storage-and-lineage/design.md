## Context

The daily report system generates narrative threads per cluster section, storing them as a JSON array in `daily_report_sections.threads`. The `Thread` struct in Go has a `PrevThreadID *uint` field, but `matchPreviousThreads()` in `generator.go` only uses tag-overlap detection to override thread status from `emerging` to `continuing` — it never assigns the previous thread's ID. Threads therefore have no cross-day identity or lineage.

The current persistence flow in `GenerateDailyReport()`:
1. LLM generates threads as `[]Thread` structs
2. `matchPreviousThreads()` overrides status but ignores `PrevThreadID`
3. Threads are marshaled to JSON and stored in `DailyReportSection.Threads` (JSONB column)
4. `SaveReport()` saves the section with embedded threads

The `BoardDailyReportTimeline.vue` frontend component renders threads inline within cluster cards in a newspaper-style modal. Each thread is an anonymous object — no ID, no lineage, no way to trace across days.

Existing infrastructure: `board_daily_reports.prev_report_id` already links reports across days. Section data includes `cluster_tag_ids` JSONB for tag overlap matching. The migration system uses explicit versioned SQL migrations in `postgres_migrations.go`.

## Goals / Non-Goals

**Goals:**
- Give every thread a unique, persistent database identity via `daily_report_threads` table
- Populate `prev_thread_id` during generation so threads form a linked list across days
- Provide API endpoints for thread lineage chain retrieval and board-level thread timeline
- Build frontend views: (A) thread lineage timeline within newspaper modal, (B) board-level Gantt-chart thread browser
- Migrate existing JSON thread data to the new table without data loss

**Non-Goals:**
- Redesigning thread status values (emerging/continuing/splitting/merging/ending remain unchanged)
- Implementing the embedding-based thread matching described in the existing spec (tag overlap matching is sufficient for now)
- Adding thread editing/merging UI
- Building real-time thread updates via WebSocket
- Changing how sections or clusters are generated

## Decisions

### 1. Independent table vs. adding columns to sections

**Decision**: Create `daily_report_threads` as a separate table with FK to `daily_report_sections`.

**Rationale**: Threads are the primary queryable unit for lineage tracing. Storing them as rows enables:
- `prev_thread_id` self-reference (impossible in JSON arrays)
- Efficient queries: "get all threads for board X across all days" without parsing JSON
- Index on `prev_thread_id` for chain traversal

**Alternative considered**: Add a generated column or `jsonb_path_query` GIN index on the existing `threads` JSONB. Rejected because self-referencing lineage within JSON is awkward, queries are slow, and the current `PrevThreadID` field is never populated anyway.

### 2. Migration strategy: extract JSON → new table, then drop column

**Decision**: Two-phase migration:
1. Create `daily_report_threads` table
2. Extract existing JSON threads: `INSERT INTO daily_report_threads (...) SELECT ... FROM daily_report_sections WHERE threads IS NOT NULL` — `prev_thread_id` left null for historical data
3. Drop `daily_report_sections.threads` column in a separate migration after verification

**Rationale**: Gradual migration with rollback window. Historical `prev_thread_id` cannot be retroactively determined (the matching function never ran with IDs), so leaving it null is correct.

**JSON field name mapping**: Migration SQL must map the current JSON field names to the new table columns: `related_tag_ids` → `tag_ids`. The `parent_thread_id` and `related_article_ids` fields are not carried over (prev_thread_id=NULL for historical data, article associations not stored in the new table).

### 3. Lineage population: assign prev_thread_id during matchPreviousThreads()

**Decision**: Modify `matchPreviousThreads()` to receive previous threads as `[]DailyReportThread` (with DB IDs) and assign `prev_thread_id` when a tag-overlap match is found.

**Rationale**: The existing matching logic (tag intersection) is already correct for detecting continuation. The only missing piece is recording *which* previous thread was matched. The function already iterates over `prevThreadList` and finds `bestMatchIdx` — we just need to pass the DB IDs along.

Additionally, `findPreviousReport()` needs to Preload `"Sections.Threads"` so that the GORM-loaded `DailyReportSection.Threads` association provides the previous day's threads with their DB IDs. `getPrevThreadSummaries()` also needs updating to read from this GORM association instead of JSON unmarshaling.

### 4. Thread chain retrieval: recursive query vs. iterative API

**Decision**: Use a PostgreSQL recursive CTE (`WITH RECURSIVE`) in a new repository function `GetThreadLineage(threadID)` to fetch the full chain (all ancestors + descendants) in one query.

**Rationale**: Thread chains are short (typically 2-7 days). A single recursive query is simpler than N+1 API calls. The CTE walks both directions: from the given thread backward via `prev_thread_id` to the root, then forward to find all descendants.

### 5. Board thread timeline API: single endpoint returning all threads with prev_thread_id

**Decision**: New endpoint `GET /api/semantic-boards/:id/thread-timeline?days=30` returns all `daily_report_threads` for that board within the date range, including `prev_thread_id` and `period_date` (joined from the report). The frontend assembles the Gantt chart locally.

**Rationale**: Simpler than a paginated or graph-based API. Board thread counts are modest (typically 20-80 threads across 30 days). The frontend can build lineage chains client-side from the flat list using `prev_thread_id`.

### 6. Frontend architecture: two new components, no routing changes

**Decision**: 
- **View A** (`ThreadLineagePanel.vue`): Side panel within the existing newspaper modal. Triggered by clicking a thread. Fetches lineage via `GET /api/daily-reports/threads/:id/lineage`. Renders vertical timeline.
- **View B** (`BoardThreadBrowser.vue`): New component accessible via a button/link in the `BoardDailyReportTimeline.vue` panel (or a new tab). No new Nuxt route — uses component toggle. Fetches data via `GET /api/semantic-boards/:id/thread-timeline`.

**Rationale**: Keeps the daily report feature self-contained. No route changes needed.

## Risks / Trade-offs

- **[Risk] Migration data loss if threads JSON has unexpected shapes** → Mitigation: Migration uses `jsonb_array_elements` with error handling; column drop is in a separate migration that can be deferred. JSON field names (`related_tag_ids` not `tag_ids`) are correctly mapped in migration SQL.
- **[Risk] Upsert invalidates downstream prev_thread_id references** → Mitigation: `SaveReport` sets `prev_thread_id=NULL` on any threads that reference the report's threads before deleting them. Report regeneration implies content has changed, so broken lineage is expected.
- **[Risk] Thread matching quality — tag overlap may produce false lineage links** → Mitigation: This is the existing matching strategy; this change only records the match, not changes the matching algorithm. Future improvements to matching are a separate concern.
- **[Risk] Board thread timeline query performance on boards with long history** → Mitigation: `days` parameter capped at 30. Indexes on `report_id`, `prev_thread_id`, and `board_daily_reports.semantic_board_id` keep queries fast.
- **[Trade-off] Historical threads have no `prev_thread_id`** → Accepted. Backfilling lineage for historical data would require re-running the matching algorithm with DB IDs, which is complex and low value. The UI will simply show these as chain-starting nodes.
- **[Trade-off] Frontend Gantt chart is a custom component, not a charting library** → Accepted for now. Thread counts per board are small enough that a CSS grid/Flexbox solution works. Can upgrade to a library later if needed.
