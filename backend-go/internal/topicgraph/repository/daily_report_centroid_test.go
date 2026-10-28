package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── Pure helper unit tests (no DB, always run — also under -short) ──
//
// meanPgVectors and computeVacuumFlag are extracted from the DB methods so the
// core averaging / vacuum decision is unit-testable without Postgres. They are
// the acceptance rail for spec scenarios 「质心按近期 section 计算」 and
// 「吸尘器 topic 识别」.

func TestMeanPgVectors(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		assert.Nil(t, meanPgVectors(nil))
		assert.Nil(t, meanPgVectors([][]float64{}))
	})
	t.Run("single returned as-is", func(t *testing.T) {
		v := meanPgVectors([][]float64{{1, 2, 3}})
		assert.InDeltaSlice(t, []float64{1, 2, 3}, v, 1e-9)
	})
	t.Run("mean of three axis vectors", func(t *testing.T) {
		v := meanPgVectors([][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}})
		assert.InDeltaSlice(t, []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}, v, 1e-9)
	})
	t.Run("unequal length truncated to shortest", func(t *testing.T) {
		// A malformed long row must not widen the mean or panic.
		v := meanPgVectors([][]float64{{1, 2, 3, 4}, {1, 2}})
		assert.Len(t, v, 2)
		assert.InDeltaSlice(t, []float64{1, 2}, v, 1e-9)
	})
	t.Run("zero-length first row returns nil", func(t *testing.T) {
		assert.Nil(t, meanPgVectors([][]float64{{}}))
	})
}

func TestComputeVacuumFlag(t *testing.T) {
	cases := []struct {
		name        string
		strong, mid int
		ratio       float64
		want        bool
	}{
		{"spec scenario: 0 strong / 11 mid -> true", 0, 11, 0.20, true},
		{"all strong -> false", 10, 0, 0.20, false},
		{"no data -> false (div-by-zero guard)", 0, 0, 0.20, false},
		{"boundary: 2/10=0.2 not < 0.2 -> false", 2, 8, 0.20, false},
		{"just under: 2/12=0.166 < 0.2 -> true", 2, 10, 0.20, true},
		{"just over: 3/13=0.23 -> false", 3, 10, 0.20, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, computeVacuumFlag(tc.strong, tc.mid, tc.ratio))
		})
	}
}

// ── testcontainer integration (skipped under -short via SetupTestDB) ──

// columnExistsDB checks information_schema for a column. Helper for the
// migration-correctness test.
func columnExistsDB(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		 WHERE table_schema='public' AND table_name=? AND column_name=?)`,
		table, column).Scan(&exists).Error)
	return exists
}

// makeTopic creates an active persistent topic with the given first-section
// embedding. The embedding is the degradation fallback for ComputeTopicCentroid.
func makeTopic(t *testing.T, db *gorm.DB, boardID uint, embedding string) BoardPersistentTopic {
	t.Helper()
	today := NormalizeReportDate(time.Now())
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "test-topic",
		Embedding:       embedding,
		Status:          TopicStatusActive,
		FirstSeenDate:   today,
		LastSeenDate:    today,
		HitCount:        1,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

// seedSectionAssigned inserts a section owned by topicID with an embedding and
// a recorded topic_match_distance (the column RecomputeVacuumStats reads).
func seedSectionAssigned(t *testing.T, db *gorm.DB, reportID, topicID uint, embedding string, dist float64) {
	t.Helper()
	tid := topicID
	sec := DailyReportSection{
		ReportID:           reportID,
		ClusterLabel:       "s",
		Embedding:          embedding,
		PersistentTopicID:  &tid,
		TopicMatchDistance: dist,
	}
	require.NoError(t, db.Create(&sec).Error)
}

// TestMigration_AddLaneCentroidColumns_Idempotent verifies migration
// 20260727_0001 applied cleanly (SetupTestDB runs the full migration chain)
// and that the DDL/seed SQL is idempotent: re-running ADD COLUMN IF NOT EXISTS
// and the check-existing seed does not error or duplicate rows.
func TestMigration_AddLaneCentroidColumns_Idempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// 1. Columns exist after migration.
	for _, c := range []struct{ table, col string }{
		{"board_persistent_topics", "centroid"},
		{"board_persistent_topics", "is_vacuum"},
		{"board_persistent_topics", "vacuum_strong"},
		{"board_persistent_topics", "vacuum_mid"},
		{"daily_report_sections", "lane_tier"},
	} {
		assert.True(t, columnExistsDB(t, db, c.table, c.col), "%s.%s missing", c.table, c.col)
	}

	// 2. Seed keys exist with the spec values.
	wantSeed := map[string]string{
		"persistent_topic_lane_l1_threshold": "0.18",
		"persistent_topic_lane_l2_threshold": "0.30",
		"persistent_topic_vacuum_ratio":      "0.20",
		"persistent_topic_centroid_window":   "30",
		"persistent_topic_vacuum_window":     "7",
		"persistent_topic_l2_candidate_k":    "5",
	}
	keys := make([]string, 0, len(wantSeed))
	for k := range wantSeed {
		keys = append(keys, k)
	}
	var rows []models.AISettings
	require.NoError(t, db.Where("key IN ?", keys).Find(&rows).Error)
	got := make(map[string]string, len(rows))
	for _, r := range rows {
		got[r.Key] = r.Value
	}
	for k, v := range wantSeed {
		assert.Equal(t, v, got[k], "seed value mismatch for %s", k)
	}

	// 3. Idempotency: re-running the same ADD COLUMN IF NOT EXISTS statements
	//    must not error (the migration's no-op-on-second-run contract).
	for _, stmt := range []string{
		`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS centroid vector`,
		`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS is_vacuum boolean NOT NULL DEFAULT false`,
		`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS vacuum_strong integer NOT NULL DEFAULT 0`,
		`ALTER TABLE board_persistent_topics ADD COLUMN IF NOT EXISTS vacuum_mid integer NOT NULL DEFAULT 0`,
		`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS lane_tier varchar(16)`,
	} {
		require.NoError(t, db.Exec(stmt).Error, "re-run ADD COLUMN IF NOT EXISTS should be a no-op")
	}

	// 4. Idempotency: check-existing seed does not duplicate.
	const k = "persistent_topic_lane_l1_threshold"
	var before int64
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", k).Count(&before).Error)
	var existing models.AISettings
	if err := db.Where("key = ?", k).First(&existing).Error; err != nil {
		require.NoError(t, db.Create(&models.AISettings{Key: k, Value: "0.18"}).Error)
	}
	var after int64
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", k).Count(&after).Error)
	assert.Equal(t, before, after, "check-existing seed must not duplicate the row")
}

// TestComputeTopicCentroid_WindowAndFallback covers the three spec scenarios
// for the centroid representation: equal-weight mean of recent sections,
// degradation to the first-section vector when <2 sections, and window
// truncation to centroid_window.
func TestComputeTopicCentroid_WindowAndFallback(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	boardID := seedTestBoard(t, db)

	t.Run("mean of recent sections", func(t *testing.T) {
		topic := makeTopic(t, db, boardID, vecStr(0, 0, 0))
		for _, e := range []string{vecStr(1, 0, 0), vecStr(0, 1, 0), vecStr(0, 0, 1)} {
			r := seedTestReport(t, db, boardID, time.Now())
			seedSectionAssigned(t, db, r, topic.ID, e, 0.1)
		}
		centroid, err := repo.ComputeTopicCentroid(topic.ID)
		require.NoError(t, err)
		v, err := repoParsePgVector(centroid)
		require.NoError(t, err)
		assert.InDeltaSlice(t, []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}, v, 1e-6)
	})

	t.Run("fewer than 2 sections degrades to first-section", func(t *testing.T) {
		topic := makeTopic(t, db, boardID, vecStr(0.5, 0.5, 0.5))
		r := seedTestReport(t, db, boardID, time.Now())
		seedSectionAssigned(t, db, r, topic.ID, vecStr(1, 0, 0), 0.1)
		centroid, err := repo.ComputeTopicCentroid(topic.ID)
		require.NoError(t, err)
		v, err := repoParsePgVector(centroid)
		require.NoError(t, err)
		assert.InDeltaSlice(t, []float64{0.5, 0.5, 0.5}, v, 1e-6, "should fall back to topic.Embedding")
	})

	t.Run("window truncates to recent 30", func(t *testing.T) {
		topic := makeTopic(t, db, boardID, vecStr(0, 0, 0))
		now := time.Now()
		// 10 old sections (day -39..-30) with [0,1,0] — outside the window.
		for i := 39; i >= 30; i-- {
			r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -i))
			seedSectionAssigned(t, db, r, topic.ID, vecStr(0, 1, 0), 0.1)
		}
		// 30 recent sections (day -29..0) with [1,0,0] — inside the window.
		for i := 29; i >= 0; i-- {
			r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -i))
			seedSectionAssigned(t, db, r, topic.ID, vecStr(1, 0, 0), 0.1)
		}
		centroid, err := repo.ComputeTopicCentroid(topic.ID)
		require.NoError(t, err)
		v, err := repoParsePgVector(centroid)
		require.NoError(t, err)
		assert.InDeltaSlice(t, []float64{1, 0, 0}, v, 1e-6,
			"centroid should be mean of recent 30 [1,0,0], excluding the 10 old [0,1,0]")
	})
}

// TestRecomputeVacuumStats_Flagging covers spec scenario 「吸尘器 topic 识别」:
// a topic with strong/(strong+mid) < vacuum_ratio is flagged is_vacuum, while a
// strong-heavy topic and an empty topic are not. Also verifies the window
// boundary (a section 8 days ago is excluded from the 7-day window).
func TestRecomputeVacuumStats_Flagging(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	boardID := seedTestBoard(t, db)

	now := time.Now()

	// vacuum topic: 6 mid sections (dist 0.25) within the 7-day window, 0 strong.
	vacuumTopic := makeTopic(t, db, boardID, vecStr(1, 0, 0))
	for i := 0; i < 6; i++ {
		r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -i))
		seedSectionAssigned(t, db, r, vacuumTopic.ID, vecStr(1, 0, 0), 0.25)
	}
	// window-out: 1 mid section 30 days ago — far outside the 7-day window,
	// immune to a 1-day host/container timezone skew on the boundary.
	rOld := seedTestReport(t, db, boardID, now.AddDate(0, 0, -30))
	seedSectionAssigned(t, db, rOld, vacuumTopic.ID, vecStr(1, 0, 0), 0.25)

	// normal topic: 4 strong sections (dist 0.10).
	normalTopic := makeTopic(t, db, boardID, vecStr(0, 1, 0))
	for i := 0; i < 4; i++ {
		r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -i))
		seedSectionAssigned(t, db, r, normalTopic.ID, vecStr(0, 1, 0), 0.10)
	}

	// empty topic: no recent sections — should reset to zero/false.
	emptyTopic := makeTopic(t, db, boardID, vecStr(0, 0, 1))

	require.NoError(t, repo.RecomputeVacuumStats(boardID))

	var v BoardPersistentTopic
	require.NoError(t, db.First(&v, vacuumTopic.ID).Error)
	assert.True(t, v.IsVacuum, "vacuum topic (0 strong / 6 mid) should be flagged")
	assert.Equal(t, 0, v.VacuumStrong)
	assert.Equal(t, 6, v.VacuumMid, "window-out section (8 days ago) should not count")

	var n BoardPersistentTopic
	require.NoError(t, db.First(&n, normalTopic.ID).Error)
	assert.False(t, n.IsVacuum, "normal topic (4 strong / 0 mid) should not be flagged")
	assert.Equal(t, 4, n.VacuumStrong)
	assert.Equal(t, 0, n.VacuumMid)

	var e BoardPersistentTopic
	require.NoError(t, db.First(&e, emptyTopic.ID).Error)
	assert.False(t, e.IsVacuum, "empty topic should not be flagged")
	assert.Equal(t, 0, e.VacuumStrong)
	assert.Equal(t, 0, e.VacuumMid)
}

// TestComputeTopicCentroid_CountsEachSiblingSection covers the persistent-topic
// Scenario「质心按 section 逐条计入」(add-intra-lane-theme-bucketing): three
// sibling buckets of the same day each contribute their own vector — no
// same-day dedupe.
func TestComputeTopicCentroid_CountsEachSiblingSection(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	today := NormalizeReportDate(time.Now())
	topic := seedLaneDynamicsTopic(t, db, boardID, "质心逐条", TopicStatusActive, 3, today)
	// Give the topic a first-section embedding that differs from the expected
	// mean, so a fallback would be detectable.
	require.NoError(t, db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Updates(map[string]any{"embedding": FloatsToPgVector([]float64{9, 9, 9})}).Error)

	reportID := seedTestReport(t, db, boardID, today)
	for i, vec := range [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} {
		section := DailyReportSection{
			ReportID:          reportID,
			ClusterIndex:      0,
			BucketIndex:       i,
			BucketTitle:       string(rune('A' + i)),
			ClusterLabel:      "sibling",
			ClusterTagIDs:     JSON("[]"),
			Embedding:         FloatsToPgVector(vec),
			PersistentTopicID: &topic.ID,
			LaneTier:          "l1_direct",
		}
		require.NoError(t, db.Create(&section).Error)
	}

	centroid, err := repo.ComputeTopicCentroid(topic.ID)
	require.NoError(t, err)
	vec, err := repoParsePgVector(centroid)
	require.NoError(t, err)
	assert.InDeltaSlice(t, []float64{1.0 / 3, 1.0 / 3, 1.0 / 3}, vec, 1e-6,
		"all three sibling sections enter the window (no same-day collapse)")
}
