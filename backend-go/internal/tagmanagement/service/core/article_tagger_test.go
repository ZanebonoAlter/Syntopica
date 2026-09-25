package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
)

func setupArticleTaggerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	return db
}

func TestCreateArticleTopicTagLink(t *testing.T) {
	db := setupArticleTaggerTestDB(t)
	article, tag := createArticleTaggerFixtures(t, db)
	link := models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Score: 0.7, Source: "llm"}

	articleExists, err := createArticleTopicTagLink(&link)

	require.NoError(t, err)
	require.True(t, articleExists)
	require.NotZero(t, link.ID)
}

func TestCreateArticleTopicTagLinkSkipsDeletedArticle(t *testing.T) {
	db := setupArticleTaggerTestDB(t)
	article, tag := createArticleTaggerFixtures(t, db)
	require.NoError(t, db.Delete(&article).Error)
	link := models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Score: 0.7, Source: "llm"}

	articleExists, err := createArticleTopicTagLink(&link)

	require.NoError(t, err)
	require.False(t, articleExists)

	var count int64
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).Where("article_id = ?", article.ID).Count(&count).Error)
	require.Zero(t, count)
}

func createArticleTaggerFixtures(t *testing.T, db *gorm.DB) (models.Article, models.TopicTag) {
	t.Helper()
	feed := models.Feed{Title: "Tagger Test", URL: "https://example.com/tagger-test"}
	require.NoError(t, db.Create(&feed).Error)

	article := models.Article{FeedID: feed.ID, Title: "Tagger Test Article"}
	require.NoError(t, db.Create(&article).Error)

	tag := models.TopicTag{Label: "Tagger Test", Slug: "tagger-test", Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return article, tag
}

// --- fix-tagging-pollution ---

// setupTaggerBehaviorTest = setupReuseTestDB + tag cache 清理：GetTagCache 是
// 进程级全局，跨测试存活而 ResetTestData 只重置数据，不清缓存会让本文件
// 用例吃到上一用例的悬空缓存项（假绿/悬空 link）。
func setupTaggerBehaviorTest(t *testing.T, router *fakeTagChatRouter) *gorm.DB {
	t.Helper()
	db := setupReuseTestDB(t, router) // 本行不得改成自调用（递归栈溢出）
	GetTagCache().Clear()
	return db
}

// createTaggerArticle builds a single fresh article for tagger-path tests
// (feed + article with a unique link so cross-feed reuse stays out of the way).
func createTaggerArticle(t *testing.T, db *gorm.DB, feedTitle, articleTitle, link string) models.Article {
	t.Helper()
	feed := models.Feed{Title: feedTitle, URL: "https://example.com/" + Slugify(feedTitle)}
	require.NoError(t, db.Create(&feed).Error)
	article := models.Article{FeedID: feed.ID, Title: articleTitle, Link: link}
	require.NoError(t, db.Create(&article).Error)
	return article
}

func articleTagLabels(t *testing.T, db *gorm.DB, articleID uint) []string {
	t.Helper()
	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", articleID).Find(&links).Error)
	ids := make([]uint, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.TopicTagID)
	}
	labels := make([]string, 0, len(ids))
	if len(ids) > 0 {
		var tags []models.TopicTag
		require.NoError(t, db.Where("id IN ?", ids).Find(&tags).Error)
		for _, tag := range tags {
			labels = append(labels, tag.Label)
		}
	}
	return labels
}

// 白盒（test-cases.md）：filterGenericLabels 对 nil/空切片入参返回空切片；
// 黑名单只做 Slugify 后完整匹配，不做子串匹配。
func TestFilterGenericLabels(t *testing.T) {
	require.Empty(t, filterGenericLabels(nil))
	require.Empty(t, filterGenericLabels([]TopicTag{}))
	require.Empty(t, filterGenericLabels([]TopicTag{{Label: "新闻"}, {Label: "技术"}, {Label: "快讯"}}))

	kept := filterGenericLabels([]TopicTag{{Label: "新闻"}, {Label: "GLM Coding Plan"}, {Label: "新技术趋势"}})
	require.Len(t, kept, 2)
	require.Equal(t, []string{"GLM Coding Plan", "新技术趋势"}, []string{kept[0].Label, kept[1].Label})
}

// fix-tagging-pollution spec「LLM 调用成功但零标签」：融合提取成功但候选总数为
// 零 → 文章不写入任何 article_topic_tags 行、不产生 heuristic 来源标签。旧契约
// 下分类名「新闻」会经回填（extractor :59-61）降级 heuristic 写库，本用例守住
// 零候选断根路径。
func TestTagArticleSuccessfulZeroCandidatesWritesNoRows(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{content: `{"event_person_tags":[],"keyword_tags":[]}`})
	db := setupTaggerBehaviorTest(t, router)

	article := createTaggerArticle(t, db, "快讯源", "薄内容快讯", "https://example.com/flash/zero")

	require.NoError(t, TagArticle(context.Background(), &article, "快讯源", "新闻"))

	var count int64
	require.NoError(t, db.Model(&models.ArticleTopicTag{}).Where("article_id = ?", article.ID).Count(&count).Error)
	require.Zero(t, count, "successful extraction with zero candidates must leave the article untagged")
	require.Equal(t, 1, router.callCount("tag_extraction_merged"))
}

// fix-tagging-pollution spec「LLM 调用失败降级 heuristic」：连接拒绝重试耗尽 →
// 降级 heuristic 提取且来源标记 heuristic（兜底仅保留在「调用失败」触发）。
func TestTagArticleCallFailureFallsBackToHeuristic(t *testing.T) {
	router := newFakeTagChatRouter()
	for i := 0; i < 3; i++ {
		router.enqueue("tag_extraction_merged", fakeTagChatResponse{err: errors.New("connection refused")})
	}
	db := setupTaggerBehaviorTest(t, router)

	// 正文含 NVIDIA 模式词，保证 heuristic 兜底产出非泛词标签；分类名「新闻」
	// 由兜底产出但必须被黑名单拦下，恰好覆盖 heuristic 路径的泛词拦截。
	article := createTaggerArticle(t, db, "断流快讯源", "NVIDIA 发布新显卡", "https://example.com/flash-down/1")
	article.Content = "NVIDIA announces a new datacenter GPU."
	require.NoError(t, db.Save(&article).Error)

	require.NoError(t, TagArticle(context.Background(), &article, "断流快讯源", "新闻"))

	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", article.ID).Find(&links).Error)
	require.NotEmpty(t, links, "call failure must still fall back to heuristic extraction")
	for _, link := range links {
		require.Equal(t, "heuristic", link.Source)
	}
	labels := articleTagLabels(t, db, article.ID)
	require.Contains(t, labels, "NVIDIA")
	require.NotContains(t, labels, "新闻", "heuristic path must drop the generic category label")
	require.Equal(t, 3, router.callCount("tag_extraction_merged"), "retry budget is exactly 3")
}

// fix-tagging-pollution spec「分类名回填词被拦截」：llm 路径产出 label=「新闻」的
// 候选 → 持久化前被丢弃，合法标签照常入库，泛词不进 topic_tags 池。
func TestTagArticleDropsGenericLabelCandidates(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{
		content: `{"event_person_tags":[],"keyword_tags":[{"label":"新闻","category":"keyword","description":"新闻资讯分类"},{"label":"美联储议息","category":"keyword","description":"美联储利率决议会议"}]}`,
	})
	db := setupTaggerBehaviorTest(t, router)

	article := createTaggerArticle(t, db, "泛词测试源", "泛词拦截", "https://example.com/generic/1")

	require.NoError(t, TagArticle(context.Background(), &article, "泛词测试源", "新闻"))

	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", article.ID).Find(&links).Error)
	require.Len(t, links, 1, "generic label must be dropped before persist; legal tag survives")
	require.Equal(t, []string{"美联储议息"}, articleTagLabels(t, db, article.ID))

	var genericCount int64
	require.NoError(t, db.Model(&models.TopicTag{}).Where("slug = ?", Slugify("新闻")).Count(&genericCount).Error)
	require.Zero(t, genericCount, "generic label must not reach the topic_tags pool")
}

// fix-tagging-pollution spec「黑名单不误伤正常标签」：黑名单只做 Slugify 后完整
// 匹配，「GLM Coding Plan」等复合正常标签照常入库。
func TestTagArticleKeepsNonGenericCompoundLabels(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{
		content: `{"event_person_tags":[],"keyword_tags":[{"label":"GLM Coding Plan","category":"keyword","description":"智谱编程订阅计划"}]}`,
	})
	db := setupTaggerBehaviorTest(t, router)

	article := createTaggerArticle(t, db, "编程社区", "GLM Coding Plan 上线", "https://example.com/coding-plan/1")

	require.NoError(t, TagArticle(context.Background(), &article, "编程社区", ""))

	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", article.ID).Find(&links).Error)
	require.Len(t, links, 1)
	require.Equal(t, []string{"GLM Coding Plan"}, articleTagLabels(t, db, article.ID))
	require.Equal(t, "llm", links[0].Source)
}

// fix-tagging-pollution spec「跨 feed 复用不复制泛词标签」：sibling 带「新闻」关
// 联 → 「新闻」行不被复制到新文章，其余合法标签照常复制。
func TestTagArticleReuseSkipsGenericSiblingTags(t *testing.T) {
	router := newFakeTagChatRouter()
	db := setupTaggerBehaviorTest(t, router)

	copyA, copyB, tags := reuseSiblingFixture(t, db, "https://example.com/shared/generic-filter")
	generic := models.TopicTag{Label: "新闻", Slug: Slugify("新闻"), Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&generic).Error)

	siblingTags := append(append([]models.TopicTag{}, tags...), generic)
	for i, tag := range siblingTags {
		require.NoError(t, db.Create(&models.ArticleTopicTag{
			ArticleID: copyA.ID, TopicTagID: tag.ID, Score: 0.5 + float64(i)*0.1, Source: "llm",
		}).Error)
	}

	require.NoError(t, TagArticle(context.Background(), &copyB, "榜B", ""))

	var got []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", copyB.ID).Find(&got).Error)
	require.Len(t, got, 3, "legal sibling tags are copied, the generic one is not")
	for _, link := range got {
		require.NotEqual(t, generic.ID, link.TopicTagID, "generic label link must not be copied")
	}
	require.Zero(t, router.callCount("tag_extraction_merged"), "reuse still short-circuits the AI")
}

// 白盒（test-cases.md）：sibling 标签全部命中黑名单 → 过滤后 siblingLinks 为空
// → reuse 返回 false，文章落回 AI 提取而非顶着泛词复制零行后短路。
func TestTagArticleReuseAllGenericSiblingTagsFallsBackToAI(t *testing.T) {
	router := newFakeTagChatRouter()
	router.enqueue("tag_extraction_merged", fakeTagChatResponse{
		content: `{"event_person_tags":[],"keyword_tags":[{"label":"智能眼镜","category":"keyword","description":"可穿戴显示设备"}]}`,
	})
	db := setupTaggerBehaviorTest(t, router)

	copyA, copyB, _ := reuseSiblingFixture(t, db, "https://example.com/shared/all-generic")
	generic := models.TopicTag{Label: "新闻", Slug: Slugify("新闻"), Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&generic).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: copyA.ID, TopicTagID: generic.ID, Score: 0.9, Source: "llm"}).Error)

	require.NoError(t, TagArticle(context.Background(), &copyB, "榜B", ""))

	require.Equal(t, 1, router.callCount("tag_extraction_merged"), "no reusable sibling tags → AI extraction must run")
	var links []models.ArticleTopicTag
	require.NoError(t, db.Where("article_id = ?", copyB.ID).Find(&links).Error)
	require.Len(t, links, 1)
	require.Equal(t, []string{"智能眼镜"}, articleTagLabels(t, db, copyB.ID))
	require.NotEqual(t, tagSourceReuse, links[0].Source)
}
