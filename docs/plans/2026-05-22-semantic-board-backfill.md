# Semantic Board Backfill Plan

## Scope

- Implement OpenSpec `semantic-label-board-system` phase 6 tasks `6.1`-`6.4`.
- Keep the implementation service-scoped; API wiring is deferred to phase 8.
- Do not add a persistent backfill table in this phase because the accepted data-model tasks did not introduce one.

## Approach

1. Add `SemanticBoardBackfillService` with `all`, `unassigned`, and `board` modes.
2. Use an in-memory job registry for async execution, progress, and failure records.
3. Reuse `SemanticBoardMatchingService.MatchTopicTag` so each processed tag rewrites `topic_tag_board_labels` with current matching config.
4. For `board` mode, process tags currently assigned to the target board plus tags that match the target board through the same direct/indirect matching rules.
5. Treat `board` mode as an affected-tag backfill: each selected tag is fully rematched across all active SemanticBoards, not patched only for the target board.
6. Add focused tests for full backfill, unassigned-only, board-scoped, idempotent reruns, and failure recording.

## Verification

- `rtk go test ./internal/domain/tagging -run TestSemanticBoardBackfill -v`
