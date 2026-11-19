package repository

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── 线索相关事件读模型 PG 集成测试（replace-lane-lifeline-with-topic-events 7.1）──
//
// 一次性 testcontainer PG（golden schema）；禁 SQLite（repository 层红线）。
// 匹配口径是 **thread 向量余弦**（design D3），覆盖 test-cases.md 主链路 1–12
// 步与白盒分支 B1–B15：窗口边界/不含当期、阈值与档位边界、不叠加时间衰减、
// 不做语义归并、稳定排序与全量返回、confirmed 只读判定、来源文章缺失为 null、
// 缺向量跳过与三态、只读幂等。HTTP 语义在 handler 测试。

// trTestRepo provisions the golden-schema PG and a fresh repo.
func trTestRepo(t *testing.T) *TopicGraphRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	return repo
}

// trDay builds a report date in the same normalization the production path uses.
func trDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

// trBaseVector is the current thread's vector in the score tests: it keeps the
// arithmetic exact (norm 20) so band boundaries land on real boundaries.
var trBaseVector = []float64{20, 0, 0, 0, 0}

// trCosineVector builds a 5-dim vector whose cosine similarity to trBaseVector
// is (up to the column's 6-decimal rounding) `target`. Only for values away
// from the cutoffs; exact boundary values use integer vectors instead
// (trExactStrongVector / trExactThresholdVector) so a one-ulp drift cannot
// flip an assertion.
func trCosineVector(target float64) []float64 {
	x := target * trBaseVector[0]
	return []float64{x, math.Sqrt(trBaseVector[0]*trBaseVector[0] - x*x), 0, 0, 0}
}

// trExactStrongVector: dot 340 / norm² 400 → similarity 0.85 exactly (B2).
var trExactStrongVector = []float64{17, 10, 3, 1, 1}

// trExactThresholdVector: dot 300 / norm² 400 → similarity 0.75 exactly (B4).
var trExactThresholdVector = []float64{15, 11, 7, 2, 1}

func trSeedReport(t *testing.T, db *gorm.DB, boardID uint, day time.Time) uint {
	t.Helper()
	report := BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      NormalizeReportDate(day),
		Title:           "日报 " + day.Format("2006-01-02"),
		Summary:         "摘要",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

func trSeedSection(t *testing.T, db *gorm.DB, reportID uint, topicID *uint) uint {
	t.Helper()
	section := DailyReportSection{
		ReportID:          reportID,
		ClusterLabel:      "章节",
		ArticleCount:      1,
		Embedding:         FloatsToPgVector([]float64{0}),
		PersistentTopicID: topicID,
	}
	require.NoError(t, db.Create(&section).Error)
	return section.ID
}

// trThreadSpec describes one seeded thread. Embedding nil = the column stays
// NULL (the "no vector" case, B7/B8).
type trThreadSpec struct {
	Title      string
	Summary    string
	Embedding  []float64
	TagIDs     []uint
	ArticleIDs []uint
}

func trSeedThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, spec trThreadSpec) uint {
	t.Helper()
	tags, err := json.Marshal(spec.TagIDs)
	require.NoError(t, err)
	arts, err := json.Marshal(spec.ArticleIDs)
	require.NoError(t, err)
	summary := spec.Summary
	if summary == "" {
		summary = "摘要：" + spec.Title
	}
	thread := DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             spec.Title,
		Summary:           summary,
		TagIDs:            JSON(tags),
		RelatedArticleIDs: JSON(arts),
	}
	query := db
	if spec.Embedding == nil {
		// embedding 是非空字面量列：零值 "" 非法，Omit 才落 NULL。
		query = db.Omit("embedding")
	} else {
		thread.Embedding = FloatsToPgVector(spec.Embedding)
	}
	require.NoError(t, query.Create(&thread).Error)
	return thread.ID
}

func trSeedTag(t *testing.T, db *gorm.DB, label, category, description string) uint {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: "slug-" + label, Category: category, Status: "active", Description: description}
	require.NoError(t, db.Create(&tag).Error)
	return tag.ID
}

func trSeedFeed(t *testing.T, db *gorm.DB, title string) uint {
	t.Helper()
	feed := models.Feed{Title: title, URL: "https://example.com/" + title}
	require.NoError(t, db.Create(&feed).Error)
	return feed.ID
}

func trSeedArticle(t *testing.T, db *gorm.DB, feedID uint, title string, pub time.Time) uint {
	t.Helper()
	article := models.Article{FeedID: feedID, Title: title, PubDate: &pub}
	require.NoError(t, db.Create(&article).Error)
	return article.ID
}

// trSeedMarker freezes a key-event marker carrying the given tag snapshot.
func trSeedMarker(t *testing.T, db *gorm.DB, boardID, topicID uint, sourceKey string, tagIDs []uint, state string) {
	t.Helper()
	raw, err := json.Marshal(tagIDs)
	require.NoError(t, err)
	marker := TopicKeyEventMarker{
		SemanticBoardID:   boardID,
		SourceKey:         sourceKey,
		PersistentTopicID: &topicID,
		SourcePeriodDate:  trDay(2026, 11, 16),
		TagIDsSnapshot:    JSON(raw),
		State:             state,
	}
	require.NoError(t, db.Create(&marker).Error)
}

type trWorld struct {
	repo    *TopicGraphRepository
	db      *gorm.DB
	boardID uint
	topicID uint
}

func trSeedWorld(t *testing.T) trWorld {
	t.Helper()
	repo := trTestRepo(t)
	boardID := seedTestBoard(t, repo.db)
	topicID := keyEventSeedTopic(t, repo.db, boardID, "测试泳道")
	return trWorld{repo: repo, db: repo.db, boardID: boardID, topicID: topicID}
}

// report seeds a report on `day` for the world's board.
func (w trWorld) report(t *testing.T, day time.Time) uint {
	t.Helper()
	return trSeedReport(t, w.db, w.boardID, day)
}

// threadInReport seeds one topic-owned section + one thread inside an existing
// report (several threads may share one report / section).
func (w trWorld) threadInReport(t *testing.T, reportID uint, spec trThreadSpec) uint {
	t.Helper()
	sectionID := trSeedSection(t, w.db, reportID, &w.topicID)
	return trSeedThread(t, w.db, reportID, sectionID, spec)
}

// reportWithThread seeds one report + one topic-owned section + one thread.
func (w trWorld) reportWithThread(t *testing.T, day time.Time, spec trThreadSpec) (reportID, threadID uint) {
	t.Helper()
	reportID = w.report(t, day)
	return reportID, w.threadInReport(t, reportID, spec)
}

// trRelations is the recommendation list of one current thread.
func trRelations(resp *ThreadRelationsResponse, threadID uint) []ThreadRelation {
	return resp.Threads[threadID]
}

func trFind(relations []ThreadRelation, threadID uint) *ThreadRelation {
	for i := range relations {
		if relations[i].ThreadID == threadID {
			return &relations[i]
		}
	}
	return nil
}

// ── 主链路 1 / 变体 9、11、13；B14：窗口边界与「不含当期」─────────────────

func TestThreadRelations_WindowExcludesCurrentPeriod(t *testing.T) {
	w := trSeedWorld(t)
	vec := []float64{1, 0}

	currentReport, currentThread := w.reportWithThread(t, trDay(2026, 11, 17), trThreadSpec{Title: "当期线索", Embedding: vec})
	// 同一期内的另一条线索：当期线索不得作为自己的推荐来源（B14）。
	samePeriodThread := w.threadInReport(t, currentReport, trThreadSpec{Title: "同屏线索", Embedding: vec})

	_, lastWindowDay := w.reportWithThread(t, trDay(2026, 11, 16), trThreadSpec{Title: "窗口末日来源", Embedding: vec})
	_, firstWindowDay := w.reportWithThread(t, trDay(2026, 11, 10), trThreadSpec{Title: "窗口首日来源", Embedding: vec})
	_, outsideDay := w.reportWithThread(t, trDay(2026, 11, 9), trThreadSpec{Title: "窗外来源", Embedding: vec})

	resp, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	assert.Equal(t, 7, resp.Days)
	assert.Equal(t, "2026-11-10", resp.WindowStart)
	assert.Equal(t, "2026-11-16", resp.WindowEnd)
	assert.Equal(t, ThreadRelationStatusReady, resp.Status)
	assert.Equal(t, 0, resp.SkippedNoEmbedding)

	rels := trRelations(resp, currentThread)
	require.Len(t, rels, 2, "当期线索不得推荐自己，窗外来源不计入")
	assert.Equal(t, lastWindowDay, rels[0].ThreadID, "分数相同则 period_date 降序")
	assert.Equal(t, firstWindowDay, rels[1].ThreadID)
	assert.Equal(t, "2026-11-16", rels[0].PeriodDate)
	assert.Nil(t, trFind(rels, samePeriodThread))
	assert.Nil(t, trFind(rels, outsideDay))
	// 同期的另一条线索也是「当期线索」，它同样只看见窗口内的候选——二者互不
	// 推荐（B14：窗口右端不含 report_date）。total 因此是 2×2 条。
	assert.Nil(t, trFind(trRelations(resp, samePeriodThread), currentThread), "同期线索互不推荐")
	assert.Len(t, trRelations(resp, samePeriodThread), 2)
	assert.Equal(t, 4, resp.Total)

	// 变体 11：第 8 天的来源在 7 天档看不到、30 天档看得到。
	wide, err := w.repo.BuildThreadRelations(currentReport, 30)
	require.NoError(t, err)
	assert.Equal(t, 30, wide.Days)
	assert.Equal(t, "2026-10-18", wide.WindowStart)
	assert.Len(t, trRelations(wide, currentThread), 3, "30 天档把 11.09 的来源纳入")
}

// ── 主链路 2/3 + B1–B6：余弦阈值与档位边界 ────────────────────────────────

func TestThreadRelations_SimilarityThresholdAndBands(t *testing.T) {
	w := trSeedWorld(t)

	currentReport, currentThread := w.reportWithThread(t, trDay(2026, 11, 17),
		trThreadSpec{Title: "当期线索", Embedding: trBaseVector})

	strong90 := w.threadInReport(t, w.report(t, trDay(2026, 11, 16)), trThreadSpec{
		Title: "相似 0.90", Embedding: []float64{18, 7, 5, 1, 1}}) // 360/400 = 0.90（B1）
	strong85 := w.threadInReport(t, w.report(t, trDay(2026, 11, 15)), trThreadSpec{
		Title: "相似 0.85", Embedding: trExactStrongVector}) // 340/400 = 0.85（B2）
	related8499 := w.threadInReport(t, w.report(t, trDay(2026, 11, 14)), trThreadSpec{
		Title: "相似 0.8499", Embedding: trCosineVector(0.8499)}) // B3
	related75 := w.threadInReport(t, w.report(t, trDay(2026, 11, 13)), trThreadSpec{
		Title: "相似 0.75", Embedding: trExactThresholdVector}) // 300/400 = 0.75（B4）
	below7499 := w.threadInReport(t, w.report(t, trDay(2026, 11, 12)), trThreadSpec{
		Title: "相似 0.7499", Embedding: trCosineVector(0.7499)}) // B5
	negated := w.threadInReport(t, w.report(t, trDay(2026, 11, 11)), trThreadSpec{
		Title: "反向向量", Embedding: []float64{-20, 0, 0, 0, 0}}) // B6

	resp, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	rels := trRelations(resp, currentThread)
	require.Len(t, rels, 4, "只有 ≥0.75 的四条入选（B5/B6 排除）")

	require.NotNil(t, trFind(rels, strong90))
	require.NotNil(t, trFind(rels, strong85))
	require.NotNil(t, trFind(rels, related8499))
	require.NotNil(t, trFind(rels, related75))
	assert.Nil(t, trFind(rels, below7499), "0.7499 不得入选（B5）")
	assert.Nil(t, trFind(rels, negated), "负相似度不得入选（B6）")

	assert.InDelta(t, 0.90, trFind(rels, strong90).Score, 1e-9)
	assert.Equal(t, 0.85, trFind(rels, strong85).Score, "0.85 是精确值，不是 0.8499…")
	assert.InDelta(t, 0.8499, trFind(rels, related8499).Score, 1e-6)
	assert.Equal(t, 0.75, trFind(rels, related75).Score)

	assert.Equal(t, ThreadRelationLevelStrong, trFind(rels, strong90).Level)
	assert.Equal(t, ThreadRelationLevelStrong, trFind(rels, strong85).Level, "0.85 边界必须落 strong（B2）")
	assert.Equal(t, ThreadRelationLevelRelated, trFind(rels, related8499).Level, "0.8499 落 related（B3）")
	assert.Equal(t, ThreadRelationLevelRelated, trFind(rels, related75).Level, "0.75 边界必须入选且为 related（B4）")

	// 排序：score 降序，且分数就是余弦本身（未被任何权重改写）。
	for i := 1; i < len(rels); i++ {
		assert.GreaterOrEqual(t, rels[i-1].Score, rels[i].Score)
	}
}

// ── 主链路 4：分数不因时间远近被改写 ──────────────────────────────────────

func TestThreadRelations_ScoreIsNotReweightedByRecency(t *testing.T) {
	w := trSeedWorld(t)

	currentReport, currentThread := w.reportWithThread(t, trDay(2026, 11, 17),
		trThreadSpec{Title: "当期线索", Embedding: trBaseVector})
	olderHigher := w.threadInReport(t, w.report(t, trDay(2026, 11, 10)), trThreadSpec{
		Title: "更旧但更相关", Embedding: trCosineVector(0.86)})
	newerLower := w.threadInReport(t, w.report(t, trDay(2026, 11, 16)), trThreadSpec{
		Title: "更新但只是同领域", Embedding: trCosineVector(0.80)})

	resp, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	rels := trRelations(resp, currentThread)
	require.Len(t, rels, 2)

	assert.InDelta(t, 0.86, trFind(rels, olderHigher).Score, 1e-6, "score 是余弦，不被时间衰减改写")
	assert.InDelta(t, 0.80, trFind(rels, newerLower).Score, 1e-6)
	assert.Equal(t, olderHigher, rels[0].ThreadID, "更旧但分数更高的排前面")
	assert.Equal(t, newerLower, rels[1].ThreadID)
	assert.Equal(t, "2026-11-10", rels[0].PeriodDate)
}

// ── 主链路 5/6/7 + B13：不归并、稳定排序、全量返回 ────────────────────────

func TestThreadRelations_NoSemanticMergeAndStableSort(t *testing.T) {
	w := trSeedWorld(t)
	vec := []float64{1, 0}

	currentReport, currentThread := w.reportWithThread(t, trDay(2026, 11, 17), trThreadSpec{Title: "当期线索", Embedding: vec})
	sourceReport := w.report(t, trDay(2026, 11, 15))
	sectionID := trSeedSection(t, w.db, sourceReport, &w.topicID)
	seeded := make([]uint, 0, 5)
	for _, title := range []string{"候选一", "候选二", "候选三", "候选四", "候选五"} {
		seeded = append(seeded, trSeedThread(t, w.db, sourceReport, sectionID, trThreadSpec{Title: title, Embedding: vec}))
	}

	first, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	rels := trRelations(first, currentThread)
	require.Len(t, rels, 5, "内容高度相似也各自成条（不归并），且后端不截断")
	assert.Equal(t, 5, first.Total)

	// score 与 period_date 全部相同 → 按 thread_id 升序（严格全序，B13）。
	for i, id := range seeded {
		assert.Equal(t, id, rels[i].ThreadID, "并列时按 thread_id 升序")
		assert.InDelta(t, 1.0, rels[i].Score, 1e-12)
	}
	for i := 1; i < len(rels); i++ {
		assert.Less(t, rels[i-1].ThreadID, rels[i].ThreadID)
	}

	second, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	require.Equal(t, first, second, "同一输入两次查询结果等价")
}

// ── 主链路 8/9/10 + B9–B12：confirmed 只读判定与来源文章 ─────────────────

func TestThreadRelations_SourceArticleAndConfirmed(t *testing.T) {
	w := trSeedWorld(t)
	vec := []float64{1, 0}
	tagEvent := trSeedTag(t, w.db, "已确认事件", models.TagCategoryEvent, "事件描述")
	tagSuggested := trSeedTag(t, w.db, "只有建议的事件", models.TagCategoryEvent, "")
	tagDismissed := trSeedTag(t, w.db, "被不采纳的事件", models.TagCategoryEvent, "")
	tagKeyword := trSeedTag(t, w.db, "某公司", models.TagCategoryKeyword, "主体标签不是事件")

	feedID := trSeedFeed(t, w.db, "示例来源")
	articleID := trSeedArticle(t, w.db, feedID, "候选来源文章", trDay(2026, 11, 16))

	currentReport, currentThread := w.reportWithThread(t, trDay(2026, 11, 17), trThreadSpec{Title: "当期线索", Embedding: vec})

	withArticle := w.threadInReport(t, w.report(t, trDay(2026, 11, 16)), trThreadSpec{
		Title: "有来源文章且已确认", Embedding: vec, TagIDs: []uint{tagEvent}, ArticleIDs: []uint{articleID, 999998}})
	noArticles := w.threadInReport(t, w.report(t, trDay(2026, 11, 15)), trThreadSpec{
		Title: "没有来源文章", Embedding: vec, TagIDs: []uint{tagKeyword}})
	missingArticle := w.threadInReport(t, w.report(t, trDay(2026, 11, 14)), trThreadSpec{
		Title: "文章已被删", Embedding: vec, ArticleIDs: []uint{999999}})
	onlySuggested := w.threadInReport(t, w.report(t, trDay(2026, 11, 13)), trThreadSpec{
		Title: "只有 AI 建议", Embedding: vec, TagIDs: []uint{tagSuggested}})
	onlyDismissed := w.threadInReport(t, w.report(t, trDay(2026, 11, 12)), trThreadSpec{
		Title: "被不采纳", Embedding: vec, TagIDs: []uint{tagDismissed}})

	// 已确认注记只挂到 tagEvent；keyword 标签虽然也在快照里但不算 event。
	trSeedMarker(t, w.db, w.boardID, w.topicID, "ke-confirmed", []uint{tagEvent, tagKeyword}, KeyEventStateConfirmed)
	trSeedMarker(t, w.db, w.boardID, w.topicID, "ke-suggested", []uint{tagSuggested}, KeyEventStateSuggested)
	trSeedMarker(t, w.db, w.boardID, w.topicID, "ke-dismissed", []uint{tagDismissed}, KeyEventStateDismissed)

	resp, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	rels := trRelations(resp, currentThread)
	require.Len(t, rels, 5)

	confirmed := trFind(rels, withArticle)
	require.NotNil(t, confirmed)
	assert.True(t, confirmed.Confirmed, "event 标签命中 confirmed 注记（B11）")
	require.NotNil(t, confirmed.SourceArticle)
	assert.Equal(t, articleID, confirmed.SourceArticle.ID, "取 related_article_ids 首篇")
	assert.Equal(t, "候选来源文章", confirmed.SourceArticle.Title)
	assert.Equal(t, "示例来源", confirmed.SourceArticle.Feed)
	require.NotNil(t, confirmed.SourceArticle.PublishedAt)
	assert.Equal(t, "有来源文章且已确认", confirmed.Title)
	assert.Equal(t, "摘要：有来源文章且已确认", confirmed.Summary)

	noArticleRel := trFind(rels, noArticles)
	require.NotNil(t, noArticleRel)
	assert.Nil(t, noArticleRel.SourceArticle, "related_article_ids 为空 → source_article null（B9）")
	assert.False(t, noArticleRel.Confirmed, "keyword 标签不因出现在注记快照里就加星")
	assert.Equal(t, "没有来源文章", noArticleRel.Title, "标题摘要照常返回")

	missingRel := trFind(rels, missingArticle)
	require.NotNil(t, missingRel)
	assert.Nil(t, missingRel.SourceArticle, "首篇文章已被删 → source_article null（B10）")

	assert.False(t, trFind(rels, onlySuggested).Confirmed, "suggested 不加星")
	assert.False(t, trFind(rels, onlyDismissed).Confirmed, "dismissed 不加星")

	// 只读：注记状态一个都没被改动。
	var states []string
	require.NoError(t, w.db.Model(&TopicKeyEventMarker{}).Order("source_key ASC").Pluck("state", &states).Error)
	assert.Equal(t, []string{KeyEventStateConfirmed, KeyEventStateDismissed, KeyEventStateSuggested}, states)

	raw, err := json.Marshal(missingRel)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"source_article":null`)
}

// ── 主链路 11/12 + B7/B8 + 变体 8：缺向量 → skipped/partial ────────────────

func TestThreadRelations_MissingEmbeddingPartial(t *testing.T) {
	w := trSeedWorld(t)
	vec := []float64{1, 0}

	currentReport := w.report(t, trDay(2026, 11, 17))
	withVector := w.threadInReport(t, currentReport, trThreadSpec{Title: "有向量的线索", Embedding: vec})
	withoutVector := w.threadInReport(t, currentReport, trThreadSpec{Title: "缺向量的线索"})

	sourceReport := w.report(t, trDay(2026, 11, 16))
	sourceWithVector := w.threadInReport(t, sourceReport, trThreadSpec{Title: "历史有向量", Embedding: vec})
	sourceWithoutVector := w.threadInReport(t, sourceReport, trThreadSpec{Title: "历史缺向量"})

	resp, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	assert.Equal(t, 1, resp.SkippedNoEmbedding, "当期缺向量的线索计入 skipped（B7）")
	assert.Equal(t, ThreadRelationStatusPartial, resp.Status, "有跳过不得标 ready")
	assert.Equal(t, 1, resp.Total)

	rels := trRelations(resp, withVector)
	require.Len(t, rels, 1)
	assert.Equal(t, sourceWithVector, rels[0].ThreadID)
	assert.Nil(t, resp.Threads[withoutVector], "缺向量的当期线索不出现在 threads 里")
	assert.Nil(t, resp.Threads[sourceWithoutVector], "缺向量的历史线索不进候选池（B8）")
	for _, rels := range resp.Threads {
		for _, rel := range rels {
			assert.NotEqual(t, sourceWithoutVector, rel.ThreadID)
		}
	}
}

// ── 主链路 21 + B15：全部低于阈值 / 全缺向量 → threads={}、status=empty ────

func TestThreadRelations_EmptyThreadsObject(t *testing.T) {
	w := trSeedWorld(t)

	currentReport, currentThread := w.reportWithThread(t, trDay(2026, 11, 17),
		trThreadSpec{Title: "当期线索", Embedding: []float64{1, 0}})
	// 正交（相似度 0）+ 维度不符（NaN）：都低于阈值，不得入选。
	w.reportWithThread(t, trDay(2026, 11, 15), trThreadSpec{Title: "正交线索", Embedding: []float64{0, 1}})
	w.reportWithThread(t, trDay(2026, 11, 14), trThreadSpec{Title: "维度不符", Embedding: []float64{1, 0, 0}})

	resp, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	assert.Equal(t, ThreadRelationStatusEmpty, resp.Status)
	assert.Equal(t, 0, resp.Total)
	assert.Equal(t, 0, resp.SkippedNoEmbedding)
	assert.Empty(t, resp.Threads)
	assert.Nil(t, resp.Threads[currentThread])
	require.NotNil(t, resp.Threads, "threads 必须是空 map 而不是 nil（JSON 里是 {}）")
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"threads":{}`)

	// 全部当期线索都缺向量：empty 优先于 partial（没有推荐可言，就谈不上部分）。
	emptyReport := w.report(t, trDay(2026, 10, 1))
	w.threadInReport(t, emptyReport, trThreadSpec{Title: "缺向量线索"})
	allSkipped, err := w.repo.BuildThreadRelations(emptyReport, 7)
	require.NoError(t, err)
	assert.Equal(t, ThreadRelationStatusEmpty, allSkipped.Status)
	assert.Empty(t, allSkipped.Threads)
	assert.Equal(t, 1, allSkipped.SkippedNoEmbedding)
}

// ── 变体 4/10：无话题归属或空窗口 → 无候选 ────────────────────────────────

func TestThreadRelations_NoTopicOrNoWindowCandidates(t *testing.T) {
	w := trSeedWorld(t)
	vec := []float64{1, 0}

	// 当期线索所在 section 没有话题归属 → 没有「同一话题内」的候选面。
	orphanReport := w.report(t, trDay(2026, 11, 17))
	orphanSection := trSeedSection(t, w.db, orphanReport, nil)
	orphanThread := trSeedThread(t, w.db, orphanReport, orphanSection, trThreadSpec{Title: "无归属线索", Embedding: vec})
	w.reportWithThread(t, trDay(2026, 11, 15), trThreadSpec{Title: "别的话题来源", Embedding: vec})

	resp, err := w.repo.BuildThreadRelations(orphanReport, 7)
	require.NoError(t, err)
	assert.Equal(t, ThreadRelationStatusEmpty, resp.Status)
	assert.Empty(t, resp.Threads)
	assert.Nil(t, resp.Threads[orphanThread])
	assert.Equal(t, 0, resp.SkippedNoEmbedding, "无话题归属不算缺向量")
}

// ── 变体 12：只读幂等（两次请求 DB 无写、响应等价）─────────────────────────

func TestThreadRelations_ReadOnlyAndIdempotent(t *testing.T) {
	w := trSeedWorld(t)
	vec := []float64{1, 0}
	tagA := trSeedTag(t, w.db, "只读事件", models.TagCategoryEvent, "只读描述")
	feedID := trSeedFeed(t, w.db, "只读来源")
	articleID := trSeedArticle(t, w.db, feedID, "只读文章", trDay(2026, 11, 15))

	currentReport, _ := w.reportWithThread(t, trDay(2026, 11, 17), trThreadSpec{Title: "当期线索", Embedding: vec, TagIDs: []uint{tagA}})
	w.reportWithThread(t, trDay(2026, 11, 15), trThreadSpec{Title: "来源线索", Embedding: vec, TagIDs: []uint{tagA}, ArticleIDs: []uint{articleID}})
	trSeedMarker(t, w.db, w.boardID, w.topicID, "ke-readonly", []uint{tagA}, KeyEventStateConfirmed)

	tableModels := map[string]any{
		"reports":  &BoardDailyReport{},
		"sections": &DailyReportSection{},
		"threads":  &DailyReportThread{},
		"tags":     &models.TopicTag{},
		"markers":  &TopicKeyEventMarker{},
		"articles": &models.Article{},
	}
	countAll := func() map[string]int64 {
		t.Helper()
		out := map[string]int64{}
		for name, model := range tableModels {
			var n int64
			require.NoError(t, w.db.Model(model).Count(&n).Error)
			out[name] = n
		}
		return out
	}

	before := countAll()
	first, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	second, err := w.repo.BuildThreadRelations(currentReport, 7)
	require.NoError(t, err)
	assert.Equal(t, first, second, "响应等价")
	assert.Equal(t, before, countAll(), "读请求不得写任何表")
}

// ── 日报不存在 → 哨兵错误（handler 映射 404）──────────────────────────────

func TestThreadRelations_ReportNotFound(t *testing.T) {
	w := trSeedWorld(t)
	resp, err := w.repo.BuildThreadRelations(999999, 7)
	require.ErrorIs(t, err, ErrThreadRelationReportNotFound)
	assert.Nil(t, resp)
}

// ── 天数归一化（spec「非法天数回落并回显」的 repository 侧）───────────────

func TestNormalizeThreadRelationDays(t *testing.T) {
	cases := map[int]int{-1: 7, 0: 7, 7: 7, 14: 7, 30: 30, 365: 7}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeThreadRelationDays(in), "days=%d", in)
	}
}
