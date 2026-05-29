# Board Match Auto-Trigger & Daily Report Fallback Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Fix the broken board matching pipeline by auto-triggering `MatchTopicTag` after embedding completion, adding daily report fallback computation, and marking downgraded matches.

**Architecture:** Three-layer fix: (1) DB migration adds `downgraded` column, (2) backend adds auto-trigger in `processNext`, fallback in `collectBoardTags`, and `downgraded` flag through the matching/API chain, (3) frontend displays downgraded info in MatchDetailPanel and tag chips.

**Tech Stack:** Go (Gin/GORM), Vue 3 + TypeScript, PostgreSQL

---

## Task 1: DB Migration — `topic_tag_board_labels.downgraded` + Model Update

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go`
- Modify: `backend-go/internal/domain/models/semantic_label.go`

**Step 1: Add migration**

In `postgres_migrations.go`, add a new migration entry AFTER the last one (`20260526_0001`). Version: `20260528_0001`:

```go
{
    Version:     "20260528_0001",
    Description: "Add downgraded column to topic_tag_board_labels for max_sim threshold reduction tracking.",
    Up: func(db *gorm.DB) error {
        return db.Exec("ALTER TABLE topic_tag_board_labels ADD COLUMN IF NOT EXISTS downgraded BOOLEAN NOT NULL DEFAULT false").Error
    },
},
```

**Step 2: Update model struct**

In `backend-go/internal/domain/models/semantic_label.go`, add `Downgraded` to `TopicTagBoardLabel`:

```go
type TopicTagBoardLabel struct {
	TopicTagID      uint      `gorm:"primaryKey;not null" json:"topic_tag_id"`
	SemanticBoardID uint      `gorm:"primaryKey;not null" json:"semantic_board_id"`
	Score           float64   `gorm:"not null;default:0" json:"score"`
	MatchReason     string    `gorm:"type:text" json:"match_reason"`
	Downgraded      bool      `gorm:"not null;default:false" json:"downgraded"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	TopicTag      *TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticBoard *SemanticLabel `gorm:"foreignKey:SemanticBoardID;constraint:OnDelete:CASCADE" json:"semantic_board,omitempty"`
}
```

**Step 3: Verify**

Run: `cd backend-go && go build ./...`
Expected: Builds successfully

---

## Task 2: Matching Core — Downgraded Flag in `evaluateSemanticBoardMatches`

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching.go`

**Step 1: Add Downgraded to SemanticBoardMatchResult**

At line ~43, update struct:

```go
type SemanticBoardMatchResult struct {
	SemanticBoardID uint
	Score           float64
	MatchReason     string
	Downgraded      bool
}
```

**Step 2: Calculate downgraded in evaluateSemanticBoardMatches**

In the loop body, after the existing `minHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))` (line ~161), add:

```go
downgraded := minHits < config.DirectMaxSimMinHits
```

Then in the `switch` case for max_sim (line ~166-168), include `Downgraded`:

```go
case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
    score = maxSimilarity
    matchReason = "max_sim"
```

And in the match append (line ~174), include `Downgraded`:

```go
if matchReason != "" {
    matches = append(matches, SemanticBoardMatchResult{SemanticBoardID: boardID, Score: score, MatchReason: matchReason, Downgraded: downgraded})
}
```

Note: For hit_rate, direct_hit, and weighted rules, `downgraded` will be `false` (only true when `matchReason == "max_sim"` AND `minHits < config.DirectMaxSimMinHits`). Since `downgraded` is only set meaningfully in the max_sim case, compute it before the switch so it's available, but only the max_sim branch produces `downgraded=true`.

Actually, to be cleaner — only set `downgraded = true` inside the max_sim case, otherwise it stays `false`. Refactored approach:

```go
hits := int(math.Round(hitRate * float64(max(len(tagAuxiliaries), config.MinEffectiveSample))))
minHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))
downgraded := false
switch {
case hitRate > config.DirectHitRate:
    score = config.HitRateSimBlend*maxSimilarity + (1-config.HitRateSimBlend)*hitRate
    matchReason = "hit_rate"
case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
    score = maxSimilarity
    matchReason = "max_sim"
    if minHits < config.DirectMaxSimMinHits {
        downgraded = true
    }
case weighted >= config.WeightedThreshold:
    score = weighted
    matchReason = "weighted"
}
if matchReason != "" {
    matches = append(matches, SemanticBoardMatchResult{SemanticBoardID: boardID, Score: score, MatchReason: matchReason, Downgraded: downgraded})
}
```

**Step 3: Update replaceTopicTagBoardLabels to persist Downgraded**

In `replaceTopicTagBoardLabels` (line ~335), update the row creation:

```go
row := models.TopicTagBoardLabel{TopicTagID: topicTagID, SemanticBoardID: match.SemanticBoardID, Score: match.Score, MatchReason: match.MatchReason, Downgraded: match.Downgraded}
```

**Step 4: Add test for downgraded marking**

In `backend-go/internal/domain/tagging/semantic_board_matching_test.go`, add:

```go
func TestEvaluateSemanticBoardMatches_DowngradedMark(t *testing.T) {
    // This tests the downgraded flag specifically
    // We need a tag with 1 auxiliary, hitting max_sim with minHits degraded to 1

    config := defaultTestConfig()
    config.DirectMaxSim = 0.8
    config.DirectMaxSimMinHits = 2
    config.DirectMaxSimMinHitRate = 0.3
    config.SimThreshold = 0.5
    config.MinEffectiveSample = 3

    // tag has 1 auxiliary → minHits = min(2, 1) = 1 < 2 → downgraded
    tagAux := []models.SemanticLabel{
        {ID: 1, Embedding: ptr(pgVectorStr([]float64{0.9, 0.1, 0.0}))},
    }
    boardAux := []boardAuxiliaryLabel{
        {BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptr(pgVectorStr([]float64{0.85, 0.15, 0.0}))},
    }

    results := evaluateSemanticBoardMatches(tagAux, boardAux, config)
    if len(results) == 0 {
        t.Fatal("expected a match")
    }
    if results[0].MatchReason != "max_sim" {
        t.Fatalf("expected max_sim, got %s", results[0].MatchReason)
    }
    if !results[0].Downgraded {
        t.Fatal("expected downgraded=true for N=1 tag")
    }

    // tag has 3 auxiliaries → minHits = min(2, 3) = 2 → NOT downgraded
    tagAux3 := []models.SemanticLabel{
        {ID: 1, Embedding: ptr(pgVectorStr([]float64{0.9, 0.1, 0.0}))},
        {ID: 2, Embedding: ptr(pgVectorStr([]float64{0.88, 0.12, 0.0}))},
        {ID: 3, Embedding: ptr(pgVectorStr([]float64{0.86, 0.14, 0.0}))},
    }
    results3 := evaluateSemanticBoardMatches(tagAux3, boardAux, config)
    if len(results3) == 0 {
        t.Fatal("expected a match")
    }
    if results3[0].Downgraded {
        t.Fatal("expected downgraded=false for N=3 tag")
    }
}
```

Note: Look at existing test helpers (e.g., `defaultTestConfig`, `pgVectorStr`, `ptr`) already in the test file to ensure consistency.

**Step 5: Verify**

Run: `cd backend-go && go build ./... && go vet ./... && go test ./internal/domain/tagging/... -run TestEvaluateSemanticBoardMatches_DowngradedMark -v`
Expected: Test passes

---

## Task 3: Auto-Trigger — Singleton + processNext Integration

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching.go` (add singleton getter)
- Modify: `backend-go/internal/domain/tagging/embedding_queue.go` (add auto-trigger in processNext)

**Step 1: Add getSemanticBoardMatchingService singleton**

In `semantic_board_matching.go`, add after `NewSemanticBoardMatchingService`:

```go
var (
	semanticBoardMatchingService     *SemanticBoardMatchingService
	semanticBoardMatchingServiceOnce sync.Once
)

func getSemanticBoardMatchingService() *SemanticBoardMatchingService {
	semanticBoardMatchingServiceOnce.Do(func() {
		semanticBoardMatchingService = NewSemanticBoardMatchingService(database.DB)
	})
	return semanticBoardMatchingService
}
```

Ensure `"sync"` is imported.

**Step 2: Add auto-trigger in processNext**

In `embedding_queue.go` `processNext`, insert AFTER the event keyword embedding block (after line `s.logger.Info("event keyword embeddings generated", ...)`) and BEFORE the `// Mark completed` comment:

```go
		// Auto-trigger board matching for event tags after embedding completion
		if tag.Category == "event" {
			if matcher := getSemanticBoardMatchingService(); matcher != nil {
				if _, matchErr := matcher.MatchTopicTag(ctx, tag.ID); matchErr != nil {
					s.logger.Warn("auto board match failed", zap.Uint("tag_id", tag.ID), zap.Error(matchErr))
				}
			}
		}
```

**Step 3: Verify**

Run: `cd backend-go && go build ./...`
Expected: Builds successfully

---

## Task 4: Daily Report Fallback — collectBoardTags补算

**Files:**
- Modify: `backend-go/internal/domain/daily_report/generator.go`

**Step 1: Add fallback query and computation in collectBoardTags**

After the existing `collectBoardTags` function returns tags (after the `for` loop that builds `tags` and `articleIDSets`), add fallback logic BEFORE the final `return tags, articleIDSets, nil`:

```go
	// Fallback: find event tags with auxiliaries but no board labels
	var unmatchedTagIDs []uint
	s.db.Model(&models.TopicTag{}).
		Select("DISTINCT topic_tags.id").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Joins("JOIN topic_tag_semantic_labels ON topic_tag_semantic_labels.topic_tag_id = topic_tags.id").
		Where("topic_tags.status = ? AND topic_tags.category = ?", "active", models.TagCategoryEvent).
		Where("articles.pub_date >= ? AND articles.pub_date < ?", startOfDay, endOfDay).
		Where("NOT EXISTS (SELECT 1 FROM topic_tag_board_labels WHERE topic_tag_board_labels.topic_tag_id = topic_tags.id)").
		Limit(50).
		Pluck("topic_tags.id", &unmatchedTagIDs)

	if len(unmatchedTagIDs) > 0 {
		log.Printf("[daily-report] fallback: found %d unmatched event tags for board %d, computing matches", len(unmatchedTagIDs), boardID)
		matcher := tagging.NewSemanticBoardMatchingService(s.db)
		for _, tid := range unmatchedTagIDs {
			matches, err := matcher.MatchTopicTag(context.Background(), tid)
			if err != nil {
				log.Printf("[daily-report] fallback match failed for tag %d: %v", tid, err)
				continue
			}
			// Check if any match is for the target board
			matched := false
			for _, m := range matches {
				if m.SemanticBoardID == boardID {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			// This tag matches our board — add it to results
			var t models.TopicTag
			if err := s.db.First(&t, tid).Error; err != nil {
				continue
			}
			var artIDs []uint
			s.db.Model(&models.ArticleTopicTag{}).
				Select("DISTINCT article_topic_tags.article_id").
				Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
				Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?", tid, startOfDay, endOfDay).
				Pluck("article_topic_tags.article_id", &artIDs)

			tags = append(tags, TagInput{
				ID: t.ID, Label: t.Label, Category: t.Category,
				Description: t.Description, Source: t.Source,
				ArticleCount: len(artIDs),
			})
			articleIDSets = append(articleIDSets, artIDs)
		}
	}
```

**IMPORTANT**: `collectBoardTags` is currently a package-level function using `database.DB` directly. It needs access to the database through `database.DB` (same pattern it already uses). The import for `tagging` package may create a circular dependency — check if `daily_report` already imports `tagging`. If not, we should use `tagging.NewSemanticBoardMatchingService(database.DB)` with the proper import alias.

**Alternative if circular import**: Since `SemanticBoardMatchingService` only needs `*gorm.DB`, we can just construct it inline: `tagging.NewSemanticBoardMatchingService(database.DB)` — but need to check import chain.

**Step 2: Verify**

Run: `cd backend-go && go build ./...`
Expected: Builds successfully (check for circular imports; if found, restructure accordingly)

---

## Task 5: Backend API — getTagMatchDetail Returns Downgraded + effective_min_hits

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go`

**Step 1: Add fields to matchDetailResponse DTO**

```go
type matchDetailResponse struct {
	TopicTagID           uint                    `json:"topic_tag_id"`
	TopicTagLabel        string                  `json:"topic_tag_label"`
	SemanticBoardID      uint                    `json:"semantic_board_id"`
	MatchReason          string                  `json:"match_reason"`
	Score                float64                 `json:"score"`
	Downgraded           bool                    `json:"downgraded"`
	EffectiveMinHits     int                     `json:"effective_min_hits"`
	Config               matchDetailConfigDTO    `json:"config"`
	DirectHitAuxiliaries []directHitAuxiliaryDTO `json:"direct_hit_auxiliaries"`
	TagAuxiliaryCount    int                     `json:"tag_auxiliary_count"`
	Hits                 int                     `json:"hits"`
	HitRate              float64                 `json:"hit_rate"`
	MaxSimilarity        float64                 `json:"max_similarity"`
	Pairs                []matchDetailPairDTO    `json:"pairs"`
}
```

**Step 2: Populate fields in getTagMatchDetail**

In the `getTagMatchDetail` handler, after getting `stored` and before `respondOK`, compute:

```go
effectiveMinHits := min(config.DirectMaxSimMinHits, len(tagAuxiliaries))
```

Then in the respondOK call, add:

```go
respondOK(c, matchDetailResponse{
    // ... existing fields ...
    Downgraded:       stored.Downgraded,
    EffectiveMinHits: effectiveMinHits,
    // ... existing fields ...
})
```

Note: `stored.Downgraded` comes from the persisted `TopicTagBoardLabel` record, which was written by `replaceTopicTagBoardLabels`.

**Step 3: Verify**

Run: `cd backend-go && go build ./...`
Expected: Builds successfully

---

## Task 6: Backend API — getBoardArticles Returns Downgraded

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go`

**Step 1: Add Downgraded to filteredTagRow and boardArticleTagDTO**

Update `filteredTagRow`:

```go
type filteredTagRow struct {
    ArticleID    uint    `gorm:"column:article_id"`
    ID           uint    `gorm:"column:id"`
    Label        string  `gorm:"column:label"`
    Category     string  `gorm:"column:category"`
    MatchReason  string  `gorm:"column:match_reason"`
    Score        float64 `gorm:"column:score"`
    Downgraded   bool    `gorm:"column:downgraded"`
}
```

Update the query SELECT to include `tbl.downgraded`:

```go
Select("att.article_id, tt.id, tt.label, tt.category, tbl.match_reason, tbl.score, tbl.downgraded").
```

Update `boardArticleTagDTO`:

```go
type boardArticleTagDTO struct {
    ID          uint    `json:"id"`
    Label       string  `json:"label"`
    Category    string  `json:"category"`
    MatchReason string  `json:"match_reason"`
    Score       float64 `json:"score"`
    Downgraded  bool    `json:"downgraded"`
}
```

Update the tagMap building to include Downgraded:

```go
tagMap[tr.ArticleID] = append(tagMap[tr.ArticleID], boardArticleTagDTO{
    ID:          tr.ID,
    Label:       tr.Label,
    Category:    tr.Category,
    MatchReason: tr.MatchReason,
    Score:       tr.Score,
    Downgraded:  tr.Downgraded,
})
```

**Step 2: Verify**

Run: `cd backend-go && go build ./...`
Expected: Builds successfully

---

## Task 7: Frontend API Types Update

**Files:**
- Modify: `front/app/api/semanticBoards.ts`

**Step 1: Update MatchDetailResponse**

Add `downgraded` and `effective_min_hits`:

```ts
export interface MatchDetailResponse {
  topic_tag_id: number
  topic_tag_label: string
  semantic_board_id: number
  match_reason: string
  score: number
  downgraded: boolean
  effective_min_hits: number
  config: MatchDetailConfig
  direct_hit_auxiliaries: DirectHitAuxiliary[]
  tag_auxiliary_count: number
  hits: number
  hit_rate: number
  max_similarity: number
  pairs: MatchDetailPair[]
}
```

**Step 2: Update BoardArticleTag**

Add `downgraded`:

```ts
export interface BoardArticleTag {
  id: number
  label: string
  category: string
  match_reason: string
  score: number
  downgraded: boolean
}
```

**Step 3: Verify**

Run: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"`
Expected: No type errors

---

## Task 8: Frontend — MatchDetailPanel Downgraded Display

**Files:**
- Modify: `front/app/features/tags/components/MatchDetailPanel.vue`

**Step 1: Update flowSteps computed — Step ④ for max_sim**

In the `flowSteps` computed property, find step ④ (max_sim). When the step is matched (`match_reason === 'max_sim'`) and `d.downgraded` is true, append a deprecation note to the label.

Specifically, in the step ④ matched state section, after the existing success label like:
```
✓S${maxSim.toFixed(2)} ✓${hits}≥${minHits}命中 ✓R=${hitRate.toFixed(2)} → 满足！
```

Add a conditional warning when `d.downgraded`:
```
 ⚠降级匹配（原阈值${c.direct_max_sim_min_hits}，因仅有${tagAuxCount}个辅助标签降为${d.effective_min_hits}）
```

**Step 2: Verify**

Run: `cd front && pnpm lint` then `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"`
Expected: Lint passes, no type errors

---

## Task 9: Frontend — Tag Chip Downgraded Styling

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Step 1: Update matchReasonColor for downgraded**

Modify the `matchReasonColor` function to accept an optional `downgraded` parameter:

```ts
function matchReasonColor(reason: string, downgraded?: boolean): string {
  const colors: Record<string, string> = {
    direct_hit: '#22c55e',
    hit_rate:   '#3b82f6',
    max_sim:    '#f59e0b',
    weighted:   '#94a3b8',
  }
  const color = colors[reason] || '#94a3b8'
  return downgraded ? color + '80' : color  // append 50% opacity for downgraded
}
```

**Step 2: Update chip template**

In the tag chip rendering, update the `:style` binding:

```html
:style="{ borderColor: matchReasonColor(tag.match_reason, tag.downgraded) }"
```

And update the chip content to show "↓" for downgraded:

```html
{{ tag.label }} {{ tag.score.toFixed(2) }}{{ tag.downgraded ? '↓' : '' }}
```

**Step 3: Update matchInfoLabel for downgraded**

If `matchInfoLabel` is used elsewhere, ensure it also reflects the downgraded state (add "↓" suffix).

**Step 4: Verify**

Run: `cd front && pnpm lint` then `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"`
Expected: Lint passes, no type errors

---

## Task 10: Full Verification

**Step 1: Backend full check**

Run: `cd backend-go && golangci-lint run ./... && go vet ./... && go test ./internal/domain/tagging/... ./internal/domain/daily_report/... && go build ./...`

**Step 2: Frontend full check**

Run: `cd front && pnpm lint`
Then: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck && pnpm build"`

---

## Dependency Graph

```
Task 1 (DB migration + model) ─────┐
                                    ├→ Task 2 (matching core) ──→ Task 3 (auto-trigger)
                                    │                            Task 4 (daily report fallback)
                                    ├→ Task 5 (match-detail API)
                                    ├→ Task 6 (board articles API)
                                    │
Task 7 (frontend types)  ──────────┼→ Task 8 (MatchDetailPanel)
                                    └→ Task 9 (tag chip styling)

Task 10 (full verification) ← all tasks
```

**Parallelizable groups:**
- Group A (backend core): Tasks 1→2→3, Task 4 (after Task 1), Tasks 5+6 (after Task 1)
- Group B (frontend): Tasks 7→8, Task 7→9
