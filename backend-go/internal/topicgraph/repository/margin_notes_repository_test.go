package repository

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── 页边注三表 repository 集成测试（testcontainer PostgreSQL）──────────────
// 覆盖 test-cases.md：jsonb 数组契约（空写 [] 不写 null、保序去重、WB-7 null
// 标量拒绝）、级联删除连带问答、术语归一化（WB-5/6）hit_count 累计不重复建条、
// 管理列表筛选。schema 来自版本化迁移 20260922_0003（SetupTestDB 的生产迁移
// 链会建齐三表，无需 AutoMigrate）。

func setupMarginNotesTestDB(t *testing.T) *TopicGraphRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	return repo
}

func mustCreateAnnotation(t *testing.T, repo *TopicGraphRepository, reportID, sectionID uint, threadID *uint, quoted string) *ReportAnnotation {
	t.Helper()
	a := &ReportAnnotation{
		ReportID:          reportID,
		SectionID:         sectionID,
		ThreadID:          threadID,
		QuotedText:        quoted,
		AnchorOffsetStart: 3,
		AnchorOffsetEnd:   20,
	}
	require.NoError(t, repo.CreateAnnotation(a))
	return a
}

func mustInsertQA(t *testing.T, repo *TopicGraphRepository, annotationID uint, question, answer string, cited []uint, terms []string) *AnnotationQA {
	t.Helper()
	citedJSON := []byte("[]")
	if cited != nil {
		var err error
		citedJSON, err = json.Marshal(cited)
		require.NoError(t, err)
	}
	termsJSON := []byte("[]")
	if terms != nil {
		var err error
		termsJSON, err = json.Marshal(terms)
		require.NoError(t, err)
	}
	qa := &AnnotationQA{
		AnnotationID:    annotationID,
		Question:        question,
		Answer:          answer,
		CitedArticleIDs: JSON(citedJSON),
		ExtractedTerms:  JSON(termsJSON),
		Operation:       "daily_report.margin_note_qa",
	}
	require.NoError(t, repo.InsertAnnotationQA(qa))
	return qa
}

// ── CRUD + 级联删除 ──────────────────────────────────────────────────────

func TestAnnotation_CRUD_ListOrderAndQAPreload(t *testing.T) {
	repo := setupMarginNotesTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	sectionID := seedTestSection(t, repo.db, reportID, "利率与流动性")

	a1 := mustCreateAnnotation(t, repo, reportID, sectionID, nil, "lead 划词批注（无 thread）")
	tid := seedTestThread(t, repo.db, reportID, sectionID)
	a2 := mustCreateAnnotation(t, repo, reportID, sectionID, &tid, "thread 摘要划词批注")
	mustInsertQA(t, repo, a1.ID, "逆回购是什么", "央行工具…", []uint{3, 1}, []string{"逆回购"})
	mustInsertQA(t, repo, a2.ID, "追问到期不续做", "影响流动性…", nil, nil)

	got, err := repo.ListAnnotationsByReport(reportID)
	require.NoError(t, err)
	require.Len(t, got, 2, "creation order (≈正文出现顺序)")
	assert.Equal(t, a1.ID, got[0].ID)
	assert.Equal(t, a2.ID, got[1].ID)
	require.Len(t, got[0].QAs, 1)
	require.Len(t, got[1].QAs, 1)
	assert.Equal(t, "逆回购是什么", got[0].QAs[0].Question)
	assert.Nil(t, got[0].ThreadID, "lead 批注 thread 为 NULL（PR-4）")
	require.NotNil(t, got[1].ThreadID)

	single, err := repo.GetAnnotation(a2.ID)
	require.NoError(t, err)
	require.Len(t, single.QAs, 1)
}

func TestAnnotation_DeleteCascadesQAs(t *testing.T) {
	repo := setupMarginNotesTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	sectionID := seedTestSection(t, repo.db, reportID, "section")

	a := mustCreateAnnotation(t, repo, reportID, sectionID, nil, "待删除批注")
	mustInsertQA(t, repo, a.ID, "q1", "a1", nil, nil)
	mustInsertQA(t, repo, a.ID, "q2", "a2", nil, nil)

	require.NoError(t, repo.DeleteAnnotation(a.ID))

	// 问答连带删除（FK CASCADE + 显式删除双保险）。
	var qaCount, annCount int64
	require.NoError(t, repo.db.Model(&AnnotationQA{}).Where("annotation_id = ?", a.ID).Count(&qaCount).Error)
	require.NoError(t, repo.db.Model(&ReportAnnotation{}).Where("id = ?", a.ID).Count(&annCount).Error)
	assert.Zero(t, qaCount, "删除批注必须连带删除全部问答")
	assert.Zero(t, annCount)

	// 再删一次 → not found。
	require.ErrorIs(t, repo.DeleteAnnotation(a.ID), gorm.ErrRecordNotFound)
}

// ── jsonb 数组契约 ──────────────────────────────────────────────────────

func TestAnnotationQA_JSONBArrayContract(t *testing.T) {
	repo := setupMarginNotesTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	sectionID := seedTestSection(t, repo.db, reportID, "section")
	a := mustCreateAnnotation(t, repo, reportID, sectionID, nil, "契约验证")

	// nil cited（service 未给引用）→ 落库为 []，不得为 null。
	qaNil := &AnnotationQA{AnnotationID: a.ID, Question: "q", Answer: "ans"}
	require.NoError(t, repo.InsertAnnotationQA(qaNil))

	// 保序去重：[3,1,3,2,1] → [3,1,2]。
	dupJSON, err := json.Marshal([]uint{3, 1, 3, 2, 1})
	require.NoError(t, err)
	qaDup := &AnnotationQA{AnnotationID: a.ID, Question: "q2", Answer: "ans2", CitedArticleIDs: JSON(dupJSON)}
	require.NoError(t, repo.InsertAnnotationQA(qaDup))

	var rows []AnnotationQA
	require.NoError(t, repo.db.Where("annotation_id = ?", a.ID).Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, JSON("[]"), rows[0].CitedArticleIDs, "空引用必须写 [] 而非 null")
	assert.Equal(t, JSON("[]"), rows[0].ExtractedTerms)

	// PG jsonb 文本化会加空格（[3, 1, 2]），解析后比较保序去重结果。
	var gotIDs []uint
	require.NoError(t, json.Unmarshal(rows[1].CitedArticleIDs, &gotIDs))
	assert.Equal(t, []uint{3, 1, 2}, gotIDs, "保序去重")

	// WB-7：null 标量 → 拒绝写入。
	reject := &AnnotationQA{AnnotationID: a.ID, Question: "q", Answer: "a", CitedArticleIDs: JSON("null")}
	require.ErrorContains(t, repo.InsertAnnotationQA(reject), "array contract")
	var count int64
	require.NoError(t, repo.db.Model(&AnnotationQA{}).Where("annotation_id = ?", a.ID).Count(&count).Error)
	assert.EqualValues(t, 2, count, "被拒绝的 null 引用不得落库")

	// 非数组标量 → 拒绝。
	notArray := &AnnotationQA{AnnotationID: a.ID, Question: "q", Answer: "a", CitedArticleIDs: JSON(`5`)}
	require.ErrorContains(t, repo.InsertAnnotationQA(notArray), "not a json array")

	// extracted_terms 同契约：null 标量拒绝、空写 []。
	termsNull := &AnnotationQA{AnnotationID: a.ID, Question: "q", Answer: "a", ExtractedTerms: JSON("null")}
	require.ErrorContains(t, repo.InsertAnnotationQA(termsNull), "array contract")
	termsEmpty := &AnnotationQA{AnnotationID: a.ID, Question: "q3", Answer: "a3"}
	require.NoError(t, repo.InsertAnnotationQA(termsEmpty))
	var last AnnotationQA
	require.NoError(t, repo.db.Where("annotation_id = ?", a.ID).Order("id DESC").First(&last).Error)
	assert.Equal(t, JSON("[]"), last.ExtractedTerms, "空术语必须写 [] 而非 null")

	// ── D7 cited_web_sources 同契约（WS-5）：空写 []、保序去重、缺 url 拒绝。
	webOK := &AnnotationQA{AnnotationID: a.ID, Question: "q4", Answer: "a4", CitedWebSources: JSON(
		`[{"url":"https://b.example.com/y","title":"二"},{"url":"https://a.example.com/x","title":"一"},{"url":"https://a.example.com/x","title":"重复"}]`)}
	require.NoError(t, repo.InsertAnnotationQA(webOK))
	var webRow AnnotationQA
	require.NoError(t, repo.db.Where("annotation_id = ?", a.ID).Order("id DESC").First(&webRow).Error)
	var gotWeb []MarginNoteWebSource
	require.NoError(t, json.Unmarshal(webRow.CitedWebSources, &gotWeb))
	require.Len(t, gotWeb, 2, "按 url 去重、保序")
	assert.Equal(t, "https://b.example.com/y", gotWeb[0].URL)
	assert.Equal(t, "https://a.example.com/x", gotWeb[1].URL)

	webNil := &AnnotationQA{AnnotationID: a.ID, Question: "q5", Answer: "a5", CitedWebSources: JSON("null")}
	require.ErrorContains(t, repo.InsertAnnotationQA(webNil), "array contract")
	webNoURL := &AnnotationQA{AnnotationID: a.ID, Question: "q6", Answer: "a6", CitedWebSources: JSON(`[{"title":"无 url"}]`)}
	require.ErrorContains(t, repo.InsertAnnotationQA(webNoURL), "empty url")
	webEmpty := &AnnotationQA{AnnotationID: a.ID, Question: "q7", Answer: "a7"}
	require.NoError(t, repo.InsertAnnotationQA(webEmpty))
	var emptyWebRow AnnotationQA
	require.NoError(t, repo.db.Where("annotation_id = ?", a.ID).Order("id DESC").First(&emptyWebRow).Error)
	assert.Equal(t, JSON("[]"), emptyWebRow.CitedWebSources, "无网络来源必须写 [] 而非 null")
}

// ── 术语归一化 upsert（WB-5/6/7）─────────────────────────────────────────

func TestUpsertTermNotes_NormalizationFoldsVariants(t *testing.T) {
	repo := setupMarginNotesTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	now := time.Now()

	created, err := repo.UpsertTermNotes(&boardID, now, []string{"逆回购", "流动性对冲"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"逆回购", "流动性对冲"}, created)

	// WB-5：全角/大小写/首尾空白变体 → 命中既有条目 hit_count+1，不新建；
	// 同一次调用内同 norm 的两个变体（ＦＥＤ / " FED "）去重为一条。
	created2, err := repo.UpsertTermNotes(&boardID, now.Add(time.Hour), []string{"　逆回购　", " 流动性对冲 ", "ＦＥＤ", " FED "})
	require.NoError(t, err)
	assert.Equal(t, []string{"fed"}, created2, "只有新词（归一化后）返回为新建")

	var rows []TermNote
	require.NoError(t, repo.db.Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 3, "变体必须命中既有条目，不得新建")

	byNorm := make(map[string]TermNote, len(rows))
	for _, row := range rows {
		byNorm[row.TermNorm] = row
	}
	assert.Equal(t, 2, byNorm["逆回购"].HitCount, "同词再次出现 hit_count 累计")
	assert.Equal(t, 2, byNorm["流动性对冲"].HitCount, "首尾空白变体命中既有条目")
	assert.Equal(t, 1, byNorm["fed"].HitCount)
	assert.Equal(t, "ＦＥＤ", byNorm["fed"].TermDisplay, "display 保留首次出现的原始形态")

	assert.NotNil(t, byNorm["逆回购"].FirstSeenBoardID)
	assert.EqualValues(t, boardID, *byNorm["逆回购"].FirstSeenBoardID)
	require.NotNil(t, byNorm["逆回购"].FirstSeenDate)
	assert.Equal(t, now.Format("2006-01-02"), byNorm["逆回购"].FirstSeenDate.Format("2006-01-02"))
	assert.Nil(t, byNorm["逆回购"].Embedding, "P1 不写 embedding")
}

func TestUpsertTermNotes_EmptyAndWhitespaceFiltered(t *testing.T) {
	repo := setupMarginNotesTestDB(t)
	now := time.Now()

	created, err := repo.UpsertTermNotes(nil, now, []string{"", "  ", "　", "逆回购"})
	require.NoError(t, err)
	assert.Equal(t, []string{"逆回购"}, created, "WB-6：空串/纯空白过滤不入库")

	created, err = repo.UpsertTermNotes(nil, now, []string{"", "　"})
	require.NoError(t, err)
	assert.Empty(t, created)

	var count int64
	require.NoError(t, repo.db.Model(&TermNote{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

// ── 归属校验 ─────────────────────────────────────────────────────────────

func TestValidateAnnotationTarget(t *testing.T) {
	repo := setupMarginNotesTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	otherReportID := seedTestReport(t, repo.db, boardID, time.Now().AddDate(0, 0, 1))
	sectionID := seedTestSection(t, repo.db, reportID, "section")
	threadID := seedTestThread(t, repo.db, reportID, sectionID)

	assert.NoError(t, repo.ValidateAnnotationTarget(reportID, sectionID, nil), "lead 批注（无 thread）合法")
	assert.NoError(t, repo.ValidateAnnotationTarget(reportID, sectionID, &threadID))
	// 头条 highlight 派生（无 section 归属）：section_id=0 哨兵值合法
	assert.NoError(t, repo.ValidateAnnotationTarget(reportID, 0, nil), "无 section 归属的头条批注合法")
	assert.ErrorContains(t, repo.ValidateAnnotationTarget(reportID, 0, &threadID), "does not belong", "thread 批注必须有 section 归属")

	err := repo.ValidateAnnotationTarget(otherReportID, sectionID, nil)
	assert.ErrorContains(t, err, "does not belong")

	err = repo.ValidateAnnotationTarget(reportID, otherReportID, &threadID)
	assert.ErrorContains(t, err, "does not belong")
}

// ── 文章摘录（QA prompt 上下文）─────────────────────────────────────────

func TestGetArticleExcerpts_ArchivedExemptAndOrder(t *testing.T) {
	repo := setupMarginNotesTestDB(t)

	archived := models.Article{Title: "已归档旧文", Content: strings.Repeat("逆回购", 400), Archived: true}
	live := models.Article{Title: "当天文章", Description: "描述兜底"}
	missing := models.Article{Title: "会被跳过"}
	require.NoError(t, repo.db.Create(&archived).Error)
	require.NoError(t, repo.db.Create(&live).Error)
	require.NoError(t, repo.db.Create(&missing).Error)

	// 传入顺序决定输出顺序；archived 行必须可读（daily-report 红线 10，与
	// thread 引用同口径豁免 archived 过滤）；缺失 id 静默跳过。
	excerpts, err := repo.GetArticleExcerpts([]uint{archived.ID, 999999, live.ID}, 20)
	require.NoError(t, err)
	require.Len(t, excerpts, 2)
	assert.Equal(t, archived.ID, excerpts[0].ID)
	assert.Len(t, []rune(excerpts[0].Excerpt), 20, "摘录截断 ≤300 runes 的截断逻辑（此处用 20 验证）")
	assert.Equal(t, live.ID, excerpts[1].ID)
	assert.Equal(t, "描述兜底", excerpts[1].Excerpt, "Content 为空回落 Description")

	empty, err := repo.GetArticleExcerpts(nil, 300)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// ── 管理列表 ─────────────────────────────────────────────────────────────

func TestListAnnotationsForManagement_FilterAndPaging(t *testing.T) {
	repo := setupMarginNotesTestDB(t)
	boardA := seedTestBoard(t, repo.db)
	boardB := seedTestBoardNamed(t, repo.db, "margin-board-b")

	rA := seedTestReport(t, repo.db, boardA, time.Now())
	rB := seedTestReport(t, repo.db, boardB, time.Now())
	sA := seedTestSection(t, repo.db, rA, "A-section")
	sB := seedTestSection(t, repo.db, rB, "B-section")

	// 三条批注：A 报告两条（一老一新）、B 报告一条；A 老的带术语与提问。
	oldA := mustCreateAnnotation(t, repo, rA, sA, nil, "央行逆回购操作解读")
	time.Sleep(10 * time.Millisecond) // 保证 created_at 可分（PG 时间精度）
	newA := mustCreateAnnotation(t, repo, rA, sA, nil, "无关划词")
	rBAnnotation := mustCreateAnnotation(t, repo, rB, sB, nil, "另一版块划词")
	mustInsertQA(t, repo, oldA.ID, "什么是流动性对冲", "对冲解释", nil, []string{"流动性对冲", "逆回购"})

	rows, total, err := repo.ListAnnotationsForManagement(AnnotationListFilter{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, rows, 3)
	// 按日期倒序：rB（id=3）→ newA（id=2）→ oldA（id=1，同刻 id 倒序兑底）。
	assert.Equal(t, rBAnnotation.ID, rows[0].ID)
	assert.Equal(t, newA.ID, rows[1].ID)
	assert.Equal(t, oldA.ID, rows[2].ID, "最早的在前三行末位")
	assert.EqualValues(t, boardA, rows[2].BoardID)
	assert.Equal(t, []string{"流动性对冲", "逆回购"}, rows[2].Terms, "术语 chips 聚合自问答轮")
	require.Len(t, rows[2].QAs, 1)

	// 版块筛选。
	rowsB, totalB, err := repo.ListAnnotationsForManagement(AnnotationListFilter{BoardID: &boardB})
	require.NoError(t, err)
	assert.EqualValues(t, 1, totalB)
	assert.Equal(t, rB, rowsB[0].ReportID)
	// 关键词命中划词 / 提问 / 术语。
	for q, want := range map[string]uint{"逆回购操作": oldA.ID, "流动性对冲": oldA.ID, "无关划词": newA.ID} {
		rowsQ, totalQ, err := repo.ListAnnotationsForManagement(AnnotationListFilter{Query: q})
		require.NoError(t, err, "query %q", q)
		assert.EqualValues(t, 1, totalQ, "query %q", q)
		if assert.NotEmpty(t, rowsQ, "query %q", q) {
			assert.Equal(t, want, rowsQ[0].ID, "query %q", q)
		}
	}

	// 分页。
	rowsP, _, err := repo.ListAnnotationsForManagement(AnnotationListFilter{Page: 2, PageSize: 2})
	require.NoError(t, err)
	assert.Len(t, rowsP, 1, "3 条 × pageSize 2 → 第 2 页 1 条")
}

// seedTestBoardNamed 同 seedTestBoard 但允许自定义 slug（同测试需多个 board）。
func seedTestBoardNamed(t *testing.T, db *gorm.DB, slug string) uint {
	t.Helper()
	board := models.SemanticLabel{
		Label:     slug,
		Slug:      slug,
		LabelType: "board",
		Status:    "active",
	}
	require.NoError(t, db.Create(&board).Error)
	return board.ID
}

// seedTestThread 建 DailyReportThread（vector 列填合法值，见 testing.md 陷阱表）。
func seedTestThread(t *testing.T, db *gorm.DB, reportID, sectionID uint) uint {
	t.Helper()
	thread := DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             "央行逆回购",
		Summary:           "以利率招标方式开展 3000 亿元逆回购操作",
		RelatedArticleIDs: JSON("[101,102]"),
		Embedding:         FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, db.Create(&thread).Error)
	return thread.ID
}
