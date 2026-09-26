package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// TestCollectBoardTags_PGHonorsDowngradedColumn is a regression guard for the
// SQLite↔PostgreSQL GROUP BY semantic gap (see standard/backend/testing.md,
// "改了发往真实 DB 的 SQL ... 必须有 testcontainer PG 用例").
//
// Background: the quality-scoring-observability change added
// topic_tag_board_labels.downgraded to the SELECT of collectBoardTags but
// initially omitted it from GROUP BY. SQLite (used by pure-logic unit tests)
// tolerates a non-aggregated SELECT column absent from GROUP BY, so all unit
// tests stayed green. PostgreSQL rejects it at runtime with SQLSTATE 42803,
// which made daily-report generation fail for every board in production.
//
// This test runs the real collectBoardTags query against a testcontainer
// pgvector container (production-isomorphic) so the GROUP BY contract is
// actually exercised. If the column is ever dropped from GROUP BY again, this
// test fails with 42803.
func TestCollectBoardTags_PGHonorsDowngradedColumn(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// collectBoardTags reads the package-level repository.Repo singleton
	// (repository.Repo.DB()). Inject the test DB so the real SQL runs against
	// the isolated pgvector container.
	repository.Repo = repository.NewTopicGraphRepository(db)

	// --- seed the JOIN chain collectBoardTags walks:
	// feed -> article -> article_topic_tag <- topic_tag <- topic_tag_board_labels <- board
	feed := models.Feed{Title: "test-feed", URL: "https://example.com/feed-collect-board-tags"}
	require.NoError(t, db.Create(&feed).Error)

	board := models.SemanticLabel{Label: "test-board", Slug: "test-board-collect", LabelType: "board"}
	require.NoError(t, db.Create(&board).Error)

	// Two event tags: one non-downgraded direct_hit, one downgraded max_sim.
	tagDirect := models.TopicTag{
		Slug: "ai-chip", Label: "AI芯片", Category: models.TagCategoryEvent, Status: "active",
	}
	tagDowngraded := models.TopicTag{
		Slug: "gpu-shortage", Label: "GPU短缺", Category: models.TagCategoryEvent, Status: "active",
	}
	require.NoError(t, db.Create(&tagDirect).Error)
	require.NoError(t, db.Create(&tagDowngraded).Error)

	// Board labels carry the per-tag match quality we assert on later.
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagDirect.ID, SemanticBoardID: board.ID,
		MatchReason: "direct_hit", Score: 1.0, Downgraded: false,
	}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagDowngraded.ID, SemanticBoardID: board.ID,
		MatchReason: "max_sim", Score: 0.7, Downgraded: true,
	}).Error)

	// Article published "today" (inside collectBoardTags' [startOfDay, endOfDay)).
	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	art1 := models.Article{FeedID: feed.ID, Title: "a1", PubDate: &pubDate}
	art2 := models.Article{FeedID: feed.ID, Title: "a2", PubDate: &pubDate}
	require.NoError(t, db.Create(&art1).Error)
	require.NoError(t, db.Create(&art2).Error)

	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: art1.ID, TopicTagID: tagDirect.ID}).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: art2.ID, TopicTagID: tagDowngraded.ID}).Error)

	// --- act: run the real query. Pre-fix this returned SQLSTATE 42803.
	tags, articleIDSets, err := collectBoardTags(board.ID, now)
	require.NoError(t, err, "collectBoardTags must not fail on PostgreSQL (GROUP BY must include all non-aggregated SELECT columns)")
	require.Len(t, tags, 2, "both seeded event tags should be collected")
	require.Len(t, articleIDSets, 2)

	// Assert the downgraded flag survives the GROUP BY round-trip — this is
	// the exact data the production bug silently dropped / crashed on.
	byLabel := make(map[string]repository.TagInput, len(tags))
	for _, tg := range tags {
		byLabel[tg.Label] = tg
	}
	assert.Equal(t, false, byLabel["AI芯片"].Downgraded, "direct_hit tag should be non-downgraded")
	assert.Equal(t, "direct_hit", byLabel["AI芯片"].MatchReason)
	assert.Equal(t, true, byLabel["GPU短缺"].Downgraded, "downgraded flag must round-trip through PostgreSQL GROUP BY")
	assert.Equal(t, "max_sim", byLabel["GPU短缺"].MatchReason)
}

// TestCollectBoardTags_PopulatesArticleContext verifies the representative article
// titles+summaries are injected into TagInput.ArticleContext, grounding the daily report
// LLM prompts in actual event content (fix for headline confusion).
func TestCollectBoardTags_PopulatesArticleContext(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)

	feed := models.Feed{Title: "ctx-feed", URL: "https://example.com/feed-ctx"}
	require.NoError(t, db.Create(&feed).Error)

	board := models.SemanticLabel{Label: "ctx-board", Slug: "ctx-board", LabelType: "board"}
	require.NoError(t, db.Create(&board).Error)

	tag := models.TopicTag{
		Slug: "rate-cut", Label: "降准", Category: models.TagCategoryEvent, Status: "active",
	}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tag.ID, SemanticBoardID: board.ID,
		MatchReason: "direct_hit", Score: 1.0, Downgraded: false,
	}).Error)

	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	art := models.Article{
		FeedID:           feed.ID,
		Title:            "央行宣布降准",
		AIContentSummary: "央行决定下调存款准备金率0.5个百分点。",
		PubDate:          &pubDate,
	}
	require.NoError(t, db.Create(&art).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: art.ID, TopicTagID: tag.ID}).Error)

	tags, _, err := collectBoardTags(board.ID, now)
	require.NoError(t, err)
	require.Len(t, tags, 1)

	ctx := tags[0].ArticleContext
	assert.NotEmpty(t, ctx, "ArticleContext should be populated from representative article")
	assert.Contains(t, ctx, "央行宣布降准", "ArticleContext should include article title")
	assert.Contains(t, ctx, "央行决定下调", "ArticleContext should include article summary")
}
