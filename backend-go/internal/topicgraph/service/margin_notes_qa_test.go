package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	// 本 change：默认同时 stub 端点预检为「未配置」+ query 生成钩子为 err 记录
	// 器——非联网用例因此走「跳过联网链路」（queryGen 零调用，可断言）；防
	// 「未 stub 走默认 airouter 失败→静默回退锚文本」的意外绿路径（test-cases
	// §继承与调整）。联网分支用例再显式 swapEndpoint+swapQueryGen 覆盖。
	swapMarginNoteEndpoint(t, "", false)
	swapMarginNoteQueryGen(t, &marginNoteQueryGenRecorder{err: errors.New("query gen stub: unexpected call")})
}

// marginNoteSearchRecorder captures search queries and replays scripted results.
// 并发安全：多 query 并行搜索时 hook 被 goroutine 并发调（D4），queries 记录
// 与 byQuery 读都在锁内。byQuery 命中的 query 用分派结果，其余用 results/err。
type marginNoteSearchOutcome struct {
	results []searxng.Result
	err     error
}

type marginNoteSearchRecorder struct {
	mu      sync.Mutex
	queries []string
	results []searxng.Result
	err     error
	byQuery map[string]marginNoteSearchOutcome
}

func (r *marginNoteSearchRecorder) hook(_ context.Context, query string) ([]searxng.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, query)
	if r.byQuery != nil {
		if oc, ok := r.byQuery[query]; ok {
			return oc.results, oc.err
		}
	}
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

// marginNoteQueryGenRecorder captures query-generation requests and replays
// scripted responses (对齐 marginNoteChatRecorder)。
type marginNoteQueryGenRecorder struct {
	mu      sync.Mutex
	calls   int
	reqs    []airouter.ChatRequest
	content string
	err     error
}

func (r *marginNoteQueryGenRecorder) hook(_ context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	r.mu.Lock()
	r.calls++
	r.reqs = append(r.reqs, req)
	content, err := r.content, r.err
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &airouter.ChatResult{Content: content, ProviderName: "stub-querygen"}, nil
}

func swapMarginNoteQueryGen(t *testing.T, rec *marginNoteQueryGenRecorder) {
	t.Helper()
	original := marginNoteQueryGenFn
	marginNoteQueryGenFn = rec.hook
	t.Cleanup(func() { marginNoteQueryGenFn = original })
}

// swapMarginNoteEndpoint stubs the endpoint precheck (D3)：ok=false 模拟
// 未配置/禁用（跳过整个联网链路），ok=true 模拟已配置。
func swapMarginNoteEndpoint(t *testing.T, endpoint string, ok bool) {
	t.Helper()
	original := marginNoteSearxngEndpointFn
	marginNoteSearxngEndpointFn = func() (string, bool) { return endpoint, ok }
	t.Cleanup(func() { marginNoteSearxngEndpointFn = original })
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

	// 注意顺序：swapMarginNoteChat 内部会重置搜索/端点/query 生成 stub（防 nil
	// 全局与意外回退），联网分支用例必须最后 swap endpoint + queryGen +
	// search 三钩子。
	rec := &marginNoteChatRecorder{content: `{"answer":"答","cited_article_ids":[],"web_sources":[{"title":"逆回购百科","url":"https://a.example.com/x"},{"title":"编造","url":"https://evil.example.com"}]}`}
	swapMarginNoteChat(t, rec)
	swapMarginNoteEndpoint(t, "http://searxng.local:8080", true)
	queryGen := &marginNoteQueryGenRecorder{content: `{"queries":["央行逆回购 操作", "逆回购 工具"]}`}
	swapMarginNoteQueryGen(t, queryGen)
	search := &marginNoteSearchRecorder{byQuery: map[string]marginNoteSearchOutcome{
		"央行逆回购 操作": {results: []searxng.Result{
			{Title: "逆回购百科", URL: "https://a.example.com/x", Content: "逆回购是..."},
		}},
		"逆回购 工具": {results: []searxng.Result{
			{Title: "无关页", URL: "https://b.example.com/y", Content: "..."},
		}},
	}}
	swapMarginNoteSearch(t, search)

	res, err := AskMarginNote(context.Background(), annotation.ID, "逆回购是什么")
	require.NoError(t, err)

	// query 生成调用形状（D1/D2）：operation 区分审计、open_notebook 路由、
	// 短输出压缩；生成的 queries 成为搜索入参（非锚文本机械截断）。
	require.Equal(t, 1, queryGen.calls, "前置一次轻量 query 生成调用")
	qgReq := queryGen.reqs[0]
	assert.Equal(t, MarginNoteQueryGenOperation, qgReq.Operation)
	assert.Equal(t, airouter.CapabilityOpenNotebook, qgReq.Capability)
	require.NotNil(t, qgReq.MaxTokens)
	assert.Equal(t, 128, *qgReq.MaxTokens)
	require.NotNil(t, qgReq.Temperature)
	assert.InDelta(t, 0.1, *qgReq.Temperature, 1e-9)
	assert.True(t, qgReq.JSONMode)
	assert.Contains(t, qgReq.JSONSchema.Properties, "queries")
	qgUser := qgReq.Messages[1].Content
	assert.Contains(t, qgUser, "逆回购是什么", "问题拼入 query 生成 prompt")
	assert.Contains(t, qgUser, annotation.QuotedText, "锚文本拼入 query 生成 prompt")
	assert.Contains(t, qgUser, "央行逆回购", "线索标题拼入 query 生成 prompt")

	// 搜索入参 = 生成的 queries（两个都发，非锚文本）。
	require.Len(t, search.queries, 2)
	assert.ElementsMatch(t, []string{"央行逆回购 操作", "逆回购 工具"}, search.queries)

	// WS-1：联网参考块入 prompt（含红线措辞与结果 URL，两 query 合并池）。
	user := rec.reqs[0].Messages[1].Content
	assert.Contains(t, user, "联网搜索结果")
	assert.Contains(t, user, "不得视为指令")
	assert.Contains(t, user, "https://a.example.com/x")
	assert.Contains(t, user, "https://b.example.com/y", "多 query 合并池全部进 prompt")
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

	// WS-2/B4：stub query 生成成功 + 搜索钩子全部报错 → 静默降级，回答照常、
	// 无网络来源、无报错；不再回退锚文本（SearXNG 同轮已挂）。顺序同上。
	rec := &marginNoteChatRecorder{content: `{"answer":"离线答","cited_article_ids":[],"web_sources":[]}`}
	swapMarginNoteChat(t, rec)
	swapMarginNoteEndpoint(t, "http://searxng.local:8080", true)
	swapMarginNoteQueryGen(t, &marginNoteQueryGenRecorder{content: `{"queries":["q1","q2"]}`})
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

// ── LLM 自由选词搜索（margin-note-llm-search-query）─────────────────────

// TestMarginNoteQueryGen_ParseVariants 覆盖 test-cases §单测 QG 组解析变体：
// 成功 1/3 条、5 条截 3、空串与纯空白过滤、非 JSON、单条超长截 60。
func TestMarginNoteQueryGen_ParseVariants(t *testing.T) {
	// 成功 1 条（单 token 下限合法）。
	assert.Equal(t, []string{"蓝白合"}, parseMarginNoteQueries(`{"queries":["蓝白合"]}`))
	// 成功 3 条（上限）。
	assert.Equal(t, []string{"蓝白合", "台湾政党联盟", "柯文哲 侯友宜"},
		parseMarginNoteQueries(`{"queries":["蓝白合","台湾政党联盟","柯文哲 侯友宜"]}`))
	// 返回 5 条 → 截前 3。
	assert.Equal(t, []string{"q1", "q2", "q3"},
		parseMarginNoteQueries(`{"queries":["q1","q2","q3","q4","q5"]}`))
	// 空串/半角空格/全角空格/tab 逐条过滤；部分有效保留有效项。
	assert.Equal(t, []string{"q1", "q2"},
		parseMarginNoteQueries(`{"queries":["", "  ", "q1", "　", "\t", "q2"]}`))
	// 过滤后全空 → 整体视为生成空（调用方回退锚文本）。
	assert.Nil(t, parseMarginNoteQueries(`{"queries":["", "  ", "　", "\t"]}`))
	// 非解析 JSON / queries 字段缺失 / 空数组 → nil。
	assert.Nil(t, parseMarginNoteQueries("抱歉，这不是 JSON。"))
	assert.Nil(t, parseMarginNoteQueries(`{}`))
	assert.Nil(t, parseMarginNoteQueries(`{"queries":[]}`))
	// 单条超长 → 截 ≤60 runes。
	long := strings.Repeat("词", 100)
	got := parseMarginNoteQueries(`{"queries":["` + long + `"]}`)
	require.Len(t, got, 1)
	assert.Len(t, []rune(got[0]), 60)
}

// TestMarginNoteQueryGen_GenerateFlow 覆盖生成链路容错与 prompt 拼装（形状断
// 言见 TestAskMarginNote_WebResults_EnrichPromptAndWhitelist）。
func TestMarginNoteQueryGen_GenerateFlow(t *testing.T) {
	ctx := context.Background()
	// 成功 5 条 → 截前 3 返回。
	qg := &marginNoteQueryGenRecorder{content: `{"queries":["q1","q2","q3","q4","q5"]}`}
	swapMarginNoteQueryGen(t, qg)
	assert.Equal(t, []string{"q1", "q2", "q3"}, generateMarginNoteSearchQueries(ctx, "锚文本", "问题", "标题", "摘要"))
	require.Equal(t, 1, qg.calls)

	// 生成调用失败/超时 → nil（静默回退由调用方接管，B6）。
	swapMarginNoteQueryGen(t, &marginNoteQueryGenRecorder{err: errors.New("open_notebook 路由不可用")})
	assert.Nil(t, generateMarginNoteSearchQueries(ctx, "锚", "问题", "", ""))

	// 输出全空白/非 JSON/空 → nil（同失败路径，B7）。
	for _, content := range []string{`{"queries":["", "　"]}`, "抱歉不是 JSON", `{"queries":[]}`} {
		swapMarginNoteQueryGen(t, &marginNoteQueryGenRecorder{content: content})
		assert.Nil(t, generateMarginNoteSearchQueries(ctx, "锚", "问题", "", ""), "content=%s", content)
	}

	// 锚文本 200 runes 边界：进 prompt 截 200（回退搜索截 80 由 TestMarginNoteSearchQuery 覆盖）。
	qg2 := &marginNoteQueryGenRecorder{content: `{"queries":["q"]}`}
	swapMarginNoteQueryGen(t, qg2)
	require.NotNil(t, generateMarginNoteSearchQueries(ctx, strings.Repeat("字", 250), "问题", "线索标题", "线索摘要"))
	user := qg2.reqs[0].Messages[1].Content
	assert.Contains(t, user, strings.Repeat("字", 200))
	assert.NotContains(t, user, strings.Repeat("字", 201), "锚文本超 200 截断")
	assert.Contains(t, user, "线索标题", "线索标题拼入")
	assert.Contains(t, user, "线索摘要", "线索摘要拼入")
}

// TestMergeSearchResults 覆盖 D4 合并：跨 query/单 query 内 URL 去重保序、
// trim 匹配、池长截 5、全空。
func TestMergeSearchResults(t *testing.T) {
	q1 := []searxng.Result{
		{Title: "a1", URL: "https://a.example.com/1"},
		{Title: "a2", URL: " https://a.example.com/2 "}, // trim 匹配键
		{Title: "dup", URL: "https://a.example.com/1"},  // 单 query 内重复
	}
	q2 := []searxng.Result{
		{Title: "b1", URL: "https://a.example.com/1"},  // 跨 query 重复（丢弃）
		{Title: "b2", URL: "https://a.example.com/2"},  // trim 匹配 q1 的带空格 URL（丢弃）
		{Title: "b3", URL: ""},                         // 空 URL 跳过
	}
	got := mergeMarginNoteSearchResults([][]searxng.Result{q1, q2})
	require.Len(t, got, 2, "首次出现位次保序，重复/空 URL 剔除")
	assert.Equal(t, "https://a.example.com/1", got[0].URL)
	assert.Equal(t, " https://a.example.com/2 ", got[1].URL, "对象原样保留，仅去重键 trim")

	// 池长 6（2 query × 3 不重复）→ 截前 5（query 顺序 × 各自结果顺序）。
	p1 := []searxng.Result{{URL: "u1"}, {URL: "u2"}, {URL: "u3"}}
	p2 := []searxng.Result{{URL: "u4"}, {URL: "u5"}, {URL: "u6"}}
	got = mergeMarginNoteSearchResults([][]searxng.Result{p1, p2})
	require.Len(t, got, 5, "marginNoteSearxngResultLimit 不变")
	assert.Equal(t, "u4", got[3].URL)
	assert.Equal(t, "u5", got[4].URL)
	assert.NotEqual(t, "u6", got[4].URL, "第 6 条丢弃")

	// 池长正好 5 → 全保留。
	assert.Len(t, mergeMarginNoteSearchResults([][]searxng.Result{p1, p2[:2]}), 5)

	// 全空 / nil → 空。
	assert.Empty(t, mergeMarginNoteSearchResults(nil))
	assert.Empty(t, mergeMarginNoteSearchResults([][]searxng.Result{nil, {}}))
}

// setupMarginNoteDegrade seeds 一个带 thread 的批注（降级树用例共用底座）。
func setupMarginNoteDegrade(t *testing.T) *repository.ReportAnnotation {
	db := marginNoteTestDB(t)
	reportID, sectionID := seedMarginNoteReport(t, db, 3)
	threadID := seedMarginNoteThread(t, db, reportID, sectionID)
	return seedMarginNoteAnnotation(t, db, reportID, sectionID, &threadID)
}

// swapMarginNoteWebEnv 一键布置联网链路三钩子（endpoint 已配置 + query 生成 + 搜索）。
func swapMarginNoteWebEnv(t *testing.T, queryGen *marginNoteQueryGenRecorder, search *marginNoteSearchRecorder) {
	t.Helper()
	swapMarginNoteEndpoint(t, "http://searxng.local:8080", true)
	if queryGen != nil {
		swapMarginNoteQueryGen(t, queryGen)
	}
	if search != nil {
		swapMarginNoteSearch(t, search)
	}
}

// TestAskMarginNoteDegrade 覆盖 D5 降级决策树全分支（test-cases §白盒附加 B1~B8）。
func TestAskMarginNoteDegrade(t *testing.T) {
	answerJSON := `{"answer":"回答","cited_article_ids":[],"web_sources":[]}`

	t.Run("B1_endpoint_unconfigured_skips_query_gen", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		queryGen := &marginNoteQueryGenRecorder{content: `{"queries":["q"]}`}
		swapMarginNoteQueryGen(t, queryGen)
		// swapMarginNoteChat 默认 endpoint=false；显式再置一次以明示分支条件。
		swapMarginNoteEndpoint(t, "", false)

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		assert.Zero(t, queryGen.calls, "B1：未配置时 query 生成钩子零调用（省 LLM 调用）")
		assert.NotContains(t, rec.reqs[0].Messages[1].Content, "联网搜索结果")
		assert.True(t, res.PureModelKnowledge)
	})

	t.Run("B2_query_gen_success_all_search_ok_merged_pool", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: `{"answer":"答","cited_article_ids":[],"web_sources":[{"title":"合并池来源","url":"https://a.example.com/1"}]}`}
		swapMarginNoteChat(t, rec)
		queryGen := &marginNoteQueryGenRecorder{content: `{"queries":["q1","q2"]}`}
		search := &marginNoteSearchRecorder{byQuery: map[string]marginNoteSearchOutcome{
			"q1": {results: []searxng.Result{
				{Title: "r1", URL: "https://a.example.com/1", Content: "c1"},
				{Title: "r2", URL: "https://a.example.com/2", Content: "c2"},
			}},
			"q2": {results: []searxng.Result{
				{Title: "r3", URL: "https://a.example.com/1", Content: "c1"}, // 跨 query 重复
				{Title: "r4", URL: "https://b.example.com/3", Content: "c3"},
			}},
		}}
		swapMarginNoteWebEnv(t, queryGen, search)

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		user := rec.reqs[0].Messages[1].Content
		assert.Contains(t, user, "https://a.example.com/1", "合并去重结果进 prompt")
		assert.Contains(t, user, "https://a.example.com/2")
		assert.Contains(t, user, "https://b.example.com/3")
		require.Len(t, res.CitedWebSources, 1, "合并池作为白名单候选集（跨 query 命中）")
		assert.Equal(t, "https://a.example.com/1", res.CitedWebSources[0].URL)
		assert.False(t, res.PureModelKnowledge)
	})

	t.Run("B3_partial_search_failure_survivors_used", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		queryGen := &marginNoteQueryGenRecorder{content: `{"queries":["q1","q2"]}`}
		search := &marginNoteSearchRecorder{byQuery: map[string]marginNoteSearchOutcome{
			"q1": {results: []searxng.Result{{Title: "r1", URL: "https://a.example.com/1", Content: "c1"}}},
			"q2": {err: errors.New("searxng fetch: timeout")},
		}}
		swapMarginNoteWebEnv(t, queryGen, search)

		_, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err, "单个 query 失败不拖垮其他")
		user := rec.reqs[0].Messages[1].Content
		assert.Contains(t, user, "https://a.example.com/1", "成功 query 的结果照用")
	})

	t.Run("B4_all_search_failed_no_quoted_fallback", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		queryGen := &marginNoteQueryGenRecorder{content: `{"queries":["q1","q2"]}`}
		search := &marginNoteSearchRecorder{err: errors.New("searxng fetch: connection refused")}
		swapMarginNoteWebEnv(t, queryGen, search)

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		assert.Empty(t, res.CitedWebSources)
		assert.NotContains(t, rec.reqs[0].Messages[1].Content, "联网搜索结果", "全部失败 → 无联网块")
		// D4：不再回退锚文本——SearXNG 同轮已挂，二跳只加延迟。
		require.Len(t, search.queries, 2, "只发起生成的 2 个 query")
		assert.ElementsMatch(t, []string{"q1", "q2"}, search.queries)
		for _, q := range search.queries {
			assert.NotContains(t, q, "逆回购操作", "B4：不回退锚文本 query")
		}
		assert.True(t, res.PureModelKnowledge)
	})

	t.Run("B5_all_search_empty_same_as_B4", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		queryGen := &marginNoteQueryGenRecorder{content: `{"queries":["q1","q2"]}`}
		search := &marginNoteSearchRecorder{byQuery: map[string]marginNoteSearchOutcome{
			"q1": {}, "q2": {},
		}}
		swapMarginNoteWebEnv(t, queryGen, search)

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		assert.Empty(t, res.CitedWebSources)
		assert.NotContains(t, rec.reqs[0].Messages[1].Content, "联网搜索结果", "各 query 结果全空 → 无联网块")
		require.Len(t, search.queries, 2)
		assert.True(t, res.PureModelKnowledge)
	})

	t.Run("B6_query_gen_failed_fallback_quoted_search", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: `{"answer":"答","cited_article_ids":[],"web_sources":[{"title":"锚文本搜到的","url":"https://a.example.com/x"}]}`}
		swapMarginNoteChat(t, rec)
		queryGen := &marginNoteQueryGenRecorder{err: errors.New("open_notebook 全部失败")}
		search := &marginNoteSearchRecorder{results: []searxng.Result{
			{Title: "锚文本搜到的", URL: "https://a.example.com/x", Content: "c"},
		}}
		swapMarginNoteWebEnv(t, queryGen, search)

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		// 回退 = 现状规则：锚文本 query（截前 80 runes）单 query 搜索。
		require.Len(t, search.queries, 1)
		assert.Equal(t, "以利率招标方式开展 3000 亿元逆回购操作", search.queries[0])
		assert.Contains(t, rec.reqs[0].Messages[1].Content, "https://a.example.com/x", "回退搜索结果进 prompt")
		require.Len(t, res.CitedWebSources, 1)
		assert.False(t, res.PureModelKnowledge)
	})

	t.Run("B8_fallback_search_also_failed_silent_final", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		queryGen := &marginNoteQueryGenRecorder{err: errors.New("timeout")}
		search := &marginNoteSearchRecorder{err: errors.New("searxng fetch: timeout")}
		swapMarginNoteWebEnv(t, queryGen, search)

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err, "双保险耗尽 → 静默终态，问答照常")
		assert.Empty(t, res.CitedWebSources)
		assert.NotContains(t, rec.reqs[0].Messages[1].Content, "联网搜索结果")
		assert.True(t, res.PureModelKnowledge)
	})
}

// ── 联网搜索状态三态（add-searxng-health-indicator）──────────────────

// TestAskMarginNote_WebSearchStatus_ThreeStates 覆盖故事一步 1~3：联网失效
// →empty、未启用→disabled、正常→ok；每轮独立落库 annotation_qas.
// web_search_status（新行非 NULL）；B6 形态（query 生成失败但回退搜索成功）
// 同为 ok（状态描述搜索池，不描述引用）。
func TestAskMarginNote_WebSearchStatus_ThreeStates(t *testing.T) {
	answerJSON := `{"answer":"答","cited_article_ids":[],"web_sources":[]}`

	t.Run("disabled_endpoint_unconfigured", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		swapMarginNoteEndpoint(t, "", false) // 预检不过（B1 形态）

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		assert.Equal(t, MarginNoteWebSearchStatusDisabled, res.WebSearchStatus)
		var qa repository.AnnotationQA
		require.NoError(t, repository.Repo.DB().First(&qa, "annotation_id = ?", annotation.ID).Error)
		require.NotNil(t, qa.WebSearchStatus)
		assert.Equal(t, MarginNoteWebSearchStatusDisabled, *qa.WebSearchStatus)
	})

	t.Run("ok_search_results_entered_pool", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		swapMarginNoteWebEnv(t,
			&marginNoteQueryGenRecorder{content: `{"queries":["q1"]}`},
			&marginNoteSearchRecorder{results: []searxng.Result{{Title: "r1", URL: "https://a.example.com/1", Content: "c1"}}})

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		assert.Equal(t, MarginNoteWebSearchStatusOK, res.WebSearchStatus)
		var qa repository.AnnotationQA
		require.NoError(t, repository.Repo.DB().First(&qa, "annotation_id = ?", annotation.ID).Error)
		require.NotNil(t, qa.WebSearchStatus)
		assert.Equal(t, MarginNoteWebSearchStatusOK, *qa.WebSearchStatus)
	})

	t.Run("empty_all_engines_dead_or_no_results", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		swapMarginNoteWebEnv(t,
			&marginNoteQueryGenRecorder{content: `{"queries":["q1","q2"]}`},
			&marginNoteSearchRecorder{err: errors.New("searxng fetch: all engines timed out")}) // 本次事故形态：HTTP 200 但引擎全挂

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err, "联网失效不得阻断问答")
		assert.Equal(t, "答", res.Answer)
		assert.Equal(t, MarginNoteWebSearchStatusEmpty, res.WebSearchStatus)
		var qa repository.AnnotationQA
		require.NoError(t, repository.Repo.DB().First(&qa, "annotation_id = ?", annotation.ID).Error)
		require.NotNil(t, qa.WebSearchStatus)
		assert.Equal(t, MarginNoteWebSearchStatusEmpty, *qa.WebSearchStatus)
	})

	t.Run("ok_via_quoted_fallback_after_query_gen_failure", func(t *testing.T) {
		annotation := setupMarginNoteDegrade(t)
		rec := &marginNoteChatRecorder{content: answerJSON}
		swapMarginNoteChat(t, rec)
		swapMarginNoteWebEnv(t,
			&marginNoteQueryGenRecorder{err: errors.New("timeout")},
			&marginNoteSearchRecorder{results: []searxng.Result{{Title: "锚文本搜到", URL: "https://a.example.com/x", Content: "c"}}})

		res, err := AskMarginNote(context.Background(), annotation.ID, "问题")
		require.NoError(t, err)
		assert.Equal(t, MarginNoteWebSearchStatusOK, res.WebSearchStatus, "回退搜索有结果 → ok（B6 形态）")
	})
}

// TestAskMarginNote_QueryChangesWithQuestion 覆盖 spec Scenario「搜索词随问题
// 语义变化」：同一批注先后两问，搜索入参随 query 生成输出变化（非固定锚文本）。
func TestAskMarginNote_QueryChangesWithQuestion(t *testing.T) {
	annotation := setupMarginNoteDegrade(t)
	rec := &marginNoteChatRecorder{content: `{"answer":"答","cited_article_ids":[],"web_sources":[]}`}
	swapMarginNoteChat(t, rec)
	queryGen := &marginNoteQueryGenRecorder{content: `{"queries":["蓝白合 政党联盟"]}`}
	search := &marginNoteSearchRecorder{}
	swapMarginNoteWebEnv(t, queryGen, search)

	_, err := AskMarginNote(context.Background(), annotation.ID, "蓝白合是什么")
	require.NoError(t, err)
	require.Equal(t, 1, queryGen.calls)

	// 追问语义不同的问题：query 生成输出随问题变化，搜索入参随之变化。
	queryGen.mu.Lock()
	queryGen.content = `{"queries":["2024台湾大选 两岸关系"]}`
	queryGen.mu.Unlock()
	_, err = AskMarginNote(context.Background(), annotation.ID, "2024 大选对两岸关系什么影响")
	require.NoError(t, err)
	require.Equal(t, 2, queryGen.calls)
	require.Len(t, search.queries, 2, "每轮各搜一次（本轮各 1 个 query）")
	assert.Equal(t, "蓝白合 政党联盟", search.queries[0])
	assert.Equal(t, "2024台湾大选 两岸关系", search.queries[1])
	assert.NotEqual(t, search.queries[0], search.queries[1], "两问搜索词互不相同")
	for _, q := range search.queries {
		assert.NotEqual(t, "以利率招标方式开展 3000 亿元逆回购操作", q, "搜索词非固定锚文本")
	}
}
