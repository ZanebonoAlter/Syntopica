package repository

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 滚动窗口取数单点实现（tasks 2.1 / 2.3） ──────────────────────────────────
// 契约（spec「滚动窗口成员取数的单点实现」）：
//   - 谓词：归属本话题 + embedding 非空；
//   - 排序：报告期日期倒序，同期按 section id 倒序；
//   - 上限：窗口大小（persistent_topic_centroid_window，默认 30）；
//   - 质心计算与首义读端点取到**同一批** section。

// TestCentroidWindowExt_PredicateOrderAndLimit locks the 取数口径 of
// ListCentroidWindowSections: the window holds the topic's own sections with a
// vector, ordered by report period DESC / section id DESC, capped by the
// configured window size.
func TestCentroidWindowExt_PredicateOrderAndLimit(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	now := time.Now()
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	other := seedAnchorTopic(t, repo.db, boardID, "lane B", TopicStatusActive, []float64{0, 0, 1})

	// 10 reports × 3 sections = 30 recent sections (inside the window).
	type dayEntry struct {
		date string
		ids  []uint
	}
	entries := make([]dayEntry, 0, 10)
	reportIDs := make([]uint, 0, 10)
	for d := 0; d < 10; d++ {
		reportID := seedTestReport(t, repo.db, boardID, now.AddDate(0, 0, -d))
		reportIDs = append(reportIDs, reportID)
		var report BoardDailyReport
		require.NoError(t, repo.db.First(&report, reportID).Error)
		ids := make([]uint, 0, 3)
		for i := 0; i < 3; i++ {
			s := seedAnchorSection(t, repo.db, reportID, fmt.Sprintf("s-%d-%d", d, i), []float64{1, 0, 0}, &topic.ID)
			ids = append(ids, s.ID)
		}
		entries = append(entries, dayEntry{date: report.PeriodDate.UTC().Format("2006-01-02"), ids: ids})
	}
	newestReportID := reportIDs[0]

	// Excluded by the predicate: no vector.
	noVector := seedAnchorSection(t, repo.db, newestReportID, "no-vector", nil, &topic.ID)
	// Excluded by the predicate: belongs to another topic.
	otherTopic := seedAnchorSection(t, repo.db, newestReportID, "other-topic", []float64{1, 0, 0}, &other.ID)
	// Excluded by the window cap: older than the 30 newest sections.
	oldReportID := seedTestReport(t, repo.db, boardID, now.AddDate(0, 0, -40))
	oldSection := seedAnchorSection(t, repo.db, oldReportID, "too-old", []float64{0, 1, 0}, &topic.ID)

	rows, err := repo.ListCentroidWindowSections(topic.ID, DefaultPersistentTopicConfig().CentroidWindow)
	require.NoError(t, err)
	require.Len(t, rows, 30, "the window is capped by persistent_topic_centroid_window (default 30)")

	// Order: report period DESC, same period by section id DESC.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].date > entries[j].date })
	expected := make([]uint, 0, 30)
	for _, e := range entries {
		for i := len(e.ids) - 1; i >= 0; i-- {
			expected = append(expected, e.ids[i])
		}
	}
	for i, id := range expected {
		assert.Equal(t, id, rows[i].SectionID, "window order must be period_date DESC, id DESC")
	}

	// The three excluded sections must not show up in the window.
	for _, row := range rows {
		assert.NotEqual(t, noVector.ID, row.SectionID, "a section without a vector is not in the window")
		assert.NotEqual(t, otherTopic.ID, row.SectionID, "another topic's section is not in this window")
		assert.NotEqual(t, oldSection.ID, row.SectionID, "sections older than the window cap are excluded")
	}
}

// TestCentroidWindowExt_ReadEndpointAndCentroidShareTheSameWindow is the
// equivalence case: the sections the centroid computation consumes and the
// sections the anchor endpoint reports are the very same ones (逐 id 相等),
// and the centroid value is the α-blend over exactly that window.
func TestCentroidWindowExt_ReadEndpointAndCentroidShareTheSameWindow(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	topic := seedAnchorTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{0, 0, 1})
	vectors := [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {1, 1, 0}}
	for i, v := range vectors {
		seedAnchorSection(t, repo.db, reportID, fmt.Sprintf("s%d", i), v, &topic.ID)
	}

	window, err := repo.ListCentroidWindowSections(topic.ID, DefaultPersistentTopicConfig().CentroidWindow)
	require.NoError(t, err)
	require.Len(t, window, len(vectors))

	state, err := repo.GetTopicAnchorState(topic.ID)
	require.NoError(t, err)
	require.Len(t, state.Centroid.SectionIDs, len(window))
	for i := range window {
		assert.Equal(t, window[i].SectionID, state.Centroid.SectionIDs[i],
			"the read endpoint must report exactly the sections the centroid consumed")
		assert.Equal(t, window[i].SectionID, state.Centroid.Members[i].SectionID)
	}

	// The centroid value must be the α-blend over that very window.
	centroid, err := repo.ComputeTopicCentroid(topic.ID)
	require.NoError(t, err)
	got, err := repoParsePgVector(centroid)
	require.NoError(t, err)

	alpha := LoadPersistentTopicConfig(repo.db).CentroidAnchorWeight
	anchor := []float64{0, 0, 1}
	mean := make([]float64, 3)
	for _, v := range vectors {
		for i := range mean {
			mean[i] += v[i] / float64(len(vectors))
		}
	}
	want := make([]float64, 3)
	for i := range want {
		want[i] = alpha*anchor[i] + (1-alpha)*mean[i]
	}
	assert.InDeltaSlice(t, want, got, 1e-6)
}
