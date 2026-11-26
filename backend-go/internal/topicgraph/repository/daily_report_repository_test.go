package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

func TestNormalizeReportDateKeepsRequestedDate(t *testing.T) {
	requested, err := time.ParseInLocation("2006-01-02", "2026-05-26", models.ShanghaiTZ)
	require.NoError(t, err)

	got := NormalizeReportDate(requested)

	require.Equal(t, "2026-05-26", got.Format("2006-01-02"))
	require.Equal(t, time.UTC, got.Location())
	require.Equal(t, 12, got.Hour())
}

func TestListTodayReportBriefs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	today := time.Now().Format("2006-01-02")
	seedTestReport(t, db, boardID, NormalizeReportDate(time.Now()))

	briefs, err := repo.ListTodayReportBriefs(today)
	require.NoError(t, err)
	require.Len(t, briefs, 1)
	require.Equal(t, boardID, briefs[0].SemanticBoardID)
	require.Equal(t, today, briefs[0].PeriodDate)
}

func TestListReports_DefaultDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	// Seed reports: today, 3 days ago, 10 days ago
	seedTestReport(t, db, boardID, now)
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -3))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -10))

	// days=0 should default to 7, returning today and 3 days ago
	items, err := repo.ListReports(boardID, 0)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, now.Format("2006-01-02"), items[0].PeriodDate)
	require.Equal(t, now.AddDate(0, 0, -3).Format("2006-01-02"), items[1].PeriodDate)
}

func TestListReports_SevenDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	seedTestReport(t, db, boardID, now)
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -5))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -10))

	items, err := repo.ListReports(boardID, 7)
	require.NoError(t, err)
	require.Len(t, items, 2) // 10 days ago is outside 7-day window
	require.Equal(t, now.Format("2006-01-02"), items[0].PeriodDate)
}

func TestListReports_FortyTwoDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	seedTestReport(t, db, boardID, now)
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -20))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -40))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -50)) // outside 42 days

	// days=42 should NOT be truncated to 30
	items, err := repo.ListReports(boardID, 42)
	require.NoError(t, err)
	require.Len(t, items, 3) // 50 days ago is outside 42-day window
	// Verify descending order
	require.Equal(t, now.Format("2006-01-02"), items[0].PeriodDate)
	require.Equal(t, now.AddDate(0, 0, -20).Format("2006-01-02"), items[1].PeriodDate)
	require.Equal(t, now.AddDate(0, 0, -40).Format("2006-01-02"), items[2].PeriodDate)
}

func TestGetReportByID_AttachesTopicBriefs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "Tracked topic",
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          TopicStatusActive,
		FirstSeenDate:   now,
		LastSeenDate:    now,
		HitCount:        3,
		ConsecutiveHits: 3,
	}
	require.NoError(t, db.Create(&topic).Error)

	sectionID := seedTestSection(t, db, reportID, "tracked section")
	require.NoError(t, db.Model(&DailyReportSection{}).
		Where("id = ?", sectionID).
		Update("persistent_topic_id", topic.ID).Error)

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)
	require.NotNil(t, report.Sections[0].PersistentTopicID)
	require.Equal(t, topic.ID, *report.Sections[0].PersistentTopicID)
	require.NotNil(t, report.Sections[0].PersistentTopic)
	require.Equal(t, topic.ID, report.Sections[0].PersistentTopic.ID)
	require.Equal(t, topic.Label, report.Sections[0].PersistentTopic.Label)
	require.Equal(t, TopicStatusActive, report.Sections[0].PersistentTopic.Status)
}

func TestGetReportByID_SectionWithoutThreadsMarshalsEmptyArray(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	// Section with zero thread rows — mirrors the production data where the
	// per-cluster threads LLM call failed and the orchestrator degraded to
	// saving the section without threads.
	seedTestSection(t, db, reportID, "no-thread section")

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)

	data, err := json.Marshal(report)
	require.NoError(t, err)
	// threads must serialize as [] (never omitted / null): the frontend reader
	// calls section.threads.filter directly (DailyReportTopicSection.vue).
	require.Contains(t, string(data), `"threads":[]`)
	require.NotNil(t, report.Sections[0].Threads)
	require.Empty(t, report.Sections[0].Threads)
}

// ---- 读路径折叠同内容副本（dedupe-thread-article-copies） ----

// seedThreadCopyArticle 落一篇带 link 的文章（feedID 只作跨 feed 副本的区分，
// articles.feed_id 无外键约束）。
func seedThreadCopyArticle(t *testing.T, db *gorm.DB, feedID uint, title, link string) uint {
	t.Helper()
	article := models.Article{FeedID: feedID, Title: title, Link: link}
	require.NoError(t, db.Create(&article).Error)
	return article.ID
}

// seedThreadCopyThread 落一条线索（related_article_ids 直接给数组字面量）。
func seedThreadCopyThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, title string, articleIDs []uint) uint {
	t.Helper()
	refs, err := json.Marshal(articleIDs)
	require.NoError(t, err)
	thread := DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             title,
		Summary:           "summary",
		RelatedArticleIDs: JSON(refs),
		Embedding:         FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, db.Create(&thread).Error)
	return thread.ID
}

// storedThreadArticleIDs 回查 daily_report_threads.related_article_ids 的落库值。
func storedThreadArticleIDs(t *testing.T, db *gorm.DB, threadID uint) []uint {
	t.Helper()
	var raw string
	require.NoError(t, db.Raw("SELECT related_article_ids::text FROM daily_report_threads WHERE id = ?", threadID).Scan(&raw).Error)
	var ids []uint
	require.NoError(t, json.Unmarshal([]byte(raw), &ids))
	return ids
}

// TestGetReportByID_FoldsSameLinkArticleCopies 覆盖 spec「线索下同 link 副本
// 折叠」：同一篇内容被两个 feed 各抓一份时，响应里的 related_article_ids 只留
// 首个出现的 id、顺序不变；落库值不得被改写（红线 45）。
func TestGetReportByID_FoldsSameLinkArticleCopies(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	sectionID := seedTestSection(t, db, reportID, "copies section")

	// 跨 feed 的同 link 副本 + 一篇不同内容 + 一篇空 link。
	copyA := seedThreadCopyArticle(t, db, 9001, "特朗普与内塔尼亚胡激烈争吵", "https://example.com/same-story")
	copyB := seedThreadCopyArticle(t, db, 9002, "特朗普与内塔尼亚胡激烈争吵", "https://example.com/same-story")
	other := seedThreadCopyArticle(t, db, 9003, "另一篇稿件", "https://example.com/other-story")
	noLink := seedThreadCopyArticle(t, db, 9004, "没有链接的稿件", "")
	threadID := seedThreadCopyThread(t, db, reportID, sectionID, "同 link 副本线索",
		[]uint{copyA, copyB, other, noLink})

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)
	require.Len(t, report.Sections[0].Threads, 1)
	// 副本被折叠、其余原位保留：空 link 的文章不参与折叠。
	require.Equal(t, []uint{copyA, other, noLink},
		jsonUintIDs(report.Sections[0].Threads[0].RelatedArticleIDs))

	// 落库值仍是未折叠的四个 id（红线 45：存储口径不变）。
	require.Equal(t, []uint{copyA, copyB, other, noLink}, storedThreadArticleIDs(t, db, threadID))
}

// TestGetReportByID_FoldsSameLinkArticleCopies_IgnoresQueryAndFragment 覆盖
// spec「归一后 link 相同也折叠」：link 仅差 query / fragment 判为同一内容。
func TestGetReportByID_FoldsSameLinkArticleCopies_IgnoresQueryAndFragment(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	sectionID := seedTestSection(t, db, reportID, "normalized section")

	first := seedThreadCopyArticle(t, db, 9101, "稿件", "https://Example.com/story?utm_source=rss")
	second := seedThreadCopyArticle(t, db, 9102, "稿件", "https://example.com/story#reply12")
	threadID := seedThreadCopyThread(t, db, reportID, sectionID, "归一后同内容线索",
		[]uint{first, second})

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)
	require.Len(t, report.Sections[0].Threads, 1)
	require.Equal(t, []uint{first}, jsonUintIDs(report.Sections[0].Threads[0].RelatedArticleIDs))
	require.Equal(t, []uint{first, second}, storedThreadArticleIDs(t, db, threadID))
}

// TestGetReportByID_LinkQueryFailureKeepsOriginalIDs 覆盖可用性变体「错误态」：
// 折叠所需的 link 查询失败时降级为原样返回——详情接口不报错、不折叠。
func TestGetReportByID_LinkQueryFailureKeepsOriginalIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	sectionID := seedTestSection(t, db, reportID, "degraded section")

	copyA := seedThreadCopyArticle(t, db, 9201, "稿件", "https://example.com/same-story")
	copyB := seedThreadCopyArticle(t, db, 9202, "稿件", "https://example.com/same-story")
	seedThreadCopyThread(t, db, reportID, sectionID, "降级线索", []uint{copyA, copyB})

	// 只让折叠用的 articles 查询失败（GetReportByID 的其它读路径不查 articles）。
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:injected_articles_query_fail", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "articles" {
			_ = tx.AddError(errors.New("injected article link query failure"))
		}
	}))
	defer func() {
		require.NoError(t, db.Callback().Query().Remove("test:injected_articles_query_fail"))
	}()

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)
	require.Len(t, report.Sections[0].Threads, 1)
	require.Equal(t, []uint{copyA, copyB}, jsonUintIDs(report.Sections[0].Threads[0].RelatedArticleIDs))
}

// TestGetReportByID_KeepsDanglingArticleIDReference 覆盖变体 #11 的集成层
// （越界引用）：thread 的 related_article_ids 含一个库里查不到的 article id
// 时，该 id 原样保留在响应里、详情接口不报错，其余元素照常折叠。
func TestGetReportByID_KeepsDanglingArticleIDReference(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	sectionID := seedTestSection(t, db, reportID, "dangling section")

	copyA := seedThreadCopyArticle(t, db, 9301, "稿件", "https://example.com/same-story")
	copyB := seedThreadCopyArticle(t, db, 9302, "稿件", "https://example.com/same-story")
	dangling := uint(987654) // 库里不存在的 article id（越界引用）
	threadID := seedThreadCopyThread(t, db, reportID, sectionID, "越界引用线索",
		[]uint{dangling, copyA, copyB})

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)
	require.Len(t, report.Sections[0].Threads, 1)
	// 越界 id 原样保留（内容身份不可判定 → 不参与折叠）；同 link 副本照常折叠。
	require.Equal(t, []uint{dangling, copyA},
		jsonUintIDs(report.Sections[0].Threads[0].RelatedArticleIDs))
	// 落库值仍是原始三个 id（读路径折叠不改库，红线 45）。
	require.Equal(t, []uint{dangling, copyA, copyB}, storedThreadArticleIDs(t, db, threadID))
}

// TestGetReportByID_LinkQueryFailureThenRetryFolds 覆盖变体 #16「部分失败后重试」：
// link 查询失败的那一次降级为不折叠；移除注入后重试必须恢复正常折叠——失败
// 不污染状态（折叠无共享可变状态，每次读路径都重新加载）。
func TestGetReportByID_LinkQueryFailureThenRetryFolds(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	sectionID := seedTestSection(t, db, reportID, "retry section")

	copyA := seedThreadCopyArticle(t, db, 9401, "稿件", "https://example.com/same-story")
	copyB := seedThreadCopyArticle(t, db, 9402, "稿件", "https://example.com/same-story")
	seedThreadCopyThread(t, db, reportID, sectionID, "重试线索", []uint{copyA, copyB})

	// 第一次调用：只让折叠用的 articles 查询失败 → 降级为不折叠。
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:injected_articles_query_fail_retry", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "articles" {
			_ = tx.AddError(errors.New("injected article link query failure"))
		}
	}))

	degraded, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, degraded.Sections, 1)
	require.Len(t, degraded.Sections[0].Threads, 1)
	require.Equal(t, []uint{copyA, copyB},
		jsonUintIDs(degraded.Sections[0].Threads[0].RelatedArticleIDs))

	// 移除注入后重试：行为恢复正常（已折叠）。
	require.NoError(t, db.Callback().Query().Remove("test:injected_articles_query_fail_retry"))

	retried, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, retried.Sections, 1)
	require.Len(t, retried.Sections[0].Threads, 1)
	require.Equal(t, []uint{copyA},
		jsonUintIDs(retried.Sections[0].Threads[0].RelatedArticleIDs))
}

func seedTestBoard(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	board := models.SemanticLabel{
		Label:     "test-board",
		Slug:      "test-board",
		LabelType: "board",
		Status:    "active",
	}
	err := db.Create(&board).Error
	require.NoError(t, err)
	return board.ID
}

func seedTestReport(t *testing.T, db *gorm.DB, boardID uint, date time.Time) uint {
	t.Helper()
	// The (semantic_board_id, period_date) unique index (offline-catchup H2)
	// makes repeat calls with the same board+day illegal. Tests that want
	// several reports on "the same day" only care about the section rows, so
	// nudge the day forward until the slot is free. String equality matches
	// SaveReport's own lookup (the PG column is a DATE).
	day := NormalizeReportDate(date)
	for {
		var taken int64
		require.NoError(t, db.Model(&BoardDailyReport{}).
			Where("semantic_board_id = ? AND period_date = ?", boardID, day.Format("2006-01-02")).
			Count(&taken).Error)
		if taken == 0 {
			break
		}
		day = day.AddDate(0, 0, 1)
	}
	report := BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      day,
		Title:           "Test Report",
		Summary:         "Test Summary",
		Status:          "completed",
	}
	err := db.Create(&report).Error
	require.NoError(t, err)
	return report.ID
}

func seedTestSection(t *testing.T, db *gorm.DB, reportID uint, label string) uint {
	t.Helper()
	// daily_report_sections.embedding is a non-nullable `vector` column (model field
	// is `string`, zero value ""); pgvector rejects the empty string. Production
	// always populates Embedding via FloatsToPgVector before insert, so mirror
	// that here. The column is untyped-dimension in the test DB (runtime
	// EnsureVectorDimensionOnce is never invoked), so a 1-dim vector is accepted.
	section := DailyReportSection{
		ReportID:     reportID,
		ClusterLabel: label,
		ArticleCount: 1,
		Embedding:    FloatsToPgVector([]float64{0}),
	}
	err := db.Create(&section).Error
	require.NoError(t, err)
	return section.ID
}

func seedTestRelation(t *testing.T, db *gorm.DB, fromID, toID uint, distance float64) {
	t.Helper()
	rel := SectionRelation{
		FromSectionID: fromID,
		ToSectionID:   toID,
		Distance:      distance,
	}
	err := db.Create(&rel).Error
	require.NoError(t, err)
}

func TestGetSectionLifecycle_MultiHopChain(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -2))
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	r3 := seedTestReport(t, db, boardID, now)

	// #40 -> #50 -> #60
	s40 := seedTestSection(t, db, r1, "section-40")
	s50 := seedTestSection(t, db, r2, "section-50")
	s60 := seedTestSection(t, db, r3, "section-60")

	seedTestRelation(t, db, s40, s50, 0.8)
	seedTestRelation(t, db, s50, s60, 0.7)

	// Query from s60 should return all 3 sections
	resp, err := repo.GetSectionLifecycle(s60)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 3)
	require.Len(t, resp.Relations, 2)
}

func TestGetSectionLifecycle_Fork(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -2))
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	// #40 -> #50, #50 -> #60, #50 -> #61
	s40 := seedTestSection(t, db, r1, "section-40")
	s50 := seedTestSection(t, db, r2, "section-50")
	s60 := seedTestSection(t, db, r2, "section-60")
	s61 := seedTestSection(t, db, r2, "section-61")

	seedTestRelation(t, db, s40, s50, 0.8)
	seedTestRelation(t, db, s50, s60, 0.7)
	seedTestRelation(t, db, s50, s61, 0.6)

	// Query from s60 should return all 4 sections and 3 relations
	resp, err := repo.GetSectionLifecycle(s60)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 4)
	require.Len(t, resp.Relations, 3)
}

func TestGetSectionLifecycle_Merge(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -2))
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	// #50 -> #70, #55 -> #70, #70 -> #80
	s50 := seedTestSection(t, db, r1, "section-50")
	s55 := seedTestSection(t, db, r1, "section-55")
	s70 := seedTestSection(t, db, r2, "section-70")
	s80 := seedTestSection(t, db, r2, "section-80")

	seedTestRelation(t, db, s50, s70, 0.8)
	seedTestRelation(t, db, s55, s70, 0.7)
	seedTestRelation(t, db, s70, s80, 0.6)

	// Query from s50 should return all 4 sections and 3 relations
	resp, err := repo.GetSectionLifecycle(s50)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 4)
	require.Len(t, resp.Relations, 3)
}

func TestGetSectionLifecycle_IsolatedSection(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now)
	s20 := seedTestSection(t, db, r1, "section-20")

	// Isolated section: no relations
	resp, err := repo.GetSectionLifecycle(s20)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 1)
	require.Equal(t, s20, resp.Sections[0].ID)
	require.Len(t, resp.Relations, 0)
}

func TestGetSectionLifecycle_Cycle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now)

	// #50 -> #60 -> #70 -> #50 (cycle)
	s50 := seedTestSection(t, db, r1, "section-50")
	s60 := seedTestSection(t, db, r1, "section-60")
	s70 := seedTestSection(t, db, r1, "section-70")

	seedTestRelation(t, db, s50, s60, 0.8)
	seedTestRelation(t, db, s60, s70, 0.7)
	seedTestRelation(t, db, s70, s50, 0.6)

	// Should return all 3 sections once and 3 relations
	resp, err := repo.GetSectionLifecycle(s50)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 3)
	require.Len(t, resp.Relations, 3)
}

func TestGetSectionLifecycle_ExternalRelationIsolation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now)

	// Component A: #50 -> #60 -> #70
	s50 := seedTestSection(t, db, r1, "section-50")
	s60 := seedTestSection(t, db, r1, "section-60")
	s70 := seedTestSection(t, db, r1, "section-70")
	// Component B: #80 (isolated)
	s80 := seedTestSection(t, db, r1, "section-80")

	seedTestRelation(t, db, s50, s60, 0.8)
	seedTestRelation(t, db, s60, s70, 0.7)

	// Query from s50: should NOT include s80 or any relation involving s80
	resp, err := repo.GetSectionLifecycle(s50)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 3)
	require.Len(t, resp.Relations, 2)

	// Verify no relation references s80
	for _, rel := range resp.Relations {
		require.NotEqual(t, s80, rel.FromID)
		require.NotEqual(t, s80, rel.ToID)
	}
}

// =============================================================================
// ListTopicRecentBriefs tests (Slice D: lane context injection)
// =============================================================================

func seedTestActiveTopic(t *testing.T, db *gorm.DB, boardID uint, label string, lastSeen time.Time) uint {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          TopicStatusActive,
		FirstSeenDate:   lastSeen,
		LastSeenDate:    lastSeen,
		HitCount:        1,
	}
	err := db.Create(&topic).Error
	require.NoError(t, err)
	return topic.ID
}

func seedTestCandidateTopic(t *testing.T, db *gorm.DB, boardID uint, label string, lastSeen time.Time) uint {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          TopicStatusCandidate,
		FirstSeenDate:   lastSeen,
		LastSeenDate:    lastSeen,
		HitCount:        1,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic.ID
}

// seedTestTopicTag creates a topic_tags row with the given label/status.
func seedTestTopicTag(t *testing.T, db *gorm.DB, label string, status string) uint {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: "slug-" + label, Status: status, Category: "event"}
	require.NoError(t, db.Create(&tag).Error)
	return tag.ID
}

// seedTestSectionWithTopic creates a section whose ClusterTagIDs reference the
// given tag ids (fact fingerprint source). Pass no tagIDs for a NULL
// cluster_tag_ids column; pass JSON{} (empty) via seedEmptyTagIDs for an empty
// JSON array.
func seedTestSectionWithTopic(t *testing.T, db *gorm.DB, reportID uint, label string, topicID uint, tagIDs ...uint) uint {
	t.Helper()
	section := DailyReportSection{
		ReportID:          reportID,
		ClusterLabel:      label,
		ArticleCount:      1,
		Embedding:         FloatsToPgVector([]float64{0}),
		PersistentTopicID: &topicID,
	}
	if tagIDs != nil {
		raw, err := json.Marshal(tagIDs)
		require.NoError(t, err)
		section.ClusterTagIDs = JSON(raw)
	}
	require.NoError(t, db.Create(&section).Error)
	return section.ID
}

// TestListTopicRecentBriefs_TagLabelInjection covers spec scenario
// 「candidate 话题近期内容注入」/「L2 候选预筛注入」的 repo 侧：注入内容 SHALL 为
// section 当天实际 tag 标签（cluster_tag_ids 解析出的 topic_tags.label），
// 而非被话题 label 硬覆盖的 cluster_label（零信息复读）。
func TestListTopicRecentBriefs_TagLabelInjection(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "AI 编程工具平台化竞争", now)
	tagA := seedTestTopicTag(t, db, "Codex 推出第三方模型接入", "active")
	tagB := seedTestTopicTag(t, db, "GitHub Copilot X 发布新功能", "active")

	reportID := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	sectionID := seedTestSectionWithTopic(t, db, reportID, "开发者 Agent 平台化", topicID, tagA, tagB)

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1)
	require.Contains(t, briefs, topicID)

	items := briefs[topicID]
	require.Len(t, items, 1)
	assert.Equal(t, sectionID, items[0].SectionID)
	// Fact fingerprint: actual tag labels, in cluster_tag_ids order.
	assert.Equal(t, []string{"Codex 推出第三方模型接入", "GitHub Copilot X 发布新功能"}, items[0].TagLabels)
	// The frozen cluster_label SHALL NOT be the injected content anymore.
	assert.NotContains(t, items[0].TagLabels, "开发者 Agent 平台化")
}

func TestListTopicRecentBriefs_SevenDayWindow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "以黎冲突升级", now)
	tag1 := seedTestTopicTag(t, db, "真主党越境打击", "active")
	tag2 := seedTestTopicTag(t, db, "以军空袭黎南部", "active")

	// Section within 7 days
	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -3))
	s1 := seedTestSectionWithTopic(t, db, r1, "真主党越境打击", topicID, tag1)

	// Section outside 7 days (10 days ago)
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -10))
	_ = seedTestSectionWithTopic(t, db, r2, "以军空袭黎南部", topicID, tag2)

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1)

	// Only the 3-day-ago section should be present (7-day window)
	items := briefs[topicID]
	assert.Len(t, items, 1)
	assert.Equal(t, s1, items[0].SectionID)
	assert.Equal(t, []string{"真主党越境打击"}, items[0].TagLabels)
}

func TestListTopicRecentBriefs_PerTopicLimit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "AI 算力生态", now)
	tagID := seedTestTopicTag(t, db, "算力扩产", "active")

	// Create 6 sections (limit is 5), descending by date; keep their ids so the
	// per-topic trim (newest-first) can be asserted by SectionID.
	sectionIDs := make([]uint, 0, 6)
	for i := 0; i < 6; i++ {
		r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -(i+1)))
		s := seedTestSectionWithTopic(t, db, r, fmt.Sprintf("section-%d", i+1), topicID, tagID)
		sectionIDs = append(sectionIDs, s)
	}

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1)

	// Should be truncated to 5 newest sections (section-1..5, newest first).
	items := briefs[topicID]
	assert.Len(t, items, 5, "per-topic limit = 5")
	gotIDs := make([]uint, 0, 5)
	for _, item := range items {
		gotIDs = append(gotIDs, item.SectionID)
	}
	assert.Equal(t, sectionIDs[:5], gotIDs, "newest 5 sections kept, newest first")
}

// TestListTopicRecentBriefs_PerSectionTagCap asserts the per-section tag-label
// cap: a section whose cluster_tag_ids lists 7 tags yields at most 5 labels,
// preserving the cluster's own array order.
func TestListTopicRecentBriefs_PerSectionTagCap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "大模型监管与安全", now)
	tagIDs := make([]uint, 0, 7)
	for i := 0; i < 7; i++ {
		tagIDs = append(tagIDs, seedTestTopicTag(t, db, fmt.Sprintf("tag-%d", i+1), "active"))
	}
	r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	seedTestSectionWithTopic(t, db, r, "many-tags", topicID, tagIDs...)

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs[topicID], 1)
	assert.Len(t, briefs[topicID][0].TagLabels, 5, "per-section tag cap = 5")
	assert.Equal(t, []string{"tag-1", "tag-2", "tag-3", "tag-4", "tag-5"}, briefs[topicID][0].TagLabels,
		"cap keeps the cluster's own tag order")
}

// TestListTopicRecentBriefs_MergedDisabledTagsFiltered asserts that
// merged/disabled tags (topic_tags.status != 'active') are filtered out of the
// injected labels while the remaining active labels survive in order.
func TestListTopicRecentBriefs_MergedDisabledTagsFiltered(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "中东局势", now)
	tagA := seedTestTopicTag(t, db, "active-tag", "active")
	tagM := seedTestTopicTag(t, db, "merged-tag", "merged")
	r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	// Array order: A, M — M filtered, A kept.
	seedTestSectionWithTopic(t, db, r, "mixed", topicID, tagA, tagM)

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs[topicID], 1)
	assert.Equal(t, []string{"active-tag"}, briefs[topicID][0].TagLabels,
		"merged/disabled tag labels dropped from injection")
}

// TestListTopicRecentBriefs_CandidateIncluded covers spec scenario
// 「candidate 话题近期内容注入」（candidate-topic-l2-gate）：candidate 话题现流经
// L2 裁决，briefs 注入范围 SHALL 覆盖 active 与 candidate 两类。
func TestListTopicRecentBriefs_CandidateIncluded(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	candidateID := seedTestCandidateTopic(t, db, boardID, "候选话题", now)
	tagID := seedTestTopicTag(t, db, "candidate-section-tag", "active")

	reportID := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	sectionID := seedTestSectionWithTopic(t, db, reportID, "candidate-section", candidateID, tagID)

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1, "candidate topic included in briefs")
	require.Contains(t, briefs, candidateID)
	require.Len(t, briefs[candidateID], 1)
	assert.Equal(t, sectionID, briefs[candidateID][0].SectionID)
	assert.Equal(t, []string{"candidate-section-tag"}, briefs[candidateID][0].TagLabels)
}

// TestListTopicRecentBriefs_ExcludesTodaySections asserts the same-day
// exclusion: sections from today's report SHALL NOT be injected as "recent
// content" — otherwise an earlier run of today (or a same-day rerun) that
// mis-attached tags would self-corroborate as briefs evidence
// （卡里巴夫同日重跑自证回路根因）。次日运行时昨日 section 才作为证据。
func TestListTopicRecentBriefs_ExcludesTodaySections(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "观察期话题", now)
	todayTag := seedTestTopicTag(t, db, "今日误挂 tag", "active")
	ydayTag := seedTestTopicTag(t, db, "昨日事实 tag", "active")

	yesterday := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	ydaySection := seedTestSectionWithTopic(t, db, yesterday, "昨日 section", topicID, ydayTag)
	today := seedTestReport(t, db, boardID, now)
	seedTestSectionWithTopic(t, db, today, "今日 section", topicID, todayTag)

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	require.Contains(t, briefs, topicID)
	require.Len(t, briefs[topicID], 1, "only yesterday's section is injected")
	assert.Equal(t, ydaySection, briefs[topicID][0].SectionID)
	assert.Equal(t, []string{"昨日事实 tag"}, briefs[topicID][0].TagLabels,
		"today's mis-attached tag must not self-corroborate")
}

func TestListTopicRecentBriefs_NoTopicsNil(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	assert.Nil(t, briefs)
}

func TestListTopicRecentBriefs_MultipleTopics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicA := seedTestActiveTopic(t, db, boardID, "Topic A", now)
	topicB := seedTestActiveTopic(t, db, boardID, "Topic B", now)
	tagA := seedTestTopicTag(t, db, "tag-a", "active")
	tagB := seedTestTopicTag(t, db, "tag-b", "active")

	r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	seedTestSectionWithTopic(t, db, r, "Section A", topicA, tagA)
	seedTestSectionWithTopic(t, db, r, "Section B", topicB, tagB)

	briefs, err := repo.ListTopicRecentBriefs(boardID, now, 7, 5)
	require.NoError(t, err)
	assert.Len(t, briefs, 2)
	assert.Contains(t, briefs, topicA)
	assert.Contains(t, briefs, topicB)
	assert.Len(t, briefs[topicA], 1)
	assert.Len(t, briefs[topicB], 1)
	assert.Equal(t, []string{"tag-a"}, briefs[topicA][0].TagLabels)
	assert.Equal(t, []string{"tag-b"}, briefs[topicB][0].TagLabels)
}

func TestListReports_AttachesUniqueActiveWatchSummaries(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	newerReportID := seedTestReport(t, db, boardID, now)
	olderReportID := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	active, err := repo.CreateWatch(CreateWatchInput{SemanticBoardID: boardID, Label: "ASML", Type: WatchTypeKeyword})
	require.NoError(t, err)
	paused, err := repo.CreateWatch(CreateWatchInput{SemanticBoardID: boardID, Label: "paused watch", Type: WatchTypeLabel})
	require.NoError(t, err)
	pausedStatus := WatchStatusPaused
	_, err = repo.UpdateWatch(paused.ID, nil, nil, &pausedStatus)
	require.NoError(t, err)
	deleted, err := repo.CreateWatch(CreateWatchInput{SemanticBoardID: boardID, Label: "deleted watch", Type: WatchTypeLabel})
	require.NoError(t, err)

	for _, sectionID := range []uint{101, 102} {
		require.NoError(t, db.Create(&TopicWatchHit{
			WatchID: active.ID, SectionID: sectionID, ReportID: newerReportID,
			PeriodDate: now, Reason: "active hit",
		}).Error)
	}
	require.NoError(t, db.Create(&TopicWatchHit{
		WatchID: paused.ID, SectionID: 103, ReportID: newerReportID,
		PeriodDate: now, Reason: "paused hit",
	}).Error)
	require.NoError(t, db.Create(&TopicWatchHit{
		WatchID: deleted.ID, SectionID: 104, ReportID: olderReportID,
		PeriodDate: now.AddDate(0, 0, -1), Reason: "deleted hit",
	}).Error)
	require.NoError(t, repo.DeleteWatch(deleted.ID))

	items, err := repo.ListReports(boardID, 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, newerReportID, items[0].ID)
	assert.Equal(t, olderReportID, items[1].ID)
	require.Len(t, items[0].ActiveWatchSummaries, 1)
	assert.Equal(t, ActiveWatchSummary{WatchID: active.ID, Label: "ASML", Type: WatchTypeKeyword}, items[0].ActiveWatchSummaries[0])
	assert.Empty(t, items[1].ActiveWatchSummaries)
}

// ---- highlights read-path normalization (fix-daily-report-highlights-null) ----

// TestGetReportByID_NormalizesNullHighlights (B14): historical rows hold a
// JSON null scalar (or SQL NULL) in the highlights column. The read path
// normalizes both shapes to an empty array — the API must never answer
// "highlights": null (the frontend would hide the whole "今日重点" block).
func TestGetReportByID_NormalizesNullHighlights(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	// Row 1: highlights holds the JSON null scalar (the pre-2026-11-04 write
	// path marshalled a nil slice into "null").
	nullScalarID := seedTestReport(t, db, boardID, now)
	require.NoError(t, db.Exec(
		"UPDATE board_daily_reports SET highlights = 'null'::jsonb WHERE id = ?", nullScalarID).Error)

	// Row 2: highlights column left empty → SQL NULL.
	nullColumnID := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	for _, id := range []uint{nullScalarID, nullColumnID} {
		report, err := repo.GetReportByID(id)
		require.NoError(t, err)
		assert.Equal(t, JSON("[]"), report.Highlights, "report %d: highlights must be normalized to []", id)

		data, err := json.Marshal(report)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"highlights":[]`)
		assert.NotContains(t, string(data), `"highlights":null`)
	}
}

// TestGetReportByID_KeepsHighlightsArray：真实 highlights 数组原样透出（条数
// 与内容不变）；read path 只补历史 null→[] 归一，不回写、不改写快照数值。
// 事件线身份（story_id / story_heat / is_progress）整体退役，读取时不再解析。
func TestDailyReportHighlightsSnapshotRead(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	// 合法聚簇引用 = 本版块的一个 event 标签（cluster_id 是代表标签 id）。
	eventTag := models.TopicTag{Label: "同事件标签", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&eventTag).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: eventTag.ID, SemanticBoardID: boardID}).Error)

	storedHighlights := `[{"title":"要闻一","reason":"理由一","tag_ids":[1]},` +
		`{"cluster_id":` + fmt.Sprint(eventTag.ID) + `,"article_count":3,"cluster_mode":"向量聚簇","data_as_of":"2026-11-06","title":"要闻二","reason":"理由二","tag_ids":[2]},` +
		`{"title":"要闻三","reason":"理由三","tag_ids":[1,2]}]`
	require.NoError(t, db.Exec(
		"UPDATE board_daily_reports SET highlights = ?::jsonb WHERE id = ?", storedHighlights, reportID).Error)

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	var decoded []Highlight
	require.NoError(t, json.Unmarshal(report.Highlights, &decoded))
	require.Len(t, decoded, 3)

	// 首条历史（无 cluster）重点保持无快照 —— 不伪造聚簇身份或篇数。
	require.Nil(t, decoded[0].ClusterID)
	require.Nil(t, decoded[0].ArticleCount)
	require.Empty(t, decoded[0].ClusterMode)
	// 新重点的快照逐字保留。
	require.NotNil(t, decoded[1].ClusterID)
	require.Equal(t, eventTag.ID, *decoded[1].ClusterID)
	require.NotNil(t, decoded[1].ArticleCount)
	require.Equal(t, 3, *decoded[1].ArticleCount)
	require.Equal(t, "向量聚簇", decoded[1].ClusterMode)
	require.Equal(t, "2026-11-06", decoded[1].DataAsOf)

	// 退役的事件线字段不再出现在响应里。
	payload := string(report.Highlights)
	require.NotContains(t, payload, "story_id")
	require.NotContains(t, payload, "story_heat")
	require.NotContains(t, payload, "is_progress")
}

// TestGetReportByID_RetiresThreadStoryIdentity（2.3）：历史 thread 的 story_id
// 只读保留在库中，API SHALL NOT 对外透出事件线身份；坏 cluster 引用读取时
// 置空（同版块一跳解析），且不回写历史 JSON。
func TestDailyReportRetiresThreadStoryIdentity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	sectionID := seedTestSection(t, db, reportID, "section")
	story := BoardStory{BoardID: boardID, Title: "历史事件线", FirstSeenDate: now, LastSeenDate: now}
	require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&story).Error)
	storyID := story.ID
	require.NoError(t, db.Omit("Embedding").Create(&DailyReportThread{
		ReportID: reportID, SectionID: sectionID, Title: "历史事件",
		StoryID: &storyID, StoryManual: true, DisplayTier: ThreadDisplayTierFlash,
	}).Error)
	// 坏 cluster 引用（标签不存在）+ 合法引用（本版块 event 标签）。
	require.NoError(t, db.Exec(
		"UPDATE board_daily_reports SET highlights = ?::jsonb WHERE id = ?",
		`[{"cluster_id":999999,"title":"坏引用","reason":"r","tag_ids":[]},{"cluster_id":0,"title":"零","reason":"r","tag_ids":[]}]`, reportID).Error)

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)

	// thread 的 story_id 不出现在 JSON 里（结构体 json:"-"）。
	data, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(data), "story_id")

	// 历史值仍在库中（只读保留，不被清空）。
	var stored DailyReportThread
	require.NoError(t, db.Where("report_id = ?", reportID).First(&stored).Error)
	require.NotNil(t, stored.StoryID)
	require.Equal(t, storyID, *stored.StoryID)
	require.True(t, stored.StoryManual)
	require.Equal(t, ThreadDisplayTierFlash, stored.DisplayTier)

	// 坏 cluster 引用读取时置空。
	var highlights []Highlight
	require.NoError(t, json.Unmarshal(report.Highlights, &highlights))
	require.Len(t, highlights, 2)
	require.Nil(t, highlights[0].ClusterID, "不存在的聚簇引用必须置空")
	require.Nil(t, highlights[1].ClusterID, "0 不是合法聚簇身份")
}

// TestDailyReportSaveThreadsRetiresStoryIdentity（2.3）：新生成 / 重建的日报
// 恒写 story_id=NULL、story_manual=false、display_tier=body——即使调用方传入
// 残留值也不得落库；scope 仍由 tag_ids 中 event 标签数推导（≥2 = composite）。
func TestDailyReportSaveThreadsRetiresStoryIdentity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	t.Cleanup(func() { Repo = nil })

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	report := &BoardDailyReport{SemanticBoardID: boardID, PeriodDate: now, Title: "t", Status: "completed", Highlights: JSON("[]")}

	eventA := models.TopicTag{Label: "事件甲", Category: models.TagCategoryEvent, Status: "active"}
	eventB := models.TopicTag{Label: "事件乙", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&eventA).Error)
	require.NoError(t, db.Create(&eventB).Error)

	// 调用方显式传入历史残留值：插入路径必须清零。
	staleStoryID := uint(777)
	threads := [][]DailyReportThread{{
		{
			Title:       "残留身份线程",
			TagIDs:      JSON(mustJSONIDs([]uint{eventA.ID})),
			StoryID:     &staleStoryID,
			StoryManual: true,
			DisplayTier: ThreadDisplayTierFlash,
		},
		{
			Title:  "综合稿",
			TagIDs: JSON(mustJSONIDs([]uint{eventA.ID, eventB.ID})),
		},
	}}
	require.NoError(t, repo.SaveReport(report, []DailyReportSection{{ClusterLabel: "s"}}, threads))

	var stored []DailyReportThread
	require.NoError(t, db.Where("report_id = ?", report.ID).Order("id ASC").Find(&stored).Error)
	require.Len(t, stored, 2)
	for _, thread := range stored {
		require.Nil(t, thread.StoryID, "新日报的 story_id 恒 NULL")
		require.False(t, thread.StoryManual, "story_manual 恒 false")
		require.Equal(t, ThreadDisplayTierBody, thread.DisplayTier, "display_tier 恒 body")
	}
	require.Equal(t, ThreadScopeSingle, stored[0].Scope, "单 event 标签 = single")
	require.Equal(t, ThreadScopeComposite, stored[1].Scope, "≥2 event 标签 = composite")

	// highlights 落库恒为数组（空重点写 []，不写 null）。
	updated, err := repo.FinalizeReportHighlights(report, nil)
	require.NoError(t, err)
	require.True(t, updated)
	var payload string
	require.NoError(t, db.Raw("SELECT highlights::text FROM board_daily_reports WHERE id = ?", report.ID).Scan(&payload).Error)
	require.JSONEq(t, `[]`, payload)
}

// TestDailyReportRerunKeepsRetiredStoryData（review R-1/R-2）：硬停写后重跑一期
// 历史日报不得改写已退役的派生数据——story_signals 照旧保留、
// board_stories.participant_count 不被重算（旧实现会在提交后重算这两项）。
func TestDailyReportRerunKeepsRetiredStoryData(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	t.Cleanup(func() { Repo = nil })

	boardID := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	reportID := seedTestReport(t, db, boardID, day)
	sectionID := seedTestSection(t, db, reportID, "section")

	// 历史身份与派生存量：一条真事件线 + 一行证据 + 一个与其证据不相符的
	// participant_count（旧实现重算后会把它改成 0 并删掉证据）。
	story := BoardStory{BoardID: boardID, Title: "历史事件线", FirstSeenDate: day, LastSeenDate: day, ParticipantCount: 7}
	require.NoError(t, db.Omit("Embedding", "SubjectEmbedding").Create(&story).Error)
	storyID := story.ID
	require.NoError(t, db.Omit("Embedding").Create(&DailyReportThread{
		ReportID: reportID, SectionID: sectionID, Title: "历史事件", StoryID: &storyID,
	}).Error)
	// story_signals.article_id 有外键，证据文章必须真实存在。
	evidence := models.Article{FeedID: 9, Title: "历史证据", Link: "https://example.com/history"}
	require.NoError(t, db.Create(&evidence).Error)
	require.NoError(t, db.Create(&StorySignal{
		StoryID: storyID, ArticleID: evidence.ID, FeedID: 9, ParticipantKey: "feed:9",
		Kind: StorySignalKindEditorial, ObservedAt: day, ObservedAtSource: "fallback",
	}).Error)

	// 同日重跑：整份覆盖重建（threads/sections 删后重插）。
	report := &BoardDailyReport{SemanticBoardID: boardID, PeriodDate: day, Title: "重建", Status: "completed", Highlights: JSON("[]")}
	require.NoError(t, repo.SaveReport(report, []DailyReportSection{{ClusterLabel: "s"}}, [][]DailyReportThread{{{Title: "重建线索"}}}))
	require.Equal(t, reportID, report.ID)

	var signals int64
	require.NoError(t, db.Model(&StorySignal{}).Where("story_id = ?", storyID).Count(&signals).Error)
	require.EqualValues(t, 1, signals, "重建历史报不得删除 / 重写 story_signals")
	var reloaded BoardStory
	require.NoError(t, db.First(&reloaded, storyID).Error)
	require.Equal(t, 7, reloaded.ParticipantCount, "重建历史报不得重算 participant_count")
}

// mustJSONIDs renders a uint slice as a JSON array literal for jsonb columns.
func mustJSONIDs(ids []uint) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// TestTopicLifelineKeepsZeroMatchDistance（fix-lane-metric-provenance 1.7/1.8）：
// 归属距离恰为 0 是合法结果（完美锚定），生命线响应不得因为 `omitempty` 把
// 该字段省略——省略后前端会把接近度读成「未知」而不是 1.00。
func TestTopicLifelineKeepsZeroMatchDistance(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)

	topic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "零距离话题", Status: TopicStatusActive, Source: TopicSourceAuto,
		Embedding:     FloatsToPgVector([]float64{0}),
		FirstSeenDate: now, LastSeenDate: now,
	}
	require.NoError(t, db.Create(&topic).Error)

	section := seedTestSection(t, db, reportID, "零距离分区")
	require.NoError(t, db.Model(&DailyReportSection{}).
		Where("id = ?", section).
		Update("persistent_topic_id", topic.ID).Error)
	require.NoError(t, db.Model(&DailyReportSection{}).
		Where("id = ?", section).
		Update("topic_match_distance", 0.0).Error)

	resp, err := repo.GetTopicLifeline(topic.ID)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 1)
	assert.Zero(t, resp.Sections[0].TopicMatchDistance)

	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"topic_match_distance":0`, "距离 0 必须显式下发，不得被省略")
}
