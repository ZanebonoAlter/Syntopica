## Why

Narrative threads currently live as anonymous JSON arrays inside `daily_report_sections.threads`, with no independent identity or cross-day lineage. The `Thread.PrevThreadID` field exists in the Go struct but is never populated — `matchPreviousThreads()` detects tag-overlap matches and overrides status (emerging→continuing) without recording *which* previous thread was matched. This makes it impossible to trace how a narrative thread evolves across days, which blocks two high-value features: (1) a thread detail timeline showing a thread's history within the daily report modal, and (2) a board-level Gantt-chart overview of all thread lifecycles.

Additionally, **section-level lineage matching via `cluster_tag_ids` Jaccard similarity is broken**: daily clustering generates ephemeral tag IDs with zero cross-day overlap, and older tags are deleted from the database. This means `matchPreviousSections()` never finds matches, `prev_section_id` is always NULL, and the Gantt chart shows only isolated single-day nodes despite visually obvious event continuity.

## What Changes

- **New `daily_report_threads` table**: Independent storage for threads with their own primary key, `report_id`, `section_id`, and a `prev_thread_id` self-referencing foreign key for lineage chaining.
- **BREAKING**: `daily_report_sections.threads` JSONB column will be removed after data migration. All thread data moves to the new table.
- **BREAKING**: `GenerateDailyReport` return signature changes to `(*BoardDailyReport, []DailyReportSection, [][]DailyReportThread, error)` to carry thread data for persistence.
- **BREAKING**: `matchPreviousSections()` replaced with embedding-based semantic matching via pgvector
- **Section embedding generation**: `GenerateDailyReport` pipeline now batch-embeds each section's `cluster_label` using the existing embedding model (2560-dim). Embeddings stored in `daily_report_sections.embedding vector(2560)` column.
- **DB-side matching**: `SaveReport()` uses pgvector `<=>` cosine distance to find the nearest section across all existing sections in the same board. Distance < 0.3 → set `prev_section_id` and `status='continuing'`.
- **Historical data backfill**: All existing 315 sections get embeddings generated and `prev_section_id` populated via the same pgvector matching.
- Populate `prev_thread_id`: `matchPreviousThreads()` in `generator.go` will now assign `prev_thread_id` using the matched previous thread's DB ID from the `daily_report_threads` table.
- **Thread generation writes to new table**: `GenerateDailyReport` pipeline persists threads as rows in `daily_report_threads` instead of JSON into sections.
- **Data migration**: Extract existing JSON threads from `daily_report_sections.threads` into `daily_report_threads` rows. Historical `prev_thread_id` values left null (cannot retroactively match).
- **API updates**: Report detail API returns threads as structured objects with `id`, `prev_thread_id`, `report_id`, `section_id`. New endpoint `GET /api/boards/:id/thread-timeline` returns all threads for a board across days for Gantt visualization.
- **Frontend Thread detail panel (View A)**: Within the newspaper modal, clicking a thread opens a side panel showing a vertical timeline of that thread's lineage chain across days.
- **Frontend Board thread browser (View B)**: New view accessible from the board's daily report area, showing a Gantt-chart timeline with dates as columns, thread lineage chains as rows, nodes colored by status.

## Capabilities

### New Capabilities
- `thread-storage`: Independent `daily_report_threads` table, GORM model, repository CRUD, data migration from JSON, and removal of the `daily_report_sections.threads` JSON column.
- `thread-lineage`: Population of `prev_thread_id` in `matchPreviousThreads()` using DB IDs, API endpoints for thread chain traversal and board-level thread timeline, and frontend views for thread detail timeline (newspaper modal side panel) and board-level Gantt-chart thread browser.

### Modified Capabilities
- `daily-report-system`: Section lineage matching switches from tag Jaccard to embedding-based pgvector cosine distance; `daily_report_sections` gains `embedding vector(2560)` column; `SaveReport()` performs DB-side matching; thread generation pipeline changes from JSON embedding to independent table writes; report detail API response shape changes (threads become top-level objects with IDs instead of inline JSON); `matchPreviousThreads()` now assigns `prev_thread_id`; historical section embedding backfill.

## Impact

- **Database**: New `daily_report_threads` table (migration `20260529_*`). Column `daily_report_sections.threads` dropped after migration. Data migration extracts existing JSON threads.
- **Backend**: `models.go` (new `DailyReportThread` GORM model), `generator.go` (thread persistence + `prev_thread_id` assignment), `repository.go` (new SaveReport flow with thread rows, new query functions), `handler.go` (updated API responses + new endpoints).
- **Frontend API types**: `dailyReports.ts` — `DailyReportThread` gains `id`, `prev_thread_id`, `report_id`, `section_id`. `DailyReportSection.threads` type shifts to reference these enriched objects.
- **Frontend components**: `BoardDailyReportTimeline.vue` (thread click opens lineage panel), new `ThreadLineagePanel.vue` component, new `BoardThreadBrowser.vue` component with Gantt-chart layout.
- **No external dependencies added**.
