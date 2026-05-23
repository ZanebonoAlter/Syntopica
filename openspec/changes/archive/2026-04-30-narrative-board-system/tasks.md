## 1. Data Model & Infrastructure

- [x] 1.1 Create `models/narrative_board.go` — NarrativeBoard model with fields: ID, PeriodDate, Name, Description, ScopeType, ScopeCategoryID, EventTagIDs (JSON), AbstractTagIDs (JSON), PrevBoardIDs (JSON), CreatedAt
- [x] 1.2 Add `BoardID *uint` and expand `Source` values to include `'abstract'` in `models/narrative.go`
- [x] 1.3 Run auto-migration to create narrative_boards table and add board_id column to narrative_summaries
- [x] 1.4 Add API routes in `internal/app/router.go` for board endpoints: GET /api/narratives/boards/timeline, GET /api/narratives/boards/:id

## 2. Board Collection & Input

- [x] 2.1 Create `narrative/board_collector.go` — CollectUnclassifiedEventTagsByCategory (adapt from existing collector.go)
- [x] 2.2 Add CollectAbstractTreeInputsByCategory (reuse existing logic, adapt to return flat list for board input)
- [x] 2.3 Add CollectPreviousDayBoards — query narrative_boards for previous day within same scope, return [{id, name, description}]
- [x] 2.4 Add CollectPreviousBoardNarratives — given prev_board_ids, collect their narratives for Pass 1 context

## 3. LLM Pass 0 — Board Partitioning

- [x] 3.1 Create `narrative/board_generator.go` — Define board partitioning system prompt: instruct LLM to partition events into boards, assign abstract tags, annotate prev_board_ids, enforce one-event-per-board constraint
- [x] 3.2 Implement buildBoardPartitionPrompt — format events (with descriptions), abstract tags (with descriptions + child summaries), and previous day's boards as user message
- [x] 3.3 Implement parseBoardPartitionResponse — parse LLM JSON output into []BoardPartition, validate tag IDs exist, enforce one-event-per-board constraint in code
- [x] 3.4 Implement GenerateBoardsForCategory — orchestrate collection → prompt → LLM call → parse → save narrative_boards records

## 4. Abstract Tag Narrative Cards

- [x] 4.1 Implement mapAbstractTagToNarrativeCard — convert abstract tag to NarrativeOutput with source='abstract', title=label, summary=description, related_tag_ids=child IDs
- [x] 4.2 Implement computeAbstractTagStatus — query child tag article counts over 3-day window, return emerging/continuing/ending
- [x] 4.3 Integrate abstract tag cards into board flow — after board creation, map assigned abstract tags to narrative cards and save

## 5. LLM Pass 1 — Per-Board Narrative Generation

- [x] 5.1 Adapt narrative system prompt for board-scoped generation — prompt includes board context (name, description), board's event tags with descriptions, abstract tag cards as reference, previous board's narratives
- [x] 5.2 Implement GenerateNarrativesForBoard — single LLM call per board with board-scoped context and previous narratives
- [x] 5.3 Save narratives with board_id — all generated narratives (LLM + abstract cards) saved with board_id reference

## 6. Global Board Merge

- [x] 6.1 Implement CollectAllCategoryBoards — load all boards for a date across categories
- [x] 6.2 Create global merge LLM prompt — show all category boards, ask LLM which should be merged
- [x] 6.3 Implement parseGlobalMergeResponse — parse merge decisions, validate board IDs
- [x] 6.4 Implement MergeGlobalBoards — create global boards, reassign narratives' board_id, mark merged category boards

## 7. Fallback & Post-Processing

- [x] 7.1 Implement fallbackNarrativeAssociation — for narratives with unresolvable parent_ids, call LLM with full context, retry up to 3 times
- [x] 7.2 Implement deriveBoardConnections — iterate narratives, find parent_ids pointing to narratives in different boards, compute board-to-board edges
- [x] 7.3 Adapt feedbackNarrativesToTags — keep existing logic, ensure it works with board_id field

## 8. Main Service Rewrite

- [x] 8.1 Rewrite `NarrativeService.GenerateAndSaveForCategory` — Phase 1: collect inputs → threshold check → Pass 0 (boards) → Pass 1 (per-board narratives) → save
- [x] 8.2 Rewrite `NarrativeService.GenerateAndSave` — call per-category generation → global merge → fallback → derive connections
- [x] 8.3 Remove legacy code: GenerateCrossCategoryNarratives, GenerateWatchedTagNarratives, old Pass 2 direct event narrative generation
- [x] 8.4 Rewrite `jobs/narrative_summary.go` scheduler — update to use new flow

## 9. API Handlers

- [x] 9.1 Implement getBoardTimeline handler — return boards grouped by day with narrative counts and aggregate statuses
- [x] 9.2 Implement getBoardDetail handler — return single board with full narratives (LLM + abstract cards)
- [x] 9.3 Update getNarratives handler — support board_id filter parameter
- [x] 9.4 Update getNarrativeTimeline handler — return board-grouped timeline when boards exist

## 10. Frontend — Board Canvas

- [x] 10.1 Create NarrativeBoardCanvas.client.vue — P5.js canvas rendering board nodes as large collapsible rectangles
- [x] 10.2 Implement board node rendering — large rectangle with board name, narrative count, status badge, fold/unfold state
- [x] 10.3 Implement board drill-down — click board node toggles expanded state, renders narrative sub-nodes within board boundary
- [x] 10.4 Implement narrative sub-node rendering — smaller cards within expanded board, abstract tag cards with dashed border
- [x] 10.5 Implement narrative-level edges — bezier lines connecting narrative nodes across days, derived from parent_ids
- [x] 10.6 Implement board-level background edges — subtle lines between board boundaries derived from narrative connections
- [x] 10.7 Implement board status aggregation — compute aggregate board status from narrative statuses

## 11. Frontend — Panel Integration

- [x] 11.1 Update NarrativePanel.vue — replace NarrativeCanvas with NarrativeBoardCanvas, add board API calls
- [x] 11.2 Update API layer in topicGraph.ts — add board timeline and detail API methods
- [x] 11.3 Implement scope filter for boards — toggle global vs per-category boards
- [x] 11.4 Update NarrativeDetailCard.vue — handle abstract tag card display (source='abstract' styling)

## 12. Verification

- [ ] 12.1 Backend tests — unit tests for board partitioning, global merge, abstract tag mapping, status computation
- [x] 12.2 Backend build — `cd backend-go && go build ./... && go test ./...`
- [x] 12.3 Frontend typecheck — `cd front && pnpm exec nuxi typecheck`
- [x] 12.4 Frontend build — `cd front && pnpm build`
- [ ] 12.5 Manual E2E — trigger narrative generation, verify boards created, narratives grouped, canvas renders correctly
