# Co-tag Event Matching + Hierarchy C+D Cleanup

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve event tag matching accuracy via co-tag graph traversal, and fix abstract tag hierarchy depth issues with cross-layer dedup + depth limits.

**Architecture:** Two independent features. Feature A adds a co-tag graph traversal step in `findOrCreateTag` that expands event tag candidates using article co-occurrence before LLM judgment. Feature B adds cross-layer dedup and depth guards in `MatchAbstractTagHierarchy` and `linkAbstractParentChild`, and rewrites the periodic hierarchy cleanup to use the same C+D approach instead of the old tree-traversal prompt.

**Tech Stack:** Go, GORM, PostgreSQL with pgvector, LLM via airouter.

---

## Task 1: Co-tag Expansion Service — Core Function

**Files:**
- Create: `backend-go/internal/domain/topicanalysis/cotag_expansion.go`
- Create: `backend-go/internal/domain/topicanalysis/cotag_expansion_test.go`

**Context:** This is the core function that performs co-tag graph traversal for event tag matching. It has two modes:
- **Raw event tag** (from `tagArticle` path): uses the article's existing tags as co-tags
- **Abstract event tag**: aggregates keywords from all descendant event tags' articles, scaled by subtree depth

### Step 1: Write the test file skeleton

Create `backend-go/internal/domain/topicanalysis/cotag_expansion_test.go`:

```go
package topicanalysis

import (
	"testing"
)

func TestExpandEventCandidatesByArticleCoTags(t *testing.T) {
	// Integration test - requires database
	// Will be tested via cmd/co-tag-backfill or manual verification
}

func TestCalculateCoTagTopN(t *testing.T) {
	tests := []struct {
		name         string
		subtreeDepth int
		expected     int
	}{
		{"raw event tag depth 0", 0, 5},
		{"depth 1 abstract", 1, 5},
		{"depth 2 abstract", 2, 7},
		{"depth 3 abstract", 3, 9},
		{"depth 5 abstract", 5, 13},
		{"depth 10 capped", 10, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCoTagTopN(tt.subtreeDepth)
			if got != tt.expected {
				t.Errorf("calculateCoTagTopN(%d) = %d, want %d", tt.subtreeDepth, got, tt.expected)
			}
		})
	}
}

func TestAggregateKeywordsByChildCoverage(t *testing.T) {
	// Integration test - verify keyword aggregation by child tag coverage count
}
```

### Step 2: Run test to verify it compiles

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestCalculateCoTagTopN -v`
Expected: Compile error (calculateCoTagTopN not defined)

### Step 3: Write core implementation

Create `backend-go/internal/domain/topicanalysis/cotag_expansion.go`:

```go
package topicanalysis

import (
	"context"
	"fmt"
	"sort"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

const (
	coTagBaseTopN      = 5
	coTagDepthBonus    = 2
	coTagMaxTopN       = 15
	coTagMaxArticlesPerTag = 10
	coTagMaxCandidates = 20
)

func calculateCoTagTopN(subtreeDepth int) int {
	if subtreeDepth <= 1 {
		return coTagBaseTopN
	}
	n := coTagBaseTopN + (subtreeDepth-1)*coTagDepthBonus
	if n > coTagMaxTopN {
		return coTagMaxTopN
	}
	return n
}

type coTagCandidate struct {
	KeywordTagID uint
	KeywordLabel string
	Coverage     int
	ArticleCount int
}

// ExpandEventCandidatesByArticleCoTags finds additional event tag candidates
// by traversing the co-tag graph from a source article or abstract tag's children.
// For raw event tags: articleID must be set, abstractTagID = 0.
// For abstract event tags: abstractTagID must be set, articleID = 0.
// Returns TagCandidate slice suitable for merging into the LLM judgment pipeline.
func ExpandEventCandidatesByArticleCoTags(ctx context.Context, articleID uint, abstractTagID uint, existingCandidateIDs []uint) ([]TagCandidate, error) {
	var keywordTagIDs []uint
	var topN int

	if articleID > 0 {
		keywords := getTopArticleKeywords(articleID, coTagBaseTopN)
		for _, kw := range keywords {
			keywordTagIDs = append(keywordTagIDs, kw.TagID)
		}
		topN = coTagBaseTopN
	} else if abstractTagID > 0 {
		subtreeDepth := getAbstractSubtreeDepth(abstractTagID)
		topN = calculateCoTagTopN(subtreeDepth)
		keywords := aggregateKeywordsByChildCoverage(abstractTagID, topN)
		for _, kw := range keywords {
			keywordTagIDs = append(keywordTagIDs, kw.KeywordTagID)
		}
	} else {
		return nil, nil
	}

	if len(keywordTagIDs) == 0 {
		return nil, nil
	}

	existingSet := make(map[uint]bool, len(existingCandidateIDs))
	for _, id := range existingCandidateIDs {
		existingSet[id] = true
	}

	eventTagArticleMap := findEventTagsViaKeywords(keywordTagIDs, existingSet, coTagMaxCandidates)
	if len(eventTagArticleMap) == 0 {
		return nil, nil
	}

	eventTagIDs := make([]uint, 0, len(eventTagArticleMap))
	for id := range eventTagArticleMap {
		eventTagIDs = append(eventTagIDs, id)
	}

	var eventTags []models.TopicTag
	if err := database.DB.Where("id IN ? AND category = ? AND status = ?", eventTagIDs, "event", "active").Find(&eventTags).Error; err != nil {
		return nil, fmt.Errorf("load event tags: %w", err)
	}

	logging.Infof("co-tag expansion: found %d additional event candidates (topN=%d, source=article:%d/abstract:%d)",
		len(eventTags), topN, articleID, abstractTagID)

	result := make([]TagCandidate, 0, len(eventTags))
	for _, tag := range eventTags {
		result = append(result, TagCandidate{
			Tag:        &tag,
			Similarity: 0.80,
		})
	}
	return result, nil
}

type articleKeyword struct {
	TagID uint
	Score float64
}

func getTopArticleKeywords(articleID uint, topN int) []articleKeyword {
	var links []models.ArticleTopicTag
	database.DB.Where("article_id = ?", articleID).
		Order("score DESC").
		Limit(topN).
		Find(&links)

	result := make([]articleKeyword, 0, len(links))
	for _, l := range links {
		result = append(result, articleKeyword{TagID: l.TopicTagID, Score: l.Score})
	}
	return result
}

func aggregateKeywordsByChildCoverage(abstractTagID uint, topN int) []coTagCandidate {
	childEventTagIDs := getDescendantEventTagIDs(abstractTagID)
	if len(childEventTagIDs) == 0 {
		return nil
	}

	type coverageRow struct {
		KeywordTagID uint `gorm:"column:keyword_tag_id"`
		Coverage     int  `gorm:"column:coverage"`
		ArticleCount int  `gorm:"column:article_count"`
	}

	var rows []coverageRow
	query := `
		SELECT att2.topic_tag_id AS keyword_tag_id,
		       COUNT(DISTINCT att1.topic_tag_id) AS coverage,
		       COUNT(DISTINCT att2.article_id) AS article_count
		FROM article_topic_tags att1
		JOIN article_topic_tags att2 ON att1.article_id = att2.article_id
		                            AND att2.topic_tag_id != att1.topic_tag_id
		JOIN topic_tags tt ON tt.id = att2.topic_tag_id
		WHERE att1.topic_tag_id IN ?
		  AND tt.category = 'keyword'
		  AND (tt.status = 'active' OR tt.status = '' OR tt.status IS NULL)
		GROUP BY att2.topic_tag_id
		ORDER BY coverage DESC, article_count DESC
		LIMIT ?
	`
	if err := database.DB.Raw(query, childEventTagIDs, topN).Scan(&rows).Error; err != nil {
		logging.Warnf("aggregateKeywordsByChildCoverage: query failed: %v", err)
		return nil
	}

	var tagIDs []uint
	for _, r := range rows {
		tagIDs = append(tagIDs, r.KeywordTagID)
	}
	labelMap := make(map[uint]string)
	var tags []models.TopicTag
	database.DB.Where("id IN ?", tagIDs).Find(&tags)
	for _, t := range tags {
		labelMap[t.ID] = t.Label
	}

	result := make([]coTagCandidate, 0, len(rows))
	for _, r := range rows {
		result = append(result, coTagCandidate{
			KeywordTagID: r.KeywordTagID,
			KeywordLabel: labelMap[r.KeywordTagID],
			Coverage:     r.Coverage,
			ArticleCount: r.ArticleCount,
		})
	}
	return result
}

func getDescendantEventTagIDs(abstractTagID uint) []uint {
	query := `
		WITH RECURSIVE descendants AS (
			SELECT child_id FROM topic_tag_relations WHERE parent_id = ? AND relation_type = 'abstract'
			UNION
			SELECT r.child_id FROM topic_tag_relations r
			JOIN descendants d ON r.parent_id = d.child_id
			WHERE r.relation_type = 'abstract'
		)
		SELECT d.child_id FROM descendants d
		JOIN topic_tags t ON t.id = d.child_id
		WHERE t.category = 'event' AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
	`
	var ids []uint
	if err := database.DB.Raw(query, abstractTagID).Scan(&ids).Error; err != nil {
		logging.Warnf("getDescendantEventTagIDs: query failed for %d: %v", abstractTagID, err)
		return nil
	}
	return ids
}

func getAbstractSubtreeDepth(tagID uint) int {
	query := `
		WITH RECURSIVE tree AS (
			SELECT child_id, 1 AS depth FROM topic_tag_relations WHERE parent_id = ? AND relation_type = 'abstract'
			UNION
			SELECT r.child_id, t.depth + 1
			FROM topic_tag_relations r
			JOIN tree t ON r.parent_id = t.child_id
			WHERE r.relation_type = 'abstract'
		)
		SELECT COALESCE(MAX(depth), 0) FROM tree
	`
	var maxDepth int
	if err := database.DB.Raw(query, tagID).Scan(&maxDepth).Error; err != nil {
		return 0
	}
	return maxDepth
}

func findEventTagsViaKeywords(keywordTagIDs []uint, excludeIDs map[uint]bool, maxCandidates int) map[uint]int {
	type eventRow struct {
		EventTagID uint `gorm:"column:event_tag_id"`
		HitCount   int  `gorm:"column:hit_count"`
	}

	var rows []eventRow
	query := `
		SELECT att2.topic_tag_id AS event_tag_id, COUNT(DISTINCT att2.article_id) AS hit_count
		FROM article_topic_tags att1
		JOIN article_topic_tags att2 ON att1.article_id = att2.article_id
		JOIN topic_tags tt ON tt.id = att2.topic_tag_id
		WHERE att1.topic_tag_id IN ?
		  AND tt.category = 'event'
		  AND (tt.status = 'active' OR tt.status = '' OR tt.status IS NULL)
		GROUP BY att2.topic_tag_id
		ORDER BY hit_count DESC
		LIMIT ?
	`
	if err := database.DB.Raw(query, keywordTagIDs, maxCandidates).Scan(&rows).Error; err != nil {
		logging.Warnf("findEventTagsViaKeywords: query failed: %v", err)
		return nil
	}

	result := make(map[uint]int, len(rows))
	for _, r := range rows {
		if excludeIDs[r.EventTagID] {
			continue
		}
		result[r.EventTagID] = r.HitCount
	}
	return result
}

func mergeCandidateLists(embeddingCandidates, coTagCandidates []TagCandidate) []TagCandidate {
	seen := make(map[uint]bool, len(embeddingCandidates)+len(coTagCandidates))
	result := make([]TagCandidate, 0, len(embeddingCandidates)+len(coTagCandidates))

	for _, c := range embeddingCandidates {
		if c.Tag == nil || seen[c.Tag.ID] {
			continue
		}
		seen[c.Tag.ID] = true
		result = append(result, c)
	}
	for _, c := range coTagCandidates {
		if c.Tag == nil || seen[c.Tag.ID] {
			continue
		}
		seen[c.Tag.ID] = true
		result = append(result, c)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Similarity > result[j].Similarity
	})

	return result
}
```

### Step 4: Run tests

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestCalculateCoTagTopN -v`
Expected: PASS (6 test cases)

### Step 5: Build check

Run: `cd backend-go && go build ./...`
Expected: No errors

### Step 6: Commit

```bash
git add backend-go/internal/domain/topicanalysis/cotag_expansion.go backend-go/internal/domain/topicanalysis/cotag_expansion_test.go
git commit -m "feat: add co-tag expansion service for event tag matching"
```

---

## Task 2: Integrate Co-tag Expansion into findOrCreateTag

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` (lines ~150-310, findOrCreateTag function)
- Modify: `backend-go/internal/domain/topicextraction/article_tagger.go` (tagArticle function, pass articleID to findOrCreateTag)

**Context:** The `findOrCreateTag` function currently doesn't receive the article ID. We need to pass it through from `tagArticle` so the co-tag expansion can query the article's existing tags. The expansion only runs for event category tags, only in the `candidates` branch, after embedding search and before LLM judgment.

### Step 1: Add articleID parameter to findOrCreateTag

In `tagger.go`, change `findOrCreateTag` signature from:

```go
func findOrCreateTag(ctx context.Context, tag topictypes.TopicTag, source string, articleContext string) (*models.TopicTag, error) {
```

to:

```go
func findOrCreateTag(ctx context.Context, tag topictypes.TopicTag, source string, articleContext string, articleID uint) (*models.TopicTag, error) {
```

Update all callers:
- `article_tagger.go:125` — pass `article.ID` (from `tagArticle`)
- `tagger.go:103` — pass `0` (from `TagSummary`, no article context)

### Step 2: Add co-tag expansion in the candidates branch

In `tagger.go`, inside `findOrCreateTag`, after the `case "candidates":` block gets candidates, add the co-tag expansion:

```go
case "candidates":
    candidates := matchResult.Candidates
    logging.Infof("findOrCreateTag: label=%q category=%s matchType=candidates candidateCount=%d topSimilarity=%.4f", tag.Label, category, len(candidates), matchResult.Similarity)

    if category == "event" {
        var existingIDs []uint
        for _, c := range candidates {
            if c.Tag != nil {
                existingIDs = append(existingIDs, c.Tag.ID)
            }
        }
        coTagCandidates, coTagErr := topicanalysis.ExpandEventCandidatesByArticleCoTags(ctx, articleID, 0, existingIDs)
        if coTagErr != nil {
            logging.Warnf("co-tag expansion failed for %q: %v", tag.Label, coTagErr)
        } else if len(coTagCandidates) > 0 {
            candidates = topicanalysis.MergeCandidateLists(candidates, coTagCandidates)
            logging.Infof("findOrCreateTag: label=%q expanded to %d candidates via co-tag traversal", tag.Label, len(candidates))
        }
    }

    result, judgmentErr := topicanalysis.ExtractAbstractTag(ctx, candidates, tag.Label, category, topicanalysis.WithCaller("findOrCreateTag"))
    // ... rest unchanged
```

### Step 3: Build check

Run: `cd backend-go && go build ./...`
Expected: No errors

### Step 4: Commit

```bash
git add backend-go/internal/domain/topicextraction/tagger.go backend-go/internal/domain/topicextraction/article_tagger.go
git commit -m "feat: integrate co-tag expansion into event tag matching pipeline"
```

---

## Task 3: Integrate Co-tag Expansion for Abstract Event Tags

**Files:**
- Modify: `backend-go/internal/domain/topicextraction/tagger.go` (abstract tag creation branch in `findOrCreateTag`)
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` (if a reusable helper is extracted)

**Context:** When an abstract event tag is created, it should also benefit from co-tag expansion. The difference is: instead of using article co-tags, it aggregates keywords from all descendant event tags' articles. The insertion point is the abstract creation branch in `findOrCreateTag`, where the new abstract tag and its first concrete child are both available. If the logic becomes too large, extract a helper into `abstract_tag_service.go` and call it from there.

### Step 1: Add co-tag expansion in the abstract creation branch

In `tagger.go`, locate the `result.HasAbstract()` branch inside `findOrCreateTag`. After creation and before the function returns, add:

The expansion should happen at the end of the abstract creation path in `findOrCreateTag`, where both the new abstract tag and its child tag are already known:

```go
if result.HasAbstract() {
    mergeTargetID := uint(0)
    if result.HasMerge() {
        mergeTargetID = result.Merge.Target.ID
    }
    for _, c := range candidates {
        if c.Tag != nil && c.Tag.ID != mergeTargetID {
            if delErr := topicanalysis.DeleteTagEmbedding(c.Tag.ID); delErr != nil {
                logging.Warnf("Failed to delete embedding for child tag %d: %v", c.Tag.ID, delErr)
            }
        }
    }
    newTag, childErr := createChildOfAbstract(ctx, es, tag, category, kind, source, articleContext, string(aliasesJSON), result.Abstract.Tag)
    if childErr != nil {
        logging.Warnf("Failed to create child of abstract %d: %v", result.Abstract.Tag.ID, childErr)
        break
    }

    // Co-tag expansion for abstract event tags
    if result.Abstract.Tag.Category == "event" {
        var existingIDs []uint
        for _, c := range candidates {
            if c.Tag != nil {
                existingIDs = append(existingIDs, c.Tag.ID)
            }
        }
        existingIDs = append(existingIDs, newTag.ID, result.Abstract.Tag.ID)
        coTagCandidates, coTagErr := topicanalysis.ExpandEventCandidatesByArticleCoTags(ctx, 0, result.Abstract.Tag.ID, existingIDs)
        if coTagErr != nil {
            logging.Warnf("abstract co-tag expansion failed for abstract %d: %v", result.Abstract.Tag.ID, coTagErr)
        } else if len(coTagCandidates) > 0 {
            go func() {
                abstractCandidates := topicanalysis.MergeCandidateLists(nil, coTagCandidates)
                judgmentResult, jErr := topicanalysis.ExtractAbstractTag(ctx, abstractCandidates, result.Abstract.Tag.Label, "event", topicanalysis.WithCaller("abstract_co_tag_expansion"))
                if jErr != nil || judgmentResult == nil || !judgmentResult.HasAction() {
                    return
                }
                if judgmentResult.HasMerge() && judgmentResult.Merge.Target != nil {
                    if mergeErr := topicanalysis.MergeTags(result.Abstract.Tag.ID, judgmentResult.Merge.Target.ID); mergeErr != nil {
                        logging.Warnf("abstract co-tag merge failed: %v", mergeErr)
                    }
                }
            }()
        }
    }

    return newTag, nil
}
```

### Step 2: Build check

Run: `cd backend-go && go build ./...`
Expected: No errors

### Step 3: Commit

```bash
git add backend-go/internal/domain/topicextraction/tagger.go backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "feat: co-tag expansion for abstract event tags"
```

---

## Task 4: Co-tag Backfill CLI Command

**Files:**
- Create: `backend-go/cmd/co-tag-backfill/main.go`

**Context:** One-time CLI tool that iterates over all active event tags, runs co-tag expansion, and merges any discovered matches. For non-abstract tags, the expansion should aggregate signals across multiple related articles instead of depending on a single sample article.

### Step 1: Create the CLI command

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicanalysis"
	"my-robot-backend/internal/platform/config"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

func main() {
	dryRun := flag.Bool("dry-run", true, "Only print what would be done, don't execute")
	batchSize := flag.Int("batch", 50, "Number of tags to process per batch")
	flag.Parse()

	cfg := config.LoadConfig()
	database.InitDB(cfg)
	defer database.CloseDB()

	ctx := context.Background()

	var totalTags int64
	database.DB.Model(&models.TopicTag{}).
		Where("category = ? AND status = ?", "event", "active").
		Count(&totalTags)
	fmt.Printf("Found %d active event tags to process\n", totalTags)

	var tags []models.TopicTag
	database.DB.Where("category = ? AND status = ?", "event", "active").
		Order("id ASC").
		Find(&tags)

	processed := 0
	merged := 0
	abstracted := 0
	errors := 0

	for _, tag := range tags {
		processed++
		if processed%*batchSize == 0 {
			fmt.Printf("Progress: %d/%d (merged=%d, abstracted=%d, errors=%d)\n",
				processed, len(tags), merged, abstracted, errors)
		}

		var articleIDs []uint
		database.DB.Model(&models.ArticleTopicTag{}).
			Where("topic_tag_id = ?", tag.ID).
			Limit(5).
			Pluck("article_id", &articleIDs)

		var coTagCandidates []topicanalysis.TagCandidate
		if len(articleIDs) > 0 {
			var existingIDs []uint
			existingIDs = append(existingIDs, tag.ID)
			for _, articleID := range articleIDs {
				expanded, err := topicanalysis.ExpandEventCandidatesByArticleCoTags(ctx, articleID, 0, existingIDs)
				if err != nil {
					logging.Warnf("co-tag backfill: expansion failed for tag %d (%s) article %d: %v", tag.ID, tag.Label, articleID, err)
					continue
				}
				coTagCandidates = topicanalysis.MergeCandidateLists(coTagCandidates, expanded)
			}
			if len(coTagCandidates) == 0 {
				logging.Infof("co-tag backfill: no candidates found after aggregating %d articles for tag %d (%s)", len(articleIDs), tag.ID, tag.Label)
			}
		}

		if tag.Source == "abstract" {
			var existingIDs []uint
			existingIDs = append(existingIDs, tag.ID)
			var err error
			coTagCandidates, err = topicanalysis.ExpandEventCandidatesByArticleCoTags(ctx, 0, tag.ID, existingIDs)
			if err != nil {
				logging.Warnf("co-tag backfill: abstract expansion failed for tag %d (%s): %v", tag.ID, tag.Label, err)
				errors++
				continue
			}
		}

		if len(coTagCandidates) == 0 {
			continue
		}

		fmt.Printf("[%s] Tag %d (%q): found %d co-tag candidates\n",
			map[bool]string{true: "DRY-RUN", false: "LIVE"}[*dryRun],
			tag.ID, tag.Label, len(coTagCandidates))

		if *dryRun {
			for _, c := range coTagCandidates {
				if c.Tag != nil {
					fmt.Printf("  → candidate: %d (%q, sim=%.4f)\n", c.Tag.ID, c.Tag.Label, c.Similarity)
				}
			}
			continue
		}

		result, err := topicanalysis.ExtractAbstractTag(ctx, coTagCandidates, tag.Label, "event", topicanalysis.WithCaller("co_tag_backfill"))
		if err != nil || result == nil || !result.HasAction() {
			continue
		}

		if result.HasMerge() && result.Merge.Target != nil {
			sourceID := tag.ID
			targetID := result.Merge.Target.ID
			if sourceID == targetID {
				continue
			}
			if mergeErr := topicanalysis.MergeTags(sourceID, targetID); mergeErr != nil {
				logging.Warnf("co-tag backfill: merge %d→%d failed: %v", sourceID, targetID, mergeErr)
				errors++
			} else {
				merged++
				fmt.Printf("  ✓ merged %d (%q) into %d (%q)\n", sourceID, tag.Label, targetID, result.Merge.Target.Label)
			}
		}

		if result.HasAbstract() {
			abstracted++
			fmt.Printf("  ✓ created abstract tag %d (%q)\n", result.Abstract.Tag.ID, result.Abstract.Tag.Label)
		}

		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\nDone: processed=%d, merged=%d, abstracted=%d, errors=%d\n",
		processed, merged, abstracted, errors)
}
```

### Step 2: Build check

Run: `cd backend-go && go build ./cmd/co-tag-backfill/...`
Expected: No errors

### Step 3: Commit

```bash
git add backend-go/cmd/co-tag-backfill/
git commit -m "feat: add co-tag backfill CLI command"
```

---

## Task 5: Cross-layer Dedup in MatchAbstractTagHierarchy

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` (MatchAbstractTagHierarchy function, ~line 1624-1676)

**Context:** Currently `MatchAbstractTagHierarchy` only searches abstract tags (`FindSimilarAbstractTags`). We still want full-tree dedup, but we cannot rely on every node in the tree having an embedding. The safer approach is two-step:
1. Reuse the existing `FindSimilarTags` / keyword-style retrieval path to get embedding-backed anchors.
2. From those anchors, walk back to related event tags inside the same tree and evaluate those event tags as dedup targets.

This keeps the goal of "same-tree any-level dedup" while staying compatible with the current embedding retention rules.

### Step 1: Add tree-aware dedup candidate discovery

Add helper functions in `abstract_tag_service.go`:

```go
func findCrossLayerDuplicateCandidates(ctx context.Context, abstractTagID uint, category string) ([]TagCandidate, error) {
	es := NewEmbeddingService()
	thresholds := es.GetThresholds()

	var abstractTag models.TopicTag
	if err := database.DB.First(&abstractTag, abstractTagID).Error; err != nil {
		return nil, err
	}

	allTreeTagIDs := getAllTreeTagIDs(abstractTagID)
	if len(allTreeTagIDs) == 0 {
		return nil, nil
	}

	keywordAnchors, err := es.FindSimilarTags(ctx, &abstractTag, category, 20, EmbeddingTypeSemantic)
	if err != nil {
		return nil, err
	}

	var anchorIDs []uint
	for _, c := range keywordAnchors {
		if c.Tag == nil || c.Tag.ID == abstractTagID {
			continue
		}
		if c.Similarity < thresholds.HighSimilarity {
			continue
		}
		anchorIDs = append(anchorIDs, c.Tag.ID)
	}

	if len(anchorIDs) == 0 {
		return nil, nil
	}

	relatedEventIDs, err := findRelatedEventTagsFromAnchors(anchorIDs)
	if err != nil {
		return nil, err
	}

	treeSet := make(map[uint]struct{}, len(allTreeTagIDs))
	for _, id := range allTreeTagIDs {
		treeSet[id] = struct{}{}
	}

	var result []TagCandidate
	for _, eventID := range relatedEventIDs {
		if eventID == abstractTagID {
			continue
		}
		if _, ok := treeSet[eventID]; !ok {
			continue
		}
		candidate, err := buildTreeDedupCandidate(ctx, &abstractTag, eventID)
		if err != nil {
			continue
		}
		if candidate.Similarity >= thresholds.HighSimilarity {
			result = append(result, candidate)
		}
	}
	return result, nil
}

// findRelatedEventTagsFromAnchors bridges from keyword/similar-tag anchors back to event tags.
// buildTreeDedupCandidate should reuse the existing similarity scoring path so the final
// decision still compares event concepts, not just keyword labels.

Concrete implementation notes:
- `findRelatedEventTagsFromAnchors(anchorIDs)` should reuse the co-tag pipeline from Task 1 instead of inventing a second graph traversal path.
- Recommended flow for `findRelatedEventTagsFromAnchors`:
  1. For each anchor tag, load up to `coTagMaxArticlesPerTag` related articles from `article_topic_tags`.
  2. For those articles, reuse `getTopArticleKeywords(articleID, coTagBaseTopN)` to collect keyword IDs.
  3. Merge and dedupe all collected keyword IDs.
  4. Reuse `findEventTagsViaKeywords(keywordTagIDs, excludeIDs, coTagMaxCandidates)` to get related event tags.
  5. Return event tag IDs sorted by support count descending so `MatchAbstractTagHierarchy` can evaluate the strongest candidates first.
- `excludeIDs` should include the current abstract tag, the full current tree (`getAllTreeTagIDs`), and the anchor tag IDs themselves when they are already in-tree. This avoids rediscovering the same node as a duplicate candidate.
- `buildTreeDedupCandidate` should only be used to build a shortlist score, not as the final merge authority. The shortlist score can be derived from support count and any existing direct embedding hit on the same target.
- Final merge decision rule: for the top 1-3 cross-layer candidates, call a small LLM judgment helper (for example `judgeCrossLayerDuplicate`) or reuse the existing pairwise merge judgment path. Only call `MergeTags` if the judgment explicitly says the two tags are duplicates. Do not merge solely because the shortlist score is high.

func getAllTreeTagIDs(tagID uint) []uint {
	query := `
		WITH RECURSIVE tree_up AS (
			SELECT id, parent_id FROM (
				SELECT ? AS id, NULL::bigint AS parent_id
			UNION
				SELECT child_id, parent_id FROM topic_tag_relations
				WHERE child_id = ? AND relation_type = 'abstract'
			) sub
			UNION
			SELECT CASE WHEN r.child_id = tu.id THEN r.parent_id ELSE r.child_id END,
			       CASE WHEN r.child_id = tu.id THEN NULL ELSE r.parent_id END
			FROM tree_up tu
			JOIN topic_tag_relations r ON (r.child_id = tu.id OR r.parent_id = tu.id)
			WHERE r.relation_type = 'abstract'
		),
		tree_down AS (
			SELECT parent_id AS root_id FROM topic_tag_relations WHERE child_id = ? AND relation_type = 'abstract'
			UNION
			SELECT ? AS root_id WHERE NOT EXISTS (SELECT 1 FROM topic_tag_relations WHERE child_id = ? AND relation_type = 'abstract')
		),
		full_tree AS (
			SELECT r.child_id AS tag_id FROM topic_tag_relations r
			WHERE r.parent_id = (SELECT root_id FROM tree_down LIMIT 1) AND r.relation_type = 'abstract'
			UNION
			SELECT td.tag_id FROM (
				WITH RECURSIVE sub AS (
					SELECT child_id AS tag_id FROM topic_tag_relations
					WHERE parent_id = (SELECT root_id FROM tree_down LIMIT 1) AND relation_type = 'abstract'
					UNION
					SELECT r.child_id FROM topic_tag_relations r
					JOIN sub s ON r.parent_id = s.tag_id
					WHERE r.relation_type = 'abstract'
				)
				SELECT tag_id FROM sub
			) td
		)
		SELECT DISTINCT tag_id FROM full_tree
		UNION SELECT DISTINCT id FROM tree_up WHERE id IS NOT NULL
	`
	var ids []uint
	if err := database.DB.Raw(query, tagID, tagID, tagID, tagID, tagID).Scan(&ids).Error; err != nil {
		logging.Warnf("getAllTreeTagIDs: query failed for %d: %v", tagID, err)
		return nil
	}
	return ids
}
```

**Note:** The recursive SQL above is complex. A simpler approach is to use two `WITH RECURSIVE` queries: one to walk up to root, then one to walk down from root. This can be split into helper functions. The implementer should test this carefully.

Recommended minimum tests for this task:
- root tag returns the full tree including itself and descendants
- nested child tag returns siblings, ancestors, and descendants in the same tree
- cross-layer dedup candidate discovery ignores nodes outside the current tree
- shortlist generation remains stable when some normal nodes in the tree have no embedding

### Step 2: Modify MatchAbstractTagHierarchy to include cross-layer dedup

Replace the existing `MatchAbstractTagHierarchy` function:

```go
func MatchAbstractTagHierarchy(ctx context.Context, abstractTagID uint) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("MatchAbstractTagHierarchy panic for tag %d: %v", abstractTagID, r)
		}
	}()

	var abstractTag models.TopicTag
	if err := database.DB.First(&abstractTag, abstractTagID).Error; err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: tag %d not found: %v", abstractTagID, err)
		return
	}

	es := NewEmbeddingService()
	thresholds := es.GetThresholds()

	// NEW: Cross-layer dedup - find embedding-backed anchors, then resolve to same-tree event tags
	treeDuplicates, err := findCrossLayerDuplicateCandidates(ctx, abstractTagID, abstractTag.Category)
	if err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: cross-layer dedup search failed: %v", err)
	} else {
		for _, dup := range treeDuplicates {
			if dup.Tag == nil {
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: cross-layer duplicate found: tag %d (%q) sim=%.4f with %d (%q)",
				abstractTagID, abstractTag.Label, dup.Similarity, dup.Tag.ID, dup.Tag.Label)
			shouldMerge, reason, judgeErr := judgeCrossLayerDuplicate(ctx, abstractTagID, dup.Tag.ID)
			if judgeErr != nil {
				logging.Warnf("MatchAbstractTagHierarchy: cross-layer judge failed: %v", judgeErr)
				continue
			}
			if !shouldMerge {
				logging.Infof("MatchAbstractTagHierarchy: candidate %d rejected by cross-layer judge for %d: %s", dup.Tag.ID, abstractTagID, reason)
				continue
			}
			if mergeErr := MergeTags(abstractTagID, dup.Tag.ID); mergeErr != nil {
				logging.Warnf("MatchAbstractTagHierarchy: cross-layer merge failed: %v", mergeErr)
			} else {
				logging.Infof("MatchAbstractTagHierarchy: merged %d into %d (cross-layer dedup, reason=%s)", abstractTagID, dup.Tag.ID, reason)
				return
			}
		}
	}

	// Existing logic: search abstract tags for hierarchy matching
	candidates, err := es.FindSimilarAbstractTags(ctx, abstractTagID, abstractTag.Category, 0)
	if err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: failed to find similar abstract tags for %d: %v", abstractTagID, err)
		return
	}
	if len(candidates) == 0 {
		return
	}

	for _, candidate := range candidates {
		if candidate.Tag == nil {
			continue
		}

		if candidate.Similarity >= thresholds.HighSimilarity {
			if err := mergeOrLinkSimilarAbstract(ctx, abstractTagID, candidate.Tag.ID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: merge/link failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, err)
			}
			continue
		}

		if candidate.Similarity < thresholds.LowSimilarity {
			continue
		}

		// NEW: Depth check before linking
		childDepth := getAbstractSubtreeDepth(abstractTagID)
		parentDepth := getTagDepthFromRoot(candidate.Tag.ID)

		if childDepth+parentDepth+1 > 4 {
			alternativeID, reason, err := aiJudgeAlternativePlacement(ctx, abstractTagID, candidate.Tag.ID)
			if err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: depth-limit AI judgment failed: %v", err)
				continue
			}
			if alternativeID > 0 {
				logging.Infof("MatchAbstractTagHierarchy: depth limit triggered, AI suggests tag %d for %d: %s", alternativeID, abstractTagID, reason)
				if linkErr := linkAbstractParentChild(abstractTagID, alternativeID); linkErr != nil {
					logging.Warnf("MatchAbstractTagHierarchy: alternative placement failed: %v", linkErr)
				}
			}
			continue
		}

		parentID, childID, err := aiJudgeAbstractHierarchy(ctx, abstractTagID, candidate.Tag.ID)
		if err != nil {
			logging.Warnf("MatchAbstractTagHierarchy: AI judgment failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, err)
			continue
		}
		if err := linkAbstractParentChild(childID, parentID); err != nil {
			logging.Warnf("MatchAbstractTagHierarchy: failed to link %d under %d: %v", childID, parentID, err)
			continue
		}
		logging.Infof("Abstract hierarchy: %d is child of %d (AI judged, similarity=%.4f)", childID, parentID, candidate.Similarity)
	}
}
```

### Step 3: Add helper functions for depth and AI alternative placement

```go
func getTagDepthFromRoot(tagID uint) int {
	query := `
		WITH RECURSIVE ancestors AS (
			SELECT parent_id, 1 AS depth FROM topic_tag_relations
			WHERE child_id = ? AND relation_type = 'abstract'
			UNION
			SELECT r.parent_id, a.depth + 1
			FROM topic_tag_relations r
			JOIN ancestors a ON r.child_id = a.parent_id
			WHERE r.relation_type = 'abstract'
		)
		SELECT COALESCE(MAX(depth), 0) FROM ancestors
	`
	var depth int
	if err := database.DB.Raw(query, tagID).Scan(&depth).Error; err != nil {
		return 0
	}
	return depth
}

func aiJudgeAlternativePlacement(ctx context.Context, tagID uint, suggestedParentID uint) (uint, string, error) {
	var tag, suggestedParent models.TopicTag
	if err := database.DB.First(&tag, tagID).Error; err != nil {
		return 0, "", err
	}
	if err := database.DB.First(&suggestedParent, suggestedParentID).Error; err != nil {
		return 0, "", err
	}

	siblings := loadAbstractChildLabels(suggestedParentID, 8)
	tagChildren := loadAbstractChildLabels(tagID, 5)

	router := airouter.NewRouter()
	prompt := fmt.Sprintf(`一个抽象标签即将被放置到层级树中，但目标位置会导致层级过深（超过4层）。
请判断该标签最合适的归属。

待放置标签: %q (描述: %s)
该标签的子标签: %s

原定父标签: %q (描述: %s)
原定父标签的子标签: %s

规则:
- 不要创建新的深层级
- 优先选择合并到已有标签，或放置到更浅的层级
- 如果该标签与原定父标签的某个子标签概念重叠，返回该子标签ID

返回 JSON: {"target_id": 目标标签ID或0表示不放置, "reason": "简要说明"}`,
		tag.Label, truncateStr(tag.Description, 200), formatChildLabels(tagChildren),
		suggestedParent.Label, truncateStr(suggestedParent.Description, 200), formatChildLabels(siblings))

	req := airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "You are a tag taxonomy assistant. Respond only with valid JSON."},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"target_id": {Type: "integer", Description: "目标标签ID，0表示不放置"},
				"reason":    {Type: "string", Description: "判断理由"},
			},
			Required: []string{"target_id", "reason"},
		},
		Temperature: func() *float64 { f := 0.2; return &f }(),
		Metadata: map[string]any{
			"operation":      "depth_limit_alternative",
			"tag_id":         tagID,
			"suggested_parent": suggestedParentID,
		},
	}

	result, err := router.Chat(ctx, req)
	if err != nil {
		return 0, "", fmt.Errorf("LLM call failed: %w", err)
	}

	var parsed struct {
		TargetID uint   `json:"target_id"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		return 0, "", fmt.Errorf("parse response: %w", err)
	}
	return parsed.TargetID, parsed.Reason, nil
}
```

Add one more helper beside `aiJudgeAlternativePlacement`:

```go
func judgeCrossLayerDuplicate(ctx context.Context, sourceID uint, candidateID uint) (bool, string, error)
```

Implementation notes:
- Load both tags' labels, descriptions, parent path, and up to 5 child labels.
- Prompt should ask a single binary question: "Are these two tags the same concept and should they be merged?"
- Return `(true, reason, nil)` only for a confident duplicate verdict.
- Keep this helper focused on merge vs no-merge. Do not mix hierarchy placement into the same prompt.

### Step 4: Build check

Run: `cd backend-go && go build ./...`
Expected: No errors

### Step 5: Commit

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "feat: cross-layer dedup and depth limit in MatchAbstractTagHierarchy"
```

---

## Task 6: Depth Guard in linkAbstractParentChild

**Files:**
- Modify: `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` (linkAbstractParentChild function, ~line 1732)

**Context:** Add a depth check at the start of `linkAbstractParentChild`. If the resulting tree would exceed 4 levels, reject the insertion and call `aiJudgeAlternativePlacement` instead. A resulting depth of exactly 4 is still allowed.

Execution note:
- Task 6 should be implemented after Task 5's shared helpers (`getTagDepthFromRoot`, `aiJudgeAlternativePlacement`) are in place.
- `linkAbstractParentChild` itself should remain a hard guard. It may return an error that the caller handles, but it should not silently trigger merge behavior inside the transaction.

### Step 1: Add depth check at the top of linkAbstractParentChild

Insert after the cycle check (line ~1740), before the relation creation:

```go
func linkAbstractParentChild(childID, parentID uint) error {
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		wouldCycle, err := wouldCreateCycle(tx, parentID, childID)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if wouldCycle {
			return fmt.Errorf("would create cycle: parent=%d, child=%d", parentID, childID)
		}

		// NEW: Depth guard
		childSubtreeDepth := getAbstractSubtreeDepth(childID)
		parentAncestryDepth := getTagDepthFromRoot(parentID)
		if childSubtreeDepth+parentAncestryDepth+1 > 4 {
			return fmt.Errorf("depth limit: placing subtree(depth=%d) under parent(ancestry=%d) would exceed max depth 4", childSubtreeDepth, parentAncestryDepth)
		}

		var count int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ?", parentID, childID).
			Count(&count)
		if count > 0 {
			return nil
		}

		relation := models.TopicTagRelation{
			ParentID:     parentID,
			ChildID:      childID,
			RelationType: "abstract",
		}
		return tx.Create(&relation).Error
	})
	if err != nil {
		logging.Warnf("linkAbstractParentChild: rejected child=%d parent=%d: %v", childID, parentID, err)
		return err
	}

	go func(id uint) {
		_, _ = resolveMultiParentConflict(id)
	}(childID)
	go enqueueEmbeddingsForNormalChildren(parentID)
	go EnqueueAbstractTagUpdate(parentID, "hierarchy_linked")

	return nil
}
```

### Step 2: Build check

Run: `cd backend-go && go build ./...`
Expected: No errors

### Step 3: Commit

```bash
git add backend-go/internal/domain/topicanalysis/abstract_tag_service.go
git commit -m "feat: depth guard in linkAbstractParentChild (max 4 levels)"
```

---

## Task 7: Rewrite Hierarchy Cleanup Logic

**Files:**
- Rewrite: `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go` (replace ProcessTree/buildCleanupPrompt with C+D logic)
- Modify: `backend-go/internal/jobs/tag_hierarchy_cleanup.go` (add Phase 4 to runCleanupCycle)

**Context:** Replace the old tree-traversal+LLM-prompt cleanup with cross-layer dedup and depth compression. The scheduler framework (TagHierarchyCleanupScheduler) stays intact. Phases 1-3 (zombie, flat merge, orphan) remain unchanged. Phase 4 is new.

### Step 1: Replace hierarchy_cleanup.go internals

Keep `BuildTagForest`, `TreeNode`, `calculateTreeDepth`, `countNodes`, `collectAllTags` unchanged. Replace `ProcessTree` and all downstream functions with:

```go
func ProcessTree(node *TreeNode) (*TreeCleanupResult, error) {
	result := &TreeCleanupResult{
		TreeRootID:    node.Tag.ID,
		TreeRootLabel: node.Tag.Label,
	}

	allTags := collectAllTags(node)
	result.TagsProcessed = len(allTags)

	// Step 1: Cross-layer dedup via embedding
	dedupMerges, err := crossLayerDedup(node, allTags)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("cross-layer dedup: %v", err))
	} else {
		result.MergesApplied += dedupMerges
	}

	// Step 2: Depth compression for subtrees > max depth
	if calculateTreeDepth(node) >= MinTreeDepthForCleanup {
		compressed, compErr := compressDeepSubtrees(node)
		if compErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("depth compression: %v", compErr))
		} else {
			result.MergesApplied += compressed
		}
	}

	return result, nil
}

func crossLayerDedup(root *TreeNode, allTags []*TreeNode) (int, error) {
	es := NewEmbeddingService()
	thresholds := es.GetThresholds()
	merged := 0

	tagMap := make(map[uint]*TreeNode, len(allTags))
	for _, t := range allTags {
		tagMap[t.Tag.ID] = t
	}

	for i := 0; i < len(allTags); i++ {
		nodeA := allTags[i]
		if nodeA.Tag.Status != "active" {
			continue
		}

		for j := i + 1; j < len(allTags); j++ {
			nodeB := allTags[j]
			if nodeB.Tag.Status != "active" {
				continue
			}
			if isDirectParentChild(nodeA, nodeB) {
				continue
			}

			sim, err := computeTagSimilarity(nodeA.Tag.ID, nodeB.Tag.ID)
			if err != nil {
				continue
			}

			if sim >= thresholds.HighSimilarity {
				sourceID := nodeB.Tag.ID
				targetID := nodeA.Tag.ID
				if nodeA.Depth > nodeB.Depth {
					sourceID, targetID = targetID, sourceID
				}

				logging.Infof("hierarchy cleanup cross-layer dedup: merging %d (%q, depth=%d) into %d (%q, depth=%d) sim=%.4f",
					sourceID, tagMap[sourceID].Tag.Label, tagMap[sourceID].Depth,
					targetID, tagMap[targetID].Tag.Label, tagMap[targetID].Depth, sim)

				if mergeErr := MergeTags(sourceID, targetID); mergeErr != nil {
					logging.Warnf("hierarchy cleanup: cross-layer merge failed: %v", mergeErr)
					continue
				}
				merged++
				tagMap[sourceID].Tag.Status = "merged"
			}
		}
	}

	return merged, nil
}

func computeTagSimilarity(tagAID, tagBID uint) (float64, error) {
	var embA, embB models.TopicTagEmbedding
	if err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", tagAID, EmbeddingTypeSemantic).First(&embA).Error; err != nil {
		return 0, err
	}
	if err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", tagBID, EmbeddingTypeSemantic).First(&embB).Error; err != nil {
		return 0, err
	}
	query := "SELECT ($1::vector <=> $2::vector) AS distance"
	var distance float64
	if err := database.DB.Raw(query, embA.EmbeddingVec, embB.EmbeddingVec).Scan(&distance).Error; err != nil {
		return 0, err
	}
	return 1.0 - distance, nil
}

func compressDeepSubtrees(root *TreeNode) (int, error) {
	merged := 0
	var compress func(node *TreeNode)
	compress = func(node *TreeNode) {
		for _, child := range node.Children {
			depth := calculateTreeDepth(child)
			if depth >= MinTreeDepthForCleanup {
				childMerged, err := compressSingleSubtree(child)
				if err != nil {
					logging.Warnf("compressDeepSubtrees: failed for %d: %v", child.Tag.ID, err)
				} else {
					merged += childMerged
				}
			}
			compress(child)
		}
	}
	compress(root)
	return merged, nil
}

func compressSingleSubtree(root *TreeNode) (int, error) {
	allTags := collectAllTags(root)
	tagMap := make(map[uint]*TreeNode, len(allTags))
	for _, t := range allTags {
		tagMap[t.Tag.ID] = t
	}

	var deepLeafNodes []*TreeNode
	for _, t := range allTags {
		if t.Depth >= 3 && len(t.Children) == 0 {
			deepLeafNodes = append(deepLeafNodes, t)
		}
	}

	if len(deepLeafNodes) == 0 {
		return 0, nil
	}

	batch := make([]*TreeNode, 0, 50)
	batch = append(batch, root)
	batch = append(batch, deepLeafNodes...)
	if len(batch) > 50 {
		batch = batch[:50]
	}

	prompt := buildDepthCompressionPrompt(root, batch)
	judgment, err := callCleanupLLM(prompt)
	if err != nil {
		return 0, err
	}

	merged := 0
	for _, merge := range judgment.Merges {
		source, sOk := tagMap[merge.SourceID]
		target, tOk := tagMap[merge.TargetID]
		if !sOk || !tOk {
			continue
		}
		if source.Tag.Status != "active" || target.Tag.Status != "active" {
			continue
		}
		if isDirectParentChild(source, target) {
			continue
		}
		depthDiff := abs(source.Depth - target.Depth)
		if depthDiff < 2 {
			continue
		}
		if mergeErr := MergeTags(merge.SourceID, merge.TargetID); mergeErr != nil {
			logging.Warnf("depth compression: merge %d→%d failed: %v", merge.SourceID, merge.TargetID, mergeErr)
			continue
		}
		merged++
	}

	return merged, nil
}

func buildDepthCompressionPrompt(root *TreeNode, batch []*TreeNode) string {
	var tagInfos []tagTreeInfo
	for _, tag := range batch {
		info := tagTreeInfo{
			ID:           tag.Tag.ID,
			Label:        tag.Tag.Label,
			Description:  truncateStr(tag.Tag.Description, 200),
			Depth:        tag.Depth,
			ArticleCount: tag.ArticleCount,
		}
		for _, child := range tag.Children {
			info.ChildrenIDs = append(info.ChildrenIDs, child.Tag.ID)
		}
		if tag.Parent != nil {
			pid := tag.Parent.Tag.ID
			info.ParentID = &pid
		}
		tagInfos = append(tagInfos, info)
	}

	sort.Slice(tagInfos, func(i, j int) bool {
		if tagInfos[i].Depth != tagInfos[j].Depth {
			return tagInfos[i].Depth < tagInfos[j].Depth
		}
		return tagInfos[i].Label < tagInfos[j].Label
	})

	promptData := map[string]interface{}{
		"tree_info": map[string]interface{}{
			"root_label": root.Tag.Label,
			"max_depth":  calculateTreeDepth(root),
			"total_tags": len(batch),
			"category":   root.Tag.Category,
		},
		"tags": tagInfos,
	}

	promptJSON, _ := json.MarshalIndent(promptData, "", "  ")

	return fmt.Sprintf(`你是一位标签分类专家。请分析以下标签树中深度过深的部分，找出应该合并的重复标签。

当前标签树结构（重点关注深度 >= 3 的叶节点）：
%s

请分析并返回以下格式的 JSON：
{
  "merges": [
    {
      "source_id": 123,
      "target_id": 456,
      "reason": "这两个标签描述的是同一个概念，应该合并"
    }
  ],
  "notes": "其他观察（可选）"
}

规则：
1. 只关注深度差 >= 2 的标签对
2. source_id: 被合并的标签（通常是更深层的）
3. target_id: 保留的目标标签（通常是更浅层的）
4. 只返回真正有把握的建议
5. 不需要创建新的抽象标签`, string(promptJSON))
}
```

### Step 2: Add Phase 4 to runCleanupCycle

In `jobs/tag_hierarchy_cleanup.go`, add after Phase 3 (after the empty abstracts cleanup block, around line 282):

```go
// Phase 4: C+D hierarchy optimization
for _, category := range []string{"event"} {
	forest, err := topicanalysis.BuildTagForest(category)
	if err != nil {
		logging.Errorf("Phase 4 forest build failed for %s: %v", category, err)
		summary.Errors++
		continue
	}
	for _, tree := range forest {
		result, err := topicanalysis.ProcessTree(tree)
		if err != nil {
			logging.Errorf("Phase 4 tree processing failed for tree %d: %v", tree.Tag.ID, err)
			summary.Errors++
			continue
		}
		summary.CDMergesApplied += result.MergesApplied
		summary.Errors += len(result.Errors)
		logging.Infof("Phase 4 (%s tree %d): %d merges, %d abstracts", category, tree.Tag.ID, result.MergesApplied, result.AbstractsCreated)
	}
}
```

Also add `CDMergesApplied` to `TagHierarchyCleanupRunSummary`:

```go
type TagHierarchyCleanupRunSummary struct {
	TriggerSource     string `json:"trigger_source"`
	StartedAt         string `json:"started_at"`
	FinishedAt        string `json:"finished_at"`
	ZombieDeactivated int    `json:"zombie_deactivated"`
	FlatMergesApplied int    `json:"flat_merges_applied"`
	OrphanedRelations int    `json:"orphaned_relations"`
	MultiParentFixed  int    `json:"multi_parent_fixed"`
	EmptyAbstracts    int    `json:"empty_abstracts"`
	CDMergesApplied   int    `json:"cd_merges_applied"`
	Errors            int    `json:"errors"`
	Reason            string `json:"reason"`
}
```

Update the `Reason` string to include `cd_merges`, and keep Phase 2 `flat_merges_applied` separate from Phase 4 `cd_merges_applied`.

Execution order for Tasks 5-7:
1. Task 5 helper layer: `getAllTreeTagIDs`, `findRelatedEventTagsFromAnchors`, `buildTreeDedupCandidate`, `judgeCrossLayerDuplicate`
2. Task 5 orchestration: wire shortlist + judge flow into `MatchAbstractTagHierarchy`
3. Task 6 guardrail: add the `> 4` hard check in `linkAbstractParentChild`
4. Task 7 integration: reuse the same C+D helpers inside cleanup Phase 4, do not fork a second dedup implementation

Recommended verification sequence:
1. `go test ./internal/domain/topicanalysis -run "TestCalculateCoTagTopN|Test.*Tree.*|Test.*Depth.*" -v`
2. `go build ./...`
3. `go test ./internal/domain/topicanalysis/... -v`
4. `go test ./... -count=1` after Phase 4 is wired

### Step 3: Build check

Run: `cd backend-go && go build ./...`
Expected: No errors

### Step 4: Run existing tests

Run: `cd backend-go && go test ./internal/domain/topicanalysis/... -v`
Expected: All existing tests pass

### Step 5: Commit

```bash
git add backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go backend-go/internal/jobs/tag_hierarchy_cleanup.go
git commit -m "feat: rewrite hierarchy cleanup with C+D cross-layer dedup and depth compression"
```

---

## Task 8: Full Build and Test Verification

**Files:** None (verification only)

### Step 1: Full backend build

Run: `cd backend-go && go build ./...`
Expected: No errors

### Step 2: Full test suite

Run: `cd backend-go && go test ./... -count=1`
Expected: All tests pass

### Step 3: Verify new functions compile with targeted tests

Run: `cd backend-go && go test ./internal/domain/topicanalysis -run TestCalculateCoTagTopN -v`
Expected: PASS

---

## Task 9: Update Documentation

**Files:**
- Modify: `docs/guides/topic-graph.md` (标签匹配与抽象层级 section)

### Step 1: Add documentation for co-tag expansion

Add a new section after "### Embedding 的动态补回" in `docs/guides/topic-graph.md`:

```markdown
### Co-tag 图遍历事件匹配

#### 概述

事件标签匹配时，除了 embedding 向量搜索外，还通过文章共现标签（co-tag）图遍历扩展候选池。

#### 匹配流程

**原始 event 标签（来自文章打标签）：**

```
新 event 标签 E 来自文章 A
  ↓
获取文章 A 的 top 5 标签（按 score 降序）
  ↓
对每个 co-tag，查涉及该标签的文章（限 10 篇）
  ↓
从这些文章提取 event 标签（限 20 个）
  ↓
与 embedding 候选做并集去重，统一送 LLM 判断
```

**抽象 event 标签：**

```
抽象 event 标签 E
  ↓
获取所有后代 event 子标签
  ↓
聚合子标签涉及文章的 keyword 标签
按子标签覆盖数排序，取 top N
  ↓
topN = min(5 + (subtreeDepth-1) * 2, 15)
  ↓
用这些 keyword 查关联文章 → 提取 event 标签 → 候选池
```

#### 适用范围

- 仅 event 类别标签
- 原始标签和抽象标签都适用
- 在 embedding 搜索之后、LLM 判断之前执行
- 候选与 embedding 候选做并集，统一处理

#### 相关代码

| 文件 | 职责 |
|------|------|
| `backend-go/internal/domain/topicanalysis/cotag_expansion.go` | co-tag 图遍历核心逻辑 |
| `backend-go/internal/domain/topicextraction/tagger.go` | findOrCreateTag 中的 co-tag 扩展集成点 |
| `backend-go/cmd/co-tag-backfill/main.go` | 一次性回填 CLI 命令 |

### 跨层级去重与深度限制（C+D）

#### 概述

防止抽象标签层级过深（超过 4 层）和跨层级重复概念。两个机制：
- **跨层级去重**：先用现有相似标签/keyword 检索找到 embedding 锚点，再回查同树内相关 event 标签，使用 `HighSimilarity` 阈值合并重复
- **深度限制**：在 `linkAbstractParentChild` 中检查深度，超过 4 层时由 AI 判断替代放置方案

#### 跨层级去重

新抽象标签创建后，`MatchAbstractTagHierarchy` 先复用现有 embedding 相似检索拿到一批高置信锚点，再从这些锚点反查相关 event 标签，并筛出同一棵树内的候选节点。如果发现 `HighSimilarity` 阈值以上的重复概念，直接合并，避免同一棵树不同层级出现语义重复。

#### 深度限制

插入新的父子关系前，检查组合深度（子标签子树深度 + 父标签祖先深度 + 1）。如果 > 4：
1. 拒绝纵向插入
2. 调用 AI 判断替代放置方案
3. AI 可以建议合并到已有标签或放置到更浅层级

#### 周期清理重写

`TagHierarchyCleanupScheduler` 的清理逻辑重写为 C+D 方式：
- Phase 1-3 不变（zombie 清理、flat merge、orphan 修复）
- 新增 Phase 4：对深度 >= 3 的树做跨层级去重 + 深度压缩

#### 相关代码

| 文件 | 职责 |
|------|------|
| `backend-go/internal/domain/topicanalysis/abstract_tag_service.go` | `MatchAbstractTagHierarchy` 跨层级去重、`linkAbstractParentChild` 深度限制 |
| `backend-go/internal/domain/topicanalysis/hierarchy_cleanup.go` | `crossLayerDedup`、`compressDeepSubtrees` 重写逻辑 |
| `backend-go/internal/jobs/tag_hierarchy_cleanup.go` | Phase 4 集成 |
```

### Step 2: Commit

```bash
git add docs/guides/topic-graph.md
git commit -m "docs: add co-tag expansion and C+D hierarchy cleanup documentation"
```

---

## Summary of Changes

| Task | What | Key Files |
|------|------|-----------|
| 1 | Co-tag expansion core service | `cotag_expansion.go` + test |
| 2 | Integrate into findOrCreateTag | `tagger.go`, `article_tagger.go` |
| 3 | Abstract event tag co-tag expansion | `abstract_tag_service.go` |
| 4 | Backfill CLI command | `cmd/co-tag-backfill/main.go` |
| 5 | Cross-layer dedup in MatchAbstractTagHierarchy | `abstract_tag_service.go` |
| 6 | Depth guard in linkAbstractParentChild | `abstract_tag_service.go` |
| 7 | Rewrite hierarchy cleanup | `hierarchy_cleanup.go`, `tag_hierarchy_cleanup.go` |
| 8 | Build and test verification | — |
| 9 | Documentation update | `topic-graph.md` |

**Dependencies:** Task 2 depends on Task 1. Task 3 depends on Task 1. Tasks 5 and 6 are independent of Tasks 1-4. Task 7 depends on Task 5. Tasks 8-9 depend on all previous tasks.

**Parallel execution:** Tasks 1-4 (co-tag) and Tasks 5-6 (hierarchy) can be developed in parallel. Task 7 depends on Task 5's helper functions.
