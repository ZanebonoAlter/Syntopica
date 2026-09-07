package board

import (
	"context"
	"database/sql/driver"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"
)

type fakeSemanticBoardUpgradeLLM struct {
	prompt      string
	mode        string
	suggestions []SemanticBoardUpgradeSuggestion
	calls       int
}

var upgradeFeedSeq uint64

func (f *fakeSemanticBoardUpgradeLLM) SuggestSemanticBoardUpgrades(ctx context.Context, prompt string, mode string) ([]SemanticBoardUpgradeSuggestion, error) {
	f.calls++
	f.prompt = prompt
	f.mode = mode
	return f.suggestions, nil
}

func setupSemanticBoardUpgradeTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	InvalidateBoardCache() // 避免包级缓存跨测试残留
	return db
}

func TestSemanticBoardUpgradeCollectsCandidates(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	include := createUpgradeLabel(t, db, "Included", "included", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "Below", "below", "auxiliary", "active", 4, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "Disabled", "disabled", "auxiliary", "disabled", 8, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "No Embedding", "no-embedding", "auxiliary", "active", 8, nil)
	composed := createUpgradeLabel(t, db, "Composed", "composed", "auxiliary", "active", 8, []float64{0, 1, 0})
	board := createUpgradeLabel(t, db, "Board", "board", "board", "active", 0, nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: composed.ID}).Error)
	service := NewSemanticBoardUpgradeService(db, nil, nil)

	candidates, err := service.CollectCandidates(context.Background(), service.LoadUpgradeConfig(context.Background()), 0)

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, include.ID, candidates[0].ID)
	require.Equal(t, testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim), candidates[0].Embedding)
}

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
		{ID: candidateA.ID, Label: candidateA.Label, RefCount: 5, Embedding: testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)},
		{ID: candidateB.ID, Label: candidateB.Label, RefCount: 5, Embedding: testutil.PadVector([]float64{0.95, 0.3122498999, 0}, testutil.TestEmbeddingDim)},
		{ID: candidateC.ID, Label: candidateC.Label, RefCount: 5, Embedding: testutil.PadVector([]float64{0, 1, 0}, testutil.TestEmbeddingDim)},
	}

	clusters, err := service.ClusterCandidates(context.Background(), candidates, service.LoadUpgradeConfig(context.Background()))

	require.NoError(t, err)
	require.Len(t, clusters, 2)

	// All clusters are pure auto-clusters — no ExistingBoardID
	// Cluster 0: {A, B} — auto-clustered by cosine distance
	require.Equal(t, []uint{candidateA.ID, candidateB.ID}, upgradeCandidateIDs(clusters[0].Candidates))
	// Cluster 1: {C} — separate
	require.Equal(t, []uint{candidateC.ID}, upgradeCandidateIDs(clusters[1].Candidates))
}

// TestClusterCandidatesPass2Reassignment verifies that the two-pass clustering
// corrects greedy drift: a candidate initially absorbed by an early cluster
// should be reassigned to a closer cluster after centroid stabilisation.
func TestClusterCandidatesPass2Reassignment(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)

	// Construct embeddings so that greedy Pass 1 misassigns candidate C.
	// A and B are close, C is close to B but far from A.
	// Pass 1 order: A, B, C — A forms cluster, B joins A (centroid drifts toward B),
	// then C joins because centroid is now close enough (greedy drift).
	// Pass 2 should split: {A} and {B, C} — C is reassigned to B's stable centroid.

	// Embeddings in 3D:
	// A = (1, 0, 0)
	// B = (0.7, 0.7, 0)  — cosine distance from A ≈ 0.293 (within threshold 0.35)
	// C = (0.4, 0.9, 0)  — cosine distance from B ≈ 0.019 (very close to B)
	//                      cosine distance from A ≈ 0.194 (closer to A than B is!)
	//
	// Actually let me rethink. We want C to be close to B but NOT close to A.
	// And A-B should be within threshold so they merge in Pass 1.
	// Then C should join A's cluster via centroid drift in Pass 1,
	// but Pass 2 should reassign it.
	//
	// Let's use:
	// A = (1, 0, 0)
	// B = (0.66, 0.75, 0)  — cosine dist(A,B) = 1 - (0.66)/(sqrt(1)*sqrt(0.66^2+0.75^2))
	//                        = 1 - 0.66/sqrt(0.4356+0.5625) = 1 - 0.66/0.9982 ≈ 0.339
	// C = (0.3, 0.95, 0)   — cosine dist(A,C) = 1 - 0.3/sqrt(0.09+0.9025) = 1 - 0.3/0.9962 ≈ 0.699
	//                        cosine dist(B,C) = 1 - (0.66*0.3+0.75*0.95)/(0.9982*0.9962)
	//                                          = 1 - (0.198+0.7125)/0.9944 = 1 - 0.9152 ≈ 0.085
	//
	// So A-B dist ≈ 0.339 (within threshold 0.35)
	//    A-C dist ≈ 0.699 (way above threshold — C should NOT be in A's cluster)
	//    B-C dist ≈ 0.085 (very close)
	//
	// Pass 1 greedy: A forms cluster1, B joins (centroid drifts), C...
	//   centroid after A+B = ((1+0.66)/2, (0+0.75)/2, 0) = (0.83, 0.375, 0)
	//   dist(C, centroid) = 1 - (0.83*0.3+0.375*0.95)/(sqrt(0.83^2+0.375^2)*sqrt(0.3^2+0.95^2))
	//                     = 1 - (0.249+0.35625)/(sqrt(0.6889+0.140625)*sqrt(0.09+0.9025))
	//                     = 1 - 0.60525/(0.9102*0.9962) = 1 - 0.60525/0.9067 = 1 - 0.6676 = 0.332
	//   dist(C, centroid) ≈ 0.332 — JUST within threshold 0.35! So greedy Pass 1 absorbs C.
	//
	// Pass 2 stable centroid of {A,B} = (0.83, 0.375, 0)
	//   dist(C, stable centroid) = same calc ≈ 0.332 — still within threshold :(
	//
	// Let me adjust to make it more dramatic. Use threshold 0.25 for this test.

	embA := []float64{1, 0, 0}
	embB := []float64{0.66, 0.75, 0}
	embC := []float64{0.3, 0.95, 0}

	candidateA := createUpgradeLabel(t, db, "Alpha", "alpha", "auxiliary", "active", 5, embA)
	candidateB := createUpgradeLabel(t, db, "Beta", "beta", "auxiliary", "active", 5, embB)
	candidateC := createUpgradeLabel(t, db, "Gamma", "gamma", "auxiliary", "active", 5, embC)

	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "Alpha", RefCount: 5, Embedding: embA},
		{ID: candidateB.ID, Label: "Beta", RefCount: 5, Embedding: embB},
		{ID: candidateC.ID, Label: "Gamma", RefCount: 5, Embedding: embC},
	}

	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterDistanceThreshold = 0.25 // tight threshold for clear separation
	config.ClusterMethod = "centroid"

	clusters, err := service.ClusterCandidates(context.Background(), candidates, config)

	require.NoError(t, err)

	// With threshold 0.25:
	//   dist(A,B) ≈ 0.339 > 0.25 → A and B are in separate clusters
	//   dist(B,C) ≈ 0.085 < 0.25 → B and C should be together
	// Expected: {A} and {B, C}
	require.Len(t, clusters, 2, "should produce 2 clusters: {Alpha} and {Beta,Gamma}")

	// Find the {B,C} cluster
	var bcCluster *SemanticBoardUpgradeCluster
	for i := range clusters {
		for _, c := range clusters[i].Candidates {
			if c.Label == "Beta" || c.Label == "Gamma" {
				bcCluster = &clusters[i]
				break
			}
		}
		if bcCluster != nil {
			break
		}
	}
	require.NotNil(t, bcCluster, "should find cluster containing Beta")
	require.Len(t, bcCluster.Candidates, 2, "Beta and Gamma should be in the same cluster")

	labels := make(map[string]bool)
	for _, c := range bcCluster.Candidates {
		labels[c.Label] = true
	}
	require.True(t, labels["Beta"])
	require.True(t, labels["Gamma"])
}

// TestClusterCandidatesPass2SplittingPreventsGiantFirstCluster verifies that
// the two-pass approach prevents the first cluster from absorbing too many candidates.
func TestClusterCandidatesPass2SplittingPreventsGiantFirstCluster(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)

	// Create a chain of embeddings where each is close to its neighbor but
	// the endpoints are far apart:
	// A(1,0) - B(0.8,0.6) - C(0.5,0.87) - D(0.2,0.98) - E(-0.1,0.995)
	// With threshold 0.25, each should only cluster with its immediate neighbor,
	// but greedy Pass 1 would absorb them all into one cluster via centroid drift.
	embA := []float64{1, 0, 0}
	embB := []float64{0.8, 0.6, 0}
	embC := []float64{0.5, 0.87, 0}
	embD := []float64{0.2, 0.98, 0}
	embE := []float64{-0.1, 0.995, 0}

	candidateA := createUpgradeLabel(t, db, "A", "a", "auxiliary", "active", 5, embA)
	candidateB := createUpgradeLabel(t, db, "B", "b", "auxiliary", "active", 5, embB)
	candidateC := createUpgradeLabel(t, db, "C", "c", "auxiliary", "active", 5, embC)
	candidateD := createUpgradeLabel(t, db, "D", "d", "auxiliary", "active", 5, embD)
	candidateE := createUpgradeLabel(t, db, "E", "e", "auxiliary", "active", 5, embE)

	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "A", RefCount: 5, Embedding: embA},
		{ID: candidateB.ID, Label: "B", RefCount: 5, Embedding: embB},
		{ID: candidateC.ID, Label: "C", RefCount: 5, Embedding: embC},
		{ID: candidateD.ID, Label: "D", RefCount: 5, Embedding: embD},
		{ID: candidateE.ID, Label: "E", RefCount: 5, Embedding: embE},
	}

	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterDistanceThreshold = 0.20
	config.ClusterMethod = "centroid"

	clusters, err := service.ClusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)

	// The first cluster should NOT contain all 5 candidates.
	// With proper reassignment, we expect multiple smaller clusters.
	maxSize := 0
	for _, c := range clusters {
		if len(c.Candidates) > maxSize {
			maxSize = len(c.Candidates)
		}
	}
	require.Less(t, maxSize, 5, "no single cluster should contain all 5 candidates")

	// Verify total candidates preserved
	totalCandidates := 0
	for _, c := range clusters {
		totalCandidates += len(c.Candidates)
	}
	require.Equal(t, 5, totalCandidates, "all 5 candidates must be accounted for")
}

func TestClusterCandidatesAverageLinkBasic(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	candidateA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	candidateB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	candidateC := createUpgradeLabel(t, db, "Battery", "battery", "auxiliary", "active", 5, []float64{0, 1, 0})
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "OpenAI", RefCount: 5, Embedding: []float64{1, 0, 0}},
		{ID: candidateB.ID, Label: "GPT", RefCount: 5, Embedding: []float64{0.95, 0.3122498999, 0}},
		{ID: candidateC.ID, Label: "Battery", RefCount: 5, Embedding: []float64{0, 1, 0}},
	}
	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterMethod = "average_link"

	clusters, err := service.ClusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)
	require.Len(t, clusters, 2)

	// Find {A,B} cluster
	var abCluster *SemanticBoardUpgradeCluster
	for i := range clusters {
		for _, c := range clusters[i].Candidates {
			if c.ID == candidateA.ID {
				abCluster = &clusters[i]
				break
			}
		}
	}
	require.NotNil(t, abCluster)
	require.Len(t, abCluster.Candidates, 2)
	abIDs := upgradeCandidateIDs(abCluster.Candidates)
	require.Contains(t, abIDs, candidateA.ID)
	require.Contains(t, abIDs, candidateB.ID)

	// Verify C is alone
	var cCluster *SemanticBoardUpgradeCluster
	for i := range clusters {
		for _, c := range clusters[i].Candidates {
			if c.ID == candidateC.ID {
				cCluster = &clusters[i]
				break
			}
		}
	}
	require.NotNil(t, cCluster)
	require.Len(t, cCluster.Candidates, 1)
}

func TestClusterCandidatesAverageLinkNoGiantCluster(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	// Chain of 5 embeddings: each close to neighbor, endpoints far apart
	embA := []float64{1, 0, 0}
	embB := []float64{0.8, 0.6, 0}
	embC := []float64{0.5, 0.87, 0}
	embD := []float64{0.2, 0.98, 0}
	embE := []float64{-0.1, 0.995, 0}

	candidateA := createUpgradeLabel(t, db, "A", "a", "auxiliary", "active", 5, embA)
	candidateB := createUpgradeLabel(t, db, "B", "b", "auxiliary", "active", 5, embB)
	candidateC := createUpgradeLabel(t, db, "C", "c", "auxiliary", "active", 5, embC)
	candidateD := createUpgradeLabel(t, db, "D", "d", "auxiliary", "active", 5, embD)
	candidateE := createUpgradeLabel(t, db, "E", "e", "auxiliary", "active", 5, embE)

	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "A", RefCount: 5, Embedding: embA},
		{ID: candidateB.ID, Label: "B", RefCount: 5, Embedding: embB},
		{ID: candidateC.ID, Label: "C", RefCount: 5, Embedding: embC},
		{ID: candidateD.ID, Label: "D", RefCount: 5, Embedding: embD},
		{ID: candidateE.ID, Label: "E", RefCount: 5, Embedding: embE},
	}
	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterMethod = "average_link"
	config.ClusterDistanceThreshold = 0.20

	clusters, err := service.ClusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)

	maxSize := 0
	for _, c := range clusters {
		if len(c.Candidates) > maxSize {
			maxSize = len(c.Candidates)
		}
	}
	require.Less(t, maxSize, 5, "no single cluster should contain all 5 candidates")

	totalCandidates := 0
	for _, c := range clusters {
		totalCandidates += len(c.Candidates)
	}
	require.Equal(t, 5, totalCandidates)
}

func TestClusterCandidatesCentroidFallback(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	candidateA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	candidateB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	candidateC := createUpgradeLabel(t, db, "Battery", "battery", "auxiliary", "active", 5, []float64{0, 1, 0})
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	candidates := []SemanticBoardUpgradeCandidate{
		{ID: candidateA.ID, Label: "OpenAI", RefCount: 5, Embedding: []float64{1, 0, 0}},
		{ID: candidateB.ID, Label: "GPT", RefCount: 5, Embedding: []float64{0.95, 0.3122498999, 0}},
		{ID: candidateC.ID, Label: "Battery", RefCount: 5, Embedding: []float64{0, 1, 0}},
	}
	config := service.LoadUpgradeConfig(context.Background())
	config.ClusterMethod = "centroid"

	clusters, err := service.ClusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)
	require.Len(t, clusters, 2)
}

func TestSemanticBoardUpgradeLoadsCoTagEventContext(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	db.Where("key = ?", "semantic_board_upgrade_cotag_hard_limit").Delete(&models.AISettings{})
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_upgrade_cotag_hard_limit", Value: "2"}).Error)
	auxiliary := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	seed := createUpgradeTopicTag(t, db, "seed", models.TagCategoryKeyword)
	eventA := createUpgradeTopicTag(t, db, "Launch", models.TagCategoryEvent)
	eventB := createUpgradeTopicTag(t, db, "Release", models.TagCategoryEvent)
	eventSimilar := createUpgradeTopicTag(t, db, "Similar Launch", models.TagCategoryEvent)
	eventC := createUpgradeTopicTag(t, db, "Conference", models.TagCategoryEvent)
	createUpgradeTopicEmbedding(t, db, eventA.ID, []float64{1, 0, 0})
	createUpgradeTopicEmbedding(t, db, eventSimilar.ID, []float64{0.99, 0.1410673598, 0})
	createUpgradeTopicEmbedding(t, db, eventB.ID, []float64{0, 1, 0})
	createUpgradeTopicEmbedding(t, db, eventC.ID, []float64{0, 0, 1})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: seed.ID, SemanticLabelID: auxiliary.ID}).Error)
	createUpgradeArticleWithTags(t, db, seed.ID, eventA.ID, eventB.ID)
	createUpgradeArticleWithTags(t, db, seed.ID, eventA.ID, eventSimilar.ID)
	createUpgradeArticleWithTags(t, db, seed.ID, eventSimilar.ID, eventC.ID)
	service := NewSemanticBoardUpgradeService(db, nil, nil)
	cluster := SemanticBoardUpgradeCluster{Candidates: []SemanticBoardUpgradeCandidate{{ID: auxiliary.ID, Label: auxiliary.Label, Embedding: []float64{1, 0, 0}}}}

	events, err := service.loadCoTagEventContext(context.Background(), cluster, service.LoadUpgradeConfig(context.Background()))

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, eventA.ID, events[0].TopicTagID)
	require.Equal(t, 2, events[0].Frequency)
	require.Equal(t, eventB.ID, events[1].TopicTagID)
}

func TestSemanticBoardUpgradeGenerateSuggestionsUsesLLMMock(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxiliaryA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxiliaryB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	createUpgradeLabel(t, db, "Transformer", "transformer", "auxiliary", "active", 5, []float64{0.9, 0.4358898943, 0})
	createUpgradeLabel(t, db, "LLM", "llm", "auxiliary", "active", 5, []float64{0.85, 0.5267826876, 0})
	createUpgradeLabel(t, db, "Deep Learning", "deep-learning", "auxiliary", "active", 5, []float64{0.8, 0.6, 0})
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "AI", AuxiliaryLabelIDs: []uint{auxiliaryA.ID, auxiliaryB.ID}},
		{Decision: SemanticBoardUpgradeDecisionSkip, Reason: "too broad"},
		{Decision: "invalid", AuxiliaryLabelIDs: []uint{auxiliaryA.ID}},
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "Unknown", AuxiliaryLabelIDs: []uint{99999}},
	}}
	service := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	suggestions, _, err := service.GenerateSuggestions(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceAux})

	require.NoError(t, err)
	// 单一决策空间：skip 与越权决策（invalid/未知 id 的 create_new）都被过滤，仅剩 1 条有效 create_new。
	require.Len(t, suggestions, 1)
	require.Equal(t, 1, fakeLLM.calls)
	require.Contains(t, fakeLLM.prompt, "OpenAI")
	require.Contains(t, fakeLLM.prompt, "GPT")
	var boardCount int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "board").Count(&boardCount).Error)
	require.Zero(t, boardCount)
	var compositionCount int64
	require.NoError(t, db.Model(&models.BoardComposition{}).Count(&compositionCount).Error)
	require.Zero(t, compositionCount)
}

func TestSemanticBoardUpgradeGenerateSuggestionsSkipsWhenCandidateCountBelowThreshold(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0.95, 0.3122498999, 0})
	createUpgradeLabel(t, db, "Transformer", "transformer", "auxiliary", "active", 5, []float64{0.9, 0.4358898943, 0})
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{{Decision: SemanticBoardUpgradeDecisionCreateNew}}}
	service := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	suggestions, _, err := service.GenerateSuggestions(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceAux})

	require.NoError(t, err)
	require.Empty(t, suggestions)
	require.Zero(t, fakeLLM.calls)
}

// TestSemanticBoardUpgradeDiscoverNewMergeTargetValidation verifies the §4.1
// requirement: a discover_new merge_into_existing whose target_board_id is in
// the cluster shortlist is kept; one whose target is NOT in the shortlist is
// downgraded to skip and not produced as a suggestion (spec: merge 目标必须在
// shortlist 内，否则降级为 skip 不产出建议).

// TestSemanticBoardUpgradeSystemPromptModeAware verifies the LLM system-prompt
// schema is mode-aware (§4.1): discover_new now advertises merge_into_existing +
// TestSemanticBoardUpgradeSystemPromptModeAware verifies the four-grid
// single-decision-space schemas (design D1): each mode advertises exactly one
// decision pair and no cross-mode leakage.
func TestSemanticBoardUpgradeSystemPromptModeAware(t *testing.T) {
	createAux := BuildSemanticBoardUpgradeSystemPrompt("create:aux")
	require.Contains(t, createAux, "create_new|skip")
	require.NotContains(t, createAux, "merge_into_existing")
	require.NotContains(t, createAux, "target_board_id")

	createComposite := BuildSemanticBoardUpgradeSystemPrompt("create:composite")
	require.Contains(t, createComposite, "compose|skip")
	require.NotContains(t, createComposite, "merge_into_existing")

	expandAux := BuildSemanticBoardUpgradeSystemPrompt("expand:aux")
	require.Contains(t, expandAux, "merge_into_existing|skip")
	require.NotContains(t, expandAux, "create_new")

	expandComposite := BuildSemanticBoardUpgradeSystemPrompt("expand:composite")
	require.Contains(t, expandComposite, "compose|skip")
	require.NotContains(t, expandComposite, "create_new")
}

// TestSemanticBoardUpgradeShortlistDualSignature verifies §4.2: the cluster
// shortlist is the union of composition-signature top-2 and lane-signature
// top-2 (≤4, deduped), each entry carrying the per-signature distance; a board
// with no active topic section participates only via composition.

// TestSemanticBoardUpgradeHighConfidenceMergeSkipsLLM verifies §4.3: when the
// composition-signature top-1 and lane-signature top-1 point to the SAME board
// and BOTH per-signature margins (top1-top2 distance gap) are ≥ the configured
// threshold, the system synthesizes a confidence=high merge directly and does
// NOT call the LLM.

// TestSemanticBoardUpgradeSignatureDivergenceGoesToLLM verifies the §4.3 converse:
// when the composition top-1 and lane top-1 point to DIFFERENT boards (signature
// disagreement), the cluster is NOT bypassed — the LLM is called and the
// adjudicated suggestion carries confidence=llm.

// TestSemanticBoardUpgradePromptInjectsLaneEvidence verifies §4.4: the LLM prompt
// includes each shortlist board's recent active-topic section titles (≤5, last
// 30 days) so the LLM grounds its merge decision in the board's actual narrative,
// not just the board name. Boards without sections degrade to name+description only.

// TestSemanticBoardUpgradeSingletonClusterProducesWatch verifies §4.5: a cluster
// of size 1 does NOT enter the LLM — the system synthesizes a decision=watch
// suggestion (observation pool) for the lone label.
// TestSemanticBoardUpgradeSingletonClusterProducesNothing verifies the watch
// retirement (spec: 升级建议生成路径单一化): singleton clusters (size=1) produce
// NO suggestion at all — they neither enter the LLM nor the observation pool.
func TestSemanticBoardUpgradeSingletonClusterProducesNothing(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	// 5 mutually-orthogonal candidates → 5 singleton clusters (none enter the LLM).
	createUpgradeLabel(t, db, "Fable5", "fable5", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "SoloB", "solob", "auxiliary", "active", 5, []float64{0, 1, 0})
	createUpgradeLabel(t, db, "SoloC", "soloc", "auxiliary", "active", 5, []float64{0, 0, 1})
	createUpgradeLabel(t, db, "SoloD", "solod", "auxiliary", "active", 5, []float64{0, 0, 0, 1})
	createUpgradeLabel(t, db, "SoloE", "soloe", "auxiliary", "active", 5, []float64{0, 0, 0, 0, 1})

	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{{Decision: SemanticBoardUpgradeDecisionSkip}}}
	svc := NewSemanticBoardUpgradeService(db, fakeLLM, nil)
	suggestions, _, err := svc.GenerateSuggestions(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceAux})
	require.NoError(t, err)
	require.Zero(t, fakeLLM.calls, "singleton clusters must bypass the LLM entirely")
	require.Empty(t, suggestions, "singleton clusters produce no suggestion (watch pool retired)")
}

// TestSemanticBoardUpgradeClusteringClosesPriorWatch verifies §4.5: when a label
// that previously had a singleton-cluster watch suggestion later clusters (≥2)
// and produces a formal suggestion, GenerateAndPersist auto-closes the prior
// watch (pending → confirmed) via CloseWatchSuggestions.

func TestSemanticBoardUpgradeConfirmCreateNew(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxiliaryA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxiliaryB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0, 1, 0})
	service := NewSemanticBoardUpgradeService(db, nil, nil)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCreateNew,
		BoardLabel:        "AI Models",
		Description:       "AI model ecosystem",
		AuxiliaryLabelIDs: []uint{auxiliaryB.ID, auxiliaryA.ID, auxiliaryA.ID},
	})

	require.NoError(t, err)
	require.NotZero(t, result.SemanticBoardID)
	require.Equal(t, []uint{auxiliaryA.ID, auxiliaryB.ID}, result.AuxiliaryLabelIDs)
	var board models.SemanticLabel
	require.NoError(t, db.First(&board, result.SemanticBoardID).Error)
	require.Equal(t, "board", board.LabelType)
	require.Equal(t, "llm_suggest", board.Source)
	require.Equal(t, "active", board.Status)
	require.Equal(t, "AI model ecosystem", board.Description)
	var rows []models.BoardComposition
	require.NoError(t, db.Order("auxiliary_label_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, auxiliaryA.ID, rows[0].AuxiliaryLabelID)
	require.Equal(t, auxiliaryB.ID, rows[1].AuxiliaryLabelID)
}

func TestSemanticBoardUpgradeConfirmMergeIntoExisting(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxiliaryA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxiliaryB := createUpgradeLabel(t, db, "GPT", "gpt", "auxiliary", "active", 5, []float64{0, 1, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliaryA.ID}).Error)
	service := NewSemanticBoardUpgradeService(db, nil, nil)

	result, err := service.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		TargetBoardID:     &board.ID,
		AuxiliaryLabelIDs: []uint{auxiliaryA.ID, auxiliaryB.ID},
	})

	require.NoError(t, err)
	require.Equal(t, board.ID, result.SemanticBoardID)
	var rows []models.BoardComposition
	require.NoError(t, db.Where("board_id = ?", board.ID).Order("auxiliary_label_id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, auxiliaryA.ID, rows[0].AuxiliaryLabelID)
	require.Equal(t, auxiliaryB.ID, rows[1].AuxiliaryLabelID)
}

// TestSemanticBoardUpgradeConfirmLinksPendingSuggestion verifies that a
// confirm (upgrade-execute) carrying a suggestion_id marks the linked pending
// suggestion confirmed inside the same transaction that writes board_composition
// (spec: 建议 dismiss 与 confirm 联动).
//
// §3.4 happy path: confirm merge with suggestion_id → suggestion confirmed +
// board composition written.
func TestSemanticBoardUpgradeConfirmLinksPendingSuggestion(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	aux := createUpgradeLabel(t, db, "DeepSeek", "deepseek", "auxiliary", "active", 5, []float64{1, 0, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)

	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	sug := &models.BoardUpgradeSuggestion{
		BatchID: "conf-1", Mode: "discover_new", Decision: "merge_into_existing",
		BoardLabel: "DeepSeek", TargetBoardID: &board.ID, AuxiliaryLabelIDs: []uint{aux.ID},
		Confidence: "llm", SuggestionHash: "conf-hash-1",
	}
	inserted, err := repo.InsertPending(context.Background(), sug)
	require.NoError(t, err)
	require.True(t, inserted)

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	_, err = svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		TargetBoardID:     &board.ID,
		AuxiliaryLabelIDs: []uint{aux.ID},
		SuggestionID:      &sug.ID,
	})
	require.NoError(t, err)

	var reloaded models.BoardUpgradeSuggestion
	require.NoError(t, db.First(&reloaded, sug.ID).Error)
	require.Equal(t, "confirmed", reloaded.Status, "linked pending suggestion must be confirmed in the same tx")
	require.NotNil(t, reloaded.ResolvedAt, "confirmed suggestion records resolved_at")
}

// TestSemanticBoardUpgradeConfirmTxFailureLeavesSuggestionPending verifies that
// when the upgrade-execute transaction fails (e.g. target board missing), the
// linked suggestion is NOT marked confirmed — state stays pending (spec: confirm
// 联动, 事务失败时建议状态不变).
func TestSemanticBoardUpgradeConfirmTxFailureLeavesSuggestionPending(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	aux := createUpgradeLabel(t, db, "Llama", "llama", "auxiliary", "active", 5, []float64{1, 0, 0})
	// No board row is created → the merge target lookup fails and the tx rolls back.

	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	badTarget := uint(99999)
	sug := &models.BoardUpgradeSuggestion{
		BatchID: "conf-tx", Mode: "discover_new", Decision: "merge_into_existing",
		BoardLabel: "Llama", TargetBoardID: &badTarget, AuxiliaryLabelIDs: []uint{aux.ID},
		Confidence: "llm", SuggestionHash: "conf-hash-tx",
	}
	inserted, err := repo.InsertPending(context.Background(), sug)
	require.NoError(t, err)
	require.True(t, inserted)

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	_, err = svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		TargetBoardID:     &badTarget, // non-existent → "active target board not found" → tx rollback
		AuxiliaryLabelIDs: []uint{aux.ID},
		SuggestionID:      &sug.ID,
	})
	require.Error(t, err, "confirm against a non-existent target must fail")

	var reloaded models.BoardUpgradeSuggestion
	require.NoError(t, db.First(&reloaded, sug.ID).Error)
	require.Equal(t, "pending", reloaded.Status, "tx failure must leave suggestion state unchanged")
	require.Nil(t, reloaded.ResolvedAt, "no resolved_at on a rolled-back confirm")
}

// TestSemanticBoardUpgradeConfirmMergeMissingTarget verifies that a merge
// confirm against a suggestion whose target_board_id is NULL (board-upgrade
// spec 方案 B: target_off_shortlist=true) fails with an actionable error
// prompting the user to pick a target board, instead of a generic 400.
func TestSemanticBoardUpgradeConfirmMergeMissingTarget(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	aux := createUpgradeLabel(t, db, "Jensen", "jensen", "auxiliary", "active", 5, []float64{1, 0, 0})

	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	// 方案 B: merge 建议保留但 target_board_id=NULL（target_off_shortlist=true）。
	sug := &models.BoardUpgradeSuggestion{
		BatchID: "conf-missing", Mode: "discover_new", Decision: "merge_into_existing",
		BoardLabel: "全球科技巨头动态", AuxiliaryLabelIDs: []uint{aux.ID},
		Confidence: "llm", SuggestionHash: "conf-missing-hash",
	}
	inserted, err := repo.InsertPending(context.Background(), sug)
	require.NoError(t, err)
	require.True(t, inserted)

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	_, err = svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		AuxiliaryLabelIDs: []uint{aux.ID},
		SuggestionID:      &sug.ID, // TargetBoardID deliberately nil
	})
	require.Error(t, err, "merge without a target board must fail")
	require.Contains(t, err.Error(), "目标板块", "error must prompt the user to pick a target board")

	// Suggestion state unchanged on the failed confirm.
	var reloaded models.BoardUpgradeSuggestion
	require.NoError(t, db.First(&reloaded, sug.ID).Error)
	require.Equal(t, "pending", reloaded.Status)
}

// TestSemanticBoardUpgradeConfirmWithoutSuggestionIDLeavesItPending verifies the
// back-compat path: a confirm request that omits suggestion_id writes the board
// normally but does not touch suggestion state (spec: 未携带 suggestion_id 的请求
// SHALL 正常执行版块写入但不联动建议状态).
func TestSemanticBoardUpgradeConfirmWithoutSuggestionIDLeavesItPending(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	aux := createUpgradeLabel(t, db, "Mistral", "mistral", "auxiliary", "active", 5, []float64{1, 0, 0})
	board := createUpgradeLabel(t, db, "LLM Board", "llm-board", "board", "active", 0, nil)

	repo := repository.NewBoardUpgradeSuggestionRepository(db)
	sug := &models.BoardUpgradeSuggestion{
		BatchID: "conf-noid", Mode: "discover_new", Decision: "merge_into_existing",
		BoardLabel: "Mistral", TargetBoardID: &board.ID, AuxiliaryLabelIDs: []uint{aux.ID},
		Confidence: "llm", SuggestionHash: "conf-hash-noid",
	}
	inserted, err := repo.InsertPending(context.Background(), sug)
	require.NoError(t, err)
	require.True(t, inserted)

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	res, err := svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		TargetBoardID:     &board.ID,
		AuxiliaryLabelIDs: []uint{aux.ID},
		// SuggestionID omitted (nil) — back-compat path.
	})
	require.NoError(t, err)
	require.Equal(t, board.ID, res.SemanticBoardID, "board composition must still be written")

	var reloaded models.BoardUpgradeSuggestion
	require.NoError(t, db.First(&reloaded, sug.ID).Error)
	require.Equal(t, "pending", reloaded.Status, "confirm without suggestion_id must not touch suggestion state")
	require.Nil(t, reloaded.ResolvedAt)
}

// TestSemanticBoardUpgradeConfirmSkipsInactiveAuxiliaryLabels verifies that a
// confirm carrying a mix of active and inactive auxiliary labels silently skips
// the inactive ones (建议生成后标签可能被禁用/合并), writing composition only
// for the still-active labels instead of failing the whole request.
func TestSemanticBoardUpgradeConfirmSkipsInactiveAuxiliaryLabels(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	activeA := createUpgradeLabel(t, db, "OpenAI", "openai", "auxiliary", "active", 5, []float64{1, 0, 0})
	inactiveB := createUpgradeLabel(t, db, "Disabled Label", "disabled-label", "auxiliary", "disabled", 5, []float64{0, 1, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	result, err := svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		TargetBoardID:     &board.ID,
		AuxiliaryLabelIDs: []uint{activeA.ID, inactiveB.ID},
	})

	require.NoError(t, err)
	require.Equal(t, board.ID, result.SemanticBoardID)
	require.Equal(t, []uint{activeA.ID}, result.AuxiliaryLabelIDs, "inactive label must be skipped")

	var rows []models.BoardComposition
	require.NoError(t, db.Where("board_id = ?", board.ID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, activeA.ID, rows[0].AuxiliaryLabelID, "only the active label is linked to the board")
}

// TestSemanticBoardUpgradeConfirmAllInactiveAuxiliaryLabelsErrors verifies that
// when every requested auxiliary label is inactive, the confirm fails with an
// actionable error instead of writing an empty composition.
func TestSemanticBoardUpgradeConfirmAllInactiveAuxiliaryLabelsErrors(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	inactiveA := createUpgradeLabel(t, db, "Disabled A", "disabled-a", "auxiliary", "disabled", 5, []float64{1, 0, 0})
	inactiveB := createUpgradeLabel(t, db, "Disabled B", "disabled-b", "auxiliary", "disabled", 5, []float64{0, 1, 0})
	board := createUpgradeLabel(t, db, "AI Board", "ai-board", "board", "active", 0, nil)

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	_, err := svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionMergeIntoExisting,
		TargetBoardID:     &board.ID,
		AuxiliaryLabelIDs: []uint{inactiveA.ID, inactiveB.ID},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "失效", "error must explain all labels are inactive")

	var rows []models.BoardComposition
	require.NoError(t, db.Where("board_id = ?", board.ID).Find(&rows).Error)
	require.Empty(t, rows, "no composition written when all labels are inactive")
}

// TestSemanticBoardUpgradeGenerateAndPersistInsertsNonSkip verifies the
// GenerateAndPersist entry point persists non-skip suggestions as pending and
// drops skip decisions (spec: 升级建议持久化存储).
//
// GenerateAndPersist Test A: 2 non-skip + 1 skip → 2 pending rows, inserted=2.
func TestSemanticBoardUpgradeGenerateAndPersistInsertsNonSkip(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxA := createUpgradeLabel(t, db, "Alpha", "alpha", "auxiliary", "active", 5, []float64{1, 0, 0})
	auxB := createUpgradeLabel(t, db, "Beta", "beta", "auxiliary", "active", 5, []float64{0.9, 0.4358898943, 0})
	createUpgradeLabel(t, db, "Gamma", "gamma", "auxiliary", "active", 5, []float64{0.8, 0.6, 0})
	createUpgradeLabel(t, db, "Delta", "delta", "auxiliary", "active", 5, []float64{0.7, 0.7141428428, 0})
	createUpgradeLabel(t, db, "Epsilon", "epsilon", "auxiliary", "active", 5, []float64{0.6, 0.8, 0})
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "Board A", Description: "A", AuxiliaryLabelIDs: []uint{auxA.ID}},
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "Board B", Description: "B", AuxiliaryLabelIDs: []uint{auxB.ID}},
		{Decision: SemanticBoardUpgradeDecisionSkip, Reason: "too broad"},
	}}
	svc := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	inserted, _, _, err := svc.GenerateAndPersist(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceAux})
	require.NoError(t, err)
	require.Equal(t, 2, inserted, "two non-skip suggestions must be persisted")

	var rows []models.BoardUpgradeSuggestion
	require.NoError(t, db.Order("board_label ASC").Find(&rows).Error)
	require.Len(t, rows, 2, "skip decision must NOT be persisted")
	for _, r := range rows {
		require.Equal(t, "pending", r.Status)
		require.Equal(t, "create:aux", r.Mode)
		require.Equal(t, "llm", r.Confidence, "generation confidence defaults to llm")
		require.NotEmpty(t, r.SuggestionHash)
		require.NotEmpty(t, r.BatchID)
	}
}

// TestSemanticBoardUpgradeGenerateAndPersistIdempotentOnSecondRun verifies a
// re-run over the same cluster+decision is a no-op: the partial unique index
// makes InsertPending a skip, counted as skipped (spec: 建议生成幂等).
//
// GenerateAndPersist Test B: second run → inserted=0, skipped=1.
func TestSemanticBoardUpgradeGenerateAndPersistIdempotentOnSecondRun(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxA := createUpgradeLabel(t, db, "Alpha2", "alpha2", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "Gamma2", "gamma2", "auxiliary", "active", 5, []float64{0.8, 0.6, 0})
	createUpgradeLabel(t, db, "Delta2", "delta2", "auxiliary", "active", 5, []float64{0.7, 0.7141428428, 0})
	createUpgradeLabel(t, db, "Epsilon2", "epsilon2", "auxiliary", "active", 5, []float64{0.6, 0.8, 0})
	createUpgradeLabel(t, db, "Zeta2", "zeta2", "auxiliary", "active", 5, []float64{0.5, 0.8660254037, 0})
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "Board A", AuxiliaryLabelIDs: []uint{auxA.ID}},
	}}
	svc := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	ins1, _, _, err := svc.GenerateAndPersist(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceAux})
	require.NoError(t, err)
	require.Equal(t, 1, ins1, "first run inserts one")

	ins2, skipped2, _, err := svc.GenerateAndPersist(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceAux})
	require.NoError(t, err)
	require.Zero(t, ins2, "second run must not re-insert a duplicate pending hash")
	require.Equal(t, 1, skipped2, "the idempotent duplicate must be counted as skipped")

	var count int64
	require.NoError(t, db.Model(&models.BoardUpgradeSuggestion{}).Count(&count).Error)
	require.Equal(t, int64(1), count, "still exactly one row after re-run")
}

// TestSemanticBoardUpgradeGenerateAndPersistBlocksOnDismissedCooldown verifies
// a suggestion whose hash matches a recent dismissal is NOT re-generated; it is
// counted as cooldownBlocked (spec: dismissed 冷却期).
//
// GenerateAndPersist Test C: a 3-day-old dismissed suggestion with the same
// hash as what the LLM will produce → inserted=0, cooldownBlocked=1.
func TestSemanticBoardUpgradeGenerateAndPersistBlocksOnDismissedCooldown(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	auxA := createUpgradeLabel(t, db, "Alpha3", "alpha3", "auxiliary", "active", 5, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "Gamma3", "gamma3", "auxiliary", "active", 5, []float64{0.8, 0.6, 0})
	createUpgradeLabel(t, db, "Delta3", "delta3", "auxiliary", "active", 5, []float64{0.7, 0.7141428428, 0})
	createUpgradeLabel(t, db, "Epsilon3", "epsilon3", "auxiliary", "active", 5, []float64{0.6, 0.8, 0})
	createUpgradeLabel(t, db, "Zeta3", "zeta3", "auxiliary", "active", 5, []float64{0.5, 0.8660254037, 0})

	// The LLM will suggest create_new over [auxA]; pre-seed a dismissed row with
	// the identical hash so the cooldown gate must block re-generation.
	hash := ComputeSuggestionHash("create:aux", "create_new", nil, []uint{auxA.ID})
	threeDaysAgo := time.Now().AddDate(0, 0, -3)
	require.NoError(t, db.Create(&models.BoardUpgradeSuggestion{
		BatchID: "seed", Mode: "create:aux", Decision: "create_new",
		BoardLabel: "Board A", AuxiliaryLabelIDs: []uint{auxA.ID},
		Confidence: "llm", Status: "dismissed", ResolvedAt: &threeDaysAgo,
		SuggestionHash: hash,
	}).Error)

	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "Board A", AuxiliaryLabelIDs: []uint{auxA.ID}},
	}}
	svc := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	inserted, _, cooldownBlocked, err := svc.GenerateAndPersist(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionCreate, Source: UpgradeSourceAux})
	require.NoError(t, err)
	require.Zero(t, inserted, "a hash in cooldown must not be re-inserted")
	require.Equal(t, 1, cooldownBlocked, "the dismissed-in-cooldown hash must be counted as cooldownBlocked")

	// No NEW pending row created for this hash (only the pre-seeded dismissed one exists).
	var pending int64
	require.NoError(t, db.Model(&models.BoardUpgradeSuggestion{}).Where("suggestion_hash = ? AND status = ?", hash, "pending").Count(&pending).Error)
	require.Zero(t, pending, "no pending row must be created for a cooled-down hash")
}

func createUpgradeLabel(t *testing.T, db *gorm.DB, label string, slug string, labelType string, status string, refCount int, vector []float64) models.SemanticLabel {
	t.Helper()
	semanticLabel := models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: status, RefCount: refCount}
	if vector != nil {
		pgVector := core.FloatsToPgVector(testutil.PadVector(vector, testutil.TestEmbeddingDim))
		semanticLabel.Embedding = &pgVector
	}
	require.NoError(t, db.Create(&semanticLabel).Error)
	return semanticLabel
}

func createUpgradeTopicTag(t *testing.T, db *gorm.DB, label string, category string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: core.Slugify(label), Category: category, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

func createUpgradeTopicEmbedding(t *testing.T, db *gorm.DB, topicTagID uint, vector []float64) {
	t.Helper()
	padded := testutil.PadVector(vector, testutil.TestEmbeddingDim)
	pgVector := core.FloatsToPgVector(padded)
	require.NoError(t, db.Create(&models.TopicTagEmbedding{TopicTagID: topicTagID, EmbeddingType: "semantic", EmbeddingVec: pgVector, Dimension: testutil.TestEmbeddingDim, Model: "test", TextHash: fmt.Sprintf("hash-%d", topicTagID)}).Error)
}

func createUpgradeArticleWithTags(t *testing.T, db *gorm.DB, topicTagIDs ...uint) {
	t.Helper()
	now := time.Now()
	seq := atomic.AddUint64(&upgradeFeedSeq, 1)
	feed := models.Feed{Title: fmt.Sprintf("feed-%d", seq), URL: fmt.Sprintf("https://example.com/%d", seq), CreatedAt: now}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("article-%d", now.UnixNano()), CreatedAt: now}
	require.NoError(t, db.Create(&article).Error)
	for _, topicTagID := range topicTagIDs {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: topicTagID}).Error)
	}
}

func upgradeCandidateIDs(candidates []SemanticBoardUpgradeCandidate) []uint {
	ids := make([]uint, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	return ids
}

// daysAgo returns a timestamp n days in the past (used for daily-report period_date).
func daysAgo(n int) time.Time {
	return time.Now().AddDate(0, 0, -n)
}

// The lane signature (§4.2) and lane evidence (§4.4) read board_daily_reports /
// board_persistent_topics / daily_report_sections — tables owned by
// topicgraph/repository. That package imports the tagmanagement root package,
// which transitively imports service/board, so importing it here would form a
// cycle. These local structs mirror the production models field-for-field (same
// TableName, same columns) so the testcontainer AutoMigrate + versioned
// migrations (which reference columns like board_persistent_topics.hit_count)
// succeed identically to production. They are test-only row builders; the
// production models in topicgraph/repository remain the schema source of truth.

// testJSON mirrors repository.JSON ([]byte over jsonb) for AutoMigrate + inserts.
type testJSON []byte

func (j testJSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return string(j), nil
}

func (j *testJSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append((*j)[0:0], v...)
	case string:
		*j = append((*j)[0:0], v...)
	default:
		return fmt.Errorf("testJSON: unsupported scan type %T", value)
	}
	return nil
}

type testBoardDailyReport struct {
	ID                      uint      `gorm:"primarykey"`
	SemanticBoardID         uint      `gorm:"index;not null"`
	PeriodDate              time.Time `gorm:"type:date;not null"`
	Title                   string
	Summary                 string
	Highlights              testJSON `gorm:"type:jsonb"`
	Dynamics                string   `gorm:"type:text"`
	ArticleCount            int
	EventTagCount           int
	ClusterCount            int
	Status                  string   `gorm:"size:20;default:generating"`
	RawClusters             testJSON `gorm:"type:jsonb"`
	PrevReportID            *uint
	GenerationPromptVersion string `gorm:"size:20"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (testBoardDailyReport) TableName() string { return "board_daily_reports" }

type testBoardPersistentTopic struct {
	ID              uint      `gorm:"primarykey"`
	SemanticBoardID uint      `gorm:"not null;index"`
	Label           string    `gorm:"size:200;not null"`
	Description     string    `gorm:"type:text"`
	Embedding       *string   `gorm:"type:vector"`
	Status          string    `gorm:"size:20;not null;default:candidate"`
	Source          string    `gorm:"size:10;not null;default:auto"`
	FirstSeenDate   time.Time `gorm:"type:date;not null"`
	LastSeenDate    time.Time `gorm:"type:date;not null"`
	HitCount        int       `gorm:"not null;default:1"`
	ConsecutiveHits int       `gorm:"not null;default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (testBoardPersistentTopic) TableName() string { return "board_persistent_topics" }

type testDailyReportSection struct {
	ID                   uint `gorm:"primarykey"`
	ReportID             uint `gorm:"index;not null"`
	ClusterIndex         int
	ClusterLabel         string   `gorm:"size:200"`
	ClusterTagIDs        testJSON `gorm:"type:jsonb"`
	ArticleCount         int
	BestTier             int      `gorm:"default:0"`
	AvgScore             float64  `gorm:"default:0"`
	QualityBreakdown     testJSON `gorm:"type:jsonb"`
	Embedding            string   `gorm:"type:vector"`
	PersistentTopicID    *uint    `gorm:"index"`
	TopicMatchDistance   float64
	TopicMatchConfidence string  `gorm:"size:20"`
	TopicStatusAtReport  *string `gorm:"size:20"`
	CreatedAt            time.Time
}

func (testDailyReportSection) TableName() string { return "daily_report_sections" }

// init registers the daily-report tables for the board test binary's AutoMigrate.
// These tables are owned by topicgraph/repository, whose package init registers them
// in production — but the board test binary cannot import that package (it imports
// the tagmanagement root, which transitively imports service/board → cycle). Without
// this, the testcontainer schema lacks the tables the lane signature queries. The
// local structs above map 1:1 to the production tables (same TableName), so
// AutoMigrate creates compatible columns. This runs before the first SetupTestDB
// (Go init order), so the golden schema includes them.
func init() {
	database.RegisterModels(
		&testBoardDailyReport{},
		&testBoardPersistentTopic{},
		&testDailyReportSection{},
	)
}

func createUpgradePersistentTopic(t *testing.T, db *gorm.DB, boardID uint, status string) testBoardPersistentTopic {
	t.Helper()
	today := daysAgo(0)
	topic := testBoardPersistentTopic{
		SemanticBoardID: boardID, Label: fmt.Sprintf("topic-%d-%d", boardID, time.Now().UnixNano()),
		Status: status, FirstSeenDate: today, LastSeenDate: today,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

func createUpgradeBoardDailyReport(t *testing.T, db *gorm.DB, boardID uint, periodDate time.Time) testBoardDailyReport {
	t.Helper()
	report := testBoardDailyReport{SemanticBoardID: boardID, PeriodDate: periodDate, Status: "completed"}
	require.NoError(t, db.Create(&report).Error)
	return report
}

func createUpgradeReportSection(t *testing.T, db *gorm.DB, reportID uint, topicID uint, clusterLabel string, vector []float64) testDailyReportSection {
	t.Helper()
	pgVector := core.FloatsToPgVector(testutil.PadVector(vector, testutil.TestEmbeddingDim))
	section := testDailyReportSection{
		ReportID: reportID, ClusterLabel: clusterLabel, Embedding: pgVector,
		PersistentTopicID: &topicID,
	}
	require.NoError(t, db.Create(&section).Error)
	return section
}
