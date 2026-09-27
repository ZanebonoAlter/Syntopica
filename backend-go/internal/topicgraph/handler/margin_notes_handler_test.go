package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 页边注 handler 测试（daily-report-margin-notes tasks 3.2）──────────────
// HTTP 契约（参数/缺省/状态码）走内存 SQLite（handler 包惯例）。QA 成功路径
// 的 LLM 契约由 service 层 stub 测试覆盖（margin_notes_qa_test.go）；本文件
// 的提问用例只覆盖校验/404（不触 LLM）与「无 AI 路由 → 502」（AV-4，经真实
// airouter 空路由路径，与现有能力一致）。

func setupMarginNoteHandlerTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:marginnotes-h-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{},
		&models.AIProvider{},
		&models.AIRoute{},
		&models.AIRouteProvider{},
		&models.AICallLog{},
		&models.Article{},
		&models.SemanticLabel{},
		&repository.BoardDailyReport{},
		&repository.DailyReportSection{},
		&repository.DailyReportThread{},
		&repository.ReportAnnotation{},
		&repository.AnnotationQA{},
		&repository.TermNote{},
	))
	// Install (never restore) the repository singleton, mirroring the other
	// handler tests. database.DB is what airouter.NewStore falls back to for
	// the AV-4 path — swap in the test DB and restore afterwards.
	repository.InitRepository(db)
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })

	engine := gin.New()
	RegisterDailyReportRoutes(engine.Group("/api"))
	return engine
}

func seedMarginNoteHandlerFixtures(t *testing.T, db *gorm.DB) (reportID, sectionID, threadID, annotationID uint) {
	t.Helper()
	board := models.SemanticLabel{Label: "h-board", Slug: "h-board-" + t.Name(), LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	report := repository.BoardDailyReport{SemanticBoardID: board.ID, PeriodDate: time.Now(), Status: "completed"}
	require.NoError(t, db.Create(&report).Error)
	section := repository.DailyReportSection{ReportID: report.ID, ClusterLabel: "sec", Embedding: repository.FloatsToPgVector([]float64{0})}
	require.NoError(t, db.Create(&section).Error)
	thread := repository.DailyReportThread{
		ReportID: report.ID, SectionID: section.ID,
		Title: "央行逆回购", Summary: "开展 3000 亿元逆回购操作",
		RelatedArticleIDs: repository.JSON("[101]"),
		Embedding:         repository.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, db.Create(&thread).Error)
	annotation := &repository.ReportAnnotation{
		ReportID: report.ID, SectionID: section.ID, ThreadID: &thread.ID,
		QuotedText: "以利率招标方式开展 3000 亿元逆回购操作",
	}
	require.NoError(t, repository.Repo.CreateAnnotation(annotation))
	return report.ID, section.ID, thread.ID, annotation.ID
}

func doJSON(engine *gin.Engine, method, path string, body interface{}) (int, map[string]interface{}) {
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var payload map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &payload)
	return w.Code, payload
}

// ── 落锚：契约 + 校验（IN-1/IN-2/PR-2/归属校验）───────────────────────────

func TestCreateReportAnnotation_ContractAndValidation(t *testing.T) {
	engine := setupMarginNoteHandlerTest(t)
	db := repository.Repo.DB()
	reportID, sectionID, threadID, _ := seedMarginNoteHandlerFixtures(t, db)

	// 正常落锚（thread 归属合法）。
	code, body := doJSON(engine, http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/annotations", reportID), gin.H{
		"section_id": sectionID, "thread_id": threadID,
		"quoted_text": "开展 3000 亿元逆回购操作", "anchor_offset_start": 2, "anchor_offset_end": 14,
	})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, body["success"])
	data := body["data"].(map[string]interface{})
	ann := data["annotation"].(map[string]interface{})
	assert.EqualValues(t, reportID, ann["report_id"])
	require.NotNil(t, ann["thread_id"])

	// IN-1：quoted_text 空串 / 纯空白 → 400。
	for _, bad := range []string{"", "   ", "　"} {
		code, _ = doJSON(engine, http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/annotations", reportID), gin.H{
			"section_id": sectionID, "quoted_text": bad,
		})
		assert.Equal(t, http.StatusBadRequest, code, "quoted %q", bad)
	}

	// IN-2：quoted_text 超长（>2000 runes）→ 400。
	code, _ = doJSON(engine, http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/annotations", reportID), gin.H{
		"section_id": sectionID, "quoted_text": strings.Repeat("长", 2001),
	})
	assert.Equal(t, http.StatusBadRequest, code)

	// PR-2：report 不存在 → 404。
	code, _ = doJSON(engine, http.MethodPost, "/api/daily-reports/999999/annotations", gin.H{
		"section_id": sectionID, "quoted_text": "合法文本",
	})
	assert.Equal(t, http.StatusNotFound, code)

	// 归属校验：section 不属于该 report → 400；thread 不属于该 section → 400。
	code, _ = doJSON(engine, http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/annotations", reportID), gin.H{
		"section_id": 999999, "quoted_text": "合法文本",
	})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = doJSON(engine, http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/annotations", reportID), gin.H{
		"section_id": sectionID, "thread_id": 888888, "quoted_text": "合法文本",
	})
	assert.Equal(t, http.StatusBadRequest, code)
}

// ── 列表 / 删除 ────────────────────────────────────────────────────────

func TestListReportAnnotations_Contract(t *testing.T) {
	engine := setupMarginNoteHandlerTest(t)
	db := repository.Repo.DB()
	reportID, _, _, annotationID := seedMarginNoteHandlerFixtures(t, db)
	require.NoError(t, repository.Repo.InsertAnnotationQA(&repository.AnnotationQA{
		AnnotationID:    annotationID,
		Question:        "逆回购是什么",
		Answer:          "央行工具",
		CitedArticleIDs: repository.JSON("[]"),
		ExtractedTerms:  repository.JSON(`["逆回购","流动性对冲"]`),
		Operation:       "daily_report.margin_note_qa",
	}))

	code, body := doJSON(engine, http.MethodGet, fmt.Sprintf("/api/daily-reports/%d/annotations", reportID), nil)
	require.Equal(t, http.StatusOK, code)
	data := body["data"].(map[string]interface{})
	list := data["annotations"].([]interface{})
	require.Len(t, list, 1)
	item := list[0].(map[string]interface{})
	qas := item["qas"].([]interface{})
	require.Len(t, qas, 1)
	assert.Equal(t, "逆回购是什么", qas[0].(map[string]interface{})["question"])
	assert.Equal(t, []interface{}{"逆回购", "流动性对冲"}, item["terms"], "术语 chips 一次拉齐")

	// 不存在的报告 → 空列表（列表语义，非 404）。
	code, body = doJSON(engine, http.MethodGet, "/api/daily-reports/999999/annotations", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Empty(t, body["data"].(map[string]interface{})["annotations"].([]interface{}))
}

func TestDeleteAnnotation_Contract(t *testing.T) {
	engine := setupMarginNoteHandlerTest(t)
	db := repository.Repo.DB()
	_, _, _, annotationID := seedMarginNoteHandlerFixtures(t, db)

	// 不存在 → 404。
	code, _ := doJSON(engine, http.MethodDelete, "/api/annotations/999999", nil)
	assert.Equal(t, http.StatusNotFound, code)

	code, body := doJSON(engine, http.MethodDelete, fmt.Sprintf("/api/annotations/%d", annotationID), nil)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, body["success"])

	var count int64
	require.NoError(t, db.Model(&repository.ReportAnnotation{}).Count(&count).Error)
	assert.Zero(t, count)
}

// ── 提问：校验 / 404 / 无路由 502 ────────────────────────────────────────

func TestAskAnnotationQuestion_ValidationAnd404(t *testing.T) {
	engine := setupMarginNoteHandlerTest(t)
	db := repository.Repo.DB()
	_, _, _, annotationID := seedMarginNoteHandlerFixtures(t, db)

	// IN-3/IN-4：空问题 / 超长问题 → 400（先于任何 LLM 调用）。
	code, _ := doJSON(engine, http.MethodPost, fmt.Sprintf("/api/annotations/%d/questions", annotationID), gin.H{"question": ""})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = doJSON(engine, http.MethodPost, fmt.Sprintf("/api/annotations/%d/questions", annotationID), gin.H{"question": strings.Repeat("问", 1001)})
	assert.Equal(t, http.StatusBadRequest, code)

	// PR-2：annotation 不存在 → 404。
	code, _ = doJSON(engine, http.MethodPost, "/api/annotations/999999/questions", gin.H{"question": "逆回购是什么"})
	assert.Equal(t, http.StatusNotFound, code)
}

func TestAskAnnotationQuestion_NoAIRouteConfigured_502(t *testing.T) {
	engine := setupMarginNoteHandlerTest(t)
	db := repository.Repo.DB()
	_, _, _, annotationID := seedMarginNoteHandlerFixtures(t, db)

	// ai_routes 空 = 无默认路由 → 经真实 airouter 失败降级链 → 502 行内可重试
	//（AV-4：报错路径与现有能力一致；service 层 stub 测试覆盖回答成功路径）。
	code, body := doJSON(engine, http.MethodPost, fmt.Sprintf("/api/annotations/%d/questions", annotationID), gin.H{"question": "逆回购是什么"})
	require.Equal(t, http.StatusBadGateway, code)
	assert.Equal(t, false, body["success"])

	// 失败轮不落库（历史问答轮不受影响，可重试）。
	var count int64
	require.NoError(t, db.Model(&repository.AnnotationQA{}).Count(&count).Error)
	assert.Zero(t, count)
}

// ── 管理页列表 ────────────────────────────────────────────────────────

func TestListAnnotationsManagement_Contract(t *testing.T) {
	engine := setupMarginNoteHandlerTest(t)
	db := repository.Repo.DB()
	reportID, sectionID, _, _ := seedMarginNoteHandlerFixtures(t, db)

	code, body := doJSON(engine, http.MethodGet, "/api/annotations?page=1&page_size=20", nil)
	require.Equal(t, http.StatusOK, code)
	data := body["data"].(map[string]interface{})
	assert.EqualValues(t, 1, data["total"])
	list := data["annotations"].([]interface{})
	require.Len(t, list, 1)
	row := list[0].(map[string]interface{})
	assert.EqualValues(t, reportID, row["report_id"])
	assert.EqualValues(t, sectionID, row["section_id"])
	assert.Contains(t, row, "terms")
	assert.Contains(t, row, "qas")
	assert.Contains(t, row, "period_date")
	// 管理页行须带版块展示字段：「跳原日报」深链要 board_id，列表要 board_label
	// （缺了会落「未知版块」且跳转丢 board 参数被 TagsPage 丢弃）。
	assert.NotZero(t, row["board_id"], "board_id 供跳原日报深链")
	assert.Equal(t, "h-board", row["board_label"])

	// board_id / q 筛选参数可解析（不匹配 → 空页而非错误）。
	code, body = doJSON(engine, http.MethodGet, "/api/annotations?board_id=777777", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Zero(t, body["data"].(map[string]interface{})["total"])
	code, body = doJSON(engine, http.MethodGet, "/api/annotations?q=逆回购", nil)
	require.Equal(t, http.StatusOK, code)
	assert.EqualValues(t, 1, body["data"].(map[string]interface{})["total"], "关键词命中划词/提问/术语")

	// 非法 board_id → 400。
	code, _ = doJSON(engine, http.MethodGet, "/api/annotations?board_id=abc", nil)
	assert.Equal(t, http.StatusBadRequest, code)
}
