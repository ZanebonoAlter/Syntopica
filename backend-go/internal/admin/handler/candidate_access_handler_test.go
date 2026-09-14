package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
)

// ── 私网访问授权确认 handler 契约（Medium 7）──
//
// POST /api/discovery/candidates/:id/access-confirm：200（确认）/ 403（confirm=false 拒绝）
// / 409（非 private_pending）/ 400（非法 id）。

func newAccessConfirmContext(id, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: id}}
	return ctx, recorder
}

func TestConfirmCandidateAccessHandlerStatuses(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)
	pendingCandidate := models.FeedCandidate{
		StableKey: "rss:http://10.0.0.7/feed", Kind: "rss",
		ManualMetadata: models.MetadataMap{}, AccessScope: "private_pending", Revision: 1,
	}
	require.NoError(t, db.Create(&pendingCandidate).Error)
	publicCandidate := models.FeedCandidate{
		StableKey: "rss:http://public.example/feed", Kind: "rss",
		ManualMetadata: models.MetadataMap{}, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&publicCandidate).Error)

	// 200：确认转换。
	ctx, recorder := newAccessConfirmContext(fmt.Sprint(pendingCandidate.ID), `{"confirm":true}`)
	ConfirmCandidateAccess(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeInterestBody(t, recorder)
	require.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	require.Equal(t, "private_allowed", data["access_scope"])

	// 409：已 private_allowed，无可确认项。
	ctx, recorder = newAccessConfirmContext(fmt.Sprint(pendingCandidate.ID), `{"confirm":true}`)
	ConfirmCandidateAccess(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)

	// 403：confirm=false 显式拒绝。
	denied := models.FeedCandidate{
		StableKey: "rss:http://10.0.0.8/feed", Kind: "rss",
		ManualMetadata: models.MetadataMap{}, AccessScope: "private_pending", Revision: 1,
	}
	require.NoError(t, db.Create(&denied).Error)
	ctx, recorder = newAccessConfirmContext(fmt.Sprint(denied.ID), `{"confirm":false}`)
	ConfirmCandidateAccess(ctx)
	require.Equal(t, http.StatusForbidden, recorder.Code)

	// 409：public 候选无可确认项。
	ctx, recorder = newAccessConfirmContext(fmt.Sprint(publicCandidate.ID), `{}`)
	ConfirmCandidateAccess(ctx)
	require.Equal(t, http.StatusConflict, recorder.Code)

	// 404：不存在。
	ctx, recorder = newAccessConfirmContext("999999", `{}`)
	ConfirmCandidateAccess(ctx)
	require.Equal(t, http.StatusNotFound, recorder.Code)

	// 400：非法 id / 坏 body。
	ctx, recorder = newAccessConfirmContext("abc", `{}`)
	ConfirmCandidateAccess(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	ctx, recorder = newAccessConfirmContext(fmt.Sprint(denied.ID), `{not json`)
	ConfirmCandidateAccess(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
