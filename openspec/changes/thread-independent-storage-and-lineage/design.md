## Context

The daily report system generates narrative threads per cluster section, stored as rows in `daily_report_threads` with cross-day lineage via `prev_thread_id`. Section-level lineage uses `prev_section_id` on `daily_report_sections`.

### The Problem: Tag-based Matching is Broken

The original `matchPreviousSections()` used `cluster_tag_ids` Jaccard similarity to link sections across days. This approach **completely fails** in practice because:

1. **Tag IDs are ephemeral**: Each day's clustering generates brand-new tag IDs. For example, board 4393's sections have tag sets `[96329, 100448, 100791]` (05-26), `[102475]` (05-28), `[109520]` (05-31) — zero overlap across days.
2. **Tags get deleted**: Older tags (96329, 100448, etc.) no longer exist in `topic_tags`, so even retrospective matching is impossible.
3. **Thread tag_ids are mostly empty**: Most threads have `tag_ids=[]`, making thread-level tag matching useless too.

Despite visually obvious continuity ("山西沁源留神峪煤矿瓦斯爆炸事故" on 05-26 → "国务院留神峪煤矿事故调查组公告" on 05-31), the system cannot detect any linkage.

### The Solution: Embedding-based Semantic Matching

Using pgvector on `daily_report_sections.embedding` (2560-dim, same model as `topic_tag_embeddings`):

- Section embedding text: `cluster_label` (short, semantically dense)
- Matching: `embedding <=> $current_embedding` cosine distance, threshold < 0.3
- Scope: ALL sections in the same board (no time restriction)
- Executed: DB-side in `SaveReport()` transaction
- Backfill: Existing 315 sections get embeddings via batch job

Verified on production data: distance < 0.3 correctly links "朗维尤市日资企业工厂爆炸" to "留神峪煤矿瓦斯爆炸" variants, while keeping unrelated events separate.

## Goals / Non-Goals

**Goals:**
- Give every thread a unique, persistent database identity via `daily_report_threads` table
- Populate `prev_thread_id` during generation so threads form a linked list across days
- **Replace tag Jaccard matching with embedding-based semantic matching for section lineage**
- **Add `embedding vector(2560)` column to `daily_report_sections` and generate embeddings during report generation**
- **Backfill embeddings for all existing sections**
- Provide API endpoints for thread lineage chain retrieval and board-level thread timeline
- Build frontend views: (A) thread lineage timeline within newspaper modal, (B) board-level Gantt-chart thread browser
- Migrate existing JSON thread data to the new table without data loss

**Non-Goals:**
- Redesigning thread status values (emerging/continuing/splitting/merging/ending remain unchanged)
- Thread-level embedding matching (will use title+summary embedding in future change; tag overlap remains for threads)
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

### 3. Section lineage: embedding-based semantic matching (replacing tag Jaccard)

**Decision**: Replace `matchPreviousSections()` tag Jaccard logic with pgvector cosine distance matching on `daily_report_sections.embedding`. Embedding text = `cluster_label`. Matching scope = all sections in the same board (not just previous report). Threshold = cosine distance < 0.3.

**Rationale**: Tag-based matching is provably broken — tag IDs have zero cross-day overlap because daily clustering generates fresh tags, and older tags get deleted. Embedding matching directly captures semantic continuity regardless of tag identity. Verified on production data: distance < 0.3 correctly links same-event sections across days.

**Implementation**:
1. Add `embedding vector(2560)` column to `daily_report_sections` (migration)
2. After LLM section generation, batch-embed all `cluster_label` texts in one API call
3. Store sections with embeddings in `SaveReport()`
4. Within same transaction, use pgvector `<=>` to find nearest neighbor for each new section among all existing sections in the board (excluding current report)
5. If distance < 0.3, set `prev_section_id` and `status='continuing'`
6. `findPreviousSections()` and `matchPreviousSections()` in Go are replaced by a single repository function `MatchSectionsByEmbedding()`

**Alternative considered**: Use existing `topic_tag_embeddings` to compute section similarity indirectly. Rejected because (a) old tags are deleted so their embeddings are gone, (b) new tags may not have embeddings yet, (c) tag embedding ≠ section semantic.

### 3b. Thread lineage: keep tag-based matching (unchanged)

**Decision**: Thread-level matching continues to use tag overlap via `matchPreviousThreads()`. Thread embedding is deferred to a future change.

**Rationale**: Threads average 326 per report (max 4089). Embedding all threads would significantly increase API cost and storage. Section-level matching already provides the primary Gantt chart data. Thread embedding can be added incrementally later using `title + " " + summary` as embedding text.

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
- **[Risk] Section embedding threshold may need tuning** → Mitigation: Verified threshold 0.3 on production data (same-event sections at 0.22-0.29, unrelated events at >0.4). Can be adjusted via constant. Historical sections with no embedding will be backfilled.
- **[Risk] Embedding API cost increase** → Mitigation: ~9 sections per report average, minimal cost. Batch API reduces HTTP overhead. Max case (47 sections) still reasonable.
- **[Risk] Thread matching quality — tag overlap may produce false lineage links** → Mitigation: Thread embedding is planned for a future change. Current tag-based matching is a known limitation.
- **[Risk] Board thread timeline query performance on boards with long history** → Mitigation: `days` parameter capped at 30. Indexes on `report_id`, `prev_thread_id`, and `board_daily_reports.semantic_board_id` keep queries fast.
- **[Trade-off] Historical sections will have `prev_section_id` populated after backfill** → The backfill job will embed all existing 315 sections and run the same pgvector matching, establishing lineage chains for historical data.
- **[Trade-off] Historical threads have no `prev_thread_id`** → Accepted. Thread embedding matching is a separate future change. The UI will simply show these as chain-starting nodes.
- **[Trade-off] Frontend Gantt chart is a custom component, not a charting library** → Accepted for now. Thread counts per board are small enough that a CSS grid/Flexbox solution works. Can upgrade to a library later if needed.
