## 1. Data Layer

- [x] 1.1 Create `board_concepts` table migration: id, name, description, embedding (pgvector 1536), scope_type, scope_category_id, is_system, is_active, display_order, timestamps
- [x] 1.2 Add `board_concept_id` (nullable INT) to `narrative_boards` table with foreign key
- [x] 1.3 Add `is_system` (BOOLEAN DEFAULT false) column to `narrative_boards` table if not present
- [x] 1.4 Insert `narrative_board_embedding_threshold` = 0.7 into `ai_settings` table
- [x] 1.5 Insert `narrative_board_hotspot_threshold` = 6 into `ai_settings` table
- [x] 1.6 Add GORM model `BoardConcept` in `internal/domain/models/`

## 2. Board Concept Management Backend

- [x] 2.1 Create `internal/domain/narrative/concept_service.go`: CRUD for board concepts (ListActive, Create, Update, Deactivate)
- [x] 2.2 Create `internal/domain/narrative/concept_embedding.go`: embedding generation via existing embedding service for concept name+description
- [x] 2.3 Create `internal/domain/narrative/concept_handler.go`: REST endpoints (GET/POST/DELETE /api/narratives/board-concepts)
- [x] 2.4 Register routes in `RegisterNarrativeRoutes`
- [x] 2.5 LLM cold-start: create `concept_suggestion.go` — LLM scans active abstract tags, suggests board concepts as JSON

## 3. Tag-to-Board Matching Engine

- [x] 3.1 Create `internal/domain/narrative/concept_matcher.go`: embedding matching logic
  - [x] 3.1.1 Read threshold from `ai_settings`
  - [x] 3.1.2 Generate embedding for input tag (label + description)
  - [x] 3.1.3 Cosine similarity against all active `board_concepts.embedding`
  - [x] 3.1.4 Return best match if ≥ threshold, else nil
- [x] 3.2 Create `internal/domain/narrative/concept_matcher_test.go`: table tests for matching threshold, no-match, edge cases
- [x] 3.3 Unclassified bucket: collect unmatched tags, trigger LLM suggestion when count > 5

## 4. Daily Generation Flow Refactor

- [x] 4.1 Modify `GenerateAndSaveForCategory` to implement dual-track:
  - [x] 4.1.1 Split abstract trees by node count (≥N → hotspot, <N → matching pool)
  - [x] 4.1.2 Hotspot track: reuse `createBoardFromAbstractTree` with `is_system=true`, `board_concept_id=NULL`
  - [x] 4.1.3 Matching track: route small trees + unclassified events through `concept_matcher`
  - [x] 4.1.4 For matched concepts: create/update daily `narrative_boards` with `board_concept_id`
  - [x] 4.1.5 For unmatched: add to unclassified bucket
- [x] 4.2 Modify `GenerateAndSave` (global scope):
  - [x] 4.2.1 Remove call to `MergeGlobalBoards` / stop invoking `board_merge.go`
  - [x] 4.2.2 Global concept boards handle cross-category tag matching via embedding
- [x] 4.3 Modify prev_board matching: concept boards match by `board_concept_id`, hotspot boards match by `abstract_tag_id`
- [x] 4.4 Ensure `board_narrative_generator.go` receives concept name/description as context when board has `board_concept_id`

## 5. Frontend Board Concept Management

- [x] 5.1 Create `front/app/api/boardConcepts.ts`: API client for board concept CRUD and LLM suggestions
- [x] 5.2 Create `front/app/features/topic-graph/components/BoardConceptManager.vue`: management UI
  - [x] 5.2.1 Active concept list with name, description, match count
  - [x] 5.2.2 Accept/reject LLM suggestions section
  - [x] 5.2.3 Deactivate button
- [x] 5.3 Integrate `BoardConceptManager` into `TopicGraphPage.vue` or a modal

## 6. Frontend Narrative Panel Adaptations

- [x] 6.1 Update `NarrativePanel.vue` to load and display board concepts alongside hotspot boards
- [x] 6.2 Update `NarrativeBoardCanvas.client.vue`: distinguish concept boards (persistent style) from hotspot boards (is_system badge)
- [x] 6.3 Add "unclassified" section in `NarrativePanel.vue` showing unmatched tags
- [x] 6.4 Manual tag-to-concept assignment UI in unclassified section

## 7. Verification

- [x] 7.1 Backend: `go test ./internal/domain/narrative/...` — all tests pass
- [x] 7.2 Backend: `go build ./...` — no compilation errors
- [x] 7.3 Frontend: `pnpm exec nuxi typecheck` — no TypeScript errors
- [x] 7.4 Frontend: `pnpm test:unit` — all unit tests pass
- [ ] 7.5 Manual: cold-start LLM suggestion returns valid board concepts
- [ ] 7.6 Manual: daily generation produces boards grouped by concepts with cross-day links
- [ ] 7.7 Manual: embedding threshold change in ai_settings takes effect immediately
