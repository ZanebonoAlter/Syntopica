package handler

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

// ── GET /api/daily-reports/topics/:id/anchor 的扩展响应（task 1.4） ──────────
// 契约：响应**只增不改**——既有 5 个字段原样保留，新增 anchor_origin /
// centroid 两段；来源不可考时节点字段为 null（不虚构）。

type anchorReadExtResponse struct {
	Success bool `json:"success"`
	Data    struct {
		TopicID       uint   `json:"topic_id"`
		Source        string `json:"source"`
		SectionIDs    []uint `json:"section_ids"`
		OverrideCount int    `json:"override_count"`
		EffectiveFrom string `json:"effective_from"`
		AnchorOrigin  struct {
			Resolved   bool    `json:"resolved"`
			SectionID  *uint   `json:"section_id"`
			Title      *string `json:"title"`
			PeriodDate *string `json:"period_date"`
		} `json:"anchor_origin"`
		Centroid struct {
			WindowSize   int     `json:"window_size"`
			AnchorWeight float64 `json:"anchor_weight"`
			SectionIDs   []uint  `json:"section_ids"`
			Members      []struct {
				SectionID  uint   `json:"section_id"`
				Title      string `json:"title"`
				PeriodDate string `json:"period_date"`
			} `json:"members"`
			Degraded bool `json:"degraded"`
		} `json:"centroid"`
	} `json:"data"`
}

// TestGetTopicAnchorExt_ResponseCarriesLegacyAndNewFields covers the
// backward-compatible extension: the five legacy fields stay untouched and
// the two new blocks arrive with the rolling-window detail.
func TestGetTopicAnchorExt_ResponseCarriesLegacyAndNewFields(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-read-ext-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	s1 := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)
	s2 := seedAnchorSectionRow(t, db, reportID, "s2", &topic.ID)

	w := getAnchor(t, r, topic.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The legacy field names must still be there (响应只增不改).
	body := w.Body.String()
	for _, field := range []string{"topic_id", "source", "section_ids", "override_count", "effective_from"} {
		assert.Contains(t, body, `"`+field+`"`, "the legacy field %s must stay in the response", field)
	}

	var resp anchorReadExtResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, topic.ID, resp.Data.TopicID)
	assert.Equal(t, repository.TopicAnchorSourceAuto, resp.Data.Source)
	assert.Empty(t, resp.Data.SectionIDs)
	assert.Zero(t, resp.Data.OverrideCount)
	assert.Empty(t, resp.Data.EffectiveFrom)

	// The rolling-window block: window size + α + the participating nodes.
	assert.Positive(t, resp.Data.Centroid.WindowSize)
	assert.Positive(t, resp.Data.Centroid.AnchorWeight)
	assert.False(t, resp.Data.Centroid.Degraded)
	assert.ElementsMatch(t, []uint{s1.ID, s2.ID}, resp.Data.Centroid.SectionIDs)
	require.Len(t, resp.Data.Centroid.Members, 2)
	for i, id := range resp.Data.Centroid.SectionIDs {
		assert.Equal(t, id, resp.Data.Centroid.Members[i].SectionID)
		assert.NotEmpty(t, resp.Data.Centroid.Members[i].Title)
		assert.NotEmpty(t, resp.Data.Centroid.Members[i].PeriodDate)
	}
}

// TestGetTopicAnchorExt_AutoAnchorReportsUnresolvableOrigin covers spec
// 「自动首义不可考时显式不可考」: resolved=false and the node fields are JSON
// null — the endpoint never substitutes a guess.
func TestGetTopicAnchorExt_AutoAnchorReportsUnresolvableOrigin(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-read-ext-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)

	w := getAnchor(t, r, topic.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	data, ok := raw["data"].(map[string]any)
	require.True(t, ok, "the response must carry a data object")
	origin, ok := data["anchor_origin"].(map[string]any)
	require.True(t, ok, "the response must carry an anchor_origin object")
	assert.Equal(t, false, origin["resolved"])
	assert.Nil(t, origin["section_id"], "an unresolvable source node must be null")
	assert.Nil(t, origin["title"], "an unresolvable source node must be null")
	assert.Nil(t, origin["period_date"], "an unresolvable source node must be null")
}

// TestGetTopicAnchorExt_ManualAnchorResolvesOriginNode covers the manual
// case: after a rewrite the source node is knowable and reported with title
// and report period.
func TestGetTopicAnchorExt_ManualAnchorResolvesOriginNode(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-read-ext-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	s1 := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)

	patch := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(s1.ID)+`]}`)
	require.Equal(t, http.StatusOK, patch.Code, patch.Body.String())

	w := getAnchor(t, r, topic.ID)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp anchorReadExtResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, repository.TopicAnchorSourceManual, resp.Data.Source)
	assert.Equal(t, []uint{s1.ID}, resp.Data.SectionIDs)
	assert.Equal(t, 1, resp.Data.OverrideCount)

	assert.True(t, resp.Data.AnchorOrigin.Resolved)
	require.NotNil(t, resp.Data.AnchorOrigin.SectionID)
	assert.Equal(t, s1.ID, *resp.Data.AnchorOrigin.SectionID)
	require.NotNil(t, resp.Data.AnchorOrigin.Title)
	assert.Equal(t, "s1", *resp.Data.AnchorOrigin.Title)
	require.NotNil(t, resp.Data.AnchorOrigin.PeriodDate)
	assert.NotEmpty(t, *resp.Data.AnchorOrigin.PeriodDate)
}
