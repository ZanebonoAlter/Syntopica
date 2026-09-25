package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/searxng"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 页边注问答 service 测试（daily-report-margin-notes tasks 3.1）──────────
// LLM 经 marginNoteChatFn 注入 stub，零真实 airouter 调用；DB 用内存 SQLite
//（service 包惯例，见 lane_snapshot_test.go）。数组契约/归一化/hit_count 的
// PG 侧行为另见 repository/margin_notes_repository_test.go。

// marginNoteChatRecorder captures chat requests and replays scripted responses.
type marginNoteChatRecorder struct {
	calls   int
	reqs    []airouter.ChatRequest
	content string
	err     error
}

func (r *marginNoteChatRecorder) hook(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	r.calls++
	r.reqs = append(r.reqs, req)
	if r.err != nil {
		return nil, r.err
	}
	return &airouter.ChatResult{Content: r.content, ProviderName: "stub-provider"}, nil
}

func swapMarginNoteChat(t *testing.T, rec *marginNoteChatRecorder) {
	t.Helper()
	original := marginNoteChatFn
	marginNoteChatFn = rec.hook
	t.Cleanup(func() { marginNoteChatFn = original })
	// D7：默认同时把搜索钩子换成「禁用」stub——本包单测的 database.DB 未设，
	// 默认钩子会去打 aisettings（nil 全局）空指针；需要联网分支的用例再显式
	// swapMarginNoteSearch 覆盖。
	swapMarginNoteSearch(t, &marginNoteSearchRecorder{err: errMarginNoteSearchDisabled})
}

// marginNoteSearchRecorder captures search queries and replays scripted results.
type marginNoteSearchRecorder struct {
	queries []string
	results []searxng.Result
	err     error
}

func (r *marginNoteSearchRecorder) hook(_ context.Context, query string) ([]searxng.Result, error) {
	r.queries = append(r.queries, query)
	if r.err != nil {
		return nil, r.err
	}
	return r.results, nil
}

func swapMarginNoteSearch(t *testing.T, rec *marginNoteSearchRecorder) {
	t.Helper()
	original := marginNoteWebSearchFn
	marginNoteWebSearchFn = rec.hook
	t.Cleanup(func() { marginNoteWebSearchFn = original })
}

func marginNoteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&repository.BoardDailyReport{},
		&repository.DailyReportSection{},
		&repository.DailyReportThread{},
		&repository.ReportAnnotation{},
		&repository.AnnotationQA{},
		&repository.TermNote{},
		&models.Article{},
	))
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })
	return db
}

// seedMarginNoteReport seeds board 7 + a report dated daysAgo ago (TW-1:
// 历史旧日报提问不受重建窗口约束) and returns (reportID, sectionID).
func seedMarginNoteReport(t *testing.T, db *gorm.DB, daysAgo int) (uint, uint) {
	t.Helper()
	report := repository.BoardDailyReport{
		SemanticBoardID: 7,
		PeriodDate:      time.Now().AddDate(0, 0, -daysAgo),
		Title:           "中国宏观 第 128 期",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	section := repository.DailyReportSection{ReportID: report.ID, ClusterLabel: "利率与流动性"}
	require.NoError(t, db.Create(&section).Error)
	return report.ID, section.ID
}

func seedMarginNoteThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, relatedIDs ...uint) uint {
	t.Helper()
	idsJSON, err := json.Marshal(relatedIDs)
	require.NoError(t, err)
	thread := repository.DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             "央行逆回购",
		Summary:           "以利率招标方式开展 3000 亿元逆回购操作",
		RelatedArticleIDs: repository.JSON(idsJSON),
	}
	require.NoError(t, db.Create(&thread).Error)
	return thread.ID
}

func seedMarginNoteAnnotation(t *testing.T, db *gorm.DB, reportID, sectionID uint, threadID *uint) *repository.ReportAnnotation {
	t.Helper()
	a := &repository.ReportAnnotation{
		ReportID:   reportID,
		SectionID:  sectionID,
		ThreadID:   threadID,
		QuotedText: "以利率招标方式开展 3000 亿元逆回购操作",
	}
	require.NoError(t, repository.Repo.CreateAnnotation(a))
	return a
}

func seedMarginNoteArticles(t *testing.T, db *gorm.DB) {
	t.Helper()
	articles := []models.Article{
		{ID: 101, Title: "文章A", Content: strings.Repeat("逆回购操作放量。", 60)},
		{ID: 102, Title: "文章B", Content: "流动性保持充裕。"},
		// 103 归档：引用反查豁免 archived（daily-report 红线 10），上下文仍可读。
		{ID: 103, Title: "文章C", Content: "到期不续做影响资金面。", Archived: true},
	}
	for i := range articles {
		require.NoError(t, db.Create(&articles[i]).Error)
	}
}

// ── 全链路 ──────────────────────────────────────────────────────────────

func TestAskMarginNote_FullChain_SingleCallWhitelistAndTerms(t *testing.T) {
	db := marginNoteTestDB(t)
	seedMarginNoteArticles(t, db)
	reportID, sectionID := seedMarginNoteReport(t, db, 40) // TW-1：40 天前旧日报
	threadID := seedMarginNoteThread(t, db, reportID, sectionID, 101, 102, 103)
	annotation := seedMarginNoteAnnotation(t, db, reportID, sectionID, &threadID)

	rec := &marginNoteChatRecorder{content: `{
		"answer": "逆回购是央行向一级交易商购买债券并约定回售的短期流动性投放工具。",
		"cited_article_ids": [102, 999, 102, 103],
		"terms": ["逆回购", "流动性对冲", "中标利率", "资金面", "TMLF", "第七个会被截掉"]
	}`}
	swapMarginNoteChat(t, rec)

	res, err := AskMarginNote(context.Background(), annotation.ID, "逆回购是什么意思")
	require.NoError(t, err)

	// 单次调用 + ai-summary 红线：Operation 必填、capability open_notebook。
	require.Equal(t, 1, rec.calls, "单次 LLM 调用产出回答+引用+术语")
	req := rec.reqs[0]
	assert.Equal(t, MarginNoteQAOperation, req.Operation)
	assert.Equal(t, airouter.CapabilityOpenNotebook, req.Capability)
	assert.NotEmpty(t, req.Messages)
	require.Len(t, req.Messages, 2)

	// prompt 组装：划词 + 问题 + thread 标题/摘要 + 文章摘录（archived 豁免）。
	user := req.Messages[1].Content
	assert.Contains(t, user, annotation.QuotedText)
	assert.Contains(t, user, "逆回购是什么意思")
	assert.Contains(t, user, "央行逆回购")
	assert.Contains(t, user, "3000 亿元")
	assert.Contains(t, user, "到期不续做影响资金面", "archived 文章上下文必须可读（红线 10）")
	assert.NotContains(t, user, "[id=999]")

	// WB-3：集外剔除（999）+ 去重 + 保序；空 = 纯模型知识合法。
	assert.Equal(t, []uint{102, 103}, res.CitedArticleIDs)
	assert.False(t, res.PureModelKnowledge)
	assert.Equal(t, "stub-provider", res.Provider)

	// WB-4：terms >5 截前 5 + 归一化 upsert。
	require.Len(t, res.Terms, 5)
	assert.Equal(t, "tmlf", res.Terms[4], "归一化后大小写折叠")
	assert.Equal(t, []string{"逆回购", "流动性对冲", "中标利率", "资金面", "tmlf"}, res.NewTerms)

	// QA 轮落库 + 术语入库。
	var qa repository.AnnotationQA
	require.NoError(t, db.First(&qa, "annotation_id = ?", annotation.ID).Error)
	assert.Equal(t, res.Answer, qa.Answer)
	assert.Equal(t, MarginNoteQAOperation, qa.Operation)
	assert.Equal(t, "stub-provider", qa.Provider)
	var cited []uint
	require.NoError(t, json.Unmarshal(qa.CitedArticleIDs, &cited))
	assert.Equal(t, res.CitedArticleIDs, cited)

	var termCount int64
	require.NoError(t, db.Model(&repository.TermNote{}).Count(&termCount).Error)
	assert.EqualValues(t, 5, termCount)

	// 追问复用同端点：同词命中 hit_count+1 不新建（WB-5 service 侧）。
	rec.content = `{"answer":"二答","cited_article_ids":[],"terms":["　逆回购　"]}`
	_, err = AskMarginNote(context.Background(), annotation.ID, "追问：到期不续做会怎样")
	require.NoError(t, err)
	require.Equal(t, 2, rec.calls)
	require.NoError(t, db.Model(&repository.TermNote{}).Count(&termCount).Error)
	assert.EqualValues(t, 5, termCount, "同词归一化命中既有条目，不重复建条")
	var qas []repository.AnnotationQA
	require.NoError(t, db.Where("annotation_id = ?", annotation.ID).Order("id ASC").Find(&qas).Error)
	require.Len(t, qas, 2, "追问 = 新问答轮追加，历史轮保留")
}

// ── PR-1 / PR-2 / PR-4 ────────────────────────────────────────────────

func TestAskMarginNote_NoThreadLeadAnnotation(t *testing.T) {
	db := marginNoteTestDB(t)
	reportID, sectionID := seedMarginNoteReport(t, db, 0)
	annotation := seedMarginNoteAnnotation(t, db, reportID, sectionID, nil) // PR-4：lead 批注

	rec := &marginNoteChatRecorder{content: `{"answer":"lead 回答","cited_article_ids":[5],"terms":["M0"]}`}
	swapMarginNoteChat(t, rec)

	res, err := AskMarginNote(context.Background(), annotation.ID, "M0 是什么")
	require.NoError(t, err)
	assert.Equal(t, "lead 回答", res.Answer)
	assert.Empty(t, res.CitedArticleIDs, "无候选文章 → cited 全部剔除")
	assert.True(t, res.PureModelKnowledge, "PR-1：无文章上下文 = 纯模型知识，合法")
	assert.NotContains(t, rec.reqs[0].Messages[1].Content, "该段所在线索", "lead 无 thread 上下文")

	// qa 轮落库 cited 为 []（数组契约，null 标量不得出现）。
	var qa repository.AnnotationQA
	require.NoError(t, db.First(&qa, "annotation_id = ?", annotation.ID).Error)
	assert.Equal(t, repository.JSON("[]"), qa.CitedArticleIDs)
}

func TestAskMarginNote_AnnotationMissing_NoLLMCall(t *testing.T) {
	marginNoteTestDB(t)
	rec := &marginNoteChatRecorder{}
	swapMarginNoteChat(t, rec)

	_, err := AskMarginNote(context.Background(), 424242, "问题")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "PR-2：annotation 不存在 → 404 路径")
	assert.Zero(t, rec.calls, "PR-2：无 LLM 调用")
}

// ── 失败分支 ─────────────────────────────────────────────────────────────

func TestAskMarginNote_LLMError_QuaNotPersisted(t *testing.T) {
	db := marginNoteTestDB(t)
	reportID, sectionID := seedMarginNoteReport(t, db, 0)
	threadID := seedMarginNoteThread(t, db, reportID, sectionID)
	annotation := seedMarginNoteAnnotation(t, db, reportID, sectionID, &threadID)

	rec := &marginNoteChatRecorder{err: errors.New("open_notebook 无可用路由（provider 全部失败）")}
	swapMarginNoteChat(t, rec)

	_, err := AskMarginNote(context.Background(), annotation.ID, "问题")
	require.ErrorContains(t, err, "open_notebook", "AV-4：无路由配置报错路径与现有能力一致")
	assert.Equal(t, 1, rec.calls)

	var count int64
	require.NoError(t, db.Model(&repository.AnnotationQA{}).Count(&count).Error)
	assert.Zero(t, count, "失败轮不落库——已提交问题由前端保留（可重试）")
}

func TestAskMarginNote_AnswerParseFailure_Retryable(t *testing.T) {
	db := marginNoteTestDB(t)
	reportID, sectionID := seedMarginNoteReport(t, db, 0)
	annotation := seedMarginNoteAnnotation(t, db, reportID, sectionID, nil)

	rec := &marginNoteChatRecorder{content: "抱歉，这不是 JSON。"}
	swapMarginNoteChat(t, rec)

	_, err := AskMarginNote(context.Background(), annotation.ID, "问题")
	require.ErrorContains(t, err, "parse response")
	var count int64
	require.NoError(t, db.Model(&repository.AnnotationQA{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAskMarginNote_TermsFailureIsolated_AnswerSurvives(t *testing.T) {
	db := marginNoteTestDB(t)
	reportID, sectionID := seedMarginNoteReport(t, db, 0)
	annotation := seedMarginNoteAnnotation(t, db, reportID, sectionID, nil)

	// WB-4/D6：terms 类型异常 → Warn 隔离，回答照常返回。
	rec := &marginNoteChatRecorder{content: `{"answer":"回答没问题","cited_article_ids":[],"terms":42}`}
	swapMarginNoteChat(t, rec)
	res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
	require.NoError(t, err)
	assert.Equal(t, "回答没问题", res.Answer)
	assert.Empty(t, res.Terms)

	// D6：upsert 失败（表被删）→ Warn 隔离，回答照常返回。
	require.NoError(t, db.Migrator().DropTable("term_notes"))
	rec2 := &marginNoteChatRecorder{content: `{"answer":"第二个回答","cited_article_ids":[],"terms":["逆回购"]}`}
	swapMarginNoteChat(t, rec2)
	res, err = AskMarginNote(context.Background(), annotation.ID, "问题2")
	require.NoError(t, err, "terms upsert 失败不得阻断回答")
	assert.Equal(t, "第二个回答", res.Answer)
}

// ── 白名单/截断纯函数 ────────────────────────────────────────────────────

func TestFilterCitedAgainstCandidates(t *testing.T) {
	// 集外剔除 + 保序去重。
	assert.Equal(t, []uint{2, 5}, filterCitedAgainstCandidates([]uint{2, 9, 5, 5, 2, 7}, []uint{2, 5}))
	// 空输入合法。
	assert.Empty(t, filterCitedAgainstCandidates(nil, []uint{1}))
	assert.Empty(t, filterCitedAgainstCandidates([]uint{1}, nil), "无候选集（lead）→ 全部剔除")
}

func TestParseMarginNoteQA_AnswerStrict(t *testing.T) {
	_, err := parseMarginNoteQA(`{"cited_article_ids":[1],"terms":[]}`)
	require.ErrorContains(t, err, "empty answer", "空 answer = 解析失败（可重试）")

	parsed, err := parseMarginNoteQA(`{"answer":"好","cited_article_ids":"bad","terms":"bad"}`)
	require.NoError(t, err)
	assert.Equal(t, "好", parsed.Answer)
	assert.True(t, parsed.TermsBroken, "terms 类型异常 → TermsBroken（D6）")
	assert.Empty(t, parsed.CitedArticleIDs)
}

// ── D7 联网搜索扩充（tasks 10.3，WS-1/2/3/4/8）───────────────────────────

func TestMarginNoteSearchQuery(t *testing.T) {
	// 优先划词，超长截 80 runes（WS-8）。
	long := strings.Repeat("字", 100)
	q := MarginNoteSearchQuery(long, "问题")
	require.Len(t, []rune(q), 80)
	// 划词为空回退问题。
	assert.Equal(t, "到期不续做会怎样", MarginNoteSearchQuery("　 ", "到期不续做会怎样"))
	assert.Empty(t, MarginNoteSearchQuery("", ""))
}

func TestFilterWebSourcesAgainstCandidates(t *testing.T) {
	candidates := []searxng.Result{
		{Title: "结果一", URL: "https://a.example.com/x"},
		{Title: "结果二", URL: "https://b.example.com/y"},
	}
	// WS-3：集外剔除 + 去重 + 保序；trim 对齐。
	got := filterWebSourcesAgainstCandidates([]repository.MarginNoteWebSource{
		{Title: "编造", URL: "https://evil.example.com/z"},
		{Title: "  ", URL: " https://a.example.com/x "},
		{Title: "结果二", URL: "https://b.example.com/y"},
		{Title: "重复", URL: "https://a.example.com/x"},
	}, candidates)
	require.Len(t, got, 2)
	assert.Equal(t, "https://a.example.com/x", got[0].URL)
	assert.Equal(t, "结果一", got[0].Title, "标题缺失回退搜索结果原题")
	assert.Equal(t, "https://b.example.com/y", got[1].URL)
	// 全部集外 / 空输入 → 空数组（非 nil，数组契约）。
	assert.Empty(t, filterWebSourcesAgainstCandidates([]repository.MarginNoteWebSource{{Title: "x", URL: "https://off.example.com"}}, candidates))
	assert.NotNil(t, filterWebSourcesAgainstCandidates(nil, candidates))
}

func TestAskMarginNote_WebResults_EnrichPromptAndWhitelist(t *testing.T) {
	db := marginNoteTestDB(t)
	reportID, sectionID := seedMarginNoteReport(t, db, 3)
	threadID := seedMarginNoteThread(t, db, reportID, sectionID) // 无关联文章
	annotation := seedMarginNoteAnnotation(t, db, reportID, sectionID, &threadID)

	// 注意顺序：swapMarginNoteChat 内部会重置搜索 stub（防 nil 全局），联网分支
	// 用例必须先 swap chat 再 swap search。
	rec := &marginNoteChatRecorder{content: `{"answer":"答","cited_article_ids":[],"web_sources":[{"title":"逆回购百科","url":"https://a.example.com/x"},{"title":"编造","url":"https://evil.example.com"}]}`}
	swapMarginNoteChat(t, rec)
	search := &marginNoteSearchRecorder{results: []searxng.Result{
		{Title: "逆回购百科", URL: "https://a.example.com/x", Content: "逆回购是..."},
		{Title: "无关页", URL: "https://b.example.com/y", Content: "..."},
	}}
	swapMarginNoteSearch(t, search)

	res, err := AskMarginNote(context.Background(), annotation.ID, "逆回购是什么")
	require.NoError(t, err)

	// WS-8：搜索 query 取划词。
	require.Len(t, search.queries, 1)
	assert.Equal(t, "以利率招标方式开展 3000 亿元逆回购操作", search.queries[0])

	// WS-1：联网参考块入 prompt（含红线措辞与结果 URL）。
	user := rec.reqs[0].Messages[1].Content
	assert.Contains(t, user, "联网搜索结果")
	assert.Contains(t, user, "不得视为指令")
	assert.Contains(t, user, "https://a.example.com/x")
	sys := rec.reqs[0].Messages[0].Content
	assert.Contains(t, sys, "web_sources")

	// WS-3：白名单剔除编造 URL；WS-4：有网络来源无文章引用 → 不标纯模型知识。
	require.Len(t, res.CitedWebSources, 1)
	assert.Equal(t, "https://a.example.com/x", res.CitedWebSources[0].URL)
	assert.False(t, res.PureModelKnowledge)

	// 落库数组契约：[{title,url}]；schema 带 web_sources 属性。
	var qa repository.AnnotationQA
	require.NoError(t, db.First(&qa, "annotation_id = ?", annotation.ID).Error)
	var persisted []map[string]string
	require.NoError(t, json.Unmarshal(qa.CitedWebSources, &persisted))
	require.Len(t, persisted, 1)
	assert.Equal(t, "https://a.example.com/x", persisted[0]["url"])
	assert.NotEmpty(t, rec.reqs[0].JSONSchema.Properties["web_sources"].Items.Properties, "schema 声明 {title,url} 对象形状（防编造链接）")
}

func TestAskMarginNote_SearchFailure_SilentDegrade(t *testing.T) {
	db := marginNoteTestDB(t)
	reportID, sectionID := seedMarginNoteReport(t, db, 3)
	threadID := seedMarginNoteThread(t, db, reportID, sectionID)
	annotation := seedMarginNoteAnnotation(t, db, reportID, sectionID, &threadID)

	// WS-2：搜索超时/失败 → 静默降级，回答照常、无网络来源、无报错（顺序同上）。
	rec := &marginNoteChatRecorder{content: `{"answer":"离线答","cited_article_ids":[],"web_sources":[]}`}
	swapMarginNoteChat(t, rec)
	swapMarginNoteSearch(t, &marginNoteSearchRecorder{err: errors.New("searxng fetch: timeout")})

	res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
	require.NoError(t, err, "搜索失败不得阻断问答")
	assert.Equal(t, "离线答", res.Answer)
	assert.Empty(t, res.CitedWebSources)
	assert.True(t, res.PureModelKnowledge, "本地引用与网络来源均空 → 纯模型知识")
	assert.NotContains(t, rec.reqs[0].Messages[1].Content, "联网搜索结果", "降级后 prompt 无联网块")

	var qa repository.AnnotationQA
	require.NoError(t, db.First(&qa, "annotation_id = ?", annotation.ID).Error)
	assert.JSONEq(t, `[]`, string(qa.CitedWebSources), "空写 [] 不写 null")
}
