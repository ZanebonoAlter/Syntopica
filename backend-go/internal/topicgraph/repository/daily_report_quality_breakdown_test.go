package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// TestQualityBreakdownMigrationIdempotent verifies the quality_breakdown
// column migration can be run twice without error (DP-6 idempotency).
func TestQualityBreakdownMigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Run migrations twice — second run must be no-op.
	err := database.RunMigrations(db)
	require.NoError(t, err, "first RunMigrations")
	err = database.RunMigrations(db)
	require.NoError(t, err, "second RunMigrations (must be idempotent)")

	// Verify column exists.
	var colExists bool
	err = db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'daily_report_sections' AND column_name = 'quality_breakdown'
	)`).Scan(&colExists).Error
	require.NoError(t, err)
	assert.True(t, colExists, "quality_breakdown column should exist after migration")
}

// TestQualityBreakdownWriteReadRoundtrip writes a section with quality_breakdown
// and reads it back, verifying field-level fidelity.
func TestQualityBreakdownWriteReadRoundtrip(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)

	// downgraded 键是历史 JSON 快照的遗留字段（2026-11-12 已退役，新报告不再写入）；
	// 这里刻意保留它，验证读取端对历史键的兼容解析不受影响。
	breakdown := []map[string]any{
		{"tag_id": float64(1), "label": "AI芯片", "match_reason": "direct_hit", "score": float64(1.0), "downgraded": false},
		{"tag_id": float64(2), "label": "GPT-5发布", "match_reason": "max_sim", "score": float64(0.85), "downgraded": false},
		{"tag_id": float64(3), "label": "AI竞赛", "match_reason": "weighted", "score": float64(0.59), "downgraded": false},
	}
	breakdownJSON, err := json.Marshal(breakdown)
	require.NoError(t, err)

	section := DailyReportSection{
		ReportID:         reportID,
		ClusterLabel:     "test-section",
		ClusterTagIDs:    JSON(`[1,2,3]`),
		ArticleCount:     3,
		BestTier:         0,
		AvgScore:         0.8133,
		QualityBreakdown: JSON(breakdownJSON),
		Embedding:        FloatsToPgVector([]float64{0}),
	}
	err = db.Create(&section).Error
	require.NoError(t, err)

	// Read back via detail API
	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)

	sec := report.Sections[0]
	require.NotNil(t, sec.QualityBreakdown)
	assert.Equal(t, 0, sec.BestTier)

	var result []map[string]any
	err = json.Unmarshal(sec.QualityBreakdown, &result)
	require.NoError(t, err)
	assert.Len(t, result, 3)

	// Verify specific entries
	assert.Equal(t, "AI芯片", result[0]["label"])
	assert.Equal(t, "direct_hit", result[0]["match_reason"])
	assert.Equal(t, float64(1.0), result[0]["score"])
	assert.Equal(t, false, result[0]["downgraded"])
}

// TestQualityBreakdownHistoricNullCompatibility verifies that sections
// without quality_breakdown (NULL) are read back correctly without error.
func TestQualityBreakdownHistoricNullCompatibility(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)

	// Create a section with quality_breakdown explicitly NULL.
	section := DailyReportSection{
		ReportID:         reportID,
		ClusterLabel:     "historic-section",
		ClusterTagIDs:    JSON(`[100]`),
		ArticleCount:     1,
		BestTier:         1,
		AvgScore:         0.5,
		QualityBreakdown: nil, // NULL — historical row
		Embedding:        FloatsToPgVector([]float64{0}),
	}
	err := db.Create(&section).Error
	require.NoError(t, err)

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)

	sec := report.Sections[0]
	assert.Nil(t, sec.QualityBreakdown, "historical section quality_breakdown should be nil")
	assert.Equal(t, 1, sec.BestTier, "best_tier should still be present")
}

// TestQualityBreakdownRollbackReversible verifies that DROP COLUMN
// quality_breakdown succeeds and the table remains functional.
func TestQualityBreakdownRollbackReversible(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Drop the column
	err := db.Exec("ALTER TABLE daily_report_sections DROP COLUMN IF EXISTS quality_breakdown").Error
	require.NoError(t, err)

	// Verify column is gone
	var colExists bool
	err = db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'daily_report_sections' AND column_name = 'quality_breakdown'
	)`).Scan(&colExists).Error
	require.NoError(t, err)
	assert.False(t, colExists, "column should be dropped")

	// Re-create column (simulating re-migration)
	err = db.Exec("ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS quality_breakdown JSONB NULL").Error
	require.NoError(t, err)

	// Verify column is back
	err = db.Raw(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'daily_report_sections' AND column_name = 'quality_breakdown'
	)`).Scan(&colExists).Error
	require.NoError(t, err)
	assert.True(t, colExists, "column should be recreated")

	// Verify we can still insert and read sections
	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)

	section := DailyReportSection{
		ReportID:     reportID,
		ClusterLabel: "post-rollback-section",
		ArticleCount: 1,
		BestTier:     0,
		Embedding:    FloatsToPgVector([]float64{0}),
	}
	err = db.Create(&section).Error
	require.NoError(t, err)

	var count int64
	db.Model(&DailyReportSection{}).Count(&count)
	assert.GreaterOrEqual(t, count, int64(1))
}

// TestQualityBreakdownMigrationPreservesExistingData ensures running
// the migration does not corrupt existing sections.
func TestQualityBreakdownMigrationPreservesExistingData(t *testing.T) {
	db := testutil.SetupTestDB(t)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)

	// Create a section BEFORE the quality_breakdown column is manually verified
	section := DailyReportSection{
		ReportID:     reportID,
		ClusterLabel: "pre-migration-section",
		ArticleCount: 2,
		BestTier:     2,
		AvgScore:     0.75,
		Embedding:    FloatsToPgVector([]float64{0}),
	}
	err := db.Create(&section).Error
	require.NoError(t, err)

	// Run migration again (idempotent)
	err = database.RunMigrations(db)
	require.NoError(t, err)

	// Read back — existing data intact
	var readSection DailyReportSection
	err = db.First(&readSection, section.ID).Error
	require.NoError(t, err)
	assert.Equal(t, "pre-migration-section", readSection.ClusterLabel)
	assert.Equal(t, 2, readSection.BestTier)
	assert.Equal(t, 0.75, readSection.AvgScore)
	assert.Nil(t, readSection.QualityBreakdown, "pre-existing section should have nil quality_breakdown")
}

// TestTimelineAPIExposesQualityBreakdown verifies that GetBoardSectionTimeline
// returns quality_breakdown field (array for new sections, null for historic).
func TestTimelineAPIExposesQualityBreakdown(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	// Section with quality_breakdown populated
	breakdownJSON := json.RawMessage(`[{"tag_id":1,"label":"test","match_reason":"direct_hit","score":1.0,"downgraded":false}]`)
	section := DailyReportSection{
		ReportID:         seedTestReport(t, db, boardID, now),
		ClusterLabel:     "section-with-detail",
		ArticleCount:     1,
		BestTier:         0,
		AvgScore:         1.0,
		QualityBreakdown: JSON(breakdownJSON),
		Embedding:        FloatsToPgVector([]float64{0}),
	}
	err := db.Create(&section).Error
	require.NoError(t, err)

	// Historic section without quality_breakdown
	histSection := DailyReportSection{
		ReportID:         seedTestReport(t, db, boardID, now.AddDate(0, 0, -7)),
		ClusterLabel:     "historic-section",
		ArticleCount:     2,
		BestTier:         1,
		AvgScore:         0.6,
		QualityBreakdown: nil,
		Embedding:        FloatsToPgVector([]float64{0}),
	}
	err = db.Create(&histSection).Error
	require.NoError(t, err)

	// Test GetBoardSectionTimeline
	resp, err := repo.GetBoardSectionTimeline(boardID, 30)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(resp.Sections), 1)

	foundNew := false
	foundHist := false
	for _, s := range resp.Sections {
		if s.ID == section.ID {
			require.NotNil(t, s.QualityBreakdown, "new section quality_breakdown should not be null")
			assert.JSONEq(t, string(breakdownJSON), string(s.QualityBreakdown))
			foundNew = true
		}
		if s.ID == histSection.ID {
			assert.Nil(t, s.QualityBreakdown, "historic section quality_breakdown should be null")
			foundHist = true
		}
	}
	assert.True(t, foundNew, "new section should be in timeline")
	assert.True(t, foundHist, "historic section should be in timeline")

	// Test GetSectionLifecycle
	lcResp, lcErr := repo.GetSectionLifecycle(section.ID)
	require.NoError(t, lcErr)
	require.Len(t, lcResp.Sections, 1)
	require.NotNil(t, lcResp.Sections[0].QualityBreakdown)
	assert.JSONEq(t, string(breakdownJSON), string(lcResp.Sections[0].QualityBreakdown))
}
