# Board Direction Check & Board Editing Implementation Plan

> **REQUIRED SUB-SKILL:** Use the executing-plans skill to implement this plan task-by-task.

**Goal:** Add direction check to max_sim board matching (tag identity embedding × board embedding cosine), fix missing board embedding on LLM upgrade create_new, add board editing UI, and filter direction_mismatch tags.

**Architecture:** Backend adds `direction_mismatch` field to `topic_tag_board_labels`, `evaluateSemanticBoardMatches` gains direction check params, API returns `direction_sim`, daily report excludes mismatched tags. Frontend adds board editing dialog, direction mismatch toggle, and match detail direction display.

**Tech Stack:** Go (Gin/GORM), Vue 3 (Nuxt 4), TypeScript, PostgreSQL (pgvector)

---

## Task 1: DB Migration — direction_mismatch column

**Files:**
- Modify: `backend-go/internal/platform/database/postgres_migrations.go` (append after last migration `20260528_0001`)
- Modify: `backend-go/internal/domain/models/semantic_label.go:39-51`

**Step 1: Add migration**

Append new migration after the existing `20260528_0001` entry in `postgresMigrations()`:

```go
{
    Version:     "20260528_0002",
    Description: "Add direction_mismatch column to topic_tag_board_labels for max_sim direction check.",
    Up: func(db *gorm.DB) error {
        return db.Exec("ALTER TABLE topic_tag_board_labels ADD COLUMN IF NOT EXISTS direction_mismatch BOOLEAN NOT NULL DEFAULT false").Error
    },
},
```

**Step 2: Add DirectionMismatch field to TopicTagBoardLabel model**

In `backend-go/internal/domain/models/semantic_label.go`, add `DirectionMismatch` to `TopicTagBoardLabel` struct, after the `Downgraded` field:

```go
type TopicTagBoardLabel struct {
	TopicTagID        uint      `gorm:"primaryKey;not null" json:"topic_tag_id"`
	SemanticBoardID   uint      `gorm:"primaryKey;not null" json:"semantic_board_id"`
	Score             float64   `gorm:"not null;default:0" json:"score"`
	MatchReason       string    `gorm:"type:text" json:"match_reason"`
	Downgraded        bool      `gorm:"not null;default:false" json:"downgraded"`
	DirectionMismatch bool      `gorm:"not null;default:false" json:"direction_mismatch"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	TopicTag      *TopicTag      `gorm:"foreignKey:TopicTagID;constraint:OnDelete:CASCADE" json:"topic_tag,omitempty"`
	SemanticBoard *SemanticLabel `gorm:"foreignKey:SemanticBoardID;constraint:OnDelete:CASCADE" json:"semantic_board,omitempty"`
}
```

**Step 3: Verify**

Run: `cd backend-go && go build ./...`
Expected: compiles without error

**Step 4: Commit**

```bash
git add backend-go/internal/platform/database/postgres_migrations.go backend-go/internal/domain/models/semantic_label.go
git commit -m "feat(board-matching): add direction_mismatch column to topic_tag_board_labels"
```

---

## Task 2: Bug Fix — LLM upgrade create_new generates board embedding

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade.go:19-22,141-202`
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go:1085` (the `executeUpgrade` handler — pass embedder)
- Modify: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go` (existing tests need embedder mock)

**Step 1: Add embedder field to SemanticBoardUpgradeService**

In `semantic_board_upgrade.go`, change the struct and constructor:

```go
type SemanticBoardUpgradeService struct {
	db       *gorm.DB
	llm      semanticBoardUpgradeLLM
	embedder auxiliaryLabelEmbedder  // NEW
}

func NewSemanticBoardUpgradeService(db *gorm.DB, llm semanticBoardUpgradeLLM, embedder auxiliaryLabelEmbedder) *SemanticBoardUpgradeService {
	if db == nil {
		db = database.DB
	}
	return &SemanticBoardUpgradeService{db: db, llm: llm, embedder: embedder}
}
```

**Step 2: Generate embedding in ConfirmSuggestion create_new branch**

In `ConfirmSuggestion`, after creating the board but before the transaction commits, add embedding generation. Modify the `create_new` case:

```go
case SemanticBoardUpgradeDecisionCreateNew:
	label := strings.TrimSpace(req.BoardLabel)
	if label == "" {
		return fmt.Errorf("board label is required")
	}
	board := models.SemanticLabel{
		Label:       label,
		Slug:        uniqueSemanticLabelSlug(tx, Slugify(label)),
		LabelType:   "board",
		Description: req.Description,
		Source:      "llm_suggest",
		Status:      "active",
	}
	// Generate board embedding
	if s.embedder != nil {
		input := label
		if desc := strings.TrimSpace(req.Description); desc != "" {
			input = label + ". " + desc
		}
		pgVector, _, embedErr := s.embedder(ctx, input, auxiliaryLabelEmbeddingModeStorage)
		if embedErr != nil {
			return fmt.Errorf("generate board embedding: %w", embedErr)
		}
		board.Embedding = &pgVector
	}
	if err := tx.Create(&board).Error; err != nil {
		return err
	}
	boardID = board.ID
```

**Step 3: Update all NewSemanticBoardUpgradeService call sites**

Find all callers and pass `nil` for existing ones that don't need embedding (like `nil` LLM ones). The handler in `semantic_board_handler.go`:

In `executeUpgrade` (line ~1085):
```go
result, err := NewSemanticBoardUpgradeService(h.db, nil, semanticBoardLabelEmbedder).ConfirmSuggestion(...)
```

In `suggestUpgrades` and `getUpgradeCandidates` handlers — these pass `nil` or `semanticBoardUpgradeLLMFactory()` for LLM but don't need embedder. Update to pass `nil`:
```go
NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory(), nil)
```

In tests: pass `nil` as third argument for all existing callers.

**Step 4: Verify**

Run: `cd backend-go && go build ./...`
Expected: compiles without error

**Step 5: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_upgrade.go backend-go/internal/domain/tagging/semantic_board_handler.go backend-go/internal/domain/tagging/semantic_board_upgrade_test.go
git commit -m "fix(board-upgrade): generate board embedding on LLM create_new"
```

---

## Task 3: Board embedding backfill + description change refresh + rematch API

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go:252-286` (updateSemanticBoard)
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go:140-175` (RegisterSemanticBoardRoutes)
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go` (add backfill-embeddings + rematch-all handlers)

**Step 1: updateSemanticBoard — refresh embedding on description change**

In `updateSemanticBoard`, after the label-change embedding block (lines 267-276), add description change embedding refresh. The embedding input is unified to `label + ". " + description`:

Current code at line 267-276:
```go
if label := strings.TrimSpace(req.Label); label != "" && label != board.Label {
	pgVector, _, err := semanticBoardLabelEmbedder(c.Request.Context(), label, auxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	board.Label = label
	board.Slug = uniqueSemanticLabelSlug(h.db.WithContext(c.Request.Context()).Where("id <> ?", board.ID), Slugify(label))
	board.Embedding = &pgVector
}
```

Change to:
```go
if label := strings.TrimSpace(req.Label); label != "" && label != board.Label {
	board.Label = label
	board.Slug = uniqueSemanticLabelSlug(h.db.WithContext(c.Request.Context()).Where("id <> ?", board.ID), Slugify(label))
}
if desc := strings.TrimSpace(req.Description); desc != board.Description {
	board.Description = desc
}
// Regenerate embedding if label or description changed
if board.Label != boardOrigLabel || board.Description != boardOrigDesc {
	input := board.Label
	if board.Description != "" {
		input = board.Label + ". " + board.Description
	}
	pgVector, _, err := semanticBoardLabelEmbedder(c.Request.Context(), input, auxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	board.Embedding = &pgVector
}
```

Note: You'll need to save `boardOrigLabel` and `boardOrigDesc` before modifications:
```go
boardOrigLabel := board.Label
boardOrigDesc := board.Description
```
Add these two lines right after loading the board (after line 265).

**Step 2: Add backfill-embeddings handler**

Add new handler method:

```go
func (h *semanticBoardHandler) backfillBoardEmbeddings(c *gin.Context) {
	ctx := c.Request.Context()
	var boards []models.SemanticLabel
	if err := h.db.WithContext(ctx).
		Where("label_type = ? AND embedding IS NULL", "board").
		Find(&boards).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	count := 0
	for _, board := range boards {
		input := board.Label
		if board.Description != "" {
			input = board.Label + ". " + board.Description
		}
		pgVector, _, err := semanticBoardLabelEmbedder(ctx, input, auxiliaryLabelEmbeddingModeStorage)
		if err != nil {
			logging.Warnf("[backfill-embeddings] failed for board %d (%s): %v", board.ID, board.Label, err)
			continue
		}
		if err := h.db.WithContext(ctx).Model(&models.SemanticLabel{}).Where("id = ?", board.ID).Update("embedding", pgVector).Error; err != nil {
			logging.Warnf("[backfill-embeddings] db update failed for board %d: %v", board.ID, err)
			continue
		}
		count++
	}
	respondOK(c, gin.H{"backfilled": count, "total": len(boards)})
}
```

Add `import "syntopica-backend/internal/platform/logging"` if not already imported.

**Step 3: Add rematch-all handler**

```go
func (h *semanticBoardHandler) rematchAll(c *gin.Context) {
	ctx := c.Request.Context()
	var tagIDs []uint
	if err := h.db.WithContext(ctx).
		Model(&models.TopicTagBoardLabel{}).
		Distinct("topic_tag_id").
		Pluck("topic_tag_id", &tagIDs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	matcher := NewSemanticBoardMatchingService(h.db)
	success, failed := 0, 0
	for _, tid := range tagIDs {
		if _, err := matcher.MatchTopicTag(ctx, tid); err != nil {
			logging.Warnf("[rematch-all] failed for tag %d: %v", tid, err)
			failed++
			continue
		}
		success++
	}
	respondOK(c, gin.H{"success": success, "failed": failed, "total": len(tagIDs)})
}
```

**Step 4: Register new routes**

In `RegisterSemanticBoardRoutes`, add these routes near the other board-level routes (after the upgrade routes, before `boards.GET("/:id", ...)`)：

```go
boards.POST("/backfill-embeddings", handler.backfillBoardEmbeddings)
boards.POST("/rematch-all", handler.rematchAll)
```

**Step 5: Verify**

Run: `cd backend-go && go build ./...`
Expected: compiles without error

**Step 6: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_handler.go
git commit -m "feat(board): add backfill-embeddings + rematch-all API, refresh embedding on description change"
```

---

## Task 4: Matching Core — direction check in evaluateSemanticBoardMatches

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching.go:41-61,135-203,347-360,362-446`
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching_test.go` (add new test)

**Step 1: Add fields to config and result structs**

`SemanticBoardMatchConfig` — add field:
```go
DirectionSimThreshold float64
```

`SemanticBoardMatchResult` — add field:
```go
DirectionMismatch bool
```

**Step 2: Change evaluateSemanticBoardMatches signature**

From:
```go
func evaluateSemanticBoardMatches(tagAuxiliaries []models.SemanticLabel, boardAuxiliaries []boardAuxiliaryLabel, config SemanticBoardMatchConfig) []SemanticBoardMatchResult
```

To:
```go
func evaluateSemanticBoardMatches(tagAuxiliaries []models.SemanticLabel, boardAuxiliaries []boardAuxiliaryLabel, config SemanticBoardMatchConfig, tagEmbedding []float64, boardEmbeddings map[uint][]float64) []SemanticBoardMatchResult
```

**Step 3: Add direction check after max_sim match**

Inside `evaluateSemanticBoardMatches`, after the `max_sim` case (line ~182-186), add direction check:

```go
case maxSimilarity >= config.DirectMaxSim && hits >= minHits && hitRate >= config.DirectMaxSimMinHitRate:
	score = maxSimilarity
	matchReason = "max_sim"
	if minHits < config.DirectMaxSimMinHits {
		downgraded = true
	}
	// Direction check: only for max_sim
	directionMismatch := false
	if tagEmbedding != nil && len(tagEmbedding) > 0 {
		if boardEmb, ok := boardEmbeddings[boardID]; ok && len(boardEmb) > 0 {
			dirSim := cosineSimilarity(tagEmbedding, boardEmb)
			if dirSim < config.DirectionSimThreshold {
				directionMismatch = true
			}
		}
	}
```

Update the match append to include DirectionMismatch:
```go
if matchReason != "" {
	matches = append(matches, SemanticBoardMatchResult{
		SemanticBoardID: boardID, Score: score, MatchReason: matchReason,
		Downgraded: downgraded, DirectionMismatch: directionMismatch,
	})
}
```

Note: `directionMismatch` needs to be declared before the switch block and initialized to `false`. Add `directionMismatch := false` near where `downgraded := false` is declared. Then only set it in the `max_sim` case.

**Step 4: Add cosineSimilarity helper**

Add a reusable cosine similarity function (pure float64 slices, no pgvector parsing):

```go
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
```

**Step 5: Update replaceTopicTagBoardLabels**

Change the row creation to include `DirectionMismatch`:

```go
row := models.TopicTagBoardLabel{
	TopicTagID: topicTagID, SemanticBoardID: match.SemanticBoardID,
	Score: match.Score, MatchReason: match.MatchReason,
	Downgraded: match.Downgraded, DirectionMismatch: match.DirectionMismatch,
}
```

**Step 6: Update loadConfig**

Add to the defaults:
```go
DirectionSimThreshold: 0.5,
```

Add to the SQL WHERE IN list:
```go
"semantic_board_match_direction_sim_threshold",
```

Add to the switch:
```go
case "semantic_board_match_direction_sim_threshold":
	config.DirectionSimThreshold = parseSemanticBoardMatchFloat(setting.Value, config.DirectionSimThreshold)
```

**Step 7: Update semanticBoardMatchConfigToMap**

In `semantic_board_handler.go`, add to the returned map:
```go
"semantic_board_match_direction_sim_threshold": config.DirectionSimThreshold,
```

**Step 8: Update all existing callers of evaluateSemanticBoardMatches**

- `MatchTopicTag` (line ~92): pass `tagEmbedding` and `boardEmbeddings` (will be loaded in Task 5 — for now pass `nil, nil`)
- Tests: update all test calls to pass `nil, nil` as extra args

**Step 9: Add unit test**

Add `TestEvaluateSemanticBoardMatches_DirectionCheck` to the test file:

```go
func TestEvaluateSemanticBoardMatches_DirectionCheck(t *testing.T) {
	config := SemanticBoardMatchConfig{
		SimThreshold: 0.5, DirectHitRate: 0.5, DirectMaxSim: 0.7,
		DirectMaxSimMinHits: 1, DirectMaxSimMinHitRate: 0.2,
		MinEffectiveSample: 3, HitRateSimBlend: 0.7, WeightSim: 0.6,
		WeightDensity: 0.4, WeightedThreshold: 0.6, MaxBoards: 3,
		DirectHitMinOverlap: 2, DirectionSimThreshold: 0.5,
	}

	tagAux := []models.SemanticLabel{
		{ID: 1, Embedding: ptrStr(floatsToPgVector([]float64{0.9, 0.1, 0.0}))},
	}
	boardAux := []boardAuxiliaryLabel{
		{BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptrStr(floatsToPgVector([]float64{0.85, 0.15, 0.0}))},
	}
	// sim(0.9,0.1) × (0.85,0.15) ≈ 0.998 → max_sim match

	t.Run("direction sim above threshold → mismatch=false", func(t *testing.T) {
		tagEmb := []float64{0.9, 0.1, 0.0}
		boardEmbs := map[uint][]float64{10: {0.85, 0.15, 0.0}}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, config, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.False(t, results[0].DirectionMismatch)
	})

	t.Run("direction sim below threshold → mismatch=true", func(t *testing.T) {
		tagEmb := []float64{0.1, 0.9, 0.0}
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, config, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.True(t, results[0].DirectionMismatch)
	})

	t.Run("no tag embedding → mismatch=false (skip)", func(t *testing.T) {
		results := evaluateSemanticBoardMatches(tagAux, boardAux, config, nil, nil)
		require.Len(t, results, 1)
		require.False(t, results[0].DirectionMismatch)
	})

	t.Run("no board embedding → mismatch=false (skip)", func(t *testing.T) {
		tagEmb := []float64{0.1, 0.9, 0.0}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, config, tagEmb, nil)
		require.Len(t, results, 1)
		require.False(t, results[0].DirectionMismatch)
	})

	t.Run("non-max_sim match → no direction check", func(t *testing.T) {
		// Create config that triggers hit_rate instead
		hrConfig := config
		hrConfig.DirectHitRate = 0.01
		// Use enough tag auxiliaries with high similarity to get hit_rate
		tagAuxMulti := []models.SemanticLabel{
			{ID: 1, Embedding: ptrStr(floatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{ID: 2, Embedding: ptrStr(floatsToPgVector([]float64{0.88, 0.12, 0.0}))},
			{ID: 3, Embedding: ptrStr(floatsToPgVector([]float64{0.86, 0.14, 0.0}))},
		}
		boardAuxMulti := []boardAuxiliaryLabel{
			{BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptrStr(floatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{BoardID: 10, AuxiliaryLabelID: 101, Embedding: ptrStr(floatsToPgVector([]float64{0.88, 0.12, 0.0}))},
		}
		tagEmb := []float64{0.1, 0.9, 0.0}
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}}
		results := evaluateSemanticBoardMatches(tagAuxMulti, boardAuxMulti, hrConfig, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.False(t, results[0].DirectionMismatch)
	})
}
```

**Step 10: Verify**

Run: `cd backend-go && go build ./... && go test ./internal/domain/tagging/... -run TestEvaluateSemanticBoardMatches_DirectionCheck -v`
Expected: all tests pass

**Step 11: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_matching.go backend-go/internal/domain/tagging/semantic_board_matching_test.go backend-go/internal/domain/tagging/semantic_board_handler.go
git commit -m "feat(board-matching): add direction check to evaluateSemanticBoardMatches"
```

---

## Task 5: MatchTopicTag loads direction data

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_matching.go:70-100` (MatchTopicTag)

**Step 1: Add helper to load tag identity embedding**

Add to `SemanticBoardMatchingService`:

```go
func (s *SemanticBoardMatchingService) loadTagIdentityEmbedding(ctx context.Context, topicTagID uint) ([]float64, error) {
	var emb models.TopicTagEmbedding
	err := s.db.WithContext(ctx).
		Where("topic_tag_id = ? AND embedding_type = ?", topicTagID, "identity").
		First(&emb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // no identity embedding, skip direction check
		}
		return nil, err
	}
	if emb.Embedding == nil {
		return nil, nil
	}
	return parsePgVector(*emb.Embedding)
}
```

**Step 2: Add helper to load board embeddings**

```go
func (s *SemanticBoardMatchingService) loadBoardEmbeddings(ctx context.Context) (map[uint][]float64, error) {
	type boardEmbeddingRow struct {
		ID        uint   `gorm:"column:id"`
		Embedding *string `gorm:"column:embedding"`
	}
	var rows []boardEmbeddingRow
	err := s.db.WithContext(ctx).
		Model(&models.SemanticLabel{}).
		Select("id, embedding").
		Where("label_type = ? AND status = ? AND embedding IS NOT NULL", "board", "active").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[uint][]float64, len(rows))
	for _, row := range rows {
		if row.Embedding == nil {
			continue
		}
		vec, err := parsePgVector(*row.Embedding)
		if err != nil {
			continue
		}
		result[row.ID] = vec
	}
	return result, nil
}
```

**Step 3: Update MatchTopicTag to load and pass direction data**

In `MatchTopicTag`, after loading boardAuxiliaries and before calling `evaluateSemanticBoardMatches`:

```go
// Load direction check data
tagEmbedding, _ := s.loadTagIdentityEmbedding(ctx, topicTagID)
boardEmbeddings, _ := s.loadBoardEmbeddings(ctx)
```

Update the call:
```go
matches := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config, tagEmbedding, boardEmbeddings)
```

**Step 4: Verify**

Run: `cd backend-go && go build ./...`
Expected: compiles without error

**Step 5: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_matching.go
git commit -m "feat(board-matching): load tag identity + board embeddings for direction check"
```

---

## Task 6: Backend API — direction_sim + filtering

**Files:**
- Modify: `backend-go/internal/domain/tagging/semantic_board_handler.go` (multiple locations)

**Step 1: Update matchDetailResponse and matchDetailConfigDTO**

Add `DirectionSim *float64` to `matchDetailResponse`:
```go
type matchDetailResponse struct {
	// ... existing fields ...
	DirectionSim         *float64                `json:"direction_sim"`
	// ... rest unchanged ...
}
```

Add `DirectionSimThreshold float64` to `matchDetailConfigDTO`:
```go
type matchDetailConfigDTO struct {
	// ... existing fields ...
	DirectionSimThreshold float64 `json:"direction_sim_threshold"`
}
```

**Step 2: Update getTagMatchDetail handler**

In `getTagMatchDetail`, after computing `detail`, add direction_sim computation:

```go
// Compute direction_sim
var directionSim *float64
tagEmb, _ := matcher.loadTagIdentityEmbedding(ctx, tagID)
if tagEmb != nil {
	var board models.SemanticLabel
	if err := h.db.WithContext(ctx).Select("id, embedding").Where("id = ? AND label_type = ?", boardID, "board").First(&board).Error; err == nil && board.Embedding != nil {
		boardVec, parseErr := parsePgVector(*board.Embedding)
		if parseErr == nil {
			sim := cosineSimilarity(tagEmb, boardVec)
			directionSim = &sim
		}
	}
}
```

Then include `directionSim` in the response struct.

**Step 3: Update matchDetailConfigToDTO**

Add `DirectionSimThreshold` to the conversion:
```go
func matchDetailConfigToDTO(config SemanticBoardMatchConfig) matchDetailConfigDTO {
	return matchDetailConfigDTO{
		// ... existing fields ...
		DirectionSimThreshold: config.DirectionSimThreshold,
	}
}
```

**Step 4: Add DirectionMismatch to boardArticleTagDTO**

```go
type boardArticleTagDTO struct {
	ID                uint    `json:"id"`
	Label             string  `json:"label"`
	Category          string  `json:"category"`
	MatchReason       string  `json:"match_reason"`
	Score             float64 `json:"score"`
	Downgraded        bool    `json:"downgraded"`
	DirectionMismatch bool    `json:"direction_mismatch"`
}
```

**Step 5: Update filteredTagRow and getBoardArticles query**

Add `DirectionMismatch` to `filteredTagRow`:
```go
type filteredTagRow struct {
	ArticleID         uint    `gorm:"column:article_id"`
	ID                uint    `gorm:"column:id"`
	Label             string  `gorm:"column:label"`
	Category          string  `gorm:"column:category"`
	MatchReason       string  `gorm:"column:match_reason"`
	Score             float64 `gorm:"column:score"`
	Downgraded        bool    `gorm:"column:downgraded"`
	DirectionMismatch bool    `gorm:"column:direction_mismatch"`
}
```

Update the SELECT to include `tbl.direction_mismatch`:
```go
Select("att.article_id, tt.id, tt.label, tt.category, tbl.match_reason, tbl.score, tbl.downgraded, tbl.direction_mismatch").
```

Add filtering: check for `show_direction_mismatch` query param:
```go
showDirectionMismatch := c.Query("show_direction_mismatch") == "true"
if !showDirectionMismatch {
    tagQuery = tagQuery.Where("NOT COALESCE(tbl.direction_mismatch, false)")
}
```

Update the DTO mapping to include `DirectionMismatch`:
```go
tagMap[tr.ArticleID] = append(tagMap[tr.ArticleID], boardArticleTagDTO{
	ID: tr.ID, Label: tr.Label, Category: tr.Category,
	MatchReason: tr.MatchReason, Score: tr.Score, Downgraded: tr.Downgraded,
	DirectionMismatch: tr.DirectionMismatch,
})
```

**Step 6: Update matching config handlers**

In `semanticBoardMatchConfigToMap`, add the new key (already done in Task 4).

In the matching config PUT handler, add parsing for the new key.

**Step 7: Verify**

Run: `cd backend-go && go build ./...`
Expected: compiles without error

**Step 8: Commit**

```bash
git add backend-go/internal/domain/tagging/semantic_board_handler.go
git commit -m "feat(board-api): add direction_sim to match detail, direction_mismatch to articles, filtering"
```

---

## Task 7: Daily report excludes direction_mismatch

**Files:**
- Modify: `backend-go/internal/domain/daily_report/generator.go:596-714` (collectBoardTags)

**Step 1: Exclude direction_mismatch in main query**

Add to the main query in `collectBoardTags`, after the existing WHERE clause for `topic_tag_board_labels.semantic_board_id`:

```go
Where("NOT COALESCE(topic_tag_board_labels.direction_mismatch, false)").
```

**Step 2: Exclude direction_mismatch in fallback**

In the fallback loop (lines ~670-710), after checking `if m.SemanticBoardID == boardID`, add:

```go
if m.DirectionMismatch {
    continue
}
```

Wait — the fallback matches come from `MatchTopicTag` which returns `[]SemanticBoardMatchResult`. Check for `DirectionMismatch`:
```go
for _, m := range matches {
    if m.SemanticBoardID == boardID && !m.DirectionMismatch {
        matched = true
        break
    }
}
```

**Step 3: Verify**

Run: `cd backend-go && go build ./...`
Expected: compiles without error

**Step 4: Commit**

```bash
git add backend-go/internal/domain/daily_report/generator.go
git commit -m "feat(daily-report): exclude direction_mismatch tags from board tag collection"
```

---

## Task 8: Frontend — API types + board editing UI

**Files:**
- Modify: `front/app/api/semanticBoards.ts` (types)
- Modify: `front/app/features/tags/components/TagsPage.vue` (editing dialog + logic)

**Step 1: Update TypeScript types**

In `semanticBoards.ts`:

Add `direction_mismatch` to `BoardArticleTag`:
```typescript
export interface BoardArticleTag {
  id: number
  label: string
  category: string
  match_reason: string
  score: number
  downgraded: boolean
  direction_mismatch: boolean
}
```

Add `direction_sim` to `MatchDetailResponse`:
```typescript
export interface MatchDetailResponse {
  // ... existing fields ...
  direction_sim: number | null
}
```

Add `direction_sim_threshold` to `MatchDetailConfig`:
```typescript
export interface MatchDetailConfig {
  // ... existing fields ...
  direction_sim_threshold: number
}
```

**Step 2: Add board editing dialog to TagsPage**

Add inline editing dialog component or use a section within TagsPage. Add:
- Edit button (pencil icon) next to each board in the sidebar
- Edit dialog/modal with label and description fields
- Save calls `updateBoard` API
- On success, refresh board list

Key elements:
- `editingBoard` ref for the board being edited
- `editLabel` and `editDescription` refs for form fields
- A modal/dialog with input fields and save/cancel buttons

**Step 3: Verify**

Run: `cd front && pnpm lint`
Expected: no lint errors

**Step 4: Commit**

```bash
git add front/app/api/semanticBoards.ts front/app/features/tags/components/TagsPage.vue
git commit -m "feat(frontend): add direction_mismatch type + board editing dialog"
```

---

## Task 9: Frontend — direction_mismatch display control

**Files:**
- Modify: `front/app/features/tags/components/TagsPage.vue`

**Step 1: Add showDirectionMismatch toggle**

Add ref:
```typescript
const showDirectionMismatch = ref(false)
```

**Step 2: Filter tags in computed**

When rendering `filtered_tags` for each article, filter based on the toggle:
```typescript
// In the template or computed that renders filtered_tags per article:
// If !showDirectionMismatch, filter out tags where direction_mismatch === true
```

Pass `show_direction_mismatch` query param to `getBoardArticles` API call.

**Step 3: Style direction_mismatch tags**

Direction mismatch tags get:
- Dashed border (`border-dashed`)
- "⊘" suffix
- Slightly muted opacity

**Step 4: Add toggle UI**

Add a small toggle/checkbox near the articles tab header:
```vue
<label class="flex items-center gap-1.5 text-xs text-gray-400 cursor-pointer">
  <input type="checkbox" v-model="showDirectionMismatch" />
  显示方向不符
</label>
```

**Step 5: Verify**

Run: `cd front && pnpm lint`
Expected: no lint errors

**Step 6: Commit**

```bash
git add front/app/features/tags/components/TagsPage.vue
git commit -m "feat(frontend): hide direction_mismatch tags by default, add toggle"
```

---

## Task 10: Frontend — MatchDetailPanel direction check display

**Files:**
- Modify: `front/app/features/tags/components/MatchDetailPanel.vue`

**Step 1: Update flowSteps computed for max_sim**

When `reason === 'max_sim'`, add direction check info after the max_sim step result. Modify the max_sim step in `flowSteps` computed:

```typescript
// ④ Max Sim
if (reason === 'max_sim') {
  const dirSim = d.direction_sim
  const threshold = c.direction_sim_threshold
  const dirPass = dirSim != null && dirSim >= threshold
  const dirInfo = dirSim != null
    ? (dirPass
        ? ` 方向校验 ✓ sim=${formatScore(dirSim)}≥${formatScore(threshold)}`
        : ` ⚠方向不符 sim=${formatScore(dirSim)}<${formatScore(threshold)}`)
    : ''
  steps.push({
    id: 'max_sim', title: '④ 最高相似度规则',
    desc: '最像的那一对有多像？需同时满足三个条件',
    result: `✓Smax=${formatScore(d.max_similarity)} ✓${d.hits}≥${minHits}命中 ✓R=${formatScore(d.hit_rate)} → 满足！${d.downgraded ? ` ⚠降级匹配（...）` : ''}${dirInfo}`,
    state: 'matched',
  })
  return steps
}
```

For the "failed" max_sim step, also show direction info if the match was max_sim but this is a different step context.

**Step 2: Add direction_sim_threshold to config display**

In the `<details>` config section:
```vue
<div><dt>direction_sim_threshold（方向校验阈值）</dt><dd>{{ formatScore(detail.config.direction_sim_threshold) }}</dd></div>
```

**Step 3: Verify**

Run: `cd front && pnpm lint`
Expected: no lint errors

**Step 4: Commit**

```bash
git add front/app/features/tags/components/MatchDetailPanel.vue
git commit -m "feat(frontend): show direction check result in MatchDetailPanel"
```

---

## Task 11: Full verification

**Step 1: Backend full check**

```bash
cd backend-go && go build ./... && go vet ./... && go test ./internal/domain/tagging/... ./internal/domain/daily_report/... && golangci-lint run ./...
```

**Step 2: Frontend full check**

```bash
# WSL
cd front && pnpm lint
# Windows cmd
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"
cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"
```

**Step 3: End-to-end manual verification**

1. Start backend: `cd backend-go && go run cmd/server/main.go`
2. Call backfill: `curl -X POST http://localhost:5000/api/semantic-boards/backfill-embeddings`
3. Call rematch: `curl -X POST http://localhost:5000/api/semantic-boards/rematch-all`
4. Check matching config includes `direction_sim_threshold`
5. Frontend: verify board editing, direction mismatch toggle, match detail display
