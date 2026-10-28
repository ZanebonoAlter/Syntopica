package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── 泳道内主题分桶：数据模型 / 配置 / 落库（add-intra-lane-theme-bucketing）───
//
// repository 包禁 SQLite（standard/backend/testing.md 红线），统一走
// testutil.SetupTestDB 的 testcontainer PG。

// TestDailyReportSectionBucketColumns covers tasks 1.1: the two bucket columns
// are readable/writable, historical rows keep the DB defaults (bucket_index=0,
// bucket_title NULL), and sibling bucket sections may share a cluster_index
// ((report_id, cluster_index) deliberately has NO unique constraint).
func TestDailyReportSectionBucketColumns(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	reportID := seedTestReport(t, db, boardID, NormalizeReportDate(time.Now()))

	// Historical-row shape: insert without touching the bucket columns so the
	// DB defaults apply (the migration's whole point is "no backfill needed").
	require.NoError(t, db.Exec(`
		INSERT INTO daily_report_sections (report_id, cluster_index, cluster_label, cluster_tag_ids, article_count, best_tier, avg_score, embedding)
		VALUES (?, 0, 'legacy', '[]', 1, 1, 0.5, '[0]')
	`, reportID).Error)

	var legacy DailyReportSection
	require.NoError(t, db.Where("report_id = ? AND cluster_label = ?", reportID, "legacy").First(&legacy).Error)
	assert.Equal(t, 0, legacy.BucketIndex, "historical rows default to bucket_index=0")
	assert.Empty(t, legacy.BucketTitle, "historical rows keep bucket_title NULL")
	var legacyTitleIsNull bool
	require.NoError(t, db.Raw(`SELECT bucket_title IS NULL FROM daily_report_sections WHERE id = ?`, legacy.ID).
		Row().Scan(&legacyTitleIsNull))
	assert.True(t, legacyTitleIsNull, "bucket_title must be NULL (not empty string) on pre-migration rows")

	// Sibling buckets: same cluster_index twice, distinguished by bucket_index.
	siblings := []DailyReportSection{
		{ReportID: reportID, ClusterIndex: 1, ClusterLabel: "工具调用与编排", BucketTitle: "Agent 工具", BucketIndex: 0,
			ClusterTagIDs: JSON("[]"), Embedding: FloatsToPgVector([]float64{0})},
		{ReportID: reportID, ClusterIndex: 1, ClusterLabel: "Skill 生态", BucketTitle: "Skill 与规范", BucketIndex: 1,
			ClusterTagIDs: JSON("[]"), Embedding: FloatsToPgVector([]float64{0})},
	}
	require.NoError(t, db.Create(&siblings).Error)

	var loaded []DailyReportSection
	require.NoError(t, repo.DB().Where("report_id = ? AND cluster_index = ?", reportID, 1).
		Order("bucket_index ASC").Find(&loaded).Error)
	require.Len(t, loaded, 2)
	assert.Equal(t, "Agent 工具", loaded[0].BucketTitle)
	assert.Equal(t, 0, loaded[0].BucketIndex)
	assert.Equal(t, "Skill 与规范", loaded[1].BucketTitle)
	assert.Equal(t, 1, loaded[1].BucketIndex)
}

// TestPersistentTopicConfigBucketDefaults covers tasks 1.2: the three new
// config keys (enabled / min_threads / min_tags) default to true/6/10 and are
// overridable through ai_settings.
func TestPersistentTopicConfigBucketDefaults(t *testing.T) {
	t.Run("no rows uses defaults", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		cfg := LoadPersistentTopicConfig(db)
		assert.True(t, cfg.IntraBucketEnabled)
		assert.Equal(t, 6, cfg.IntraBucketMinThreads)
		assert.Equal(t, 10, cfg.IntraBucketMinTags)
	})

	t.Run("rows override defaults", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		for _, row := range []models.AISettings{
			{Key: "daily_report_intra_bucket_enabled", Value: "false"},
			{Key: "daily_report_intra_bucket_min_threads", Value: "9"},
			{Key: "daily_report_intra_bucket_min_tags", Value: "14"},
		} {
			require.NoError(t, db.Create(&row).Error)
		}
		cfg := LoadPersistentTopicConfig(db)
		assert.False(t, cfg.IntraBucketEnabled)
		assert.Equal(t, 9, cfg.IntraBucketMinThreads)
		assert.Equal(t, 14, cfg.IntraBucketMinTags)
	})

	t.Run("invalid rows keep defaults", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		for _, row := range []models.AISettings{
			{Key: "daily_report_intra_bucket_enabled", Value: "maybe"},
			{Key: "daily_report_intra_bucket_min_threads", Value: "0"},
			{Key: "daily_report_intra_bucket_min_tags", Value: "-3"},
		} {
			require.NoError(t, db.Create(&row).Error)
		}
		cfg := LoadPersistentTopicConfig(db)
		assert.True(t, cfg.IntraBucketEnabled)
		assert.Equal(t, 6, cfg.IntraBucketMinThreads)
		assert.Equal(t, 10, cfg.IntraBucketMinTags)
	})
}

// TestSaveReportPersistsBucketColumns covers tasks 1.3: SaveReport writes the
// bucket columns inside the same transaction that writes sections/threads.
func TestSaveReportPersistsBucketColumns(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository := NewTopicGraphRepository(db)
	Repo = repository
	defer func() { Repo = nil }()

	boardID := seedTestBoard(t, db)
	date := NormalizeReportDate(time.Now())

	report := &BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      date,
		Title:           "bucket columns",
		Status:          "completed",
	}
	sections := []DailyReportSection{
		{ClusterIndex: 3, ClusterLabel: "方向A", BucketTitle: "工具调用", BucketIndex: 0,
			ClusterTagIDs: JSON("[1,2]"), Embedding: FloatsToPgVector([]float64{1, 0}),
			LaneTier: "l3_new", MatchedTopicID: nil},
		{ClusterIndex: 3, ClusterLabel: "方向B", BucketTitle: "Skill 生态", BucketIndex: 1,
			ClusterTagIDs: JSON("[3]"), Embedding: FloatsToPgVector([]float64{0, 1}),
			LaneTier: "l3_new", MatchedTopicID: nil},
		{ClusterIndex: 4, ClusterLabel: "未分桶", ClusterTagIDs: JSON("[4]"),
			Embedding: FloatsToPgVector([]float64{1, 1}), LaneTier: "l3_new"},
	}
	batches := [][]DailyReportThread{
		{{Title: "t0"}},
		{{Title: "t1"}},
		{{Title: "t2"}},
	}
	require.NoError(t, repository.SaveReport(report, sections, batches))

	var rows []DailyReportSection
	require.NoError(t, db.Where("report_id = ?", report.ID).Order("cluster_index ASC, bucket_index ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	assert.Equal(t, 3, rows[0].ClusterIndex)
	assert.Equal(t, 0, rows[0].BucketIndex)
	assert.Equal(t, "工具调用", rows[0].BucketTitle)
	assert.Equal(t, 1, rows[1].BucketIndex)
	assert.Equal(t, "Skill 生态", rows[1].BucketTitle)
	assert.Equal(t, 4, rows[2].ClusterIndex)
	assert.Equal(t, 0, rows[2].BucketIndex, "un-bucketed section keeps bucket_index=0")
	assert.Empty(t, rows[2].BucketTitle)
}
