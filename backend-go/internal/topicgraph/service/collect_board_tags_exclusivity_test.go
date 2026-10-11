package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	core "syntopica-backend/internal/tagmanagement/service/core"
	"syntopica-backend/internal/topicgraph/repository"
)

// TestCollectBoardTags_CrossBoardExclusivity covers fix-board-match-cross-domain:
// a tag mounted on the current board with a weak score while another board holds
// a clearly stronger mount (score gap >= semantic_board_report_exclusive_gap,
// default 0.3) is excluded from this board's candidates at the SQL layer.
// Excluded-at-SQL also proves the tag cannot be resurrected by the downstream
// weighted fallback (filterTagsByQuality's inputs are exactly these rows).
func TestCollectBoardTags_CrossBoardExclusivity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)

	feed := models.Feed{Title: "test-feed", URL: "https://example.com/feed-collect-exclusive"}
	require.NoError(t, db.Create(&feed).Error)

	boardHome := models.SemanticLabel{Label: "us-news", Slug: "us-news-x", LabelType: "board"}
	boardWeak := models.SemanticLabel{Label: "cn-news", Slug: "cn-news-x", LabelType: "board"}
	require.NoError(t, db.Create(&boardHome).Error)
	require.NoError(t, db.Create(&boardWeak).Error)

	// tagBorrowed: 0.7 here (direct_hit borrowed-word attachment), 1.0 on the
	// other board — gap 0.3 >= 0.3 → excluded from boardWeak, kept on boardHome.
	tagBorrowed := models.TopicTag{Slug: "us-nonfarm", Label: "美国非农", Category: models.TagCategoryEvent, Status: "active"}
	// tagCrossGenuine: 0.7 here, 0.8 on the other board — gap 0.1 < 0.3 → kept on both.
	tagCrossGenuine := models.TopicTag{Slug: "cn-us-trade", Label: "中美贸易", Category: models.TagCategoryEvent, Status: "active"}
	// tagSolo: only mounted here → kept.
	tagSolo := models.TopicTag{Slug: "cn-local", Label: "深交所问询", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tagBorrowed).Error)
	require.NoError(t, db.Create(&tagCrossGenuine).Error)
	require.NoError(t, db.Create(&tagSolo).Error)

	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagBorrowed.ID, SemanticBoardID: boardWeak.ID,
		MatchReason: "direct_hit", Score: 0.7, Downgraded: false,
	}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagBorrowed.ID, SemanticBoardID: boardHome.ID,
		MatchReason: "max_sim", Score: 1.0, Downgraded: false,
	}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagCrossGenuine.ID, SemanticBoardID: boardWeak.ID,
		MatchReason: "weighted", Score: 0.7, Downgraded: false,
	}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagCrossGenuine.ID, SemanticBoardID: boardHome.ID,
		MatchReason: "weighted", Score: 0.8, Downgraded: false,
	}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagSolo.ID, SemanticBoardID: boardWeak.ID,
		MatchReason: "direct_hit", Score: 0.7, Downgraded: false,
	}).Error)

	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	for _, tag := range []models.TopicTag{tagBorrowed, tagCrossGenuine, tagSolo} {
		art := models.Article{FeedID: feed.ID, Title: "a-" + tag.Slug, PubDate: &pubDate}
		require.NoError(t, db.Create(&art).Error)
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: art.ID, TopicTagID: tag.ID}).Error)
	}

	// --- act: weak board's candidates
	tags, _, err := collectBoardTags(boardWeak.ID, now)
	require.NoError(t, err)
	labels := make(map[string]bool, len(tags))
	for _, tg := range tags {
		labels[tg.Label] = true
	}
	assert.False(t, labels["美国非农"], "borrowed-word tag (0.7 here vs 1.0 elsewhere, gap 0.3) must be excluded from the weak board")
	assert.True(t, labels["中美贸易"], "genuine cross-board tag (gap 0.1 < 0.3) must stay on both boards")
	assert.True(t, labels["深交所问询"], "sole-mount tag must stay")

	// --- act: strong board's candidates are unaffected
	tagsHome, _, err := collectBoardTags(boardHome.ID, now)
	require.NoError(t, err)
	labelsHome := make(map[string]bool, len(tagsHome))
	for _, tg := range tagsHome {
		labelsHome[tg.Label] = true
	}
	assert.True(t, labelsHome["美国非农"], "the stronger home board keeps the tag")
	assert.True(t, labelsHome["中美贸易"], "cross-board genuine tag stays on the other side too")
}

// TestCollectBoardTags_ExclusivityDisabledAtZeroGap covers the kill switch:
// semantic_board_report_exclusive_gap = 0 disables the subquery and restores
// legacy behavior (the borrowed-word tag is collected again).
func TestCollectBoardTags_ExclusivityDisabledAtZeroGap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)

	require.NoError(t, db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).
		Create(&models.AISettings{Key: "semantic_board_report_exclusive_gap", Value: "0"}).Error)

	feed := models.Feed{Title: "test-feed", URL: "https://example.com/feed-collect-gap0"}
	require.NoError(t, db.Create(&feed).Error)

	boardHome := models.SemanticLabel{Label: "us-news", Slug: "us-news-gap0", LabelType: "board"}
	boardWeak := models.SemanticLabel{Label: "cn-news", Slug: "cn-news-gap0", LabelType: "board"}
	require.NoError(t, db.Create(&boardHome).Error)
	require.NoError(t, db.Create(&boardWeak).Error)

	tagBorrowed := models.TopicTag{Slug: "us-nonfarm-g0", Label: "美国非农g0", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tagBorrowed).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagBorrowed.ID, SemanticBoardID: boardWeak.ID,
		MatchReason: "direct_hit", Score: 0.7, Downgraded: false,
	}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagBorrowed.ID, SemanticBoardID: boardHome.ID,
		MatchReason: "max_sim", Score: 1.0, Downgraded: false,
	}).Error)

	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	art := models.Article{FeedID: feed.ID, Title: "a-gap0", PubDate: &pubDate}
	require.NoError(t, db.Create(&art).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: art.ID, TopicTagID: tagBorrowed.ID}).Error)

	tags, _, err := collectBoardTags(boardWeak.ID, now)
	require.NoError(t, err)
	require.Len(t, tags, 1, "gap=0 disables exclusivity: legacy behavior collects the weak mount")
	assert.Equal(t, "美国非农g0", tags[0].Label)
}

// TestCollectBoardTags_FallbackCrossBoardExclusivity covers fix-board-match-cross-domain
// task 2.2 / test-cases B6/B7: the fallback path (tags with auxiliaries but no
// mounted board labels, matched on the fly) applies the same cross-board
// exclusivity — a tag whose best live match outscores this board's match by
// >= the gap is not admitted here, while a tag whose two-board gap stays below
// the gap mounts on both boards.
func TestCollectBoardTags_FallbackCrossBoardExclusivity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)

	feed := models.Feed{Title: "test-feed", URL: "https://example.com/feed-collect-fb-exclusive"}
	require.NoError(t, db.Create(&feed).Error)

	boardHome := models.SemanticLabel{Label: "us-news", Slug: "us-news-fb", LabelType: "board", Status: "active"}
	boardWeak := models.SemanticLabel{Label: "cn-news", Slug: "cn-news-fb", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&boardHome).Error)
	require.NoError(t, db.Create(&boardWeak).Error)

	// Orthogonal basis vectors (padded to TestEmbeddingDim): tag auxiliaries
	// use S1/S2/F1..F3; board compositions mount distinct-ID copies of the same
	// directions so direct_hit overlap stays 0 and scores come from the
	// similarity rules only (hit_rate / max_sim / weighted).
	vecE1 := []float64{1, 0, 0, 0, 0}
	vecE2 := []float64{0, 1, 0, 0, 0}
	vecE3 := []float64{0, 0, 1, 0, 0}
	vecE4 := []float64{0, 0, 0, 1, 0}
	vecE5 := []float64{0, 0, 0, 0, 1}
	vecOnes := []float64{1, 1, 1, 1, 1}

	auxLabel := func(label, slug string, vec []float64) models.SemanticLabel {
		t.Helper()
		pg := core.FloatsToPgVector(testutil.PadVector(vec, testutil.TestEmbeddingDim))
		l := models.SemanticLabel{Label: label, Slug: slug, LabelType: "auxiliary", Status: "active", Embedding: &pg}
		require.NoError(t, db.Create(&l).Error)
		return l
	}

	s1 := auxLabel("fb-s1", "fb-s1", vecE1)
	s2 := auxLabel("fb-s2", "fb-s2", vecE2)
	f1 := auxLabel("fb-f1", "fb-f1", vecE3)
	f2 := auxLabel("fb-f2", "fb-f2", vecE4)
	f3 := auxLabel("fb-f3", "fb-f3", vecE5)
	home1 := auxLabel("fb-home-s1", "fb-home-s1", vecE1)
	home2 := auxLabel("fb-home-s2", "fb-home-s2", vecE2)
	weak1 := auxLabel("fb-weak-s1", "fb-weak-s1", vecE1)
	weak2 := auxLabel("fb-weak-ones", "fb-weak-ones", vecOnes)

	for _, pair := range [][2]uint{
		{boardHome.ID, home1.ID}, {boardHome.ID, home2.ID},
		{boardWeak.ID, weak1.ID}, {boardWeak.ID, weak2.ID},
	} {
		require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: pair[0], AuxiliaryLabelID: pair[1]}).Error)
	}

	link := func(tagID, labelID uint) {
		t.Helper()
		require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tagID, SemanticLabelID: labelID}).Error)
	}

	// tagExcluded: Home max_sim 1.0 (S1/S2 hit, hit_rate 2/5) vs Weak weighted
	// 0.68 (S1 hit only, hit_rate 1/5) — gap 0.32 >= 0.3 → excluded from Weak.
	tagExcluded := models.TopicTag{Slug: "fb-excl", Label: "被排他tag", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tagExcluded).Error)
	for _, l := range []models.SemanticLabel{s1, s2, f1, f2, f3} {
		link(tagExcluded.ID, l.ID)
	}

	// tagKept: Home hit_rate 0.9 (2/3 hits) vs Weak weighted 0.733 — gap
	// 0.167 < 0.3 → genuine cross-board, both boards keep it.
	tagKept := models.TopicTag{Slug: "fb-kept", Label: "双挂tag", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tagKept).Error)
	for _, l := range []models.SemanticLabel{s1, s2} {
		link(tagKept.ID, l.ID)
	}

	// Seed: collectBoardTags early-returns before the fallback when the main
	// query is empty (pre-existing behavior), so each board needs at least one
	// mounted tag for the fallback path to run. Equal-score dual mount keeps
	// the seed alive on both sides of the exclusivity NOT EXISTS.
	seed := models.TopicTag{Slug: "fb-seed", Label: "种子tag", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&seed).Error)
	for _, b := range []models.SemanticLabel{boardHome, boardWeak} {
		require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
			TopicTagID: seed.ID, SemanticBoardID: b.ID,
			MatchReason: "direct_hit", Score: 0.7, Downgraded: false,
		}).Error)
	}

	now := time.Now()
	pubDate := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	for _, tag := range []models.TopicTag{tagExcluded, tagKept, seed} {
		art := models.Article{FeedID: feed.ID, Title: "a-" + tag.Slug, PubDate: &pubDate}
		require.NoError(t, db.Create(&art).Error)
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: art.ID, TopicTagID: tag.ID}).Error)
	}

	// --- act: weak board's fallback candidates
	tagsWeak, _, err := collectBoardTags(boardWeak.ID, now)
	require.NoError(t, err)
	labelsWeak := make(map[string]bool, len(tagsWeak))
	for _, tg := range tagsWeak {
		labelsWeak[tg.Label] = true
	}
	assert.False(t, labelsWeak["被排他tag"], "fallback: tag with a 1.0 match elsewhere vs 0.68 here (gap 0.32 >= 0.3) must be excluded")
	assert.True(t, labelsWeak["双挂tag"], "fallback: gap 0.167 < 0.3 must stay (genuine cross-board)")

	// --- act: home board's candidates (first call persisted the computed
	// matches, so this rides the main query; matches[0] is home itself there)
	tagsHome, _, err := collectBoardTags(boardHome.ID, now)
	require.NoError(t, err)
	labelsHome := make(map[string]bool, len(tagsHome))
	for _, tg := range tagsHome {
		labelsHome[tg.Label] = true
	}
	assert.True(t, labelsHome["被排他tag"], "home board keeps its strongest tag")
	assert.True(t, labelsHome["双挂tag"], "home board keeps the cross-board tag")
}
