package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

// TestSaveReport_LaneDrivenCentroidRefresh is the lane-driven integration test
// (testcontainer pgvector). It drives the full SaveReport → assignAndUpdateTopics
// → post-tx centroid/vacuum refresh path with sections whose LaneTier is already
// set (the bucketing is unit-tested in the service package), and asserts:
//
//	① L1 sections persist lane_tier=l1_direct + anchor_hit + owning topic
//	② L3 sections persist lane_tier=l3_new + auto_new + a freshly created candidate
//	④ the owning topic's centroid is incrementally refreshed (the α-blend of
//	   the 首义 vector and the mean of the 2 assigned sections, distinct from
//	   the initial centroid)
//	⑤ vacuum stats are recomputed post-commit from the recorded distances
func TestSaveReport_LaneDrivenCentroidRefresh(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	// Align embedding + centroid columns to the 3-dim test vectors.
	for _, table := range []string{"daily_report_sections", "board_persistent_topics"} {
		require.NoError(t, db.Exec(
			fmt.Sprintf("ALTER TABLE %s ALTER COLUMN embedding TYPE vector(3)", table)).Error, table)
	}
	require.NoError(t, db.Exec("ALTER TABLE board_persistent_topics ALTER COLUMN centroid TYPE vector(3)").Error)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	setAnchorWeight(t, db, "0.5")

	// Active topic with an initial centroid [1,0,0] (embedding is the 首义
	// fallback). After 2 sections are assigned, ComputeTopicCentroid averages
	// them → [0.9,0.3,0] and blends it with the 首义 (α=0.5) → [0.95,0.15,0],
	// distinct from the initial value.
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "T", Status: TopicStatusActive,
		Embedding: vecStr(1, 0, 0), Centroid: vecStr(1, 0, 0),
		FirstSeenDate: now, LastSeenDate: now, HitCount: 2,
	}
	require.NoError(t, db.Create(&topic).Error)
	tid := topic.ID

	// Two L1 sections (lane set directly) + one L3 section.
	secA := DailyReportSection{ReportID: reportID, ClusterLabel: "A", Embedding: vecStr(1, 0, 0), LaneTier: "l1_direct", MatchedTopicID: &tid}
	secB := DailyReportSection{ReportID: reportID, ClusterLabel: "B", Embedding: vecStr(0.8, 0.6, 0), LaneTier: "l1_direct", MatchedTopicID: &tid}
	secC := DailyReportSection{ReportID: reportID, ClusterLabel: "C", Embedding: vecStr(0, 0, 1), LaneTier: "l3_new"}
	sections := []DailyReportSection{secA, secB, secC}
	report := &BoardDailyReport{ID: reportID, SemanticBoardID: boardID, PeriodDate: now, Status: "completed"}

	require.NoError(t, repo.SaveReport(report, sections, nil))

	// ① L1 sections: lane_tier=l1_direct, anchor_hit, owning topic.
	var gotA, gotB, gotC DailyReportSection
	require.NoError(t, db.Where("cluster_label = ?", "A").First(&gotA).Error)
	require.NoError(t, db.Where("cluster_label = ?", "B").First(&gotB).Error)
	require.NoError(t, db.Where("cluster_label = ?", "C").First(&gotC).Error)
	for _, g := range []DailyReportSection{gotA, gotB} {
		assert.Equal(t, "l1_direct", g.LaneTier)
		assert.Equal(t, TopicConfAnchorHit, g.TopicMatchConfidence)
		require.NotNil(t, g.PersistentTopicID)
		assert.Equal(t, tid, *g.PersistentTopicID)
	}
	// ② L3 section: lane_tier=l3_new, auto_new, assigned to a NEW candidate.
	assert.Equal(t, "l3_new", gotC.LaneTier)
	assert.Equal(t, TopicConfAutoNew, gotC.TopicMatchConfidence)
	require.NotNil(t, gotC.PersistentTopicID)
	assert.NotEqual(t, tid, *gotC.PersistentTopicID, "L3 opens a new candidate, not the existing topic")

	// ④ centroid incremental refresh: 0.5·首义[1,0,0] + 0.5·mean([1,0,0],
	// [0.8,0.6,0]) = 0.5·[1,0,0] + 0.5·[0.9,0.3,0] = [0.95,0.15,0].
	var refreshed BoardPersistentTopic
	require.NoError(t, db.First(&refreshed, tid).Error)
	cv, err := repoParsePgVector(refreshed.Centroid)
	require.NoError(t, err)
	assert.InDeltaSlice(t, []float64{0.95, 0.15, 0}, cv, 1e-6,
		"centroid refreshed to the blend of the 首义 vector and the mean of the 2 assigned sections")

	// ⑤ vacuum recompute ran: stats populated from recorded distances
	// (secA dist 0.0 → strong, secB dist 0.2 → mid against the pre-refresh
	// centroid [1,0,0]).
	assert.GreaterOrEqual(t, refreshed.VacuumStrong+refreshed.VacuumMid, 2,
		"vacuum stats recomputed from the assigned sections")
}
