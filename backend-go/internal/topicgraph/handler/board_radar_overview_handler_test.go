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
// （data_delayed=true）依赖真实慢查询，无法在单测稳定复现，由真机
// ui-verification 空态/错误态路径兜底（见 review.md 8.2 验收）。

type radarOverviewPayload struct {
	Success bool `json:"success"`
	Data    struct {
		Date   string `json:"date"`
		Boards []struct {
			BoardID            uint   `json:"board_id"`
			Label              string `json:"label"`
			TagCount           int64  `json:"tag_count"`
			ReportDate         string `json:"report_date"`
			ReportTitle        string `json:"report_title"`
			ReportArticle      int    `json:"report_article_count"`
			ReportEvent        int    `json:"report_event_count"`
			StoriesBackfilled  bool   `json:"stories_backfilled"`
			StoryIdentityState string `json:"story_identity_state"`
			AnchorDate         string `json:"anchor_date"`
			TodaySections      int    `json:"today_sections"`
			Last3Sections      int    `json:"last3_sections"`
			Prev3Sections      int    `json:"prev3_sections"`
			Attention          int    `json:"attention"`
			DataDelayed        bool   `json:"data_delayed"`
			Days               []int  `json:"days"`
			Lanes              []struct {
				TopicID uint `json:"topic_id"`
			} `json:"lanes"`
		} `json:"boards"`
		Stories *struct {
			AnchorDate      string `json:"anchor_date"`
			HeatRuleVersion string `json:"heat_rule_version"`
			TopStories      []struct {
				BoardID          uint    `json:"board_id"`
				BoardLabel       string  `json:"board_label"`
				StoryID          uint    `json:"story_id"`
				StoryTitle       string  `json:"story_title"`
				Participants     int     `json:"participants"`
				Heat             float64 `json:"heat"`
				Ratio            float64 `json:"ratio"`
				HasBaseline      bool    `json:"has_baseline"`
				HasTodayEvidence bool    `json:"has_today_evidence"`
				AnchorDate       string  `json:"anchor_date"`
				ReportDate       string  `json:"report_date"`
			} `json:"top_stories"`
			// matrix-clue-fallback-and-rail-merge：warming/new_stories 清单与
			// new_count/new_total_count 已随右栏徽标化移除；warming_count 保留。
			WarmingCount           int    `json:"warming_count"`
			UnbackfilledBoardIDs   []uint `json:"unbackfilled_board_ids"`
			UnbackfilledBoardCount int    `json:"unbackfilled_board_count"`
			NoStoryBoardIDs        []uint `json:"no_story_board_ids"`
			NoStoryBoardCount      int    `json:"no_story_board_count"`
			LaggingBoardIDs        []uint `json:"lagging_board_ids"`
			Partial                bool   `json:"partial"`
			DegradedBoardIDs       []uint `json:"degraded_board_ids"`
		} `json:"stories"`
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
	// 事件线清单随行同响：段必须在（缺失时前端会回退版块级旧列表）。
	require.NotNil(t, payload.Data.Stories, "stories 段不得缺席")
	assert.NotEmpty(t, payload.Data.Stories.HeatRuleVersion)
	assert.Empty(t, payload.Data.Stories.TopStories)
	assert.Zero(t, payload.Data.Stories.WarmingCount)
	assert.False(t, payload.Data.Stories.Partial)
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
	assert.Equal(t, 0, row.TodaySections)
	assert.Equal(t, 0, row.Attention)
	assert.False(t, row.DataDelayed, "无数据版块是真实零值，不是超时降级")
	require.Len(t, row.Days, 14, "days 恒为 14 档")
	for i, v := range row.Days {
		assert.Zero(t, v, "days[%d] 应为 0", i)
	}
	assert.Empty(t, row.Lanes)
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
	assert.Equal(t, 12, row.ReportArticle)
	assert.Equal(t, 7, row.ReportEvent)
	assert.False(t, row.StoriesBackfilled, "无泳道线程：窗口内无 story_id → 未回填（字段上链）")
	assert.Equal(t, "2026-10-30", payload.Data.Date, "顶层 date 取各版块最新日报日期")
}

// TestGetBoardRadarOverviewStoryIdentityState：事件身份回填状态是**板级**三态
// （wire 契约）：有日报但无任何线程的版块 = 无可回填对象（不计入未回填数），
// 不得被说成「事件身份未回填」（对用户是假话）。
func TestGetBoardRadarOverviewStoryIdentityState(t *testing.T) {
	r, db := setupRadarOverviewTest(t)
	boardID := seedRadarBoard(t, db, "identity-board")
	seedRadarReport(t, db, boardID, "2026-10-30", "有日报无线程", 3, 5)

	payload := getRadarOverview(t, r)
	require.Len(t, payload.Data.Boards, 1)
	row := payload.Data.Boards[0]
	assert.Equal(t, "no_candidate", row.StoryIdentityState, "板级判定：板内无任何事件候选 → 无可回填对象")

	require.NotNil(t, payload.Data.Stories)
	assert.Contains(t, payload.Data.Stories.NoStoryBoardIDs, boardID, "无可回填对象单列")
	assert.Equal(t, 1, payload.Data.Stories.NoStoryBoardCount)
	assert.NotContains(t, payload.Data.Stories.UnbackfilledBoardIDs, boardID, "不得计入未回填数")
	assert.Equal(t, 0, payload.Data.Stories.UnbackfilledBoardCount)
}
