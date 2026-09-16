package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

// promptRecordingRouter wraps a chat router and records the prompts it saw so
// tests can assert which content the extractor was fed.
type promptRecordingRouter struct {
	inner tagChatRouter
	mu    sync.Mutex
	promo []string
}

func (p *promptRecordingRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	p.mu.Lock()
	for _, m := range req.Messages {
		p.promo = append(p.promo, m.Content)
	}
	p.mu.Unlock()
	return p.inner.Chat(ctx, req)
}

func (p *promptRecordingRouter) prompts() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.promo, "\n")
}

// setupReuseTestDB mirrors the aggregate-tagger test setup: PG golden schema +
// tagmanagement repository + noop aux service + fake extractor factory.
func setupReuseTestDB(t *testing.T, router tagChatRouter) *gorm.DB {
	t.Helper()
	db := setupArticleTaggerTestDB(t)
	database.DB = db // needed by airouter.NewStore() paths

	AuxServiceFactory = func(db *gorm.DB, embedder interface{}) AuxService {
		return &noopAuxService{}
	}
	tagExtractorFactory = func() *TagExtractor {
		return &TagExtractor{router: router}
	}
	t.Cleanup(func() {
		tagExtractorFactory = NewTagExtractor
	})
	return db
}

func reuseSiblingFixture(t *testing.T, db *gorm.DB, link string) (models.Article, models.Article, []models.TopicTag) {
	t.Helper()
	feedA := models.Feed{Title: "榜A", URL: "https://example.com/a"}
	feedB := models.Feed{Title: "榜B", URL: "https://example.com/b"}
	require.NoError(t, db.Create(&feedA).Error)
	require.NoError(t, db.Create(&feedB).Error)

	copyA := models.Article{FeedID: feedA.ID, Title: "同一篇文章", Link: link}
	copyB := models.Article{FeedID: feedB.ID, Title: "同一篇文章", Link: link}
	require.NoError(t, db.Create(&copyA).Error)
	require.NoError(t, db.Create(&copyB).Error)

	tags := []models.TopicTag{
		{Label: "美联储议息", Slug: "fed-meeting", Category: "event", Status: "active"},
		{Label: "美股", Slug: "us-stocks", Category: "keyword", Status: "active"},
		{Label: "鲍威尔", Slug: "powell", Category: "person", Status: "active"},
	}
	for i := range tags {
		require.NoError(t, db.Create(&tags[i]).Error)
	}
	return copyA, copyB, tags
}

// TestTagArticleReusesSiblingTagsWithoutAI covers spec scenario "复制已有打标
// 结果": copy B inherits copy A's tag links (score preserved, source=reuse)
// with zero AI chat calls.
func TestTagArticleReusesSiblingTagsWithoutAI(t *testing.T) {
	router := newFakeTagChatRouter()
	recorder := &promptRecordingRouter{inner: router}
	db := setupReuseTestDB(t, recorder)

	copyA, copyB, tags := reuseSiblingFixture(t, db, "https://example.com/shared/story")
	// Copy A carries the full tagging result (source llm, distinct scores).
	for i, tag := range tags {
		require.NoError(t, db.Create(&models.ArticleTopicTag{
			ArticleID: copyA.ID, TopicTagID: tag.ID, Score: 0.5 + float64(i)*0.1, Source: "llm",
		}).Error)
	}

	require.NoError(t, TagArticle(context.Background(), &copyB, "榜B", ""))

	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", copyB.ID).Order("topic_tag_id").Find(&got).Error)
	require.Len(t, got, 3)
	for _, link := range got {
		require.Equal(t, tagSourceReuse, link.Source, "reused rows must be marked source=reuse")
		var original models.ArticleTopicTag
		require.NoError(t, db.Where("article_id = ? AND topic_tag_id = ?", copyA.ID, link.TopicTagID).First(&original).Error)
		require.Equal(t, original.Score, link.Score, "score must be carried over from the sibling copy")
	}
	for _, op := range []string{"tag_extraction_event_person", "tag_extraction_keyword"} {
		require.Zero(t, router.callCount(op), "reuse path must not call the AI (op=%s)", op)
	}
	require.Empty(t, recorder.prompts(), "no prompt may reach the router on the reuse path")

	var refreshed models.Article
	require.NoError(t, db.First(&refreshed, copyB.ID).Error)
	require.Equal(t, 3, refreshed.TagCount, "denormalised tag_count must mirror the copied edges")
}

// TestTagArticleReuseSkipsConflictingTag covers the overlap variant: when copy
// B already holds one of A's tags, reuse must not produce a duplicate
// (article_id, topic_tag_id) pair.
func TestTagArticleReuseSkipsConflictingTag(t *testing.T) {
	router := newFakeTagChatRouter()
	db := setupReuseTestDB(t, router)

	copyA, copyB, tags := reuseSiblingFixture(t, db, "https://example.com/shared/story-2")
	for _, tag := range tags {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyA.ID, TopicTagID: tag.ID, Score: 0.8, Source: "llm"}).Error)
	}
	// B already has one overlapping tag (first of the three).
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyB.ID, TopicTagID: tags[0].ID, Score: 0.7, Source: "llm"}).Error)

	require.NoError(t, TagArticle(context.Background(), &copyB, "榜B", ""))

	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", copyB.ID).Order("topic_tag_id").Find(&got).Error)
	require.Len(t, got, 3, "B must end with the union of A's tags (already-present one not duplicated)")
	require.Zero(t, router.callCount("tag_extraction_event_person"))
	require.Zero(t, router.callCount("tag_extraction_keyword"))
}

// TestTagArticleWithoutSiblingExtractsViaAI covers the reverse: no sibling
// copy with tags → normal AI extraction runs and stores its own source.
func TestTagArticleWithoutSiblingExtractsViaAI(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{content: `{"tags":[]}`})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{content: `{"tags":[{"label":"美股存储芯片","category":"keyword","description":"存储芯片板块"}]}`})
	db := setupReuseTestDB(t, router)

	feed := models.Feed{Title: "独立源", URL: "https://example.com/solo"}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{FeedID: feed.ID, Title: "存储芯片普涨", Link: "https://example.com/solo/1"}
	require.NoError(t, db.Create(&article).Error)

	require.NoError(t, TagArticle(context.Background(), &article, "独立源", ""))

	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", article.ID).Find(&got).Error)
	require.NotEmpty(t, got)
	for _, link := range got {
		require.Equal(t, "llm", link.Source, "AI path rows keep their extraction source")
	}
	require.Equal(t, 1, router.callCount("tag_extraction_event_person"))
	require.Equal(t, 1, router.callCount("tag_extraction_keyword"))
}

// TestRetagArticleDoesNotReuseSiblingTags covers spec scenario "手动重打标不
// 复用": Force retag re-extracts via the AI even when a tagged sibling exists.
func TestRetagArticleDoesNotReuseSiblingTags(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{content: `{"tags":[{"label":"美债收益率下行","category":"event"}]}`})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{content: `{"tags":[]}`})
	db := setupReuseTestDB(t, router)

	copyA, copyB, tags := reuseSiblingFixture(t, db, "https://example.com/shared/story-3")
	for _, tag := range tags {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyA.ID, TopicTagID: tag.ID, Score: 0.8, Source: "llm"}).Error)
	}
	// B carries stale reuse links from an earlier run; Force must wipe them.
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyB.ID, TopicTagID: tags[0].ID, Score: 0.8, Source: tagSourceReuse}).Error)

	require.NoError(t, RetagArticle(context.Background(), &copyB, "榜B", ""))

	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", copyB.ID).Find(&got).Error)
	require.NotEmpty(t, got)
	for _, link := range got {
		require.NotEqual(t, tagSourceReuse, link.Source, "Force retag rows must come from extraction, not reuse")
	}
	require.Equal(t, 1, router.callCount("tag_extraction_event_person"), "Force retag must call the AI")
}

// TestTagArticleEmptyLinkNeverReuses guards the empty-link hole: two link-less
// articles must never "reuse" each other's tags.
func TestTagArticleEmptyLinkNeverReuses(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{content: `{"tags":[]}`})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{content: `{"tags":[]}`})
	db := setupReuseTestDB(t, router)

	feedA := models.Feed{Title: "源A", URL: "https://example.com/x"}
	feedB := models.Feed{Title: "源B", URL: "https://example.com/y"}
	require.NoError(t, db.Create(&feedA).Error)
	require.NoError(t, db.Create(&feedB).Error)
	noLinkA := models.Article{FeedID: feedA.ID, Title: "无链接A"}
	noLinkB := models.Article{FeedID: feedB.ID, Title: "无链接B"}
	require.NoError(t, db.Create(&noLinkA).Error)
	require.NoError(t, db.Create(&noLinkB).Error)
	tag := models.TopicTag{Label: "某话题", Slug: "some-topic", Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: noLinkA.ID, TopicTagID: tag.ID, Score: 0.9, Source: "llm"}).Error)

	require.NoError(t, TagArticle(context.Background(), &noLinkB, "源B", ""))

	var reused int64
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).Where("article_id = ? AND source = ?", noLinkB.ID, tagSourceReuse).Count(&reused).Error)
	require.Zero(t, reused, "empty-link articles must never reuse sibling tags")
	// Both branches were called (extraction ran; empty result → no links).
	require.Equal(t, 1, router.callCount("tag_extraction_event_person"))
	require.Equal(t, 1, router.callCount("tag_extraction_keyword"))
}

// TestTagArticleReuseStopsAtMaxArticleTags covers the cap the AI path also
// enforces (limitArticleTags): topping up a partially tagged copy must never
// push it past maxArticleTags, and the highest-scored sibling tags win the
// remaining slots.
func TestTagArticleReuseStopsAtMaxArticleTags(t *testing.T) {
	router := newFakeTagChatRouter()
	db := setupReuseTestDB(t, router)

	copyA, copyB, baseTags := reuseSiblingFixture(t, db, "https://example.com/shared/story-cap")
	// Top copy A up to the cap so reuse has more candidates than free slots.
	tags := append([]models.TopicTag{}, baseTags...)
	for i := 0; i < maxArticleTags-len(baseTags); i++ {
		extra := models.TopicTag{Label: fmt.Sprintf("候选标签%d", i), Slug: fmt.Sprintf("candidate-%d", i), Category: "keyword", Status: "active"}
		require.NoError(t, db.Create(&extra).Error)
		tags = append(tags, extra)
	}
	require.Len(t, tags, maxArticleTags)
	for i, tag := range tags {
		// Distinct scores: higher index = higher score, so the cap keeps the
		// best-scored ones first.
		require.NoError(t, db.Create(&models.ArticleTopicTag{
			ArticleID: copyA.ID, TopicTagID: tag.ID, Score: 0.1 + float64(i)*0.1, Source: "llm",
		}).Error)
	}
	// B already carries one of A's tags plus one of its own → 4 free slots.
	ownTag := models.TopicTag{Label: "B独有标签", Slug: "own-only", Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&ownTag).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyB.ID, TopicTagID: baseTags[0].ID, Score: 0.9, Source: "llm"}).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyB.ID, TopicTagID: ownTag.ID, Score: 0.9, Source: "llm"}).Error)

	require.NoError(t, TagArticle(context.Background(), &copyB, "榜B", ""))

	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", copyB.ID).Find(&got).Error)
	require.Len(t, got, maxArticleTags, "top-up must stop at the per-article cap")
	// The retained tags must include B's own tag and be sorted by score: the
	// highest-scored sibling tags win the free slots.
	kept := make(map[uint]bool, len(got))
	for _, link := range got {
		kept[link.TopicTagID] = true
	}
	require.True(t, kept[ownTag.ID], "B's pre-existing tag must survive the top-up")
	require.True(t, kept[tags[len(tags)-1].ID], "highest-scored sibling tag must win its slot")
	require.Zero(t, router.callCount("tag_extraction_event_person"), "capped top-up still skips the AI")
	require.Zero(t, router.callCount("tag_extraction_keyword"))
}

// TestTagArticleFullCopyWithOtherSiblingTagsSkipsAI covers the saturated
// variant: a copy already at the cap with a tagged sibling is left untouched
// (no top-up, no AI).
func TestTagArticleFullCopyWithOtherSiblingTagsSkipsAI(t *testing.T) {
	router := newFakeTagChatRouter()
	db := setupReuseTestDB(t, router)

	copyA, copyB, baseTags := reuseSiblingFixture(t, db, "https://example.com/shared/story-full")
	for _, tag := range baseTags {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyA.ID, TopicTagID: tag.ID, Score: 0.8, Source: "llm"}).Error)
	}
	// B is saturated with its own (disjoint) tags.
	for i := 0; i < maxArticleTags; i++ {
		own := models.TopicTag{Label: fmt.Sprintf("满标签%d", i), Slug: fmt.Sprintf("full-%d", i), Category: "keyword", Status: "active"}
		require.NoError(t, db.Create(&own).Error)
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyB.ID, TopicTagID: own.ID, Score: 0.5, Source: "llm"}).Error)
	}

	require.NoError(t, TagArticle(context.Background(), &copyB, "榜B", ""))

	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", copyB.ID).Find(&got).Error)
	require.Len(t, got, maxArticleTags, "a saturated copy keeps exactly its own tags")
	for _, link := range got {
		require.Equal(t, "llm", link.Source, "no reuse rows may be added on a saturated copy")
	}
	require.Zero(t, router.callCount("tag_extraction_event_person"))
	require.Zero(t, router.callCount("tag_extraction_keyword"))
}

// TestTagArticleAfterContentUpdateRetags covers spec scenario "处理链完成事件
// 触发重打标" end-state: after refreshExistingArticle wiped stale tags and
// cleared derived fields, a normal (non-Force) tag job extracts fresh tags from
// the NEW content via the AI path.
func TestTagArticleAfterContentUpdateRetags(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_event_person", fakeTagChatResponse{content: `{"tags":[{"label":"美债救市","category":"event"}]}`})
	router.enqueue("tag_extraction_keyword", fakeTagChatResponse{content: `{"tags":[]}`})
	recorder := &promptRecordingRouter{inner: router}
	db := setupReuseTestDB(t, recorder)

	feed := models.Feed{Title: "快讯源", URL: "https://example.com/live"}
	require.NoError(t, db.Create(&feed).Error)
	// State exactly as refreshExistingArticle leaves it: rolled title, derived
	// fields cleared, stale tags deleted.
	article := models.Article{
		FeedID: feed.ID, Title: "美债救市新标题", Link: "https://example.com/live/1",
		Description: "美债救市新内容", FirecrawlStatus: "pending", SummaryStatus: "incomplete",
	}
	require.NoError(t, db.Create(&article).Error)

	require.NoError(t, TagArticle(context.Background(), &article, "快讯源", ""))

	require.Contains(t, recorder.prompts(), "美债救市新标题", "extraction must see the rolled title")
	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", article.ID).Find(&got).Error)
	require.NotEmpty(t, got, "fresh tags must be persisted from the new content")
	for _, link := range got {
		require.Equal(t, "llm", link.Source)
	}
}
