package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// contentKeyTestDB 建一个内存 SQLite（与 rebuild_signals_test.go 同款），
// 里面只有 articles / board_stories / daily_report_threads / story_signals
// 四张表——回填的 SQL（join + keyset 分页 + 单行 UPDATE）与重算路径
// （按 thread 并集重写证据 + 重算参与者计数）都是标准 SQL，两库通用。
func contentKeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:content-keys-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Article{}, &repository.BoardStory{},
		&repository.DailyReportThread{}, &repository.StorySignal{}))
	return db
}

// seedContentKeyArticle 落一篇带指定 link 的文章。
func seedContentKeyArticle(t *testing.T, db *gorm.DB, id, feedID uint, link string) uint {
	t.Helper()
	article := models.Article{ID: id, FeedID: feedID, Title: fmt.Sprintf("article-%d", id), Link: link}
	require.NoError(t, db.Create(&article).Error)
	return id
}

// seedContentKeyStory 落一条事件线 + 一条指向它的 thread（证据文章并集）。
// participantCount 是存量计数（回填前按 feed 的旧值），回填后应回落到真实
// 内容数。
func seedContentKeyStory(t *testing.T, db *gorm.DB, storyID uint, articleIDs []uint, participantCount int) {
	t.Helper()
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	story := repository.BoardStory{ID: storyID, BoardID: 1, Title: fmt.Sprintf("story-%d", storyID),
		FirstSeenDate: day, LastSeenDate: day, ParticipantCount: participantCount}
	require.NoError(t, db.Omit("embedding", "subject_embedding").Create(&story).Error)
	parts := make([]string, 0, len(articleIDs))
	for _, id := range articleIDs {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	storyRef := storyID
	thread := repository.DailyReportThread{ID: storyID, ReportID: storyID, SectionID: storyID,
		StoryID: &storyRef, RelatedArticleIDs: repository.JSON("[" + strings.Join(parts, ",") + "]")}
	require.NoError(t, db.Omit("embedding").Create(&thread).Error)
}

// readContentKey 读一行 signal 的 content_key（区分 NULL 与空串）。
func readContentKey(t *testing.T, db *gorm.DB, storyID, articleID uint) *string {
	t.Helper()
	var value *string
	require.NoError(t, db.Raw(`SELECT content_key FROM story_signals
		WHERE story_id = ? AND article_id = ?`, storyID, articleID).Scan(&value).Error)
	return value
}

// readParticipantCount 读事件线存储的参与者计数。
func readParticipantCount(t *testing.T, db *gorm.DB, storyID uint) int {
	t.Helper()
	var count int
	require.NoError(t, db.Raw(`SELECT participant_count FROM board_stories WHERE id = ?`, storyID).Scan(&count).Error)
	return count
}

// TestBackfillContentKeysSameContentSharesKey 核心语义：同一篇内容被两个 feed
// 各抓一份 → 两条 signal 的 content_key 相同；空 link → 空串（内容身份不可
// 判定）；重跑幂等（第二次 updated=0）。
func TestBackfillContentKeysSameContentSharesKey(t *testing.T) {
	db := contentKeyTestDB(t)
	seedContentKeyArticle(t, db, 1, 10, "https://Example.com/p/1?utm=x")
	seedContentKeyArticle(t, db, 2, 20, "https://example.com/p/1#comments")
	seedContentKeyArticle(t, db, 3, 30, "")
	seedContentKeyStory(t, db, 1, []uint{1, 2, 3}, 3)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	signals := []repository.StorySignal{
		{StoryID: 1, ArticleID: 1, FeedID: 10, ParticipantKey: "feed:10", Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"},
		{StoryID: 1, ArticleID: 2, FeedID: 20, ParticipantKey: "feed:20", Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"},
	}
	require.NoError(t, db.Create(&signals).Error)
	// 第三行留 NULL：存量行（迁移前建的）content_key 为 NULL，回填必须认出来。
	require.NoError(t, db.Exec(`INSERT INTO story_signals (story_id, article_id, feed_id, participant_key, kind, observed_at, observed_at_source)
		VALUES (1, 3, 30, 'feed:30', 'editorial', ?, 'fallback')`, now).Error)

	stats, err := backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, stats.Scanned)
	assert.EqualValues(t, 3, stats.Updated)
	assert.Equal(t, 1, stats.Stories, "被回填的事件线要重算一次")

	first := readContentKey(t, db, 1, 1)
	second := readContentKey(t, db, 1, 2)
	require.NotNil(t, first)
	require.NotNil(t, second)
	assert.Equal(t, "https://example.com/p/1", *first, "规范化链接即内容身份键")
	assert.Equal(t, *first, *second, "同一篇内容的两个 feed 副本必须共用一个 content_key")
	empty := readContentKey(t, db, 1, 3)
	require.NotNil(t, empty)
	assert.Equal(t, "", *empty, "空 link → 空串（内容身份不可判定）")

	// 幂等：重跑第二次一行都不用改。
	stats, err = backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, stats.Scanned)
	assert.EqualValues(t, 0, stats.Updated, "第二次回填必须零写入")
	assert.Equal(t, 0, stats.Stories, "计数已一致的事件线不重复重算")
}

// TestBackfillContentKeysRecomputesParticipantCount 回填收尾必须重算参与者
// 计数：同一 link 的两个 feed 副本算 1 个来源，空 link 的文章单独计 1 个——
// 存量计数（按 feed 的旧值 3）回落到真实内容数 2。
func TestBackfillContentKeysRecomputesParticipantCount(t *testing.T) {
	db := contentKeyTestDB(t)
	seedContentKeyArticle(t, db, 1, 10, "https://example.com/p/1")
	seedContentKeyArticle(t, db, 2, 20, "https://example.com/p/1")
	seedContentKeyArticle(t, db, 3, 30, "")
	seedContentKeyStory(t, db, 1, []uint{1, 2, 3}, 3)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	signals := []repository.StorySignal{
		{StoryID: 1, ArticleID: 1, FeedID: 10, ParticipantKey: "feed:10", Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"},
		{StoryID: 1, ArticleID: 2, FeedID: 20, ParticipantKey: "feed:20", Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"},
		{StoryID: 1, ArticleID: 3, FeedID: 30, ParticipantKey: "feed:30", Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"},
	}
	require.NoError(t, db.Create(&signals).Error)
	require.Equal(t, 3, readParticipantCount(t, db, 1), "存量计数按 feed 计（3 个来源）")

	stats, err := backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.Updated, "空 link 的那一行已是目标值（空串），只有两个副本行要写")
	assert.Equal(t, 1, stats.Stories)
	assert.Equal(t, 2, readParticipantCount(t, db, 1),
		"同 link 的两个副本合并成 1 个来源，空 link 单独计 1 个")

	// 幂等：第二次零写入、零重算，计数保持不变。
	stats, err = backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, stats.Updated)
	assert.Equal(t, 0, stats.Stories)
	assert.Equal(t, 2, readParticipantCount(t, db, 1))
}

// TestBackfillContentKeysRepairsStaleCount 只回填 content_key、不重算计数的
// 历史欠账：content_key 已一致（回填零写入）但计数仍是旧值时，本轮回填仍要
// 把计数修正过来。
func TestBackfillContentKeysRepairsStaleCount(t *testing.T) {
	db := contentKeyTestDB(t)
	seedContentKeyArticle(t, db, 1, 10, "https://example.com/p/1")
	seedContentKeyArticle(t, db, 2, 20, "https://example.com/p/1")
	seedContentKeyStory(t, db, 1, []uint{1, 2}, 2)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	signals := []repository.StorySignal{
		{StoryID: 1, ArticleID: 1, FeedID: 10, ParticipantKey: "feed:10", ContentKey: "https://example.com/p/1", Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"},
		{StoryID: 1, ArticleID: 2, FeedID: 20, ParticipantKey: "feed:20", ContentKey: "https://example.com/p/1", Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"},
	}
	require.NoError(t, db.Create(&signals).Error)
	require.Equal(t, 2, readParticipantCount(t, db, 1))

	stats, err := backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, stats.Updated, "内容身份键已一致 → 零写入")
	assert.Equal(t, 1, stats.Stories, "计数与证据对不上 → 仍要重算")
	assert.Equal(t, 1, readParticipantCount(t, db, 1), "两个 feed 副本算 1 个来源")

	stats, err = backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, stats.Updated)
	assert.Equal(t, 0, stats.Stories, "计数修好后不再重算")
}

// TestBackfillContentKeysDryRunDoesNotWrite：dry-run 只统计将更新的行数与将
// 重算的事件线数，不写库（content_key 与 participant_count 都不动）。
func TestBackfillContentKeysDryRunDoesNotWrite(t *testing.T) {
	db := contentKeyTestDB(t)
	seedContentKeyArticle(t, db, 1, 10, "https://example.com/p/1")
	seedContentKeyStory(t, db, 1, []uint{1}, 2)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&repository.StorySignal{
		StoryID: 1, ArticleID: 1, FeedID: 10, ParticipantKey: "feed:10",
		Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date",
	}).Error)

	dryRun := commandOptions{}
	dryRun.DryRun = true
	stats, err := backfillContentKeys(context.Background(), db, dryRun)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned)
	assert.EqualValues(t, 1, stats.Updated, "dry-run 也要报出将更新的行数")
	assert.Equal(t, 1, stats.Stories, "dry-run 也要报出将重算的事件线数")

	value := readContentKey(t, db, 1, 1)
	require.NotNil(t, value)
	assert.Equal(t, "", *value, "dry-run 不得写库")
	assert.Equal(t, 2, readParticipantCount(t, db, 1), "dry-run 不得重算参与者计数")
}

// TestBackfillContentKeysBatches：超过一批（500 行）时按批推进，每批 ≤500 行，
// 全量回填后重跑零写入。
func TestBackfillContentKeysBatches(t *testing.T) {
	db := contentKeyTestDB(t)
	const total = 1200
	articles := make([]models.Article, 0, total)
	signals := make([]repository.StorySignal, 0, total)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	byStory := map[uint][]uint{}
	for i := 1; i <= total; i++ {
		id := uint(i)
		storyID := uint(1 + i%3)
		articles = append(articles, models.Article{ID: id, FeedID: 10, Title: fmt.Sprintf("bulk-%d", id), Link: fmt.Sprintf("https://example.com/bulk/%d?utm=x", id)})
		signals = append(signals, repository.StorySignal{StoryID: storyID, ArticleID: id, FeedID: 10, ParticipantKey: "feed:10",
			Kind: repository.StorySignalKindEditorial, ObservedAt: now, ObservedAtSource: "pub_date"})
		byStory[storyID] = append(byStory[storyID], id)
	}
	require.NoError(t, db.CreateInBatches(&articles, 200).Error)
	require.NoError(t, db.CreateInBatches(&signals, 200).Error)
	for storyID, ids := range byStory {
		seedContentKeyStory(t, db, storyID, ids, len(ids))
	}

	stats, err := backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, total, stats.Scanned)
	assert.EqualValues(t, total, stats.Updated)
	assert.Equal(t, 3, stats.Stories, "三条事件线各重算一次")

	stats, err = backfillContentKeys(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, total, stats.Scanned)
	assert.EqualValues(t, 0, stats.Updated, "分批回填同样幂等")
	assert.Equal(t, 0, stats.Stories)

	var filled int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM story_signals WHERE content_key <> ''`).Scan(&filled).Error)
	assert.EqualValues(t, total, filled)
}
