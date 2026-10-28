package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 重新分桶（add-intra-lane-theme-bucketing）：PG 集成 ──────────────────────
//
// 覆盖：整组替换但 thread 复用、派生字段重算、头行 ID 保留、阈值/状态/物化
// 拒绝、失败回滚、与 SaveReport 同键 advisory lock、watch 命中与页边注重派生。

type rebucketWorld struct {
	boardID    uint
	topicID    uint
	reportID   uint
	sectionID  uint
	threadIDs  []uint
	periodDate time.Time
	laneTier   string
}

func seedRebucketWorld(t *testing.T, db *gorm.DB, threadCount, tagCount int, laneTier, status string) rebucketWorld {
	t.Helper()
	today := repository.NormalizeReportDate(time.Now())

	board := models.SemanticLabel{
		Label:     fmt.Sprintf("rebucket-%s", t.Name()),
		Slug:      fmt.Sprintf("rebucket-%s", t.Name()),
		LabelType: "board",
		Status:    "active",
	}
	require.NoError(t, db.Create(&board).Error)

	topic := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID,
		Label:           "AGENT编程工具相关",
		Embedding:       repository.FloatsToPgVector([]float64{1, 0, 0}),
		Centroid:        repository.FloatsToPgVector([]float64{1, 0, 0}),
		Status:          repository.TopicStatusActive,
		Source:          repository.TopicSourceManual,
		FirstSeenDate:   today.AddDate(0, 0, -2),
		LastSeenDate:    today.AddDate(0, 0, -1),
		HitCount:        5,
	}
	require.NoError(t, db.Create(&topic).Error)

	report := repository.BoardDailyReport{
		SemanticBoardID: board.ID,
		PeriodDate:      today,
		Title:           "rebucket report",
		Status:          status,
	}
	require.NoError(t, db.Create(&report).Error)

	statusAtReport := repository.TopicStatusActive
	tagIDs := make([]uint, tagCount)
	for i := range tagIDs {
		tagIDs[i] = uint(i + 1)
	}
	breakdown := make([]map[string]any, 0, tagCount)
	for i := 0; i < tagCount; i++ {
		breakdown = append(breakdown, map[string]any{
			"tag_id":       i + 1,
			"label":        fmt.Sprintf("标签%d", i+1),
			"match_reason": "direct_hit",
			"score":        0.8 + float64(i)*0.01,
			"downgraded":   false,
		})
	}
	breakdownJSON, err := json.Marshal(breakdown)
	require.NoError(t, err)

	section := repository.DailyReportSection{
		ReportID:             report.ID,
		ClusterIndex:         2,
		ClusterLabel:         "整簇标题",
		ClusterTagIDs:        repository.JSON(marshalJSONArray(tagIDs)),
		ArticleCount:         threadCount,
		BestTier:             1,
		AvgScore:             0.85,
		QualityBreakdown:     repository.JSON(breakdownJSON),
		Embedding:            repository.FloatsToPgVector([]float64{1, 0, 0}),
		PersistentTopicID:    &topic.ID,
		TopicMatchDistance:   0.42,
		TopicMatchConfidence: repository.TopicConfAnchorHit,
		TopicStatusAtReport:  &statusAtReport,
		LaneTier:             laneTier,
	}
	require.NoError(t, db.Create(&section).Error)

	threadIDs := make([]uint, 0, threadCount)
	for i := 0; i < threadCount; i++ {
		thread := repository.DailyReportThread{
			ReportID:          report.ID,
			SectionID:         section.ID,
			Title:             fmt.Sprintf("线索%d", i),
			Summary:           fmt.Sprintf("摘要%d", i),
			TagIDs:            repository.JSON(marshalJSONArray([]uint{uint(i + 1)})),
			Confidence:        0.9,
			RelatedArticleIDs: repository.JSON(marshalJSONArray([]uint{uint(100 + i)})),
			Embedding:         repository.FloatsToPgVector([]float64{0}),
		}
		require.NoError(t, db.Create(&thread).Error)
		threadIDs = append(threadIDs, thread.ID)
	}

	return rebucketWorld{
		boardID: board.ID, topicID: topic.ID, reportID: report.ID,
		sectionID: section.ID, threadIDs: threadIDs, periodDate: today, laneTier: laneTier,
	}
}

// fakeRebucketSplit wires a deterministic 3-direction split plus 3 embeddings.
func fakeRebucketSplit(t *testing.T, threadCount int) {
	t.Helper()
	assignments := make([]map[string]int, 0, threadCount)
	for i := 0; i < threadCount; i++ {
		assignments = append(assignments, map[string]int{"line_id": i, "bucket_index": i % 3})
	}
	assignJSON, err := json.Marshal(map[string]any{"assignments": assignments})
	require.NoError(t, err)

	fakeBucketChat(t, map[string]string{
		bucketNamesOperation: `{"groups":[
			{"bucket_title":"工具调用","section_title":"Agent 工具链","line_ids":[0,3,6]},
			{"bucket_title":"Skill 生态","section_title":"Skill 规范","line_ids":[1,4]},
			{"bucket_title":"安全风控","section_title":"编码 Agent 安全","line_ids":[2,5]}]}`,
		bucketAssignOperation: string(assignJSON),
	})

	prevEmbed := rebucketEmbedFn
	rebucketEmbedFn = fakeEmbedder([]float64{1, 0, 0}, []float64{0.9, 0.1, 0}, []float64{0, 1, 0})
	t.Cleanup(func() { rebucketEmbedFn = prevEmbed })
}

// TestRebucketReplacesGroupKeepingThreads covers tasks 4.2 / Scenario 重新分桶替换一组 section.
func TestRebucketReplacesGroupKeepingThreads(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL1Direct, repository.ReportStatusCompleted)
	fakeRebucketSplit(t, 7)

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	require.Equal(t, 3, result.Buckets)
	require.Len(t, result.Sections, 3)

	for i, section := range result.Sections {
		assert.Equal(t, 2, section.ClusterIndex, "siblings keep the source cluster index")
		assert.Equal(t, i, section.BucketIndex)
		assert.NotEmpty(t, section.BucketTitle)
	}

	assert.Equal(t, world.sectionID, result.Sections[0].ID,
		"bucket 0 reuses the group head row so the old #report-section anchor stays valid")

	// Threads are never deleted; only section_id moves.
	var threads []repository.DailyReportThread
	require.NoError(t, db.Where("report_id = ?", world.reportID).Order("id ASC").Find(&threads).Error)
	require.Len(t, threads, 7, "no thread may be lost or duplicated")
	for i, thread := range threads {
		assert.Equal(t, world.threadIDs[i], thread.ID, "thread row identity is stable")
		assert.Equal(t, result.Sections[i%3].ID, thread.SectionID, "thread %d follows its bucket", i)
	}

	var sectionCount int64
	require.NoError(t, db.Model(&repository.DailyReportSection{}).
		Where("report_id = ? AND cluster_index = ?", world.reportID, 2).Count(&sectionCount).Error)
	assert.Equal(t, int64(3), sectionCount)
}

// TestRebucketKeepsGroupHead covers tasks 4.6 (deep-link compatibility): the
// group head row keeps its id even when the group shrinks or grows.
func TestRebucketKeepsGroupHead(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL1Direct, repository.ReportStatusCompleted)
	fakeRebucketSplit(t, 7)

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	require.Equal(t, world.sectionID, result.Sections[0].ID)
	var head repository.DailyReportSection
	require.NoError(t, db.First(&head, world.sectionID).Error)
	assert.Equal(t, "工具调用", head.BucketTitle)
}

// TestRebucketDerivedFields covers tasks 4.2 / Scenario 派生字段重算：every
// display field is recomputed per bucket from that bucket's own tags, while the
// report-time topic status snapshot is preserved.
func TestRebucketDerivedFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL1Direct, repository.ReportStatusCompleted)
	fakeRebucketSplit(t, 7)

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	require.Len(t, result.Sections, 3)

	// Bucket line memberships: 0 → {0,3,6} tags {1,4,7}; 1 → {1,4} tags {2,5}; 2 → {2,5} tags {3,6}.
	var bucket0TagIDs []uint
	require.NoError(t, json.Unmarshal(result.Sections[0].ClusterTagIDs, &bucket0TagIDs))
	assert.Equal(t, []uint{1, 4, 7}, bucket0TagIDs)
	assert.Equal(t, "Agent 工具链", result.Sections[0].ClusterLabel)
	assert.Equal(t, "工具调用", result.Sections[0].BucketTitle)
	assert.Equal(t, 3, result.Sections[0].ArticleCount, "article_count = distinct articles of the bucket's threads")
	assert.Equal(t, 2, result.Sections[1].ArticleCount)

	var breakdown []map[string]any
	require.NoError(t, json.Unmarshal(result.Sections[1].QualityBreakdown, &breakdown))
	require.Len(t, breakdown, 2, "breakdown only covers the bucket's own tags")
	assert.EqualValues(t, 2, breakdown[0]["tag_id"])

	// Scores come from the report's frozen breakdown (0.81/0.84 for tags 2/5).
	assert.InDelta(t, 0.825, result.Sections[1].AvgScore, 1e-6)
	assert.Equal(t, 0, result.Sections[1].BestTier, "direct_hit → best tier 0")
	assert.Equal(t, repository.TopicStatusActive, *result.Sections[1].TopicStatusAtReport,
		"history snapshot is preserved, never rewritten")

	// Each bucket computes its own distance to the topic anchor.
	assert.InDelta(t, 0.0, result.Sections[0].TopicMatchDistance, 1e-6)
	assert.InDelta(t, 0.0, result.Sections[1].TopicMatchDistance, 1e-2)
	assert.InDelta(t, 1.0, result.Sections[2].TopicMatchDistance, 1e-6)
}

// TestRebucketThresholdGate covers tasks 4.1 / Scenario 未达阈值不改数据：no LLM
// call, no write, 200-equivalent buckets=1.
func TestRebucketThresholdGate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 4, 4, laneL1Direct, repository.ReportStatusCompleted)
	fakeBucketChatError(t) // any LLM call would fail the test

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Buckets)
	require.Len(t, result.Sections, 1)
	assert.Equal(t, world.sectionID, result.Sections[0].ID)

	var threads []repository.DailyReportThread
	require.NoError(t, db.Where("report_id = ?", world.reportID).Find(&threads).Error)
	for _, thread := range threads {
		assert.Equal(t, world.sectionID, thread.SectionID, "no thread row may move on a no-op")
	}
}

// TestRebucketRejectsNonDone covers tasks 4.1 / Scenario 拒绝非完成态日报.
func TestRebucketRejectsNonDone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL1Direct, "generating")
	fakeRebucketSplit(t, 7)

	_, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.ErrorIs(t, err, repository.ErrRebucketReportNotDone)

	var sectionCount int64
	require.NoError(t, db.Model(&repository.DailyReportSection{}).Where("report_id = ?", world.reportID).Count(&sectionCount).Error)
	assert.Equal(t, int64(1), sectionCount, "rejected request must not write")
}

// TestRebucketRejectsWatchSection covers tasks 4.1 / Scenario 拒绝 watch 物化组.
func TestRebucketRejectsWatchSection(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, LaneTierWatchSentence, repository.ReportStatusCompleted)
	fakeRebucketSplit(t, 7)

	_, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.ErrorIs(t, err, repository.ErrRebucketWatchGroup)
}

// TestRebucketRollbackOnFailure covers tasks 4.3 / Scenario 失败整体回滚.
func TestRebucketRollbackOnFailure(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL1Direct, repository.ReportStatusCompleted)
	fakeBucketChatError(t)

	_, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.Error(t, err)

	var sections []repository.DailyReportSection
	require.NoError(t, db.Where("report_id = ?", world.reportID).Find(&sections).Error)
	require.Len(t, sections, 1, "rolled back: the single section is untouched")
	assert.Equal(t, world.sectionID, sections[0].ID)
	assert.Equal(t, "整簇标题", sections[0].ClusterLabel)

	var threads []repository.DailyReportThread
	require.NoError(t, db.Where("report_id = ?", world.reportID).Find(&threads).Error)
	for _, thread := range threads {
		assert.Equal(t, world.sectionID, thread.SectionID, "thread section_id must stay at the pre-call value")
	}
}

// TestRebucketSerializedWithSaveReport covers tasks 4.4 / Scenario 与同日重跑并发时串行化:
// rebucket blocks on the SAME advisory lock key SaveReport holds.
func TestRebucketSerializedWithSaveReport(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	// Below-threshold world: the rebucket path returns without any LLM call, so
	// the only thing that can delay it is the advisory lock itself.
	world := seedRebucketWorld(t, db, 4, 4, laneL1Direct, repository.ReportStatusCompleted)

	lockTx := db.Begin()
	require.NoError(t, lockTx.Error)
	require.NoError(t, repository.LockDailyReportSave(lockTx, world.boardID, world.periodDate))
	assert.Equal(t, repository.DailyReportSaveLockKey(world.boardID, world.periodDate),
		repository.DailyReportSaveLockKey(world.boardID, world.periodDate),
		"both writers derive the same (board, date) key")
	assert.NotEqual(t, repository.DailyReportSaveLockKey(world.boardID, world.periodDate),
		repository.DailyReportSaveLockKey(world.boardID+1, world.periodDate),
		"different boards must not contend")

	done := make(chan error, 1)
	go func() {
		_, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, lockTx.Rollback().Error)
		t.Fatalf("rebucket must block on the SaveReport advisory lock, finished early: %v", err)
	case <-time.After(400 * time.Millisecond):
	}

	require.NoError(t, lockTx.Commit().Error)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("rebucket did not proceed after the lock was released")
	}
}

// TestRebucketDerivesWatchHitsAndAnnotations covers Scenario watch 命中与页边注跟随重派生.
func TestRebucketDerivesWatchHitsAndAnnotations(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL1Direct, repository.ReportStatusCompleted)
	fakeRebucketSplit(t, 7)

	watch := repository.BoardTopicWatch{
		SemanticBoardID: world.boardID,
		Label:           "watch",
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&watch).Error)
	hit := repository.TopicWatchHit{
		WatchID:    watch.ID,
		SectionID:  world.sectionID,
		ReportID:   world.reportID,
		PeriodDate: world.periodDate,
		Reason:     "hit",
	}
	require.NoError(t, db.Create(&hit).Error)

	threadAnnotation := repository.ReportAnnotation{
		ReportID: world.reportID, SectionID: world.sectionID,
		ThreadID: &world.threadIDs[3], QuotedText: "thread note",
	}
	mastheadAnnotation := repository.ReportAnnotation{
		ReportID: world.reportID, SectionID: world.sectionID,
		QuotedText: "masthead note",
	}
	require.NoError(t, db.Create(&threadAnnotation).Error)
	require.NoError(t, db.Create(&mastheadAnnotation).Error)

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	require.Len(t, result.Sections, 3)

	// The old section owned all 7 threads; bucket 0 owns {0,3,6} (the largest
	// share), so the section-level hit follows that bucket.
	var reloadedHit repository.TopicWatchHit
	require.NoError(t, db.First(&reloadedHit, hit.ID).Error)
	assert.Equal(t, result.Sections[0].ID, reloadedHit.SectionID, "watch hit follows its section's threads")

	var reloadedThreadAnn repository.ReportAnnotation
	require.NoError(t, db.First(&reloadedThreadAnn, threadAnnotation.ID).Error)
	assert.Equal(t, result.Sections[0].ID, reloadedThreadAnn.SectionID, "annotation follows its thread (line 3 → bucket 0)")
	assert.NotNil(t, reloadedThreadAnn.ThreadID)

	var reloadedMasthead repository.ReportAnnotation
	require.NoError(t, db.First(&reloadedMasthead, mastheadAnnotation.ID).Error)
	assert.Equal(t, result.Sections[0].ID, reloadedMasthead.SectionID, "thread-less notes attach to the group head")

	var dangling int64
	require.NoError(t, db.Table("topic_watch_hits").
		Where("report_id = ? AND section_id NOT IN (SELECT id FROM daily_report_sections WHERE report_id = ?)", world.reportID, world.reportID).
		Count(&dangling).Error)
	assert.Zero(t, dangling, "no dangling watch hit may survive")
}

// fakeRebucketSplitInto wires a deterministic n-direction split.
func fakeRebucketSplitInto(t *testing.T, threadCount, n int, namesJSON string, vecs ...[]float64) {
	t.Helper()
	assignments := make([]map[string]int, 0, threadCount)
	for i := 0; i < threadCount; i++ {
		assignments = append(assignments, map[string]int{"line_id": i, "bucket_index": i % n})
	}
	assignJSON, err := json.Marshal(map[string]any{"assignments": assignments})
	require.NoError(t, err)
	fakeBucketChat(t, map[string]string{
		bucketNamesOperation:  namesJSON,
		bucketAssignOperation: string(assignJSON),
	})
	prevEmbed := rebucketEmbedFn
	rebucketEmbedFn = fakeEmbedder(vecs...)
	t.Cleanup(func() { rebucketEmbedFn = prevEmbed })
}

// TestRebucketShrinksGroupDeletingSurplus covers the rebucket branch「新桶数 < 旧桶数
// → 删除多余行（注记/watch 命中先重派生再删）」: the group shrinks from 3 sibling
// sections to 2, the surplus row is deleted only after its referrers moved.
func TestRebucketShrinksGroupDeletingSurplus(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 6, 6, laneL1Direct, repository.ReportStatusCompleted)

	// Promote the seed group into 3 sibling buckets (as a previous split would).
	var head repository.DailyReportSection
	require.NoError(t, db.First(&head, world.sectionID).Error)
	siblingB := head
	siblingB.ID = 0
	siblingB.BucketIndex = 1
	siblingB.BucketTitle = "旧桶B"
	require.NoError(t, db.Create(&siblingB).Error)
	siblingC := head
	siblingC.ID = 0
	siblingC.BucketIndex = 2
	siblingC.BucketTitle = "旧桶C"
	require.NoError(t, db.Create(&siblingC).Error)

	// Move threads 2,3 → B and 4,5 → C.
	require.NoError(t, db.Model(&repository.DailyReportThread{}).Where("id IN ?", []uint{world.threadIDs[2], world.threadIDs[3]}).Update("section_id", siblingB.ID).Error)
	require.NoError(t, db.Model(&repository.DailyReportThread{}).Where("id IN ?", []uint{world.threadIDs[4], world.threadIDs[5]}).Update("section_id", siblingC.ID).Error)

	// A marker + annotation anchored on a thread of the surplus section C.
	marker := repository.TopicKeyEventMarker{
		SemanticBoardID:   world.boardID,
		SourceKey:         "surplus-marker",
		PersistentTopicID: &world.topicID,
		SourceReportID:    &world.reportID,
		SourceSectionID:   &siblingC.ID,
		SourceThreadID:    &world.threadIDs[5],
		SourcePeriodDate:  world.periodDate,
		State:             repository.KeyEventStateConfirmed,
	}
	require.NoError(t, db.Create(&marker).Error)
	annotation := repository.ReportAnnotation{
		ReportID: world.reportID, SectionID: siblingC.ID,
		ThreadID: &world.threadIDs[4], QuotedText: "surplus note",
	}
	require.NoError(t, db.Create(&annotation).Error)

	fakeRebucketSplitInto(t, 6, 2,
		`{"groups":[{"bucket_title":"方向A","section_title":"A","line_ids":[0,2,4]},{"bucket_title":"方向B","section_title":"B","line_ids":[1,3,5]}]}`,
		[]float64{1, 0, 0}, []float64{0, 1, 0})

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	require.Equal(t, 2, result.Buckets)
	require.Len(t, result.Sections, 2)

	var groupCount int64
	require.NoError(t, db.Model(&repository.DailyReportSection{}).
		Where("report_id = ? AND cluster_index = ?", world.reportID, 2).Count(&groupCount).Error)
	assert.Equal(t, int64(2), groupCount, "surplus bucket row is deleted")

	var deleted int64
	require.NoError(t, db.Model(&repository.DailyReportSection{}).Where("id = ?", siblingC.ID).Count(&deleted).Error)
	assert.Zero(t, deleted, "the third (surplus) section row must be gone")

	// Threads of the deleted section moved to the bucket that owns them.
	var movedC repository.DailyReportThread
	require.NoError(t, db.First(&movedC, world.threadIDs[5]).Error)
	assert.Equal(t, result.Sections[1].ID, movedC.SectionID, "thread line 5 (odd) → bucket 1")

	var reloadedMarker repository.TopicKeyEventMarker
	require.NoError(t, db.First(&reloadedMarker, marker.ID).Error)
	require.NotNil(t, reloadedMarker.SourceSectionID)
	assert.Equal(t, result.Sections[1].ID, *reloadedMarker.SourceSectionID,
		"marker re-derives BEFORE the surplus section is deleted (no dangling/null)")

	var reloadedAnnotation repository.ReportAnnotation
	require.NoError(t, db.First(&reloadedAnnotation, annotation.ID).Error)
	assert.Equal(t, result.Sections[0].ID, reloadedAnnotation.SectionID,
		"annotation of thread line 4 (even) → bucket 0")
}

// TestRebucketDerivedFieldsInheritAssignment pins the inherited columns on the
// rebucket path (spec: 兄弟 section 继承归属与来源、快照沿用).
func TestRebucketDerivedFieldsInheritAssignment(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL2LLM, repository.ReportStatusCompleted)
	fakeRebucketSplit(t, 7)

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	for _, section := range result.Sections {
		require.NotNil(t, section.PersistentTopicID)
		assert.Equal(t, world.topicID, *section.PersistentTopicID)
		assert.Equal(t, laneL2LLM, section.LaneTier)
		assert.Equal(t, repository.TopicConfAnchorHit, section.TopicMatchConfidence)
		require.NotNil(t, section.TopicStatusAtReport)
		assert.Equal(t, repository.TopicStatusActive, *section.TopicStatusAtReport)
	}
}

// TestRebucketConvergesToOneBucket covers the rebucket branch「校验后收敛为 1 组 →
// 200 + buckets=1 且不写库」(distinct from the threshold gate): the model returns
// two names but assigns every line to bucket 0, the singleton group is absorbed
// and the result is one group → legal no-op.
func TestRebucketConvergesToOneBucket(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 7, 7, laneL1Direct, repository.ReportStatusCompleted)

	assignments := make([]map[string]int, 0, 7)
	for i := 0; i < 7; i++ {
		assignments = append(assignments, map[string]int{"line_id": i, "bucket_index": 0})
	}
	assignJSON, err := json.Marshal(map[string]any{"assignments": assignments})
	require.NoError(t, err)
	fakeBucketChat(t, map[string]string{
		bucketNamesOperation:  `{"groups":[{"bucket_title":"唯一","section_title":"A","line_ids":[0,1,2]},{"bucket_title":"另一个","section_title":"B","line_ids":[3,4,5,6]}]}`,
		bucketAssignOperation: string(assignJSON),
	})

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Buckets, "converged-to-one is a legal no-op, not a failure")
	require.Len(t, result.Sections, 1)
	assert.Equal(t, world.sectionID, result.Sections[0].ID)
	assert.Equal(t, "整簇标题", result.Sections[0].ClusterLabel, "no section row is rewritten")

	var sectionCount int64
	require.NoError(t, db.Model(&repository.DailyReportSection{}).Where("report_id = ?", world.reportID).Count(&sectionCount).Error)
	assert.Equal(t, int64(1), sectionCount)

	var threads []repository.DailyReportThread
	require.NoError(t, db.Where("report_id = ?", world.reportID).Find(&threads).Error)
	for _, thread := range threads {
		assert.Equal(t, world.sectionID, thread.SectionID)
	}
}

// TestRebucketNoOpResponseCarriesThreads pins the API contract on the no-op
// path: sections always serialize with a threads array (the reader replaces
// sections in place, so a null would blank the cards).
func TestRebucketNoOpResponseCarriesThreads(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	world := seedRebucketWorld(t, db, 4, 4, laneL1Direct, repository.ReportStatusCompleted)

	result, err := RebucketSectionGroup(context.Background(), world.reportID, world.sectionID)
	require.NoError(t, err)
	require.Len(t, result.Sections, 1)
	require.NotNil(t, result.Sections[0].Threads, "threads must be an array, never null")
	assert.Len(t, result.Sections[0].Threads, 4)
}
