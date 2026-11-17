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

// ── 人工修正留痕的只读查询端点（review R-2） ────────────────────────────
//
//	GET /api/daily-reports/topics/:id/anchor/history
//	GET /api/daily-reports/sections/:id/assignment/history
//
// 契约：append-only 留痕按时间升序返回；纯读端点不写任何列。

func getAnchorHistory(t *testing.T, r *gin.Engine, topicID uint) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/daily-reports/topics/"+anchorIDStr(topicID)+"/anchor/history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getAssignmentHistory(t *testing.T, r *gin.Engine, sectionID uint) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/daily-reports/sections/"+anchorIDStr(sectionID)+"/assignment/history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type anchorHistoryResponse struct {
	Success bool `json:"success"`
	Data    struct {
		TopicID   uint `json:"topic_id"`
		Overrides []struct {
			ID         uint   `json:"id"`
			TopicID    uint   `json:"topic_id"`
			Source     string `json:"source"`
			SectionIDs []uint `json:"section_ids"`
			OldVector  *struct {
				Dimension int       `json:"dimension"`
				Head      []float64 `json:"head"`
				Norm      float64   `json:"norm"`
			} `json:"old_vector"`
			NewVector *struct {
				Dimension int       `json:"dimension"`
				Head      []float64 `json:"head"`
				Norm      float64   `json:"norm"`
			} `json:"new_vector"`
			Operator  string    `json:"operator"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"overrides"`
	} `json:"data"`
}

type assignmentHistoryResponse struct {
	Success bool `json:"success"`
	Data    struct {
		SectionID uint `json:"section_id"`
		Entries   []struct {
			ID          uint      `json:"id"`
			SectionID   uint      `json:"section_id"`
			TopicIDFrom *uint     `json:"topic_id_from"`
			TopicIDTo   *uint     `json:"topic_id_to"`
			Action      string    `json:"action"`
			Operator    string    `json:"operator"`
			CreatedAt   time.Time `json:"created_at"`
		} `json:"entries"`
	} `json:"data"`
}

// TestGetTopicAnchorHistory_ReturnsRewritesOldestFirst covers spec「首义改写
// 历史可查」: 改写 2 次 → 历史返回 2 条、按时间升序，带改前/改后向量摘要、
// 选定节点 id 与操作者。
func TestGetTopicAnchorHistory_ReturnsRewritesOldestFirst(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-history-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	s1 := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)
	s2 := seedAnchorSectionRow(t, db, reportID, "s2", &topic.ID)

	patch := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(s1.ID)+`]}`)
	require.Equal(t, http.StatusOK, patch.Code, patch.Body.String())
	patch2 := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(s1.ID)+`,`+anchorIDStr(s2.ID)+`]}`)
	require.Equal(t, http.StatusOK, patch2.Code, patch2.Body.String())

	w := getAnchorHistory(t, r, topic.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp anchorHistoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, topic.ID, resp.Data.TopicID)
	require.Len(t, resp.Data.Overrides, 2, "两次改写 → 两条留痕")

	first := resp.Data.Overrides[0]
	second := resp.Data.Overrides[1]
	assert.Equal(t, []uint{s1.ID}, first.SectionIDs)
	assert.Equal(t, []uint{s1.ID, s2.ID}, second.SectionIDs)
	assert.Equal(t, repository.TopicAnchorSourceSelectedSections, first.Source)
	assert.Equal(t, repository.AnchorOperatorUser, first.Operator)
	require.NotNil(t, first.NewVector)
	assert.Equal(t, 3, first.NewVector.Dimension)
	assert.False(t, second.CreatedAt.Before(first.CreatedAt), "留痕按时间升序返回")
}

// TestGetTopicAnchorHistory_UnknownTopic covers the error branch.
func TestGetTopicAnchorHistory_UnknownTopic(t *testing.T) {
	r, _ := setupAnchorHandlerPGTest(t)
	w := getAnchorHistory(t, r, 987654)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestGetSectionAssignmentHistory_ReturnsDetachThenAttachOldestFirst covers
// spec「移除留痕可查」: 「移除 → 加回」两步按时间升序返回，detach 在前、
// attach 在后，且带变更前话题 id 与动作。
func TestGetSectionAssignmentHistory_ReturnsDetachThenAttachOldestFirst(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "assignment-history-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)

	detach := patchAssignment(t, r, sec.ID, `{"topic_id":null}`)
	require.Equal(t, http.StatusOK, detach.Code, detach.Body.String())
	attach := patchAssignment(t, r, sec.ID, `{"topic_id":`+anchorIDStr(topic.ID)+`}`)
	require.Equal(t, http.StatusOK, attach.Code, attach.Body.String())

	w := getAssignmentHistory(t, r, sec.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp assignmentHistoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, sec.ID, resp.Data.SectionID)
	require.Len(t, resp.Data.Entries, 2, "移除 → 加回各留一行")

	assert.Equal(t, repository.ManualAssignmentActionDetach, resp.Data.Entries[0].Action)
	require.NotNil(t, resp.Data.Entries[0].TopicIDFrom)
	assert.Equal(t, topic.ID, *resp.Data.Entries[0].TopicIDFrom)
	assert.Nil(t, resp.Data.Entries[0].TopicIDTo)

	assert.Equal(t, repository.ManualAssignmentActionAttach, resp.Data.Entries[1].Action)
	assert.Nil(t, resp.Data.Entries[1].TopicIDFrom)
	require.NotNil(t, resp.Data.Entries[1].TopicIDTo)
	assert.Equal(t, topic.ID, *resp.Data.Entries[1].TopicIDTo)
	assert.Equal(t, repository.AnchorOperatorUser, resp.Data.Entries[0].Operator)
	assert.False(t, resp.Data.Entries[1].CreatedAt.Before(resp.Data.Entries[0].CreatedAt),
		"留痕按时间升序返回")
}

// TestGetSectionAssignmentHistory_UnknownSection covers the error branch.
func TestGetSectionAssignmentHistory_UnknownSection(t *testing.T) {
	r, _ := setupAnchorHandlerPGTest(t)
	w := getAssignmentHistory(t, r, 987654)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
