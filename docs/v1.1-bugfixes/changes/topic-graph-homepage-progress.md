# Topic Graph Homepage Progress

## Status

- Phase: implementation
- Goal: build a dedicated 3D topic graph page backed by digest-window AI summaries

## Work Log

- 2026-03-11: confirmed product direction with user: topic-centric graph, direct 3D page first
- 2026-03-11: inspected current digest pipeline and verified daily/weekly windows are based on `AISummary.created_at`
- 2026-03-11: drafted implementation plan in `docs/plans/2026-03-11-topic-graph-homepage.md`
- 2026-03-11: added backend `topicgraph` domain with extractor tests, graph/detail handlers, and `/api/topic-graph/*` routes
- 2026-03-11: added frontend `主题图谱` entry, `/topics` page, 3D force-graph canvas, and topic detail sidebar
- 2026-03-11: installed `3d-force-graph`, `three`, and `three-spritetext` in `front/package.json`
- 2026-03-12: added persisted topic tag storage via `topic_tags` and `ai_summary_topics`
- 2026-03-12: wired best-effort topic tagging into queue summaries and auto-summary generation so new summaries persist tags without blocking the main summary flow
- 2026-03-12: topic detail now includes related article links plus app/search actions for digest and YouTube exploration
- 2026-03-12: optimized graph load path by removing read-time topic persistence from `topicgraph` queries and batching article lookups for topic detail payloads
- 2026-03-12: optimized frontend first paint so graph render no longer waits for the follow-up topic detail request
- 2026-03-12: redesigned the topic graph page for widescreen usage with a left-side stat rail, cleaner control header, and sticky reading rail
- 2026-03-12: reduced 3D graph label clutter by showing labels only for featured/active nodes and increasing force spacing
- 2026-03-12: raised related articles above summaries and reused article-style markdown typography for topic summary reading
- 2026-03-11: verification results
  - `go test ./internal/domain/topicgraph -v` PASS
  - `pnpm test:unit -- app/features/topic-graph/utils/buildTopicGraphViewModel.test.ts` PASS
  - `go test ./internal/domain/summaries -run SubmitBatch -v` PASS
  - `go test ./internal/jobs -run AutoSummary -v` PASS
  - `pnpm exec nuxi typecheck` PASS after fixing the existing `feed_ids` type mismatch in `front/app/stores/api.ts`
  - `go test ./...` still blocked by existing failures in `backend-go/internal/domain/digest/*`

## Next Slice

- add an explicit admin/backfill action for old summaries that do not have persisted tags yet
- evolve the topic extractor from JSON-only prompt to a stronger schema with aliases and topic descriptions
- decide whether topic clicks should deep-link into article reading state instead of external article URLs
