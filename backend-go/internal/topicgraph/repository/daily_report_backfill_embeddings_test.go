package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// seedBackfillSection inserts a section with explicit tag IDs / label /
// embedding (embedding empty string → NULL is not possible on a non-nullable
// vector column in the test DB; NULL is only needed for fill-mode tests which
// create the row via raw SQL).
func seedBackfillSection(t *testing.T, db *gorm.DB, reportID uint, label string, tagIDs JSON, embedding string) uint {
	t.Helper()
	if embedding == "" {
		embedding = FloatsToPgVector([]float64{0})
	}
	sec := DailyReportSection{
		ReportID:      reportID,
		ClusterLabel:  label,
		ClusterTagIDs: tagIDs,
		ArticleCount:  1,
		Embedding:     embedding,
	}
	require.NoError(t, db.Create(&sec).Error)
	return sec.ID
}

func seedBackfillTag(t *testing.T, db *gorm.DB, label, desc string) uint {
	t.Helper()
	tag := models.TopicTag{Slug: "slug-" + label, Label: label, Description: desc, Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag.ID
}

// TestAssembleSectionEmbedText_TagFacts asserts the content-assembly rules:
// tag label + description, per-tag article excerpt over the section's day
// window, thread-title fallback, cluster-label fallback.
func TestAssembleSectionEmbedText_TagFacts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Now())
	r1 := seedTestReport(t, db, boardID, day)

	tagID := seedBackfillTag(t, db, "美伊谈判", "核协议磋商")

	// An article linked to the tag, published on the section's day.
	feed := models.Feed{Title: "f", URL: "https://example.com/bf"}
	require.NoError(t, db.Create(&feed).Error)
	art := models.Article{FeedID: feed.ID, Title: "谈判进展", Description: "双方达成初步共识"}
	pubDate := day.Add(2 * time.Hour)
	art.PubDate = &pubDate
	require.NoError(t, db.Create(&art).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: art.ID, TopicTagID: tagID}).Error)

	secID := seedBackfillSection(t, db, r1, "标题", JSON(mustJSON(t, []uint{tagID})), "")

	got := repo.assembleSectionEmbedText([]uint{tagID}, day, secID)
	assert.Contains(t, got, "美伊谈判：核协议磋商")
	assert.Contains(t, got, "《谈判进展》双方达成初步共识")

	// A tag with no linked article on the day → article part omitted.
	tag2 := seedBackfillTag(t, db, "油价波动", "")
	got2 := repo.assembleSectionEmbedText([]uint{tag2}, day, secID)
	assert.Equal(t, "油价波动", got2)
}

// TestAssembleSectionEmbedText_FallbackChain asserts thread-titles then
// cluster_label fallbacks.
func TestAssembleSectionEmbedText_FallbackChain(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Now())
	r1 := seedTestReport(t, db, boardID, day)

	// No tags → thread titles.
	secA := seedBackfillSection(t, db, r1, "标题A", nil, "")
	require.NoError(t, db.Create(&DailyReportThread{ReportID: r1, SectionID: secA, Title: "线索甲", Embedding: FloatsToPgVector([]float64{0})}).Error)
	require.NoError(t, db.Create(&DailyReportThread{ReportID: r1, SectionID: secA, Title: "线索乙", Embedding: FloatsToPgVector([]float64{0})}).Error)
	assert.Equal(t, "线索甲\n线索乙", repo.assembleSectionEmbedText(nil, day, secA))

	// No tags, no threads → cluster_label.
	secB := seedBackfillSection(t, db, r1, "标题B", nil, "")
	assert.Equal(t, "标题B", repo.assembleSectionEmbedText(nil, day, secB))
}

// TestBackfillSectionEmbeddings_RangeFilter asserts the recompute-mode range
// filter: board_id and since_days restrict which sections are re-embedded.
func TestBackfillSectionEmbeddings_RangeFilter(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardA := seedTestBoard(t, db)
	boardB := models.SemanticLabel{Label: "test-board-bf-b", Slug: "test-board-bf-b", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&boardB).Error)
	today := NormalizeReportDate(time.Now())
	old := NormalizeReportDate(time.Now().AddDate(0, 0, -40))

	rA1 := seedTestReport(t, db, boardA, today)
	rA2 := seedTestReport(t, db, boardA, old)      // out of 30d window
	rB1 := seedTestReport(t, db, boardB.ID, today) // other board

	tagA := seedBackfillTag(t, db, "话题甲", "")
	seedBackfillSection(t, db, rA1, "A今日", JSON(mustJSON(t, []uint{tagA})), FloatsToPgVector([]float64{0.5}))
	seedBackfillSection(t, db, rA2, "A旧日", nil, FloatsToPgVector([]float64{0.6}))
	seedBackfillSection(t, db, rB1, "B今日", nil, FloatsToPgVector([]float64{0.7}))

	// Run recompute restricted to boardA, 30 days. Only "A今日" is in range.
	// The embed path needs a routed provider; register nothing → embed fails
	// per batch, sections are skipped (contract: skip-and-continue).
	_, skipped, _, err := repo.BackfillSectionEmbeddings(t.Context(), true, &boardA, 30)
	require.NoError(t, err)
	assert.Equal(t, 1, skipped, "only boardA/30d section attempted; embed failure skips it")

	// since_days=0 (unlimited) → both boardA sections attempted.
	_, skipped, _, err = repo.BackfillSectionEmbeddings(t.Context(), true, &boardA, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, skipped)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
