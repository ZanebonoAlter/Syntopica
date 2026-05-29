# Code Review: tag-cleanup LLM batch optimization

**Date:** 2026-04-26
**Files reviewed:** 4
**Depth:** standard

---

## 1. `batchGenerateTagDescriptions` silently drops descriptions > 500 chars instead of truncating

**File:** `backend-go/internal/domain/topicextraction/tagger.go:927`
**Severity:** MEDIUM
**Type:** Logic bug — inconsistent behavior

The batch result filter silently discards any description exceeding 500 runes:

```go
results := make(map[uint]string)
for _, d := range parsed.Descriptions {
    if d.Description != "" && len([]rune(d.Description)) <= 500 {
        results[d.ID] = d.Description
    }
}
```

Compare with the single-tag path (`generateTagDescription` at line 681), which **truncates** instead of discarding:

```go
if len([]rune(desc)) > 500 {
    desc = string([]rune(desc)[:500])
}
```

This means tags that get slightly long descriptions from the LLM in batch mode are completely ignored rather than saved with a truncated description. Over time, these tags will never get descriptions because `BackfillMissingDescriptions` will keep finding them without descriptions and re-querying them, but the batch LLM might keep returning long descriptions for them.

**Fix:** Truncate instead of dropping:
```go
for _, d := range parsed.Descriptions {
    if d.Description != "" {
        desc := d.Description
        if len([]rune(desc)) > 500 {
            desc = string([]rune(desc)[:500])
        }
        results[d.ID] = desc
    }
}
```

---

## 2. Batch-updated descriptions don't trigger embedding refresh

**File:** `backend-go/internal/domain/topicextraction/description_backfill.go:41-50`
**Severity:** MEDIUM
**Type:** Missing side-effect — behavioral regression

In `BackfillMissingDescriptions`, after saving a description, there's no embedding enqueue:

```go
results := batchGenerateTagDescriptions(batch)
for _, tag := range batch {
    if desc, ok := results[tag.ID]; ok {
        if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).
            Update("description", desc).Error; err != nil {
            logging.Warnf(...)
        } else {
            processed++
        }
        // ← no embedding enqueue here
    }
}
```

Compare with the single-tag path in `generateTagDescription` (line 690-693), which always enqueues after saving:

```go
qs := getEmbeddingQueueService()
if err := qs.Enqueue(tagID); err != nil {
    logging.Warnf("Failed to enqueue re-embedding after description update for tag %d: %v", tagID, err)
}
```

Tags updated via batch will have stale embeddings that don't reflect their new description text. This defeats the purpose of generating descriptions for search/matching, since the embedding won't represent the description content.

Similarly, the single-tag fallback path in `batchGenerateTagDescriptions` (line 833) calls `generateTagDescription` directly which DOES enqueue, so single-tag batches work correctly. This inconsistency makes the bug harder to notice in testing.

**Fix:** Add embedding enqueue after successful description update in `BackfillMissingDescriptions`:
```go
if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).
    Update("description", desc).Error; err != nil {
    logging.Warnf("description backfill: failed to update tag %d: %v", tag.ID, err)
} else {
    processed++
    qs := getEmbeddingQueueService()
    if err := qs.Enqueue(tag.ID); err != nil {
        logging.Warnf("description backfill: failed to enqueue embedding for tag %d: %v", tag.ID, err)
    }
}
```

---

## 3. `batchGenerateTagDescriptions` single-tag path returns nil, hiding results from caller

**File:** `backend-go/internal/domain/topicextraction/tagger.go:829-835`
**Severity:** LOW
**Type:** Inconsistent contract

When `len(tags) == 1`, the function delegates to `generateTagDescription` (which writes to DB directly) and returns `nil`:

```go
if len(tags) == 1 {
    articleContext := buildArticleContextForTag(tags[0].ID)
    if articleContext == "" {
        return nil
    }
    generateTagDescription(tags[0].ID, tags[0].Label, tags[0].Category, articleContext)
    return nil
}
```

The caller in `BackfillMissingDescriptions` checks `results[tag.ID]`, so the single tag is never counted in `processed`. The description IS saved (by `generateTagDescription`), but the reported count is wrong.

Impact is low — this only affects the return count, not the actual functionality. But it means `BackfillMissingDescriptions` will report `processed=0` when there's exactly one tag needing description.

**Fix:** Return a map with the tag ID (even though the description is already saved) or restructure to not special-case single tags:
```go
if len(tags) == 1 {
    // Still delegate to single-tag path, but return a marker so caller counts it
    articleContext := buildArticleContextForTag(tags[0].ID)
    if articleContext == "" {
        return nil
    }
    generateTagDescription(tags[0].ID, tags[0].Label, tags[0].Category, articleContext)
    // Return non-nil so caller counts this tag
    return map[uint]string{tags[0].ID: "(single-tag path)"}
}
```

Or simpler: just remove the special case and let single tags go through the batch path too.

---

## 4. Batch path loses person tag structured metadata

**File:** `backend-go/internal/domain/topicextraction/tagger.go:861-873`
**Severity:** LOW
**Type:** Behavioral inconsistency (may be intentional)

The batch LLM prompt generates generic descriptions for all tags. But the single-tag path (`generateTagDescription` → `generatePersonTagDescription`) extracts structured `person_attrs` (country, organization, role, domains) and saves them as tag metadata.

Person tags processed via batch will get a plain description but won't get their structured attributes populated. This means `BackfillMissingDescriptions` won't fully enrich person tags — they'll miss the metadata that the UI might display.

This may be intentional (batch mode is for quick description backfill, not full enrichment), but it's worth documenting.

---

## 5. `buildArticleContextForTag` queries `articles.description` but never uses it

**File:** `backend-go/internal/domain/topicextraction/description_backfill.go:67-68`
**Severity:** LOW
**Type:** Dead column in query

```go
var rows []articleRow
err := database.DB.Model(&models.ArticleTopicTag{}).
    Select("articles.title, articles.description").  // ← fetches description
    ...
```

But only `row.Title` is used in the loop at line 84-87. The `Description` field is fetched but discarded. Minor inefficiency.

**Fix:** Remove `articles.description` from the SELECT and from the struct, or use it in the context building.

---

## 6. `isDirectParentChild` and `abs` — no production callers

**File:** `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go:1116-1121, 1123-1128`
**Severity:** LOW
**Type:** Unused utility functions

Both functions have test coverage in `hierarchy_cleanup_test.go` but no production callers. `isDirectParentChild` was likely used by the old `cleanupDeepHierarchyTree` path that was replaced. `abs` was likely a utility for tree depth calculations. They can stay if you plan to use them, but they add maintenance surface area.

---

## 7. `queue_batch_processor.go` — no changes detected

**File:** `backend-go/internal/domain/topicanalysis/queue_batch_processor.go`
**Severity:** INFO

The `git diff` shows no modifications to this file in the current changeset. The `time` import is still used by `time.Now()` calls. If `time.Sleep` lines were removed in a prior commit, this file is clean.

---

## Positive observations

1. **Batch LLM call structure is sound.** The JSON schema matches the prompt, `SanitizeLLMJSON` is used correctly to handle LLM output quirks, and the `tagContext` struct properly marshals the per-tag data for the prompt.

2. **Virtual root handling in `reviewOneTree` is correct.** The `tagMap` builder at line 385 properly excludes `ID == 0` nodes, and the `isVirtual` flag at line 391 correctly allows all operations for virtual-rooted trees while protecting real root nodes.

3. **`mergeSmallTreesForReview` batching logic is correct.** The boundary checks happen before adding to the current batch, preventing off-by-one overflow of maxTrees or maxNodes limits.

4. **`createVirtualRoot` is safe.** It never gets saved to DB (no `database.DB.Create` call touches it), and all downstream code checks for `ID == 0`.

5. **The refactoring from `cleanupDeepHierarchyTree` to `reviewForestBatched` + `reviewOneTree` is clean.** The old monolithic approach was replaced with composable pieces (batch merging, virtual roots, individual tree review) that work together correctly.

6. **Error handling in `reviewOneTree` is thorough.** Each merge/move/abstract operation is validated, and failures are appended to `result.Errors` without stopping the entire review — good resilience for a batch operation.
