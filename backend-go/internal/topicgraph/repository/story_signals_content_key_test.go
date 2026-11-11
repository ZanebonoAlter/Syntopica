package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// TestStorySignalsContentKeyFromNormalizedLink 覆盖 fix-duplicate-article-heat
// 2.3：RecomputeSignalsForStory 写入的 signal 带 content_key（规范化链接），
// 同一篇内容的两个 feed 副本 content_key 相同、participant_key 仍是
// feed:<id>（向后兼容），空 link → 空串。
func TestStorySignalsContentKeyFromNormalizedLink(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, storyHeatZone)
	story := seedStoryLine(t, db, 1, "content-key", "", "", at)
	// 同一篇内容的两个 feed 副本：链接只在大小写 / query / fragment 上不同。
	feedA := seedSignalArticleWithLink(t, db, 10, "https://Example.com/p/1?utm=x", at)
	feedB := seedSignalArticleWithLink(t, db, 20, "https://example.com/p/1#comments", at)
	noLink := seedSignalArticleWithLink(t, db, 30, "", at)
	report := seedStoryReport(t, db, 1, at)
	section := seedStorySection(t, db, report, nil)
	thread := seedStoryThread(t, db, report, section, nil, []uint{feedA, feedB, noLink}, false)
	require.NoError(t, repo.AttachThreadToBoardStory(report, thread, story, false))

	var signals []StorySignal
	require.NoError(t, db.Order("article_id").Find(&signals).Error)
	require.Len(t, signals, 3)
	assert.Equal(t, "https://example.com/p/1", signals[0].ContentKey)
	assert.Equal(t, signals[0].ContentKey, signals[1].ContentKey,
		"同一篇内容的两个 feed 副本必须落同一个内容身份键")
	assert.Equal(t, "", signals[2].ContentKey, "空 link → 空串（内容身份不可判定）")
	// participant_key 保持 feed 粒度（向后兼容）。
	assert.Equal(t, "feed:10", signals[0].ParticipantKey)
	assert.Equal(t, "feed:20", signals[1].ParticipantKey)
	assert.Equal(t, "feed:30", signals[2].ParticipantKey)
	// 参与者计数按内容身份去重（fix-duplicate-article-heat 2.4）：前两行是同一篇
	// 内容的两个 feed 副本 → 1 个参与者；空 link 行退化为 feed:30 → 1 个。
	assertStoryParticipants(t, db, story, 2)
}

// seedSignalArticleWithLink 落一篇带 link 的证据文章。
func seedSignalArticleWithLink(t *testing.T, db *gorm.DB, feedID uint, link string, created time.Time) uint {
	t.Helper()
	article := models.Article{FeedID: feedID, Title: "evidence", Link: link, CreatedAt: created}
	require.NoError(t, db.Create(&article).Error)
	return article.ID
}
