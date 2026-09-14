package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// ── 推荐生命周期 handler 测试（4.4 / design D5）──
//
// 端点契约：GET /recommendations?scope=history 返回历史聚合；POST /:id/dismiss 升级为
// 「暂时不看」并返回实际到期时刻；POST /:id/exclude 长期排除；POST /:id/restore 恢复。

func setupLifecycleHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{}, &models.FeedCandidate{}, &models.FeedRecommendation{},
		&models.CandidatePreference{}, &models.RSSHubRoute{}, &models.SemanticLabel{}, &models.Feed{},
	))
	database.DB = db
	repository.InitRepository(db)
	return db
}

func newLifecycleGinContext(t *testing.T, method, rawURL, id string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, rawURL, nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	if id != "" {
		ctx.Params = gin.Params{{Key: "id", Value: id}}
	}
	return ctx, recorder
}

// lifecycleFixtureSeq 保证同一测试内多次 fixture 的 namespace/path 唯一。
var lifecycleFixtureSeq int

// lifecycleFixture 建路由 + 候选 + 推荐行，返回推荐与候选。
func lifecycleFixture(t *testing.T, db *gorm.DB, status string, expiresAt *time.Time) (models.FeedRecommendation, models.FeedCandidate) {
	t.Helper()
	lifecycleFixtureSeq++
	ns, path := fmt.Sprintf("ns%d", lifecycleFixtureSeq), fmt.Sprintf("/p%d", lifecycleFixtureSeq)
	route := models.RSSHubRoute{
		Namespace: ns, Path: path, Name: "示例源", Example: ns + path,
		UsableDirectly: true, Status: "ok", Parameters: "{}",
	}
	require.NoError(t, db.Create(&route).Error)
	enabled := true
	cand := models.FeedCandidate{
		StableKey: ns + path, Kind: "rsshub", RouteID: &route.ID,
		ManualMetadata: models.MetadataMap{}, RecommendationEnabled: &enabled,
		AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)
	rec := models.FeedRecommendation{
		RouteID: route.ID, CandidateID: &cand.ID, Source: "qa", Score: 1,
		LLMReason: "理由", Status: status, RecommendationHash: "hash-" + ns, ExpiresAt: expiresAt,
	}
	require.NoError(t, db.Create(&rec).Error)
	return rec, cand
}

func lifecycleBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

// TestLifecycleHandler_HistoryShape：scope=history 返回数组，到期行标 expired 且
// snoozed_until=null（不携带「不感兴趣」语义），活跃未到期 pending 不出现。
func TestLifecycleHandler_HistoryShape(t *testing.T) {
	db := setupLifecycleHandlerTestDB(t)
	future := time.Now().AddDate(0, 0, 7)
	past := time.Now().Add(-1 * time.Hour)
	pending, _ := lifecycleFixture(t, db, "pending", &future)
	expired, _ := lifecycleFixture(t, db, "pending", &past)

	ctx, recorder := newLifecycleGinContext(t, http.MethodGet, "/?scope=history", "")
	GetRecommendations(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	body := lifecycleBody(t, recorder)
	require.Equal(t, true, body["success"])
	items, ok := body["data"].([]any)
	require.True(t, ok, "history 返回数组")
	require.Len(t, items, 1)

	entry := items[0].(map[string]any)
	require.EqualValues(t, expired.ID, entry["id"])
	require.Equal(t, "expired", entry["status"])
	require.Nil(t, entry["snoozed_until"], "自动过期不带冷却到期时间")
	require.Equal(t, "示例源", entry["name"])
	require.Equal(t, "理由", entry["llm_reason"])
	require.NotEqualValues(t, pending.ID, entry["id"], "活跃未到期 pending 不进历史")
}

// TestLifecycleHandler_DismissReturnsExpiry：dismiss 升级为暂时不看——返回实际到期时刻，
// 写 candidate_preferences，推荐行保持 pending 且卡片退出默认列表。
func TestLifecycleHandler_DismissReturnsExpiry(t *testing.T) {
	db := setupLifecycleHandlerTestDB(t)
	future := time.Now().AddDate(0, 0, 7)
	rec, cand := lifecycleFixture(t, db, "pending", &future)

	ctx, recorder := newLifecycleGinContext(t, http.MethodPost, "/", fmt.Sprint(rec.ID))
	DismissRecommendation(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	body := lifecycleBody(t, recorder)
	require.Equal(t, true, body["success"])
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	require.NotNil(t, data["snoozed_until"], "返回实际到期时间供前端展示")

	var pref models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", cand.ID).First(&pref).Error)
	require.NotNil(t, pref.SnoozedUntil)
	require.WithinDuration(t, time.Now().AddDate(0, 0, 30), *pref.SnoozedUntil, 2*time.Minute)

	var after models.FeedRecommendation
	require.NoError(t, db.First(&after, rec.ID).Error)
	require.Equal(t, "pending", after.Status, "冷却权威在 candidate_preferences，推荐行状态不变")
	require.Nil(t, after.DismissedAt)

	// 默认列表隐藏该卡。
	ctx, recorder = newLifecycleGinContext(t, http.MethodGet, "/?status=pending", "")
	GetRecommendations(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, lifecycleBody(t, recorder)["data"])
}

// TestLifecycleHandler_ExcludeAndRestore：exclude 写长期排除并在历史标 excluded；
// restore 清字段且不产生订阅。
func TestLifecycleHandler_ExcludeAndRestore(t *testing.T) {
	db := setupLifecycleHandlerTestDB(t)
	future := time.Now().AddDate(0, 0, 7)
	rec, cand := lifecycleFixture(t, db, "pending", &future)

	ctx, recorder := newLifecycleGinContext(t, http.MethodPost, "/", fmt.Sprint(rec.ID))
	ExcludeRecommendation(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var pref models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", cand.ID).First(&pref).Error)
	require.NotNil(t, pref.ExcludedAt)

	ctx, recorder = newLifecycleGinContext(t, http.MethodGet, "/?scope=history", "")
	GetRecommendations(ctx)
	items := lifecycleBody(t, recorder)["data"].([]any)
	require.Len(t, items, 1)
	require.Equal(t, "excluded", items[0].(map[string]any)["status"])

	ctx, recorder = newLifecycleGinContext(t, http.MethodPost, "/", fmt.Sprint(rec.ID))
	RestoreRecommendation(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var restored models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", cand.ID).First(&restored).Error)
	require.Nil(t, restored.ExcludedAt)
	require.Nil(t, restored.SnoozedUntil)

	var feeds int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&feeds).Error)
	require.Zero(t, feeds, "恢复不是订阅：不创建 feed")
}
