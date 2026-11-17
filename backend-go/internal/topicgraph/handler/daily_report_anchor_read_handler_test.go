package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

// ── GET /api/daily-reports/topics/:id/anchor（泳道修正工作面的首义读侧） ──
// 契约：当前首义来源（auto / manual）+ 人工首义的节点集 + 改写次数 + 生效时机。
// 自动首义**无节点集**（不虚构节点列表）。

func getAnchor(t *testing.T, r *gin.Engine, topicID uint) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/daily-reports/topics/"+anchorIDStr(topicID)+"/anchor", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type anchorReadResponse struct {
	Success bool `json:"success"`
	Data    struct {
		TopicID       uint   `json:"topic_id"`
		Source        string `json:"source"`
		SectionIDs    []uint `json:"section_ids"`
		OverrideCount int    `json:"override_count"`
		EffectiveFrom string `json:"effective_from"`
	} `json:"data"`
}

// TestGetTopicAnchor_AutoAnchorHasNoNodeSet covers the auto case: the 首义
// was written when the topic was created — it is a single vector, so there is
// no node list to show (the workbench must not invent one).
func TestGetTopicAnchor_AutoAnchorHasNoNodeSet(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-read-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)

	w := getAnchor(t, r, topic.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp anchorReadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, topic.ID, resp.Data.TopicID)
	assert.Equal(t, repository.TopicAnchorSourceAuto, resp.Data.Source)
	assert.Empty(t, resp.Data.SectionIDs)
	assert.Zero(t, resp.Data.OverrideCount)
	assert.Empty(t, resp.Data.EffectiveFrom)

	// The node exists — it just is not part of an anchor node set.
	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
}

// TestGetTopicAnchor_ManualAnchorCarriesNodeSet covers the manual case: after
// a rewrite the read side reports the chosen node set, the rewrite count and
// the "takes effect on the next report" marker.
func TestGetTopicAnchor_ManualAnchorCarriesNodeSet(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-read-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	s1 := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)
	s2 := seedAnchorSectionRow(t, db, reportID, "s2", &topic.ID)
	s3 := seedAnchorSectionRow(t, db, reportID, "s3", &topic.ID)

	patch := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(s1.ID)+`,`+anchorIDStr(s2.ID)+`]}`)
	require.Equal(t, http.StatusOK, patch.Code, patch.Body.String())

	w := getAnchor(t, r, topic.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp anchorReadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, repository.TopicAnchorSourceManual, resp.Data.Source)
	assert.Equal(t, []uint{s1.ID, s2.ID}, resp.Data.SectionIDs)
	assert.Equal(t, 1, resp.Data.OverrideCount)
	assert.Equal(t, repository.AnchorEffectiveFromNextReport, resp.Data.EffectiveFrom)

	// A second rewrite replaces the node set (the audit trail stays append-only).
	patch2 := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(s3.ID)+`]}`)
	require.Equal(t, http.StatusOK, patch2.Code, patch2.Body.String())

	w2 := getAnchor(t, r, topic.ID)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var resp2 anchorReadResponse
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	assert.Equal(t, []uint{s3.ID}, resp2.Data.SectionIDs)
	assert.Equal(t, 2, resp2.Data.OverrideCount)

	var auditCount int64
	require.NoError(t, db.Model(&repository.TopicAnchorOverride{}).Where("topic_id = ?", topic.ID).Count(&auditCount).Error)
	assert.Equal(t, int64(2), auditCount)
}

// TestGetTopicAnchor_UnknownTopic covers the error branch: an unknown topic
// is a 404, not an empty anchor state.
func TestGetTopicAnchor_UnknownTopic(t *testing.T) {
	r, _ := setupAnchorHandlerPGTest(t)
	w := getAnchor(t, r, 987654)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
