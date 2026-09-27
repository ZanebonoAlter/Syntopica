package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// findNormalizeArticleLinkFragmentsMigration locates migration 20260920_0001's
// Up closure.
func findNormalizeArticleLinkFragmentsMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260920_0001" {
			return m.Up
		}
	}
	return nil
}

// TestNormalizeArticleLinkFragmentsMigrationMergesFragmentGroups covers the
// v2ex repair: entry links that differ only in the drifting #replyN anchor
// collapse to one row whose link is the stripped base URL; a bare exact-link
// row in the same feed is folded in as the natural keeper.
func TestNormalizeArticleLinkFragmentsMigrationMergesFragmentGroups(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findNormalizeArticleLinkFragmentsMigration()
	require.NotNil(t, up, "migration 20260920_0001 must be registered")

	feed := models.Feed{Title: "V2EX - 技术", URL: "https://example.com/v2ex-tech"}
	require.NoError(t, db.Create(&feed).Error)

	bare := models.Article{FeedID: feed.ID, Title: "同主题", Link: "https://www.v2ex.com/t/1#reply2"}
	require.NoError(t, db.Create(&bare).Error)
	rich := models.Article{FeedID: feed.ID, Title: "同主题", Link: "https://www.v2ex.com/t/1#reply18"}
	require.NoError(t, db.Create(&rich).Error)
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", rich.ID).
		Update("firecrawl_content", "正文").Error)
	// A sibling topic stored once without any fragment: normalization only.
	solo := models.Article{FeedID: feed.ID, Title: "另一帖", Link: "https://www.v2ex.com/t/2#reply4"}
	require.NoError(t, db.Create(&solo).Error)

	require.NoError(t, up(db))

	var group []models.Article
	require.NoError(t, db.Where("feed_id = ?", feed.ID).
		Where("link = ?", "https://www.v2ex.com/t/1").Find(&group).Error)
	require.Len(t, group, 1, "fragment-drift group must collapse to one row")
	require.Equal(t, rich.ID, group[0].ID, "the crawled copy must win the keeper score")

	var soloRow models.Article
	require.NoError(t, db.First(&soloRow, solo.ID).Error)
	require.Equal(t, "https://www.v2ex.com/t/2", soloRow.Link, "singleton fragment link is stripped")
}

// TestNormalizeArticleLinkFragmentsMigrationFoldsExactLinkRow covers the mixed
// group: a row already stored with the bare link plus a fragment copy merge
// into exactly one row with the bare link.
func TestNormalizeArticleLinkFragmentsMigrationFoldsExactLinkRow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findNormalizeArticleLinkFragmentsMigration()
	require.NotNil(t, up)

	feed := models.Feed{Title: "混合源", URL: "https://example.com/mixed"}
	require.NoError(t, db.Create(&feed).Error)

	bare := models.Article{FeedID: feed.ID, Title: "裸链接行", Link: "https://example.com/t/9"}
	require.NoError(t, db.Create(&bare).Error)
	frag := models.Article{FeedID: feed.ID, Title: "锚点行", Link: "https://example.com/t/9#reply7"}
	require.NoError(t, db.Create(&frag).Error)

	require.NoError(t, up(db))

	var rows []models.Article
	require.NoError(t, db.Where("feed_id = ?", feed.ID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "https://example.com/t/9", rows[0].Link)
}

// TestNormalizeArticleLinkFragmentsMigrationPreservesHashbangAndOthers covers
// the boundary: #! hashbang links (SPA route identity), plain links and
// empty-link rows all stay untouched.
func TestNormalizeArticleLinkFragmentsMigrationPreservesHashbangAndOthers(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findNormalizeArticleLinkFragmentsMigration()
	require.NotNil(t, up)

	feed := models.Feed{Title: "边界源", URL: "https://example.com/edge"}
	require.NoError(t, db.Create(&feed).Error)

	hashbang := models.Article{FeedID: feed.ID, Title: "hashbang", Link: "https://example.com/#!/article/1"}
	require.NoError(t, db.Create(&hashbang).Error)
	plain := models.Article{FeedID: feed.ID, Title: "plain", Link: "https://example.com/plain"}
	require.NoError(t, db.Create(&plain).Error)
	empty := models.Article{FeedID: feed.ID, Title: "empty", Link: ""}
	require.NoError(t, db.Create(&empty).Error)

	require.NoError(t, up(db))

	var rows []models.Article
	require.NoError(t, db.Where("feed_id = ?", feed.ID).Find(&rows).Error)
	require.Len(t, rows, 3)
	byLink := make(map[string]models.Article, len(rows))
	for _, r := range rows {
		byLink[r.Link] = r
	}
	// SetupTestDB's shared db must not take inline-pk First calls (conditions
	// stick to the shared statement) — assert via a Find, same as the dedupe
	// migration tests do.
	require.Equal(t, "https://example.com/#!/article/1", byLink["https://example.com/#!/article/1"].Link,
		"hashbang fragment must survive")
	require.Equal(t, plain.ID, byLink["https://example.com/plain"].ID,
		"plain link row must survive untouched")
	require.Contains(t, byLink, "", "empty-link row must survive")
}

// TestNormalizeArticleLinkFragmentsMigrationIdempotent covers the re-run path:
// applying the migration twice leaves the very same rows behind.
func TestNormalizeArticleLinkFragmentsMigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findNormalizeArticleLinkFragmentsMigration()
	require.NotNil(t, up)

	feed := models.Feed{Title: "幂等源", URL: "https://example.com/frag-idem"}
	require.NoError(t, db.Create(&feed).Error)
	for _, link := range []string{
		"https://example.com/t/1#reply1",
		"https://example.com/t/1#reply2",
		"https://example.com/t/2#reply3",
	} {
		require.NoError(t, db.Create(&models.Article{FeedID: feed.ID, Title: "帖", Link: link}).Error)
	}

	require.NoError(t, up(db))
	var afterFirst []models.Article
	require.NoError(t, db.Where("feed_id = ?", feed.ID).Order("id").Find(&afterFirst).Error)
	require.Len(t, afterFirst, 2)
	require.Equal(t, "https://example.com/t/1", afterFirst[0].Link)
	require.Equal(t, "https://example.com/t/2", afterFirst[1].Link)

	require.NoError(t, up(db), "re-running the migration must not fail")
	var afterSecond []models.Article
	require.NoError(t, db.Where("feed_id = ?", feed.ID).Order("id").Find(&afterSecond).Error)
	require.Equal(t, afterFirst, afterSecond)
}
