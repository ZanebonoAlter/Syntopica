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
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── GET /api/semantic-boards/overview（rework-workshop-into-radar-and-desk 8.2 / tasks 9.4）──
//
// 覆盖零值语义：无活跃版块 → 空 boards；活跃版块无任何数据 → 零值行
// （days 14 档全 0、lanes 空数组、data_delayed=false），页面不因缺数据 500；
// 日报 brief 挂行（最新一份的日期/标题/计数）。5s 预算内迟到行降级
// （data_delayed=true + data_unavailable=true、计数类字段 null）由
// fix-radar-overview-false-zero 的「未取到 ≠ 0」契约覆盖（见下方
// TestGetBoardRadarOverviewUnavailableRowWireShape）。

type radarOverviewPayload struct {
	Success bool `json:"success"`
	Data    struct {
		Date   string `json:"date"`
		Boards []struct {
			BoardID     uint   `json:"board_id"`
			Label       string `json:"label"`
			TagCount    int64  `json:"tag_count"`
			ReportDate  string `json:"report_date"`
			ReportTitle string `json:"report_title"`
			// 计数类字段可空：null = 未取到（未知），0 = 取到了且确实是 0。
			ReportArticle *int     `json:"report_article_count"`
			ReportEvent   *int     `json:"report_event_count"`
			AnchorDate    string   `json:"anchor_date"`
			TodayClues    *int     `json:"today_clues"`
			Last3Clues    *int     `json:"last3_clues"`
			Prev3Clues    *int     `json:"prev3_clues"`
			Attention     *float64 `json:"attention"`
			HeatRule      string   `json:"heat_rule_version"`
			DataDelayed   bool     `json:"data_delayed"`
			// DataUnavailable = 本次没取到该版块（超时未完成 / 泳道动态计算失败）。
			DataUnavailable bool  `json:"data_unavailable"`
			ClueDays        []int `json:"clue_days"`
			Lanes           []struct {
				TopicID uint `json:"topic_id"`
			} `json:"lanes"`
		} `json:"boards"`
		// 顶层清单段：事件线条目段整体替换为「当天线索最多的泳道」清单。
		TopLanes *struct {
			AnchorDate string `json:"anchor_date"`
			Lanes      []struct {
				BoardID    uint   `json:"board_id"`
				BoardLabel string `json:"board_label"`
				TopicID    uint   `json:"topic_id"`
				TopicLabel string `json:"topic_label"`
				ClueCount  int    `json:"clue_count"`
				AnchorDate string `json:"anchor_date"`
			} `json:"lanes"`
			Partial          bool   `json:"partial"`
			DegradedBoardIDs []uint `json:"degraded_board_ids"`
			LaggingBoardIDs  []uint `json:"lagging_board_ids"`
		} `json:"top_lanes"`
	} `json:"data"`
}

func setupRadarOverviewTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	r := gin.New()
	RegisterDailyReportRoutes(r.Group("/api"))
	return r, db
}

func seedRadarBoard(t *testing.T, db *gorm.DB, label string) uint {
	t.Helper()
	board := models.SemanticLabel{Label: label, Slug: "radar-" + label, LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	return board.ID
}

func seedRadarReport(t *testing.T, db *gorm.DB, boardID uint, day, title string, articles, events int) {
	t.Helper()
	date, err := time.Parse("2006-01-02", day)
	require.NoError(t, err)
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      date,
		Title:           title,
		ArticleCount:    articles,
		EventTagCount:   events,
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
}

func getRadarOverview(t *testing.T, r *gin.Engine) radarOverviewPayload {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/semantic-boards/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var payload radarOverviewPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	return payload
}

func TestGetBoardRadarOverviewNoActiveBoardsEmptyList(t *testing.T) {
	r, _ := setupRadarOverviewTest(t)

	payload := getRadarOverview(t, r)
	require.NotNil(t, payload.Data.Boards, "boards 必须是数组（可为空），不是 null")
	assert.Empty(t, payload.Data.Boards)
	assert.NotEmpty(t, payload.Data.Date, "空版块时 date 回退今天，不为空串")
	// 泳道清单随行同响（事件线条目段已退役）：段必须在，空态是 [] 不是 null。
	require.NotNil(t, payload.Data.TopLanes, "top_lanes 段不得缺席")
	require.NotNil(t, payload.Data.TopLanes.Lanes, "lanes 必须是数组")
	assert.Empty(t, payload.Data.TopLanes.Lanes)
	assert.Equal(t, payload.Data.Date, payload.Data.TopLanes.AnchorDate)
	assert.False(t, payload.Data.TopLanes.Partial)
}

func TestGetBoardRadarOverviewBoardWithoutDataZeroRow(t *testing.T) {
	r, db := setupRadarOverviewTest(t)
	boardID := seedRadarBoard(t, db, "cold-board")

	payload := getRadarOverview(t, r)
	require.Len(t, payload.Data.Boards, 1)
	row := payload.Data.Boards[0]
	assert.Equal(t, boardID, row.BoardID)
	assert.Equal(t, "cold-board", row.Label)
	assert.Equal(t, int64(0), row.TagCount)
	assert.Empty(t, row.ReportDate, "无日报 → report_date 空串")
	assert.Empty(t, row.AnchorDate)
	require.NotNil(t, row.TodayClues)
	assert.Equal(t, 0, *row.TodayClues, "真实零值是 0（不是 null）")
	require.NotNil(t, row.Attention)
	assert.Zero(t, *row.Attention)
	assert.False(t, row.DataDelayed, "无数据版块是真实零值，不是超时降级")
	assert.False(t, row.DataUnavailable, "真实零值行不是「没取到」")
	require.Len(t, row.ClueDays, 14, "clue_days 恒为 14 档")
	for i, v := range row.ClueDays {
		assert.Zero(t, v, "clue_days[%d] 应为 0", i)
	}
	assert.Empty(t, row.Lanes)
	assert.Equal(t, repository.LaneHeatRuleVersion, row.HeatRule, "口径版本另立（heat-v2-lane-clues-1d）")
}

func TestGetBoardRadarOverviewAttachesLatestReportBrief(t *testing.T) {
	r, db := setupRadarOverviewTest(t)
	boardID := seedRadarBoard(t, db, "brief-board")
	seedRadarReport(t, db, boardID, "2026-10-28", "旧日报", 3, 5)
	seedRadarReport(t, db, boardID, "2026-10-30", "新日报标题", 12, 7)

	payload := getRadarOverview(t, r)
	require.Len(t, payload.Data.Boards, 1)
	row := payload.Data.Boards[0]
	assert.Equal(t, "2026-10-30", row.ReportDate, "取最新一份日报")
	assert.Equal(t, "新日报标题", row.ReportTitle)
	require.NotNil(t, row.ReportArticle)
	assert.Equal(t, 12, *row.ReportArticle)
	require.NotNil(t, row.ReportEvent)
	assert.Equal(t, 7, *row.ReportEvent)
	assert.Equal(t, "2026-10-30", payload.Data.Date, "顶层 date 取各版块最新日报日期")
}

// TestGetBoardRadarOverviewUnavailableRowWireShape（fix-radar-overview-false-zero）：
// 没取到的版块行在线上必须是 data_unavailable=true + 计数类字段 null（不是 0）——
// 降级不得被渲染成「0 篇文章 · 0 个事件」这类业务结论；全局 date 也不得冒充今天。
func TestGetBoardRadarOverviewUnavailableRowWireShape(t *testing.T) {
	r, db := setupRadarOverviewTest(t)
	seedRadarBoard(t, db, "unavailable-board")

	// 预算注入为 0 → 版块来不及算完即超时降级（真实慢查询无法稳定复现）。
	restore := repository.RadarOverviewBudget
	repository.RadarOverviewBudget = 0
	t.Cleanup(func() { repository.RadarOverviewBudget = restore })

	req := httptest.NewRequest(http.MethodGet, "/api/semantic-boards/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, `"data_unavailable":true`)
	assert.Contains(t, body, `"data_delayed":true`)
	for _, field := range []string{
		"attention", "today_clues", "last3_clues", "prev3_clues",
		"report_article_count", "report_event_count",
	} {
		assert.Contains(t, body, `"`+field+`":null`, "%s 未取到必须是 null", field)
	}
	assert.NotContains(t, body, `"attention":0`, "未取到不得写成 0")

	var payload radarOverviewPayload
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	require.Len(t, payload.Data.Boards, 1)
	row := payload.Data.Boards[0]
	assert.True(t, row.DataUnavailable)
	assert.Nil(t, row.Attention)
	assert.Nil(t, row.TodayClues)
	assert.Nil(t, row.ReportArticle)
	assert.Empty(t, payload.Data.Date, "全部未取到时全局 date 不得冒充今天")
}

// TestGetBoardRadarOverviewTopLanesWireShape：顶层清单段带版块名 + 泳道名 +
// 当天线索数 + 所属版块锚点日；版块无泳道数据时清单为空、partial=false。
func TestGetBoardRadarOverviewTopLanesWireShape(t *testing.T) {
	r, db := setupRadarOverviewTest(t)
	boardID := seedRadarBoard(t, db, "lanes-board")
	date := time.Now()
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      repository.NormalizeReportDate(date),
		Title:           "有泳道的日报", Status: "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "热搜泳道", Embedding: repository.FloatsToPgVector([]float64{0}),
		Status: repository.TopicStatusActive, FirstSeenDate: report.PeriodDate, LastSeenDate: report.PeriodDate,
		HitCount: 9, ConsecutiveHits: 9,
	}
	require.NoError(t, db.Create(&topic).Error)
	section := repository.DailyReportSection{ReportID: report.ID, ClusterLabel: "桶", Embedding: repository.FloatsToPgVector([]float64{0}), PersistentTopicID: &topic.ID}
	require.NoError(t, db.Create(&section).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&repository.DailyReportThread{
			ReportID: report.ID, SectionID: section.ID, Title: "线索", Scope: repository.ThreadScopeSingle,
			Embedding: repository.FloatsToPgVector([]float64{0}),
		}).Error)
	}

	payload := getRadarOverview(t, r)
	require.NotNil(t, payload.Data.TopLanes)
	require.Len(t, payload.Data.TopLanes.Lanes, 1)
	item := payload.Data.TopLanes.Lanes[0]
	assert.Equal(t, boardID, item.BoardID)
	assert.Equal(t, "lanes-board", item.BoardLabel)
	assert.Equal(t, topic.ID, item.TopicID)
	assert.Equal(t, "热搜泳道", item.TopicLabel)
	assert.Equal(t, 3, item.ClueCount)
	assert.NotEmpty(t, item.AnchorDate, "条目自带所属版块锚点日")
	assert.False(t, payload.Data.TopLanes.Partial)
}
