package sourcestats

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// 用例来源：openspec/changes/archive/2026-09-18-add-source-board-hit-rate/test-cases.md
// B1（命中口径与去重 12 条）、B2（未打标两分 8 条）、B3（窗口参数与边界 7 条）。
// 口径权威：specs/source-board-hit-rate/spec.md。
// 窗口时间用注入的固定 now（test-cases §1 的缝），全部 UTC 以保证
// sqlite 文本时间比较与 Postgres timestamptz 比较语义一致。

var testNow = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

func setupStatsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:sourcestats-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.ArticleTopicTag{},
		&models.SemanticLabel{},
		&models.TopicTagBoardLabel{},
		&models.TagJob{},
	))
	return db
}

// fixNow 注入可替换 clock（test-cases §1 前置契约：截止时间 Go 侧算）。
func fixNow(t *testing.T) {
	t.Helper()
	orig := nowFunc
	nowFunc = func() time.Time { return testNow }
	t.Cleanup(func() { nowFunc = orig })
}

func seedStatsFeed(t *testing.T, db *gorm.DB, title string, taggingEnabled bool) models.Feed {
	t.Helper()
	feed := models.Feed{
		Title:          title,
		URL:            fmt.Sprintf("https://example.com/%s-%d.xml", title, time.Now().UnixNano()),
		TaggingEnabled: taggingEnabled,
	}
	require.NoError(t, db.Select("Title", "URL", "TaggingEnabled").Create(&feed).Error)
	// gorm 的 default:true 标签会在 Create 时把零值 false 替换成标签默认值 true，
	// 因此关闭打标的源必须事后显式 UPDATE（UpdateColumn 直写、不走 hooks）。
	if !taggingEnabled {
		require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feed.ID).UpdateColumn("tagging_enabled", false).Error)
	}
	return feed
}

func seedStatsArticle(t *testing.T, db *gorm.DB, feedID uint, pubAt *time.Time, createdAt time.Time, archived bool) models.Article {
	t.Helper()
	n := time.Now().UnixNano()
	article := models.Article{
		FeedID:    feedID,
		Title:     fmt.Sprintf("a-%d", n),
		Link:      fmt.Sprintf("https://example.com/a-%d", n),
		PubDate:   pubAt,
		CreatedAt: createdAt,
		Archived:  archived,
	}
	require.NoError(t, db.Create(&article).Error)
	return article
}

func seedStatsTag(t *testing.T, db *gorm.DB, slug string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Slug: slug, Label: slug, Status: "active", IsCanonical: true}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

func seedStatsLabel(t *testing.T, db *gorm.DB, label, slug, labelType, status string) models.SemanticLabel {
	t.Helper()
	row := models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: status}
	require.NoError(t, db.Create(&row).Error)
	return row
}

func linkStatsTagBoard(t *testing.T, db *gorm.DB, tagID, boardID uint) {
	t.Helper()
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tagID, SemanticBoardID: boardID, Score: 0.9}).Error)
}

func tagStatsArticle(t *testing.T, db *gorm.DB, articleID, tagID uint) {
	t.Helper()
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: articleID, TopicTagID: tagID, Score: 0.8, Source: "llm"}).Error)
}

func seedStatsTagJob(t *testing.T, db *gorm.DB, articleID uint, status models.JobStatus) {
	t.Helper()
	job := models.TagJob{ArticleID: articleID, Status: string(status), AvailableAt: testNow, MaxAttempts: 5}
	require.NoError(t, db.Create(&job).Error)
}

func statsFor(t *testing.T, db *gorm.DB, windowDays int) map[uint]FeedStat {
	t.Helper()
	list, err := FeedBoardHitStats(context.Background(), db, windowDays)
	require.NoError(t, err)
	byFeed := make(map[uint]FeedStat, len(list))
	for _, s := range list {
		byFeed[s.FeedID] = s
	}
	return byFeed
}

// ── B3-03/04/05：ParseWindow 白名单（纯逻辑单元）──

func TestParseWindowWhitelist(t *testing.T) {
	// TC-B3-03：不传 window → 等同 7。
	got, err := ParseWindow("")
	require.NoError(t, err)
	require.Equal(t, 7, got)

	for _, raw := range []string{"7", "30", "90"} {
		got, err := ParseWindow(raw)
		require.NoError(t, err, raw)
		require.Equal(t, raw, fmt.Sprintf("%d", got))
	}

	// TC-B3-04 / TC-B3-05：非法值 → 错误（端点映射 400），绝不静默回退默认。
	for _, raw := range []string{"14", "0", "abc", "-7", "7.5", "7x"} {
		_, err := ParseWindow(raw)
		require.Error(t, err, raw)
		require.ErrorIs(t, err, ErrInvalidWindow, raw)
	}
}

// ── B1：命中口径与去重 ──

// TC-B1-01：3 个标签、其中 2 个分别挂到 active 板块 P、Q → in_board 计 1，
// boards[P]=boards[Q]=1（合计 2 > in_board 1）。
func TestB1_01_MultiTagMultiBoardCountsOnce(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	boardP := seedStatsLabel(t, db, "P", "p", "board", "active")
	boardQ := seedStatsLabel(t, db, "Q", "q", "board", "active")
	t1 := seedStatsTag(t, db, "t1")
	t2 := seedStatsTag(t, db, "t2")
	t3 := seedStatsTag(t, db, "t3")
	linkStatsTagBoard(t, db, t1.ID, boardP.ID)
	linkStatsTagBoard(t, db, t2.ID, boardQ.ID)
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	tagStatsArticle(t, db, article.ID, t1.ID)
	tagStatsArticle(t, db, article.ID, t2.ID)
	tagStatsArticle(t, db, article.ID, t3.ID) // 第 3 个标签不挂板块

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(1), stats.Articles)
	require.Equal(t, int64(1), stats.InBoard)
	require.Len(t, stats.Boards, 2)
	perBoard := map[uint]int64{}
	var sumBoards int64
	for _, b := range stats.Boards {
		perBoard[b.BoardID] = b.Articles
		sumBoards += b.Articles
	}
	require.Equal(t, int64(1), perBoard[boardP.ID])
	require.Equal(t, int64(1), perBoard[boardQ.ID])
	require.GreaterOrEqual(t, sumBoards, stats.InBoard)
}

// TC-B1-02：3 个标签全部只挂同一板块 P → in_board 计 1；boards[P]=1。
func TestB1_02_MultiTagsSameBoardCountOnce(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	boardP := seedStatsLabel(t, db, "P", "p", "board", "active")
	t1 := seedStatsTag(t, db, "t1")
	t2 := seedStatsTag(t, db, "t2")
	t3 := seedStatsTag(t, db, "t3")
	for _, tagID := range []uint{t1.ID, t2.ID, t3.ID} {
		linkStatsTagBoard(t, db, tagID, boardP.ID)
	}
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	for _, tagID := range []uint{t1.ID, t2.ID, t3.ID} {
		tagStatsArticle(t, db, article.ID, tagID)
	}

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(1), stats.InBoard)
	require.Len(t, stats.Boards, 1)
	require.Equal(t, int64(1), stats.Boards[0].Articles)
}

// TC-B1-03：标签只挂到 status='disabled' 的板块 → 不计命中，归 tagged_no_board。
func TestB1_03_DisabledBoardIsNotHit(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	disabled := seedStatsLabel(t, db, "D", "d", "board", "disabled")
	tag := seedStatsTag(t, db, "t")
	linkStatsTagBoard(t, db, tag.ID, disabled.ID)
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	tagStatsArticle(t, db, article.ID, tag.ID)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(0), stats.InBoard)
	require.Equal(t, int64(1), stats.TaggedNoBoard)
	require.Empty(t, stats.Boards)
}

// TC-B1-04：标签只挂到 auxiliary / composite 记录 → 不计命中（不是板块）。
func TestB1_04_AuxiliaryAndCompositeAreNotBoards(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	aux := seedStatsLabel(t, db, "aux", "aux", "auxiliary", "active")
	comp := seedStatsLabel(t, db, "comp", "comp", "composite", "active")
	t1 := seedStatsTag(t, db, "t1")
	t2 := seedStatsTag(t, db, "t2")
	linkStatsTagBoard(t, db, t1.ID, aux.ID)
	linkStatsTagBoard(t, db, t2.ID, comp.ID)
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	tagStatsArticle(t, db, article.ID, t1.ID)
	tagStatsArticle(t, db, article.ID, t2.ID)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(0), stats.InBoard)
	require.Equal(t, int64(1), stats.TaggedNoBoard)
	require.Empty(t, stats.Boards)
}

// TC-B1-05：有标签但 topic_tag_board_labels 无行 → tagged_no_board，不计命中。
func TestB1_05_TagWithoutBoardLinkIsTaggedNoBoard(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	tag := seedStatsTag(t, db, "t")
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	tagStatsArticle(t, db, article.ID, tag.ID)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(0), stats.InBoard)
	require.Equal(t, int64(1), stats.TaggedNoBoard)
}

// TC-B1-06：无标签文章 → 归入 untagged 两分之一（按任务状态，详见 B2 组）。
func TestB1_06_UntaggedArticleGoesToUntaggedBuckets(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	seedStatsTagJob(t, db, article.ID, models.JobStatusCompleted)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(0), stats.InBoard)
	require.Equal(t, int64(0), stats.TaggedNoBoard)
	require.Equal(t, int64(0), stats.UntaggedPending)
	require.Equal(t, int64(1), stats.UntaggedSettled)
}

// TC-B1-07：窗口内 100 篇、80 篇归档、仅 5 篇命中且全在归档区 →
// articles=100、in_board=5、hit_rate=0.05（MUST NOT 为 0）。
func TestB1_07_ArchivedHitsCounted(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	boardP := seedStatsLabel(t, db, "P", "p", "board", "active")
	hitTag := seedStatsTag(t, db, "hit")
	linkStatsTagBoard(t, db, hitTag.ID, boardP.ID)

	for i := 0; i < 100; i++ {
		archived := i < 80
		article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), archived)
		if i < 5 { // 命中的 5 篇全部落在归档区
			tagStatsArticle(t, db, article.ID, hitTag.ID)
		}
	}

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(100), stats.Articles)
	require.Equal(t, int64(5), stats.InBoard)
	require.InDelta(t, 0.05, stats.HitRate, 1e-9)
	require.Len(t, stats.Boards, 1)
	require.Equal(t, int64(5), stats.Boards[0].Articles)
}

// TC-B1-08：源窗口内 0 篇 → 仍在结果中：articles=0、in_board=0、
// hit_rate=0、boards=[]（空数组非 null）。
func TestB1_08_FeedWithoutWindowArticlesStillListed(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "quiet", true)
	// 窗口外的一篇老文章，确认不污染 7 天窗口。
	seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-8*24*time.Hour), false)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(0), stats.Articles)
	require.Equal(t, int64(0), stats.InBoard)
	require.Equal(t, float64(0), stats.HitRate)
	require.NotNil(t, stats.Boards)
	require.Empty(t, stats.Boards)

	// JSON 序列化为 [] 而非 null（消费方按数组消费）。
	list, err := FeedBoardHitStats(context.Background(), db, 7)
	require.NoError(t, err)
	for _, s := range list {
		raw, err := json.Marshal(s.Boards)
		require.NoError(t, err)
		require.Equal(t, "[]", string(raw), "boards must serialize as [] not null")
	}
}

// TC-B1-09：pub_date 为 NULL、created_at 在窗口内 → 计入（coalesce 语义）。
func TestB1_09_NullPubDateFallsBackToCreatedAt(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(1), stats.Articles)
}

// TC-B1-10：pub_date 在窗口外、created_at 在窗口内 → coalesce 取 pub_date，
// 不计入（口径固定，不加或条件）。
func TestB1_10_OutOfWindowPubDateWinsOverCreatedAt(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	old := testNow.Add(-30 * 24 * time.Hour)
	seedStatsArticle(t, db, feed.ID, &old, testNow.Add(-time.Hour), false)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(0), stats.Articles)
}

// TC-B1-11：pub_date 恰为窗口下界时刻 → 计入（>= 语义，含边界时刻）。
func TestB1_11_ExactlyAtCutoffIsIncluded(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	cutoff := testNow.AddDate(0, 0, -7)
	seedStatsArticle(t, db, feed.ID, &cutoff, testNow.Add(-time.Hour), false)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(1), stats.Articles)
}

// TC-B1-12：命中 2 个板块 → SUM(boards.articles) ≥ in_board，且各板块计数
// 为去重后的文章数（各计 1）。
func TestB1_12_BoardSumCoversDedupedInBoard(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	boardP := seedStatsLabel(t, db, "P", "p", "board", "active")
	boardQ := seedStatsLabel(t, db, "Q", "q", "board", "active")
	t1 := seedStatsTag(t, db, "t1")
	t2 := seedStatsTag(t, db, "t2")
	linkStatsTagBoard(t, db, t1.ID, boardP.ID)
	linkStatsTagBoard(t, db, t2.ID, boardQ.ID)
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	tagStatsArticle(t, db, article.ID, t1.ID)
	tagStatsArticle(t, db, article.ID, t2.ID)

	stats := statsFor(t, db, 7)[feed.ID]
	var sumBoards int64
	for _, b := range stats.Boards {
		sumBoards += b.Articles
		require.Equal(t, int64(1), b.Articles) // 每板块按文章去重
	}
	require.GreaterOrEqual(t, sumBoards, stats.InBoard)
}

// ── B2：未打标两分 ──

// TC-B2-01 / TC-B2-02 / TC-B2-03 / TC-B2-04：按 tag_jobs 状态两分。
func TestB2_JobStatusSplitsUntagged(t *testing.T) {
	cases := []struct {
		name        string
		jobStatus   models.JobStatus
		wantPending int64
		wantSettled int64
	}{
		{"TC-B2-01 pending", models.JobStatusPending, 1, 0},
		{"TC-B2-02 leased", models.JobStatusLeased, 1, 0},
		{"TC-B2-03 completed", models.JobStatusCompleted, 0, 1},
		{"TC-B2-04 failed", models.JobStatusFailed, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixNow(t)
			db := setupStatsDB(t)
			feed := seedStatsFeed(t, db, "F", true)
			article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
			seedStatsTagJob(t, db, article.ID, tc.jobStatus)

			stats := statsFor(t, db, 7)[feed.ID]
			require.Equal(t, tc.wantPending, stats.UntaggedPending)
			require.Equal(t, tc.wantSettled, stats.UntaggedSettled)
			require.Equal(t, int64(1), stats.Articles)
			require.Equal(t, int64(0), stats.InBoard)
			require.Equal(t, int64(0), stats.TaggedNoBoard)
		})
	}
}

// TC-B2-05：无标签且无任何 tag_jobs 行（源关闭打标）→ untagged_settled，
// 且 tagging_enabled=false 照常透出（后端不过滤、不特殊处理）。
func TestB2_05_NoJobAndTaggingDisabledIsSettled(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "off", false)
	seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)

	stats := statsFor(t, db, 7)[feed.ID]
	require.False(t, stats.TaggingEnabled)
	require.Equal(t, int64(1), stats.UntaggedSettled)
	require.Equal(t, int64(0), stats.UntaggedPending)
}

// TC-B2-06：同时存在 pending 与 completed 任务行 → untagged_pending 计 1
//（EXISTS 语义，不重复计数）。
func TestB2_06_PendingWinsAndCountsOnce(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	seedStatsTagJob(t, db, article.ID, models.JobStatusPending)
	seedStatsTagJob(t, db, article.ID, models.JobStatusCompleted)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(1), stats.UntaggedPending)
	require.Equal(t, int64(0), stats.UntaggedSettled)
}

// TC-B2-07：有标签文章 → 两个 untagged 计数都不增加（即便存在任务行）。
func TestB2_07_TaggedArticleNeverInUntaggedBuckets(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	boardP := seedStatsLabel(t, db, "P", "p", "board", "active")
	tag := seedStatsTag(t, db, "t")
	linkStatsTagBoard(t, db, tag.ID, boardP.ID)
	article := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-time.Hour), false)
	tagStatsArticle(t, db, article.ID, tag.ID)
	seedStatsTagJob(t, db, article.ID, models.JobStatusPending)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(1), stats.InBoard)
	require.Equal(t, int64(0), stats.UntaggedPending)
	require.Equal(t, int64(0), stats.UntaggedSettled)
	require.Equal(t, int64(0), stats.TaggedNoBoard)
}

// TC-B2-08：恒等式 —— 每源 articles == in_board + tagged_no_board +
// untagged_pending + untagged_settled（四桶互斥且完备）。
func TestB2_08_IdentityOverMixedFeed(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	boardP := seedStatsLabel(t, db, "P", "p", "board", "active")
	hitTag := seedStatsTag(t, db, "hit")
	plainTag := seedStatsTag(t, db, "plain")
	linkStatsTagBoard(t, db, hitTag.ID, boardP.ID)

	hit := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-2*time.Hour), false)
	tagStatsArticle(t, db, hit.ID, hitTag.ID)
	noBoard := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-3*time.Hour), false)
	tagStatsArticle(t, db, noBoard.ID, plainTag.ID)
	pending := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-4*time.Hour), false)
	seedStatsTagJob(t, db, pending.ID, models.JobStatusPending)
	settled := seedStatsArticle(t, db, feed.ID, nil, testNow.Add(-5*time.Hour), false)
	seedStatsTagJob(t, db, settled.ID, models.JobStatusFailed)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(4), stats.Articles)
	require.Equal(t, stats.Articles, stats.InBoard+stats.TaggedNoBoard+stats.UntaggedPending+stats.UntaggedSettled)
}

// ── B3：窗口参数与边界 ──

// TC-B3-01：window=7 只统计 coalesce(pub_date,created_at) >= now-7d 的文章。
func TestB3_01_SevenDayWindowBoundary(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	cutoff := testNow.AddDate(0, 0, -7)
	in1 := testNow.Add(-24 * time.Hour)
	seedStatsArticle(t, db, feed.ID, nil, in1, false)
	seedStatsArticle(t, db, feed.ID, &cutoff, testNow.Add(-time.Hour), false) // 恰在下界 → 计入
	out := cutoff.Add(-time.Second)
	seedStatsArticle(t, db, feed.ID, &out, testNow.Add(-time.Hour), false)

	stats := statsFor(t, db, 7)[feed.ID]
	require.Equal(t, int64(2), stats.Articles)
}

// TC-B3-02：window=30 / window=90 按对应天数生效。
func TestB3_02_LongerWindows(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	d20 := testNow.Add(-20 * 24 * time.Hour)
	d80 := testNow.Add(-80 * 24 * time.Hour)
	seedStatsArticle(t, db, feed.ID, &d20, d20, false)
	seedStatsArticle(t, db, feed.ID, &d80, d80, false)

	require.Equal(t, int64(0), statsFor(t, db, 7)[feed.ID].Articles)
	require.Equal(t, int64(1), statsFor(t, db, 30)[feed.ID].Articles)
	require.Equal(t, int64(2), statsFor(t, db, 90)[feed.ID].Articles)
}

// TC-B3-06：同源对比 articles(7d) <= articles(30d) <= articles(90d)（窗口单调不变量）。
func TestB3_06_WindowMonotonicity(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	d1 := testNow.Add(-24 * time.Hour)
	d10 := testNow.Add(-10 * 24 * time.Hour)
	d40 := testNow.Add(-40 * 24 * time.Hour)
	for _, at := range []time.Time{d1, d10, d40} {
		seedStatsArticle(t, db, feed.ID, &at, at, false)
	}

	a7 := statsFor(t, db, 7)[feed.ID].Articles
	a30 := statsFor(t, db, 30)[feed.ID].Articles
	a90 := statsFor(t, db, 90)[feed.ID].Articles
	require.LessOrEqual(t, a7, a30)
	require.LessOrEqual(t, a30, a90)
}

// TC-B3-07：注入固定 now，文章恰在 7 天前 1 秒 → 不计入；恰在 7 天前 0 秒
//（边界）→ 计入（>= 语义）。
func TestB3_07_OneSecondBeforeCutoffExcluded(t *testing.T) {
	fixNow(t)
	db := setupStatsDB(t)
	feed := seedStatsFeed(t, db, "F", true)
	cutoff := testNow.AddDate(0, 0, -7)

	justBefore := cutoff.Add(-time.Second)
	seedStatsArticle(t, db, feed.ID, &justBefore, justBefore, false)
	require.Equal(t, int64(0), statsFor(t, db, 7)[feed.ID].Articles)

	exact := cutoff
	seedStatsArticle(t, db, feed.ID, &exact, exact, false)
	require.Equal(t, int64(1), statsFor(t, db, 7)[feed.ID].Articles)
}
