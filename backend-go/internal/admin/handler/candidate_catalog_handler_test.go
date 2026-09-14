package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/admin/service"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// ── 候选源库 handler 测试（tasks 3.1 验收：入库不订阅 / 失败输入不产生记录 / 错误码形状）──

func setupCandidateCatalogHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.FeedCandidate{}, &models.Feed{}, &models.RSSHubRoute{}, &models.RouteParamOption{},
		&models.CandidateAvailability{},
	))
	database.DB = db
	repository.InitRepository(db)
	return db
}

func newCandidateGinContext(t *testing.T, method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func decodeCandidateBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

// TestCandidateHandlerCreateNotSubscribed 入库 ≠ 订阅：POST 成功返回视图，feeds 表零行。
func TestCandidateHandlerCreateNotSubscribed(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)

	ctx, recorder := newCandidateGinContext(t, http.MethodPost,
		`{"name":"Example","feed_url":"https://example.com/feed","description":"desc"}`)
	CreateCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	body := decodeCandidateBody(t, recorder)
	require.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	require.Equal(t, "rss", data["kind"])
	require.Equal(t, "Example", data["name"])
	require.Equal(t, false, data["subscribed"])
	require.Equal(t, "candidate created (not subscribed)", body["message"])

	var feedCount int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&feedCount).Error)
	require.EqualValues(t, 0, feedCount)
	var candCount int64
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&candCount).Error)
	require.EqualValues(t, 1, candCount)
}

// TestCandidateHandlerCreateRecommendationDisabled 新建时即可关闭参与推荐：
// handler 直接绑 service.CandidateCreateInput，wire 名 recommendation_enabled 必须透传落库（响应 data 回 false）。
func TestCandidateHandlerCreateRecommendationDisabled(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)

	ctx, recorder := newCandidateGinContext(t, http.MethodPost,
		`{"name":"Paused","feed_url":"https://example.com/paused","recommendation_enabled":false}`)
	CreateCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	data := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.Equal(t, false, data["recommendation_enabled"])

	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored).Error)
	require.NotNil(t, stored.RecommendationEnabled)
	require.False(t, *stored.RecommendationEnabled)
}

// TestCandidateHandlerCreateInvalidInput 失败输入：400 + code=validation + 零写入。
func TestCandidateHandlerCreateInvalidInput(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)

	ctx, recorder := newCandidateGinContext(t, http.MethodPost, `{"name":"  ","feed_url":"notaurl"}`)
	CreateCandidate(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	body := decodeCandidateBody(t, recorder)
	require.Equal(t, false, body["success"])
	require.Equal(t, "validation", body["code"])

	var n int64
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&n).Error)
	require.EqualValues(t, 0, n)
}

// TestCandidateHandlerCreateDuplicateConflict 重复 URL：409 + code=conflict + existing_id。
func TestCandidateHandlerCreateDuplicateConflict(t *testing.T) {
	setupCandidateCatalogHandlerTestDB(t)

	ctx, recorder := newCandidateGinContext(t, http.MethodPost, `{"name":"A","feed_url":"https://example.com/feed"}`)
	CreateCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	first := decodeCandidateBody(t, recorder)["data"].(map[string]any)

	ctx, recorder = newCandidateGinContext(t, http.MethodPost, `{"name":"B","feed_url":"HTTPS://EXAMPLE.com:443/feed"}`)
	CreateCandidate(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
	body := decodeCandidateBody(t, recorder)
	require.Equal(t, "conflict", body["code"])
	require.EqualValues(t, first["id"], body["existing_id"])
}

// TestCandidateHandlerListShape GET 列表：{items, total} 形状 + 分页参数解析。
func TestCandidateHandlerListShape(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)
	enabled := true
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.FeedCandidate{
			StableKey: fmt.Sprintf("rss:handler-%d", i), Kind: "rss",
			FeedURL:               strPtrForHandlerTest(fmt.Sprintf("https://example.com/h%d", i)),
			CanonicalKey:          fmt.Sprintf("https://example.com/h%d", i),
			ManualMetadata:        models.MetadataMap{service.ManualFieldName: fmt.Sprintf("H%d", i)},
			RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
		}).Error)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?page=1&page_size=2&q=H&kind=rss&recommendation_enabled=true", nil)
	ListCandidates(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	body := decodeCandidateBody(t, recorder)
	data := body["data"].(map[string]any)
	require.EqualValues(t, 3, data["total"])
	items := data["items"].([]any)
	require.Len(t, items, 2) // page_size=2 生效
	item := items[0].(map[string]any)
	require.Equal(t, "rss", item["kind"])
	require.Contains(t, item, "subscribed")
}

// TestCandidateHandlerPatchToggleAndConflict PATCH 开关：成功路径 revision 自增；
// 旧 revision → 409 conflict 且库内值不变。
func TestCandidateHandlerPatchToggleAndConflict(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)
	enabled := true
	cand := models.FeedCandidate{
		StableKey: "rss:handler-patch", Kind: "rss",
		FeedURL:               strPtrForHandlerTest("https://example.com/patch"),
		CanonicalKey:          "https://example.com/patch",
		ManualMetadata:        models.MetadataMap{service.ManualFieldName: "Patch"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)

	// 成功：关闭推荐，revision 1 → 2。
	ctx, recorder := newCandidateGinContext(t, http.MethodPatch,
		fmt.Sprintf(`{"recommendation_enabled":false,"revision":%d}`, cand.Revision))
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", cand.ID)}}
	UpdateCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.Equal(t, false, data["recommendation_enabled"])
	require.EqualValues(t, 2, data["revision"])

	// 乐观锁冲突：传过期 revision=1 → 409，库内 enabled 仍为 false。
	ctx, recorder = newCandidateGinContext(t, http.MethodPatch, `{"recommendation_enabled":true,"revision":1}`)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", cand.ID)}}
	UpdateCandidate(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Equal(t, "conflict", decodeCandidateBody(t, recorder)["code"])

	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, cand.ID).Error)
	require.False(t, *stored.RecommendationEnabled)

	// 不存在的 id → 404 + code=not_found。
	ctx, recorder = newCandidateGinContext(t, http.MethodPatch, `{"name":"x"}`)
	ctx.Params = gin.Params{{Key: "id", Value: "9999"}}
	UpdateCandidate(ctx)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Equal(t, "not_found", decodeCandidateBody(t, recorder)["code"])
}

func strPtrForHandlerTest(s string) *string { return &s }

// TestCandidateHandlerPatchFeedURLWireName PATCH 的地址字段 wire 名必须是 feed_url——
// 前端曾发 url（后端无此 tag）→ 编辑地址被静默丢弃；本用例锁住 HTTP 边界契约：
// rss 改地址 200 + 新地址回显；rsshub 传 feed_url 400 validation（路由地址只读）。
func TestCandidateHandlerPatchFeedURLWireName(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)
	enabled := true
	cand := models.FeedCandidate{
		StableKey: "rss:handler-wire", Kind: "rss",
		FeedURL:               strPtrForHandlerTest("https://example.com/before.xml"),
		CanonicalKey:          "https://example.com/before.xml",
		ManualMetadata:        models.MetadataMap{service.ManualFieldName: "Wire"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)

	ctx, recorder := newCandidateGinContext(t, http.MethodPatch,
		fmt.Sprintf(`{"feed_url":"https://example.com/after.xml","revision":%d}`, cand.Revision))
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", cand.ID)}}
	UpdateCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.Equal(t, "https://example.com/after.xml", data["feed_url"])
	require.EqualValues(t, 2, data["revision"])

	// 旧前端 wire 名（url）不再被任何 handler 读取：只发 url 不会改地址。
	ctx, recorder = newCandidateGinContext(t, http.MethodPatch, `{"url":"https://example.com/ignored.xml"}`)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", cand.ID)}}
	UpdateCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var afterWireOldName models.FeedCandidate
	require.NoError(t, db.First(&afterWireOldName, cand.ID).Error)
	require.NotNil(t, afterWireOldName.FeedURL)
	require.Equal(t, "https://example.com/after.xml", *afterWireOldName.FeedURL)

	// rsshub 路由地址只读。
	route := models.RSSHubRoute{Namespace: "wire", Path: "/:id", Name: "Wire Route"}
	require.NoError(t, db.Create(&route).Error)
	rsshubCand := models.FeedCandidate{
		StableKey: "wire/:id", Kind: "rsshub", RouteID: &route.ID,
		ManualMetadata: models.MetadataMap{}, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&rsshubCand).Error)
	ctx, recorder = newCandidateGinContext(t, http.MethodPatch, `{"feed_url":"https://example.com/hijack.xml"}`)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", rsshubCand.ID)}}
	UpdateCandidate(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "validation", decodeCandidateBody(t, recorder)["code"])
}

// ── 目录导入导出 handler（improve-discovery-recommendations 3.2，design D8 / spec C3+C4）──

// TestCandidateHandlerExportShapeAndSafety GET export：默认安全导出（私有 scope 排除计数），
// 响应携带 Content-Disposition；条目无 subscribed/embedding/id 等禁区字段。
func TestCandidateHandlerExportShapeAndSafety(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)
	enabled := true
	require.NoError(t, db.Create(&models.FeedCandidate{
		StableKey: "rss:export-pub", Kind: "rss",
		FeedURL:               strPtrForHandlerTest("https://export.example.com/feed"),
		CanonicalKey:          "https://export.example.com/feed",
		ManualMetadata:        models.MetadataMap{service.ManualFieldName: "Export Pub"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}).Error)
	require.NoError(t, db.Create(&models.FeedCandidate{
		StableKey: "rss:export-priv", Kind: "rss",
		FeedURL:               strPtrForHandlerTest("https://10.0.0.9/feed"),
		CanonicalKey:          "https://10.0.0.9/feed",
		ManualMetadata:        models.MetadataMap{service.ManualFieldName: "Export Priv"},
		RecommendationEnabled: &enabled, AccessScope: "private_pending", Revision: 1,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ExportCandidates(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Disposition"), "attachment")

	data := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.EqualValues(t, 1, data["excluded_private"])
	export := data["export"].(map[string]any)
	require.EqualValues(t, service.CatalogTransferVersion, export["version"])
	entries := export["entries"].([]any)
	require.Len(t, entries, 1)
	entry := entries[0].(map[string]any)
	require.Equal(t, "Export Pub", entry["name"])
	for _, banned := range []string{"subscribed", "embedding", "id", "access_scope", "revision"} {
		require.NotContains(t, entry, banned)
	}
	require.NotContains(t, recorder.Body.String(), "10.0.0.9")

	// include_private=true → 400 validation（v1 不支持连私有导出）。
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?include_private=true", nil)
	ExportCandidates(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "validation", decodeCandidateBody(t, recorder)["code"])
}

// TestCandidateHandlerImportPreviewAndConfirm 预览→确认主链路：v99 → 400；
// 确认成功应用 1 条且 feeds 零行；伪造指纹 → 409 stale_preview；缺指纹 → 400。
func TestCandidateHandlerImportPreviewAndConfirm(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)

	file := `{"format":"` + service.CatalogTransferFormat + `","version":1,"entries":[{"kind":"rss","name":"Import Feed","feed_url":"https://import.example.com/feed"}]}`

	// 预览：200，counts.new=1，返回指纹与本地 revision。
	ctx, recorder := newCandidateGinContext(t, http.MethodPost, file)
	PreviewCatalogImport(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	preview := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	counts := preview["counts"].(map[string]any)
	require.EqualValues(t, 1, counts["new"])
	fp, _ := preview["fingerprint"].(string)
	require.Len(t, fp, 64)
	rev, _ := preview["local_revision"].(float64)

	// 确认：成功应用 1 条，逐项结果形状，feeds 表零行（导入 ≠ 订阅）。
	confirm := fmt.Sprintf(`{"fingerprint":%q,"local_revision":%d,"import":%s}`, fp, int(rev), file)
	ctx, recorder = newCandidateGinContext(t, http.MethodPost, confirm)
	ConfirmCatalogImport(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.Len(t, data["applied"].([]any), 1)
	var feeds int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&feeds).Error)
	require.EqualValues(t, 0, feeds)

	// v99 文件 → 400 validation，零写入。
	badFile := `{"format":"` + service.CatalogTransferFormat + `","version":99,"entries":[]}`
	ctx, recorder = newCandidateGinContext(t, http.MethodPost, badFile)
	PreviewCatalogImport(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "validation", decodeCandidateBody(t, recorder)["code"])

	// 伪造指纹 → 409 stale_preview。
	stale := fmt.Sprintf(`{"fingerprint":"deadbeef","local_revision":%d,"import":%s}`, int(rev), file)
	ctx, recorder = newCandidateGinContext(t, http.MethodPost, stale)
	ConfirmCatalogImport(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Equal(t, "stale_preview", decodeCandidateBody(t, recorder)["code"])

	// 缺指纹 → 400 validation（未预览不许确认）。
	noFp := fmt.Sprintf(`{"local_revision":%d,"import":%s}`, int(rev), file)
	ctx, recorder = newCandidateGinContext(t, http.MethodPost, noFp)
	ConfirmCatalogImport(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "validation", decodeCandidateBody(t, recorder)["code"])

	// 坏 JSON 体 → 400 validation。
	ctx, recorder = newCandidateGinContext(t, http.MethodPost, `{{not-json`)
	PreviewCatalogImport(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "validation", decodeCandidateBody(t, recorder)["code"])
}
