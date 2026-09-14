package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── High 2 / Medium 8：原生 RSS 候选进召回 + 候选级资格过滤 ──
//
// 锚 spec feed-candidate-catalog C2/C7 与 feed-discovery「Independent Board and Behavior
// Recall」：原生 rss 候选凭 CandidateEmbedding（有效文本来自 feed_url + 人工元数据，由
// 回补服务覆盖）参与检索；rsshub 候选仍关联路由取名称；资格过滤按 kind 分流——
// rsshub 看 route.status（gone/broken），原生看 candidate_availability.status=broken。

// setupNativeCandidateVector 建原生 rss 候选 + 向量（model 默认 test）。
func setupNativeCandidateVector(t *testing.T, db *gorm.DB, url string, vec []float64) uint {
	t.Helper()
	enabled := true
	cand := models.FeedCandidate{
		StableKey: "rss:" + url, Kind: "rss", FeedURL: &url, CanonicalKey: url,
		ManualMetadata:        models.MetadataMap{ManualFieldName: "原生源"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	require.NoError(t, db.Create(&models.CandidateEmbedding{
		CandidateID: cand.ID, Model: "test", Dimension: len(vec), EmbeddingVec: floatsToPgVector(vec),
	}).Error)
	return cand.ID
}

// setupGlobalBehavior 建全局行为画像（refresh 召回入口，与候选向量同模型同维）。
func setupGlobalBehavior(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: nil, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{1, 0, 0}), Dimension: 3, Model: "test",
	}).Error)
}

// TestRefresh_NativeCandidateEntersRerankAndPublishes：原生 rss 候选有向量 → 进精排集合、
// 发布卡候选身份正确（route_id=0、candidate_id 指向原生候选）。
func TestRefresh_NativeCandidateEntersRerankAndPublishes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	setupGlobalBehavior(t, db)
	nativeID := setupNativeCandidateVector(t, db, "https://native.example/feed.xml", []float64{1, 0, 0})

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	require.Contains(t, promptIDSet(prompts[0]), nativeID, "原生候选以 candidate_id 身份进精排集合")
	require.Contains(t, prompts[0], "原生源")

	var pending []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&pending).Error)
	require.Len(t, pending, 1)
	require.EqualValues(t, 0, pending[0].RouteID, "原生候选无路由")
	require.NotNil(t, pending[0].CandidateID)
	require.Equal(t, nativeID, *pending[0].CandidateID)
}

// TestRefresh_GoneRsshubRouteFilteredNativeKept：rsshub 路由 gone → 该候选被资格过滤；
// 同批原生候选照常入批（kind 分流：route 状态只约束 rsshub）。
func TestRefresh_GoneRsshubRouteFilteredNativeKept(t *testing.T) {
	db := testutil.SetupTestDB(t)
	setupGlobalBehavior(t, db)
	gone := setupRecallRoute(t, db, "gone", []float64{1, 0, 0})
	require.NoError(t, db.Model(&models.RSSHubRoute{}).Where("id = ?", gone.routeID).Update("status", "gone").Error)
	nativeID := setupNativeCandidateVector(t, db, "https://native.example/keep.xml", []float64{1, 0, 0})

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	ids := promptIDSet(prompts[0])
	require.Contains(t, ids, nativeID)
	require.NotContains(t, ids, gone.candidateID, "gone 路由的 rsshub 候选被滤")
}

// TestRefresh_BrokenNativeAvailabilityFiltered：原生候选 candidate_availability.status=broken
// → 被滤（Medium 8）；unknown 不硬过滤（未验证仍可推荐）。
func TestRefresh_BrokenNativeAvailabilityFiltered(t *testing.T) {
	db := testutil.SetupTestDB(t)
	setupGlobalBehavior(t, db)
	brokenID := setupNativeCandidateVector(t, db, "https://native.example/broken.xml", []float64{1, 0, 0})
	unknownID := setupNativeCandidateVector(t, db, "https://native.example/unknown.xml", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.CandidateAvailability{
		CandidateID: brokenID, Status: "broken",
	}).Error)
	require.NoError(t, db.Create(&models.CandidateAvailability{
		CandidateID: unknownID, Status: "unknown",
	}).Error)

	var prompts []string
	client := &mockDiscoveryClient{chatFn: captureAllChatFn(&prompts)}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)

	_, err := svc.Refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	ids := promptIDSet(prompts[0])
	require.NotContains(t, ids, brokenID, "broken 可用性原生候选被滤")
	require.Contains(t, ids, unknownID, "unknown（未验证）不得硬过滤")
}

// TestCandidateVectorConsistencyPerCandidateSkips（Medium 4 / Low 12 白盒）：候选级兼容
// 判定——部分候选兼容 → 不报错（仅跳过）；零候选兼容且存在向量行 → errRecallModelMismatch。
func TestCandidateVectorConsistencyPerCandidateSkips(t *testing.T) {
	db := testutil.SetupTestDB(t)
	compatible := setupNativeCandidateVector(t, db, "https://native.example/a.xml", []float64{1, 0, 0})
	incompatible := setupNativeCandidateVector(t, db, "https://native.example/b.xml", []float64{1, 0, 0})
	require.NoError(t, db.Model(&models.CandidateEmbedding{}).
		Where("candidate_id = ?", incompatible).Update("model", "other-model").Error)

	recSvc := NewRecommendationService(db, nil, nil)
	batcher := &pgRecallBatcher{recSvc: recSvc}
	ref := &recallVectorRef{model: "test", dim: 3}
	require.NoError(t, batcher.checkCandidateVectorConsistency(context.Background(), ref),
		"有兼容候选时只跳过不兼容者，不阻断整轮")

	// 把兼容候选也改成不兼容 → 零兼容候选 → 整轮 configuration 失败。
	require.NoError(t, db.Model(&models.CandidateEmbedding{}).
		Where("candidate_id = ?", compatible).Update("model", "other-model").Error)
	err := batcher.checkCandidateVectorConsistency(context.Background(), ref)
	require.Error(t, err)
	require.ErrorIs(t, err, errRecallModelMismatch)
}

// TestCandidateVectorConsistencyEmptyNilRef：无向量行 / ref=nil 不做校验（仅版块路）。
func TestCandidateVectorConsistencyEmptyNilRef(t *testing.T) {
	db := testutil.SetupTestDB(t)
	recSvc := NewRecommendationService(db, nil, nil)
	batcher := &pgRecallBatcher{recSvc: recSvc}
	require.NoError(t, batcher.checkCandidateVectorConsistency(context.Background(), nil))
	require.NoError(t, batcher.checkCandidateVectorConsistency(context.Background(), &recallVectorRef{model: "test", dim: 3}))
}
