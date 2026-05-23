## 1. Database Migration

- [x] 1.1 Add `abstract_tag_id` column to `narrative_boards` table (nullable uint FK → topic_tags.id, ON DELETE SET NULL)

## 2. Backend — New Board Creation Logic

- [x] 2.1 Implement `createBoardFromAbstractTree(tree AbstractTreeNode, date time.Time, categoryID uint)` — creates a NarrativeBoard from one abstract tree with deterministic naming and event_tag_ids derived from tree's child tags
- [x] 2.2 Implement `collectBoardEventTagIDs(tree AbstractTreeNode)` — traverses tree children, collects IDs of all event-category tags active on the given date
- [x] 2.3 Implement `matchPreviousBoard(abstractTagID uint, date time.Time)` — queries yesterday's Boards by abstract_tag_id for prev_board_ids matching
- [x] 2.4 Implement `createMiscBoardsFromEvents(events []TagInput, date time.Time, categoryID uint)` — creates Board(s) for unclassified events: ≤3 events creates single "其他动态" Board; >3 calls LLM via `partitionMiscEvents()`
- [x] 2.5 Implement `partitionMiscEvents(ctx, events, prevDayBoards)` — simplified LLM prompt that groups unclassified events into named Boards (reuses `boardPartitionSystemPrompt` with trimmed context)
- [x] 2.6 Wire the new board creation into `GenerateAndSaveForCategory` to replace the old high/low volume split
- [x] 2.7 Delete `generateDirectForLowVolume()` from `service.go`
- [x] 2.8 Delete `generateAbstractTagCardsForBoards()` and related helpers (`mapAbstractTagToNarrativeCard`, `computeAbstractTagStatus`) from `board_generator.go`
- [x] 2.9 Delete `GenerateNarrativesFromAbstractTrees()` and `GenerateNarrativesFromUnclassifiedEvents()` from `generator.go`
- [x] 2.10 Delete `GenerateCrossCategoryNarratives()` (already deprecated) and related constants from `generator.go`
- [x] 2.11 Update `GenerateAndSave()` main flow — remove calls to deleted functions, ensure `runFallbackAssociations`, `DeriveBoardConnections`, `runFeedbackFromTodayNarratives` still work with the new board-first model
- [x] 2.12 Update `getBoardTimeline` API — join `abstract_tag_id` to include abstract tag label/slug in BoardSummaryItem for frontend use
- [x] 2.13 Update `RegenerateAndSave` / `RegenerateAndSaveForCategory` — ensure they delete boards+narratives correctly, rebuild via new unified flow

## 3. Backend — Data Model Adjustments

- [x] 3.1 Update `NarrativeBoard` model in `models/narrative_board.go` — add `AbstractTagID *uint` field with GORM tags
- [x] 3.2 Update `BoardSummaryItem` in `service.go` — add `AbstractTagID *uint` and `AbstractTagSlug string` fields
- [x] 3.3 Run `go test ./internal/domain/narrative/...` and fix any test breakage

## 4. Frontend — Delete Legacy Code

- [x] 4.1 Delete `NarrativeCanvas.client.vue`
- [x] 4.2 Remove its import from `NarrativePanel.vue`

## 5. Frontend — Simplify NarrativePanel.vue

- [x] 5.1 Delete `boardMode` ref and the mode toggle button in the template
- [x] 5.2 Delete `scopeMode`/`selectedCategoryId`/`scopeCategories`/`scopesLoading`/`categoryTimelineDays`/`timelineDays` (all legacy timeline state)
- [x] 5.3 Delete `loadTimeline()`, `loadScopes()`, `loadCategoryTimeline()` functions
- [x] 5.4 Delete `switchScope()`, `selectCategory()`, `backToCategoryList()` functions
- [x] 5.5 Delete legacy template blocks: scope switcher for timeline mode, category list for timeline mode, category detail canvas for timeline mode
- [x] 5.6 Rename `boardScopeMode` → `scopeMode`, `boardSelectedCategoryId` → `selectedCategoryId`
- [x] 5.7 Rename `switchBoardScope` → `switchScope`, `selectBoardCategory` → `selectCategory`, `backToBoardCategoryList` → `backToCategoryList`
- [x] 5.8 Unify the `watch` on `props.date` — call only `loadBoardTimeline()` and `loadScopes()` when scope is category
- [x] 5.9 Unify `triggerGeneration` — call `regenerateNarratives` with correct scope args, then reload board timeline (not separate loadTimeline/loadCategoryTimeline)
- [x] 5.10 Ensure `totalCount`, `allBoardNarratives`, `selectedNarrative` computed properties work with single data source

## 6. Frontend — Tag Click Behavior

- [x] 6.1 In `NarrativeDetailCard`, add abstract tag detection: if tag category is event/person/keyword and it belongs to an abstract tree, highlight rather than emit
- [x] 6.2 Implement board scroll-to/expand when abstract tag is clicked: search `boardTimelineDays` for Board with matching `abstract_tag_id`, expand it
- [x] 6.3 Update `TopicGraphPage.vue` `handleNarrativeTagSelect` — if tag is abstract and same-date Board exists, navigate within panel instead of calling `handleTagSelect`

## 7. Verification

- [x] 7.1 Run `go build ./...` in `backend-go/` — no compile errors
- [x] 7.2 Run `go test ./...` in `backend-go/` — all existing tests pass (358/362, 4 pre-existing failures in other packages)
- [x] 7.3 Run `pnpm exec nuxi typecheck` in `front/` — no type errors
- [x] 7.4 Run `pnpm test:unit` in `front/` — all existing tests pass (46/47, 1 pre-existing failure)
- [x] 7.5 Run `pnpm build` in `front/` — production build succeeds
- [ ] 7.6 Manual test: trigger narrative generation, verify each abstract tree becomes a Board with correctly grouped narratives
- [ ] 7.7 Manual test: verify misc events are partitioned and have narratives
- [ ] 7.8 Manual test: verify frontend Board navigation works (global → category → Board expand → narrative detail)
