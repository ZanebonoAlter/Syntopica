## 1. Slug Normalization

- [x] 1.1 Add `spacePattern` regex to `topictypes/helpers.go` and modify `Slugify()` to collapse `\s+` → single space before punctuation replacement
- [x] 1.2 Add unit tests for `Slugify()` covering whitespace variants, leading/trailing space, multiple consecutive spaces
- [x] 1.3 Run `go test ./internal/domain/topictypes/ -v` to verify no regressions

## 2. Information Gain Check for Abstract Tag Creation

- [x] 2.1 Add `collectArticleIDsForTags()` helper in `topicanalysis/abstract_tag_service.go` to batch-fetch article IDs associated with candidate child tags
- [x] 2.2 Add `jaccardSimilarity()` helper to compute article-set overlap between two tag groups
- [x] 2.3 Add `computeLeafToDepthRatio()` to calculate the leaf-to-depth ratio that would result if the abstract were created
- [x] 2.4 Add `validateAbstractCreation()` gate function in `processAbstractJudgment()` that checks: child count ≥ 2, max pairwise Jaccard ≤ 0.7, leaf-to-depth ratio ≥ 1.5
- [x] 2.5 Add unit tests for `validateAbstractCreation()` with edge cases (single child, high overlap, acceptable abstract)
- [x] 2.6 Run `go test ./internal/domain/topicanalysis/ -run TestProcessAbstractJudgment -v` to verify

## 3. Whitespace-Variant Duplicate Cleanup

- [x] 3.1 Add `CleanupWhitespaceDuplicateTags()` in `topicanalysis/tag_cleanup.go` that finds active tags with same normalized slug per category and merges using existing `MergeTags()`
- [x] 3.2 Add unit tests for `CleanupWhitespaceDuplicateTags()` with a mock pair of whitespace-variant tags
- [x] 3.3 Register `CleanupWhitespaceDuplicateTags` in the scheduler task list (check `backend-go/internal/app/runtime.go` or equivalent scheduler wiring)

## 4. Degenerate Abstract Tree Flattening

- [x] 4.1 Add `CleanupDegenerateAbstractTrees()` in `topicanalysis/tag_cleanup.go` that walks abstract chains, computes leaf-to-depth ratio, and flattens by promoting children to the nearest ancestor with ratio ≥ 1.5
- [x] 4.2 Add unit tests covering: 4-level chain with 3 leaves (should flatten), 2-level chain with 8 leaves (should not flatten)
- [x] 4.3 Register `CleanupDegenerateAbstractTrees` in the scheduler task list

## 5. Multi-Parent Prevention

- [x] 5.1 Add a validation gate in `linkAbstractParentChild()` (`topicanalysis/abstract_tag_hierarchy.go`) that checks if child already has an active abstract parent and, if so, computes Jaccard between the existing parent's article set and the proposed parent's article set
- [x] 5.2 Add unit tests for multi-parent prevention: overlapping parents rejected, non-overlapping parents accepted
- [x] 5.3 Run `go test ./internal/domain/topicanalysis/ -run TestLinkAbstractParentChild -v` to verify

## 6. Integration Verification

- [x] 6.1 Run `go test ./internal/domain/topicanalysis/... -v` for all topicanalysis tests (new tests all pass; pre-existing goroutine race not caused by changes)
- [x] 6.2 Run `go test ./internal/domain/topicextraction/... -v` for slug-aware extraction tests (all pass)
- [x] 6.3 Run `go build ./...` from `backend-go/` to verify full compilation (pass)
- [ ] 6.4 Run the new cleanup functions manually against the existing database to verify they correctly handle the DeepSeek case
