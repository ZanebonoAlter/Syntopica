# Upgrade Suggestion Human Merge Control — Implementation Plan

> Branch: `v1.3.1`
> Related OpenSpec: `openspec/changes/upgrade-suggestion-human-merge-control/`
> Order: Backend (struct → clustering → prompt → handler DTO → tests) → Frontend (types → UI)

---

## Backend Phase 1: Struct Changes

### Step 1.1 — Add `BoardAffinity` struct and update `SemanticBoardUpgradeCluster` ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`

Add new struct before `SemanticBoardUpgradeCluster`:

```go
type BoardAffinity struct {
	BoardID            uint
	BoardLabel         string
	MatchingCandidates int
	AvgDistance         float64
}
```

Update `SemanticBoardUpgradeCluster` — remove 4 old fields, add `BoardAffinities`:

```go
// BEFORE:
type SemanticBoardUpgradeCluster struct {
	Candidates                   []SemanticBoardUpgradeCandidate
	Centroid                     []float64
	ExistingBoardID              *uint
	ExistingBoardLabel           string
	ExistingBoardDescription     string
	ExistingBoardAuxiliaryLabels []string
	Events                       []SemanticBoardUpgradeEventContext
}

// AFTER:
type SemanticBoardUpgradeCluster struct {
	Candidates       []SemanticBoardUpgradeCandidate
	Centroid         []float64
	BoardAffinities  []BoardAffinity
	Events           []SemanticBoardUpgradeEventContext
}
```

**Note**: This breaks compilation of `clusterCandidates()`, `buildSemanticBoardUpgradePrompt()`, `upgradeClustersToDTO()`, and existing tests. Fixed in subsequent steps.

**Verify**: `cd backend-go && go build ./...` — expected: compile errors in above functions.

---

## Backend Phase 2: Clustering Logic Refactor

### Step 2.1 — Write test for new clustering behavior (TDD: RED) ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

Rewrite `TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards`:

```go
func TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	candidateA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	candidateB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	candidateC := createUpgradeLabel(t, db, "Battery", "battery", "auxiliary", "active", 5, []float64{0, 1, 0})
	boardAux := createUpgradeLabel(t, db, "AI", "ai", "auxiliary", "active", 2, []float64{1, 0, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: candidateA.Label, RefCount: 5, Embedding: []float64{1, 0, 0}},
		{ID: candidateB.ID, Label: candidateB.Label, RefCount: 5, Embedding: []float64{0.95, 0.3122498999, 0}},
		{ID: candidateC.ID, Label: candidateC.Label, RefCount: 5, Embedding: []float64{0, 1, 0}},
	}

	clusters, err := service.clusterCandidates(context.Background(), candidates, service.LoadUpgradeConfig(context.Background()))

	require.NoError(t, err)
	require.Len(t, clusters, 2)

	// All clusters are pure auto-clusters — no ExistingBoardID
	// Cluster 0: {A, B} — auto-clustered by cosine distance
	require.Equal(t, []uint{candidateA.ID, candidateB.ID}, upgradeCandidateIDs(clusters[0].Candidates))
	// Cluster 1: {C} — separate
	require.Equal(t, []uint{candidateC.ID}, upgradeCandidateIDs(clusters[1].Candidates))

	// Cluster 0 has board affinity with AI Board
	require.Len(t, clusters[0].BoardAffinities, 1)
	require.Equal(t, board.ID, clusters[0].BoardAffinities[0].BoardID)
	require.Equal(t, "AI Board", clusters[0].BoardAffinities[0].BoardLabel)
	require.Equal(t, 2, clusters[0].BoardAffinities[0].MatchingCandidates)
	// avg_distance: A→boardAux dist=0, B→boardAux dist≈0.05 → avg≈0.025
	require.InDelta(t, 0.025, clusters[0].BoardAffinities[0].AvgDistance, 0.01)

	// Cluster 1 has no board affinity (C is far from boardAux)
	require.Empty(t, clusters[1].BoardAffinities)
}
```

Add new test `TestClusterCandidatesBoardAffinities`:

```go
func TestClusterCandidatesBoardAffinities(t *testing.T) {
	t.Run("no_existing_boards", func(t *testing.T) {
		db := setupSemanticBoardUpgradeTestDB(t)
		candidateA := createUpgradeLabel(t, db, "Solar", "solar", "auxiliary", "active", 5, []float64{1, 0, 0})
		candidateB := createUpgradeLabel(t, db, "Wind", "wind", "auxiliary", "active", 5, []float64{0, 1, 0})
		service := NewSemanticBoardUpgradeService(db, nil, nil)
		candidates := []SemanticBoardUpgradeCandidate{
			{ID: candidateA.ID, Label: candidateA.Label, RefCount: 5, Embedding: []float64{1, 0, 0}},
			{ID: candidateB.ID, Label: candidateB.Label, RefCount: 5, Embedding: []float64{0, 1, 0}},
		}

		clusters, err := service.clusterCandidates(context.Background(), candidates, service.LoadUpgradeConfig(context.Background()))

		require.NoError(t, err)
		for _, c := range clusters {
			require.Empty(t, c.BoardAffinities)
		}
	})

	t.Run("cluster_with_no_matching_candidates", func(t *testing.T) {
		db := setupSemanticBoardUpgradeTestDB(t)
		candidate := createUpgradeLabel(t, db, "Battery", "battery", "auxiliary", "active", 5, []float64{0, 1, 0})
		boardAux := createUpgradeLabel(t, db, "AI", "ai", "auxiliary", "active", 2, []float64{1, 0, 0})
		board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
		require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
		service := NewSemanticBoardUpgradeService(db, nil, nil)
		candidates := []SemanticBoardUpgradeCandidate{
			{ID: candidate.ID, Label: candidate.Label, RefCount: 5, Embedding: []float64{0, 1, 0}},
		}

		clusters, err := service.clusterCandidates(context.Background(), candidates, service.LoadUpgradeConfig(context.Background()))

		require.NoError(t, err)
		require.Len(t, clusters, 1)
		require.Empty(t, clusters[0].BoardAffinities)
	})

	t.Run("multiple_boards_with_partial_matches", func(t *testing.T) {
		db := setupSemanticBoardUpgradeTestDB(t)
		candidateA := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{1, 0, 0})
		candidateB := createUpgradeLabel(t, db, "LLM", "llm", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
		// Board 1: "AI" with auxiliary close to GPT
		boardAux1 := createUpgradeLabel(t, db, "AI Aux", "ai-aux", "auxiliary", "active", 2, []float64{1, 0, 0})
		board1 := createUpgradeLabel(t, db, "AI", "ai", "board", "active", 0, nil)
		require.NoError(t, db.Create(&models.BoardComposition{BoardID: board1.ID, AuxiliaryLabelID: boardAux1.ID}).Error)
		// Board 2: "ML" with auxiliary close to LLM
		boardAux2 := createUpgradeLabel(t, db, "ML Aux", "ml-aux", "auxiliary", "active", 2, []float64{0.9, 0.4358898943, 0})
		board2 := createUpgradeLabel(t, db, "ML", "ml", "board", "active", 0, nil)
		require.NoError(t, db.Create(&models.BoardComposition{BoardID: board2.ID, AuxiliaryLabelID: boardAux2.ID}).Error)

		service := NewSemanticBoardUpgradeService(db, nil, nil)
		candidates := []SemanticBoardUpgradeCandidate{
			{ID: candidateA.ID, Label: candidateA.Label, RefCount: 5, Embedding: []float64{1, 0, 0}},
			{ID: candidateB.ID, Label: candidateB.Label, RefCount: 5, Embedding: []float64{0.95, 0.3122498999, 0}},
		}

		clusters, err := service.clusterCandidates(context.Background(), candidates, service.LoadUpgradeConfig(context.Background()))

		require.NoError(t, err)
		require.Len(t, clusters, 1)
		// Both boards should appear in affinities (both have matching candidates)
		require.Len(t, clusters[0].BoardAffinities, 2)
		// Sorted by avg_distance ascending — AI board should be first (closer)
		require.Equal(t, board1.ID, clusters[0].BoardAffinities[0].BoardID)
		require.Equal(t, board2.ID, clusters[0].BoardAffinities[1].BoardID)
		// Both candidates match both boards
		require.Equal(t, 2, clusters[0].BoardAffinities[0].MatchingCandidates)
		require.Equal(t, 2, clusters[0].BoardAffinities[1].MatchingCandidates)
	})
}
```

**Verify**: `cd backend-go && go test ./internal/domain/tagging/... -v -run TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards` — expected: COMPILE ERROR (struct still has old fields, `clusterCandidates` not yet updated).

### Step 2.2 — Implement new `clusterCandidates()` (TDD: GREEN) ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`

Replace the entire `clusterCandidates` method body:

```go
func (s *SemanticBoardUpgradeService) clusterCandidates(ctx context.Context, candidates []SemanticBoardUpgradeCandidate, config SemanticBoardUpgradeConfig) ([]SemanticBoardUpgradeCluster, error) {
	boardContexts, err := s.loadExistingBoardContexts(ctx)
	if err != nil {
		return nil, err
	}

	// Pure auto-clustering — all candidates go through the same path
	clusters := make([]SemanticBoardUpgradeCluster, 0, len(candidates))
	for _, candidate := range candidates {
		matched := false
		for i := range clusters {
			if candidateFitsCluster(candidate, &clusters[i], config.ClusterDistanceThreshold) {
				addCandidateToCluster(candidate, &clusters[i])
				matched = true
				break
			}
		}
		if !matched {
			clusters = append(clusters, SemanticBoardUpgradeCluster{
				Candidates: []SemanticBoardUpgradeCandidate{candidate},
				Centroid:   candidate.Embedding,
			})
		}
	}

	// Compute board affinities for each cluster
	if len(boardContexts) > 0 {
		boardContextsByBoard := make(map[uint][]semanticBoardContext)
		for _, bc := range boardContexts {
			boardContextsByBoard[bc.BoardID] = append(boardContextsByBoard[bc.BoardID], bc)
		}
		for i := range clusters {
			var affinities []BoardAffinity
			for boardID, contexts := range boardContextsByBoard {
				matchingCount := 0
				totalMinDist := 0.0
				for _, candidate := range clusters[i].Candidates {
					minDist := -1.0
					for _, bc := range contexts {
						dist := semanticBoardUpgradeDistance(candidate.Embedding, bc.Embedding)
						if minDist < 0 || dist < minDist {
							minDist = dist
						}
					}
					if minDist >= 0 && minDist <= config.ClusterDistanceThreshold {
						matchingCount++
						totalMinDist += minDist
					}
				}
				if matchingCount > 0 {
					affinities = append(affinities, BoardAffinity{
						BoardID:            boardID,
						BoardLabel:         contexts[0].BoardLabel,
						MatchingCandidates: matchingCount,
						AvgDistance:         totalMinDist / float64(matchingCount),
					})
				}
			}
			sort.Slice(affinities, func(a, b int) bool {
				return affinities[a].AvgDistance < affinities[b].AvgDistance
			})
			clusters[i].BoardAffinities = affinities
		}
	}

	return clusters, nil
}
```

Remove dead code — delete `closestBoardContext` and `semanticBoardDetailsByID` functions entirely.

**Verify**: `cd backend-go && go test ./internal/domain/tagging/... -v -run TestSemanticBoardUpgradeClustersCandidatesWithExistingBoards` — expected: PASS (but `buildSemanticBoardUpgradePrompt` and other functions still broken).

**Verify**: `cd backend-go && go test ./internal/domain/tagging/... -v -run TestClusterCandidatesBoardAffinities` — expected: PASS.

---

## Backend Phase 3: Prompt Update

### Step 3.1 — Write test for new prompt (TDD: RED) ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

Rewrite `TestSemanticBoardUpgradePromptIncludesExistingBoardContext` → rename to `TestSemanticBoardUpgradePromptIncludesBoardAffinities`:

```go
func TestSemanticBoardUpgradePromptIncludesBoardAffinities(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	createUpgradeLabel(t, db, "Transformer", "transformer", "auxiliary", "active", 5, []float64{0.9, 0.4358898943, 0})
	createUpgradeLabel(t, db, "LLM", "llm", "auxiliary", "active", 5, []float64{0.85, 0.5267826876, 0})
	createUpgradeLabel(t, db, "Deep Learning", "deep-learning", "auxiliary", "active", 5, []float64{0.8, 0.6, 0})
	boardAux := createUpgradeLabel(t, db, "AI", "ai", "auxiliary", "active", 2, []float64{1, 0, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", board.ID).Update("description", "Artificial intelligence board").Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{{Decision: SemanticBoardUpgradeDecisionSkip}}}
	service := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	_, _, err := service.GenerateSuggestions(context.Background())

	require.NoError(t, err)
	// Prompt should NOT contain merge_into_existing
	require.NotContains(t, fakeLLM.prompt, "merge_into_existing")
	require.NotContains(t, fakeLLM.prompt, "target_board_id")
	require.NotContains(t, fakeLLM.prompt, "关联已有板块")
	// Prompt should contain board affinity reference info
	require.Contains(t, fakeLLM.prompt, "相似已有板块")
	require.Contains(t, fakeLLM.prompt, "AI Board")
}
```

**Verify**: `cd backend-go && go test ./internal/domain/tagging/... -v -run TestSemanticBoardUpgradePromptIncludesBoardAffinities` — expected: COMPILE ERROR (still broken from struct change).

### Step 3.2 — Implement new `buildSemanticBoardUpgradePrompt()` (TDD: GREEN) ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`

Replace `buildSemanticBoardUpgradePrompt`:

```go
func buildSemanticBoardUpgradePrompt(clusters []SemanticBoardUpgradeCluster) string {
	var builder strings.Builder
	builder.WriteString("你是一个语义板块分析助手。根据以下辅助标签聚类信息，判断每个簇应该：create_new（创建新板块）或 skip（跳过不处理）。\n\n")
	builder.WriteString("判断原则：\n")
	builder.WriteString("- 如果簇内标签语义集中、有明确主题且不存在对应板块 → create_new\n")
	builder.WriteString("- 如果簇内标签过于分散或过于泛化，不足以形成独立板块 → skip\n\n")
	builder.WriteString("返回 JSON 格式：{\"suggestions\": [{\"decision\": \"create_new|skip\", \"board_label\": \"板块名称\", \"description\": \"板块描述\", \"auxiliary_label_ids\": [id1, id2], \"reason\": \"判断理由\"}]}\n\n")
	for i, cluster := range clusters {
		fmt.Fprintf(&builder, "【簇 %d】\n", i+1)
		builder.WriteString("候选辅助标签：\n")
		for _, candidate := range cluster.Candidates {
			fmt.Fprintf(&builder, "  - ID=%d: %s（引用次数=%d）\n", candidate.ID, candidate.Label, candidate.RefCount)
		}
		if len(cluster.BoardAffinities) > 0 {
			builder.WriteString("相似已有板块参考：\n")
			for _, aff := range cluster.BoardAffinities {
				fmt.Fprintf(&builder, "  - %s（ID=%d）：%d 个候选匹配，平均距离 %.4f\n", aff.BoardLabel, aff.BoardID, aff.MatchingCandidates, aff.AvgDistance)
			}
		}
		if len(cluster.Events) > 0 {
			builder.WriteString("关联事件（近期共现）：\n")
			for _, event := range cluster.Events {
				fmt.Fprintf(&builder, "  - %s（共现次数=%d）\n", event.Label, event.Frequency)
			}
		}
		builder.WriteString("\n")
	}
	return builder.String()
}
```

Update `filterSemanticBoardUpgradeSuggestions` — filter out merge_into_existing:

```go
func filterSemanticBoardUpgradeSuggestions(suggestions []SemanticBoardUpgradeSuggestion, validAuxiliaryIDs map[uint]struct{}) []SemanticBoardUpgradeSuggestion {
	filtered := make([]SemanticBoardUpgradeSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		// Only accept create_new and skip; defensively reject merge_into_existing
		if suggestion.Decision != SemanticBoardUpgradeDecisionCreateNew && suggestion.Decision != SemanticBoardUpgradeDecisionSkip {
			continue
		}
		suggestion.AuxiliaryLabelIDs = filterKnownAuxiliaryIDs(uniqueUintSlice(suggestion.AuxiliaryLabelIDs), validAuxiliaryIDs)
		if suggestion.Decision != SemanticBoardUpgradeDecisionSkip && len(suggestion.AuxiliaryLabelIDs) == 0 {
			continue
		}
		filtered = append(filtered, suggestion)
	}
	return filtered
}
```

Update system message in `airouterSemanticBoardUpgradeLLM.SuggestSemanticBoardUpgrades`:

```go
// BEFORE:
{Role: "system", Content: "Return JSON only in this shape: {\"suggestions\":[{\"decision\":\"create_new|merge_into_existing|skip\",\"board_label\":\"\",\"description\":\"\",\"auxiliary_label_ids\":[1],\"target_board_id\":1,\"reason\":\"\"}]}"},

// AFTER:
{Role: "system", Content: "Return JSON only in this shape: {\"suggestions\":[{\"decision\":\"create_new|skip\",\"board_label\":\"\",\"description\":\"\",\"auxiliary_label_ids\":[1],\"reason\":\"\"}]}"},
```

Remove `TargetBoardID` from the parsing struct in `SuggestSemanticBoardUpgrades`:

```go
// BEFORE:
var parsed struct {
    Suggestions []struct {
        Decision          SemanticBoardUpgradeDecision `json:"decision"`
        BoardLabel        string                       `json:"board_label"`
        Description       string                       `json:"description"`
        AuxiliaryLabelIDs []uint                       `json:"auxiliary_label_ids"`
        TargetBoardID     *uint                        `json:"target_board_id"`
        Reason            string                       `json:"reason"`
    } `json:"suggestions"`
}

// AFTER:
var parsed struct {
    Suggestions []struct {
        Decision          SemanticBoardUpgradeDecision `json:"decision"`
        BoardLabel        string                       `json:"board_label"`
        Description       string                       `json:"description"`
        AuxiliaryLabelIDs []uint                       `json:"auxiliary_label_ids"`
        Reason            string                       `json:"reason"`
    } `json:"suggestions"`
}
```

And update the suggestion construction to remove `TargetBoardID`:

```go
// BEFORE:
suggestions = append(suggestions, SemanticBoardUpgradeSuggestion{Decision: raw.Decision, BoardLabel: raw.BoardLabel, Description: raw.Description, AuxiliaryLabelIDs: raw.AuxiliaryLabelIDs, TargetBoardID: raw.TargetBoardID, Reason: raw.Reason})

// AFTER:
suggestions = append(suggestions, SemanticBoardUpgradeSuggestion{Decision: raw.Decision, BoardLabel: raw.BoardLabel, Description: raw.Description, AuxiliaryLabelIDs: raw.AuxiliaryLabelIDs, Reason: raw.Reason})
```

**Verify**: `cd backend-go && go test ./internal/domain/tagging/... -v -run TestSemanticBoardUpgradePromptIncludesBoardAffinities` — expected: PASS.

---

## Backend Phase 4: `GenerateSuggestions` Return Type

### Step 4.1 — Update `GenerateSuggestions` to return clusters ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_upgrade.go`

Change signature and return:

```go
// BEFORE:
func (s *SemanticBoardUpgradeService) GenerateSuggestions(ctx context.Context) ([]SemanticBoardUpgradeSuggestion, error) {
    ...
    return filterSemanticBoardUpgradeSuggestions(suggestions, validAuxiliaryIDs), nil
}

// AFTER:
func (s *SemanticBoardUpgradeService) GenerateSuggestions(ctx context.Context) ([]SemanticBoardUpgradeSuggestion, []SemanticBoardUpgradeCluster, error) {
    ...
    return filterSemanticBoardUpgradeSuggestions(suggestions, validAuxiliaryIDs), clusters, nil
}
```

Update all return paths in the method:

```go
if s.llm == nil {
    return nil, nil, fmt.Errorf("semantic board upgrade llm is required")
}
...
if len(candidates) < config.RefCountThreshold {
    return []SemanticBoardUpgradeSuggestion{}, []SemanticBoardUpgradeCluster{}, nil
}
...
return nil, nil, err  // for each error case
...
// Final return:
return filterSemanticBoardUpgradeSuggestions(suggestions, validAuxiliaryIDs), clusters, nil
```

**Verify**: `cd backend-go && go build ./...` — expected: compile error in handler and tests (caller signature changed).

---

## Backend Phase 5: Handler DTO

### Step 5.1 — Update DTO structs ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_handler.go`

Add new DTO type (near other DTO structs):

```go
type boardAffinityDTO struct {
	BoardID            uint    `json:"board_id"`
	BoardLabel         string  `json:"board_label"`
	MatchingCandidates int     `json:"matching_candidates"`
	AvgDistance         float64 `json:"avg_distance"`
}
```

Update `semanticBoardUpgradeClusterDTO`:

```go
// BEFORE:
type semanticBoardUpgradeClusterDTO struct {
	Candidates                   []semanticBoardUpgradeCandidateDTO `json:"candidates"`
	ExistingBoardID              *uint                              `json:"existing_board_id,omitempty"`
	ExistingBoardLabel           string                             `json:"existing_board_label"`
	ExistingBoardDescription     string                             `json:"existing_board_description"`
	ExistingBoardAuxiliaryLabels []string                           `json:"existing_board_auxiliary_labels"`
}

// AFTER:
type semanticBoardUpgradeClusterDTO struct {
	Candidates      []semanticBoardUpgradeCandidateDTO `json:"candidates"`
	BoardAffinities []boardAffinityDTO                 `json:"board_affinities"`
}
```

Update `semanticBoardUpgradeSuggestionDTO` — add `BoardAffinities`:

```go
type semanticBoardUpgradeSuggestionDTO struct {
	Decision          SemanticBoardUpgradeDecision `json:"decision"`
	BoardLabel        string                       `json:"board_label"`
	Description       string                       `json:"description"`
	AuxiliaryLabelIDs []uint                       `json:"auxiliary_label_ids"`
	AuxiliaryLabels   []struct {
		ID    uint   `json:"id"`
		Label string `json:"label"`
	} `json:"auxiliary_labels"`
	TargetBoardID    *uint              `json:"target_board_id,omitempty"`
	TargetBoardLabel string             `json:"target_board_label,omitempty"`
	Reason           string             `json:"reason"`
	BoardAffinities  []boardAffinityDTO `json:"board_affinities"`
}
```

### Step 5.2 — Update `upgradeClustersToDTO()` ✅

```go
// BEFORE:
func upgradeClustersToDTO(clusters []SemanticBoardUpgradeCluster) []semanticBoardUpgradeClusterDTO {
	items := make([]semanticBoardUpgradeClusterDTO, 0, len(clusters))
	for _, cluster := range clusters {
		items = append(items, semanticBoardUpgradeClusterDTO{Candidates: upgradeCandidatesToDTO(cluster.Candidates), ExistingBoardID: cluster.ExistingBoardID, ExistingBoardLabel: cluster.ExistingBoardLabel, ExistingBoardDescription: cluster.ExistingBoardDescription, ExistingBoardAuxiliaryLabels: cluster.ExistingBoardAuxiliaryLabels})
	}
	return items
}

// AFTER:
func upgradeClustersToDTO(clusters []SemanticBoardUpgradeCluster) []semanticBoardUpgradeClusterDTO {
	items := make([]semanticBoardUpgradeClusterDTO, 0, len(clusters))
	for _, cluster := range clusters {
		affDTOs := make([]boardAffinityDTO, 0, len(cluster.BoardAffinities))
		for _, aff := range cluster.BoardAffinities {
			affDTOs = append(affDTOs, boardAffinityDTO{
				BoardID:            aff.BoardID,
				BoardLabel:         aff.BoardLabel,
				MatchingCandidates: aff.MatchingCandidates,
				AvgDistance:         aff.AvgDistance,
			})
		}
		items = append(items, semanticBoardUpgradeClusterDTO{
			Candidates:      upgradeCandidatesToDTO(cluster.Candidates),
			BoardAffinities: affDTOs,
		})
	}
	return items
}
```

### Step 5.3 — Update `suggestionsToDTO()` signature and logic ✅

Change signature to accept clusters, and embed board_affinities into suggestion DTOs:

```go
// BEFORE:
func (h *semanticBoardHandler) suggestionsToDTO(ctx context.Context, suggestions []SemanticBoardUpgradeSuggestion) []semanticBoardUpgradeSuggestionDTO {

// AFTER:
func (h *semanticBoardHandler) suggestionsToDTO(ctx context.Context, suggestions []SemanticBoardUpgradeSuggestion, clusters []SemanticBoardUpgradeCluster) []semanticBoardUpgradeSuggestionDTO {
```

Add cluster lookup map after the existing `boardNames` batch lookup, before the `items` loop:

```go
	// Build candidate ID → cluster index map for board_affinities lookup
	candidateToCluster := make(map[uint]int)
	for i, cluster := range clusters {
		for _, c := range cluster.Candidates {
			candidateToCluster[c.ID] = i
		}
	}
```

Inside the `for _, s := range suggestions` loop, after setting `dto.TargetBoardLabel`, add:

```go
		// Embed board_affinities from matching cluster
		if len(s.AuxiliaryLabelIDs) > 0 {
			if clusterIdx, ok := candidateToCluster[s.AuxiliaryLabelIDs[0]]; ok {
				bas := clusters[clusterIdx].BoardAffinities
				dto.BoardAffinities = make([]boardAffinityDTO, 0, len(bas))
				for _, ba := range bas {
					dto.BoardAffinities = append(dto.BoardAffinities, boardAffinityDTO{
						BoardID:            ba.BoardID,
						BoardLabel:         ba.BoardLabel,
						MatchingCandidates: ba.MatchingCandidates,
						AvgDistance:         ba.AvgDistance,
					})
				}
			}
		}
```

### Step 5.4 — Update `suggestUpgrades` handler ✅

```go
// BEFORE:
func (h *semanticBoardHandler) suggestUpgrades(c *gin.Context) {
	service := NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory(), nil)
	suggestions, err := service.GenerateSuggestions(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"suggestions": h.suggestionsToDTO(c.Request.Context(), suggestions)})
}

// AFTER:
func (h *semanticBoardHandler) suggestUpgrades(c *gin.Context) {
	service := NewSemanticBoardUpgradeService(h.db, semanticBoardUpgradeLLMFactory(), nil)
	suggestions, clusters, err := service.GenerateSuggestions(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	respondOK(c, gin.H{"suggestions": h.suggestionsToDTO(c.Request.Context(), suggestions, clusters)})
}
```

**Verify**: `cd backend-go && go build ./...` — expected: BUILD SUCCESS.

---

## Backend Phase 6: Test Updates

### Step 6.1 — Update tests for new `GenerateSuggestions` return signature ✅

**File**: `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go`

Update `TestSemanticBoardUpgradeGenerateSuggestionsUsesLLMMock`:

```go
// BEFORE:
suggestions, err := service.GenerateSuggestions(context.Background())

// AFTER:
suggestions, _, err := service.GenerateSuggestions(context.Background())
```

Update `TestSemanticBoardUpgradeGenerateSuggestionsSkipsWhenCandidateCountBelowThreshold`:

```go
// BEFORE:
suggestions, err := service.GenerateSuggestions(context.Background())

// AFTER:
suggestions, _, err := service.GenerateSuggestions(context.Background())
```

**Verify**: `cd backend-go && go test ./internal/domain/tagging/... -v -run TestSemanticBoardUpgradeGenerateSuggestions` — expected: PASS (both tests).

### Step 6.2 — Run all affected tests ✅

```bash
cd backend-go && go test ./internal/domain/tagging/... -v
```

Expected: ALL PASS.

### Step 6.3 — Verify `ConfirmSuggestion` merge path still works ✅

```bash
cd backend-go && go test ./internal/domain/tagging/... -v -run TestSemanticBoardUpgradeConfirmMergeIntoExisting
```

Expected: PASS (no changes to `ConfirmSuggestion`, `ConfirmSemanticBoardUpgradeRequest`, or `confirmSemanticBoardUpgradeHTTPRequest`).

### Step 6.4 — Build verification ✅

```bash
cd backend-go && go build ./...
```

Expected: BUILD SUCCESS.

### Step 6.5 — Lint check ✅

```bash
cd backend-go && golangci-lint run ./internal/domain/tagging/...
```

Expected: NO ISSUES.

---

## Frontend Phase 1: TypeScript Types

### Step 7.1 — Update `UpgradeCluster` and `UpgradeSuggestion` interfaces

**File**: `front/app/api/semanticBoards.ts`

Add `BoardAffinity` interface and update existing interfaces:

```typescript
// Add new interface:
export interface BoardAffinity {
  board_id: number
  board_label: string
  matching_candidates: number
  avg_distance: number
}

// Replace UpgradeCluster:
export interface UpgradeCluster {
  candidates: UpgradeCandidate[]
  board_affinities: BoardAffinity[]
}

// Update UpgradeSuggestion — add board_affinities field:
export interface UpgradeSuggestion {
  decision: 'create_new' | 'merge_into_existing' | 'skip'
  board_label?: string
  description?: string
  target_board_id?: number
  auxiliary_label_ids: number[]
  auxiliary_labels: { id: number; label: string }[]
  target_board_label?: string
  reason: string
  board_affinities: BoardAffinity[]
}
```

**Verify**: `cd front && pnpm lint` — expected: PASS.

**Verify**: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"` — expected: TYPE ERRORS in `UpgradeSuggestionPanel.vue` (still references old fields). Fixed in next step.

---

## Frontend Phase 2: UI Update

### Step 8.1 — Update `UpgradeSuggestionPanel.vue` — remove old field references

**File**: `front/app/features/tags/components/UpgradeSuggestionPanel.vue`

In `<script setup>`, add `BoardAffinity` import:

```typescript
import type { UpgradeCandidate, UpgradeCluster, UpgradeSuggestion, BoardAffinity } from '~/api/semanticBoards'
```

No changes needed to props/emit definitions.

### Step 8.2 — Add affinity display section

In each suggestion card (inside the `v-for="(s, i) in suggestions"` div), after the tags `<div class="usp-item-tags">` and before the actions div, add:

```html
            <div v-if="s.board_affinities && s.board_affinities.length > 0" class="usp-item-affinities">
              <span class="usp-item-affinities-label">相似板块：</span>
              <span
                v-for="(aff, ai) in s.board_affinities"
                :key="ai"
                class="usp-item-affinity"
              >
                {{ aff.board_label }}
                <span class="usp-item-affinity-detail">
                  ({{ aff.matching_candidates }} candidates, avg distance {{ aff.avg_distance.toFixed(4) }})
                </span>
              </span>
            </div>
```

### Step 8.3 — Add merge dropdown for create_new suggestions

Add reactive state for tracking which suggestion has an open dropdown:

```typescript
const openMergeIndex = ref<number | null>(null)

function toggleMerge(index: number) {
  openMergeIndex.value = openMergeIndex.value === index ? null : index
}

function handleMerge(s: UpgradeSuggestion, boardId: number) {
  emit('execute', {
    ...s,
    decision: 'merge_into_existing' as const,
    target_board_id: boardId,
  })
}
```

In the actions div (`<div v-if="s.decision !== 'skip'" class="usp-item-actions">`), replace with:

```html
            <div v-if="s.decision !== 'skip'" class="usp-item-actions">
              <button
                type="button"
                class="usp-item-btn usp-item-btn--primary"
                @click="emit('execute', s)"
              >
                <Icon icon="mdi:check" width="12" />
                确认执行
              </button>
              <template v-if="s.board_affinities && s.board_affinities.length > 0">
                <div class="usp-merge-wrapper">
                  <button
                    type="button"
                    class="usp-item-btn usp-item-btn--merge"
                    @click="toggleMerge(i)"
                  >
                    <Icon icon="mdi:merge" width="12" />
                    合并到...
                  </button>
                  <div v-if="openMergeIndex === i" class="usp-merge-dropdown">
                    <button
                      v-for="aff in s.board_affinities"
                      :key="aff.board_id"
                      type="button"
                      class="usp-merge-option"
                      @click="handleMerge(s, aff.board_id)"
                    >
                      {{ aff.board_label }}
                      <span class="usp-merge-option-detail">({{ aff.matching_candidates }} matches)</span>
                    </button>
                  </div>
                </div>
              </template>
            </div>
```

### Step 8.4 — Add CSS for new elements

Add to `<style scoped>`:

```css
.usp-item-affinities {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.68rem;
  color: rgba(255, 255, 255, 0.4);
}

.usp-item-affinities-label {
  color: rgba(255, 255, 255, 0.35);
}

.usp-item-affinity {
  padding: 0.1rem 0.3rem;
  border-radius: 4px;
  background: rgba(96, 165, 250, 0.08);
  color: rgba(147, 197, 253, 0.8);
}

.usp-item-affinity-detail {
  color: rgba(147, 197, 253, 0.5);
}

.usp-merge-wrapper {
  position: relative;
}

.usp-item-btn--merge {
  border-color: rgba(96, 165, 250, 0.3);
  background: rgba(96, 165, 250, 0.08);
  color: rgba(147, 197, 253, 0.9);
}

.usp-item-btn--merge:hover {
  background: rgba(96, 165, 250, 0.16);
}

.usp-merge-dropdown {
  position: absolute;
  right: 0;
  bottom: 100%;
  margin-bottom: 4px;
  min-width: 200px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(25, 35, 50, 0.98);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  z-index: 10;
  overflow: hidden;
}

.usp-merge-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 0.45rem 0.65rem;
  border: none;
  background: none;
  color: rgba(255, 255, 255, 0.8);
  font-size: 0.72rem;
  cursor: pointer;
  transition: background 0.1s ease;
}

.usp-merge-option:hover {
  background: rgba(96, 165, 250, 0.12);
}

.usp-merge-option-detail {
  color: rgba(255, 255, 255, 0.4);
  font-size: 0.65rem;
}
```

### Step 8.5 — Close dropdown on outside click

Wrap the overlay div click handler to also close dropdown:

```html
<div v-if="visible" class="usp-overlay" @click.self="emit('cancel'); openMergeIndex = null">
```

**Verify**: `cd front && pnpm lint` — expected: PASS.

**Verify**: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"` — expected: PASS.

**Verify**: `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"` — expected: PASS.

---

## Summary of Files Changed

| File | Change |
|------|--------|
| `backend-go/internal/domain/tagging/semantic_board_upgrade.go` | BoardAffinity struct, cluster refactor, prompt update, filter update, GenerateSuggestions return type |
| `backend-go/internal/domain/tagging/semantic_board_handler.go` | DTO structs, DTO functions, suggestUpgrades handler |
| `backend-go/internal/domain/tagging/semantic_board_upgrade_test.go` | Rewrite/update 5 tests |
| `front/app/api/semanticBoards.ts` | BoardAffinity type, UpgradeCluster/UpgradeSuggestion interfaces |
| `front/app/features/tags/components/UpgradeSuggestionPanel.vue` | Affinity display, merge dropdown |

## Deleted Code

| Function/Variable | File | Reason |
|---|---|---|
| `closestBoardContext()` | `semantic_board_upgrade.go` | Phase A removal — no callers |
| `semanticBoardDetailsByID()` | `semantic_board_upgrade.go` | Phase A removal — no callers |
| `ExistingBoardID` + 3 fields | `SemanticBoardUpgradeCluster` struct | Replaced by `BoardAffinities` |
| `ExistingBoardID` + 3 fields | `semanticBoardUpgradeClusterDTO` | Replaced by `BoardAffinities` |

## Verification Checklist

- [ ] `cd backend-go && go build ./...` — builds cleanly
- [ ] `cd backend-go && golangci-lint run ./internal/domain/tagging/...` — no issues
- [ ] `cd backend-go && go test ./internal/domain/tagging/... -v` — all tests pass
- [ ] `cd front && pnpm lint` — no issues
- [ ] `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck"` — pass
- [ ] `cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm build"` — pass
- [ ] `TestSemanticBoardUpgradeConfirmMergeIntoExisting` still passes (merge path unchanged)
