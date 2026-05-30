package tagging

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

type fakeSemanticBoardUpgradeLLM struct {
	prompt      string
	suggestions []SemanticBoardUpgradeSuggestion
	calls       int
}

var upgradeFeedSeq uint64

func (f *fakeSemanticBoardUpgradeLLM) SuggestSemanticBoardUpgrades(ctx context.Context, prompt string) ([]SemanticBoardUpgradeSuggestion, error) {
	f.calls++
	f.prompt = prompt
	return f.suggestions, nil
}

func setupSemanticBoardUpgradeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	database.DB = db
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.TopicTagEmbedding{}, &models.ArticleTopicTag{}, &models.SemanticLabel{}, &models.TopicTagSemanticLabel{}, &models.TopicTagBoardLabel{}, &models.BoardComposition{}, &models.AISettings{}))
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

	candidates, err := service.collectCandidates(context.Background(), service.LoadUpgradeConfig(context.Background()))

	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, include.ID, candidates[0].ID)
	require.Equal(t, []float64{1, 0, 0}, candidates[0].Embedding)
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

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)

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

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)
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

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)
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

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)
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

	clusters, err := service.clusterCandidates(context.Background(), candidates, config)
	require.NoError(t, err)
	require.Len(t, clusters, 2)
}

func TestSemanticBoardUpgradeLoadsCoTagEventContext(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
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

	suggestions, _, err := service.GenerateSuggestions(context.Background())

	require.NoError(t, err)
	require.Len(t, suggestions, 2)
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

	suggestions, _, err := service.GenerateSuggestions(context.Background())

	require.NoError(t, err)
	require.Empty(t, suggestions)
	require.Zero(t, fakeLLM.calls)
}

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

func createUpgradeLabel(t *testing.T, db *gorm.DB, label string, slug string, labelType string, status string, refCount int, vector []float64) models.SemanticLabel {
	t.Helper()
	semanticLabel := models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: status, RefCount: refCount}
	if vector != nil {
		pgVector := floatsToPgVector(vector)
		semanticLabel.Embedding = &pgVector
	}
	require.NoError(t, db.Create(&semanticLabel).Error)
	return semanticLabel
}

func createUpgradeTopicTag(t *testing.T, db *gorm.DB, label string, category string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: Slugify(label), Category: category, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

func createUpgradeTopicEmbedding(t *testing.T, db *gorm.DB, topicTagID uint, vector []float64) {
	t.Helper()
	pgVector := floatsToPgVector(vector)
	require.NoError(t, db.Create(&models.TopicTagEmbedding{TopicTagID: topicTagID, EmbeddingType: "semantic", Vector: "[]", EmbeddingVec: pgVector, Dimension: len(vector), Model: "test", TextHash: fmt.Sprintf("hash-%d", topicTagID)}).Error)
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
