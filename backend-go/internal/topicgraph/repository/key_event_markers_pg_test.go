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

// ── topic_key_event_markers PG 集成测试（add-topic-key-event-markers 1.1/1.2）──
//
// 一次性 testcontainer PG + golden schema（migration 20261012_0001 的
// CHECK/FK 在 schema 构建时已生效）。覆盖 Scenario 映射表落在本文件的条目：
// 一线索多篇依据只标一条 / 取消不删除新闻或报告 / 重复确认只保留一个节点 /
// 迟到推荐不覆盖人工决定 / 重生成保留选择而不移植确认 / 无 section 仍能查
// 确认快照，加上确认/不采纳/取消生命周期、revision 409、指纹边界（同日/
// 跨日/证据变更/空白归一）、topic 删除/归档/合并分裂归属边界、重建重附与
// 事务回滚。全部走 repository 层（HTTP 语义在 handler 测试覆盖）。

// keyEventTestRepo provisions the golden-schema PG and a fresh repo.
func keyEventTestRepo(t *testing.T) *TopicGraphRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	return repo
}

func keyEventSeedTopic(t *testing.T, db *gorm.DB, boardID uint, label string) uint {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Status:          TopicStatusActive,
		FirstSeenDate:   NormalizeReportDate(time.Now()),
		LastSeenDate:    NormalizeReportDate(time.Now()),
	}
	// embedding/centroid 是非空 vector 列（零值 "" 是非法字面量），测试不采样向量 → 省略列落 NULL。
	require.NoError(t, db.Omit("embedding", "centroid").Create(&topic).Error)
	return topic.ID
}

// keyEventSeedThreadWithParams inserts one thread with explicit content so
// fingerprint cases can vary title/tags/articles precisely.
func keyEventSeedThreadWithParams(t *testing.T, db *gorm.DB, reportID, sectionID uint, title string, tagIDs, articleIDs []uint) uint {
	t.Helper()
	tags, _ := json.Marshal(tagIDs)
	arts, _ := json.Marshal(articleIDs)
	thread := DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             title,
		Summary:           "summary for " + title,
		TagIDs:            JSON(tags),
		RelatedArticleIDs: JSON(arts),
		Embedding:         FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, db.Create(&thread).Error)
	return thread.ID
}

// keyEventFullSetup builds the minimal world: board + topic + report +
// topic-owned section + one thread.
func keyEventFullSetup(t *testing.T, repo *TopicGraphRepository, title string, tagIDs, articleIDs []uint) (boardID, topicID, reportID, sectionID, threadID uint) {
	t.Helper()
	boardID = seedTestBoard(t, repo.db)
	topicID = keyEventSeedTopic(t, repo.db, boardID, "测试泳道")
	reportID = seedTestReport(t, repo.db, boardID, time.Now())
	sectionID = seedTestSectionWithTopic(t, repo.db, reportID, "cluster", topicID)
	threadID = keyEventSeedThreadWithParams(t, repo.db, reportID, sectionID, title, tagIDs, articleIDs)
	return
}

// ── 生命周期：确认 / 不采纳 / 取消 / 再选择 ─────────────────────────────────

func TestKeyEvent_MarkDismissUnmarkLifecycle(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, _, sectionID, threadID := keyEventFullSetup(t, repo, "模型正式发布", []uint{11}, []uint{101, 102})

	// Manual mark on a thread with NO AI suggestion: no AI permission needed.
	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.Equal(t, KeyEventStateConfirmed, m.State)
	assert.Equal(t, KeyEventActionMark, m.LastAction)
	require.NotNil(t, m.ConfirmedAt)
	assert.Equal(t, boardID, m.SemanticBoardID)
	require.NotNil(t, m.PersistentTopicID)
	assert.Equal(t, topicID, *m.PersistentTopicID)
	require.NotNil(t, m.SourceThreadID)
	assert.Equal(t, threadID, *m.SourceThreadID)

	// Repeat the same target: idempotent, one row, same revision.
	var count int64
	require.NoError(t, repo.db.Model(&TopicKeyEventMarker{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	m2, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.Equal(t, m.ID, m2.ID)
	assert.Equal(t, m.Revision, m2.Revision)
	require.NoError(t, repo.db.Model(&TopicKeyEventMarker{}).Count(&count).Error)
	assert.Equal(t, int64(1), count, "重复确认只保留一个节点")

	// 一线索多篇依据只标一条：multi-article evidence stays ONE marker and the
	// snapshot references the whole evidence set.
	var arts []uint
	require.NoError(t, json.Unmarshal([]byte(m2.ArticleIDsSnapshot), &arts))
	assert.Equal(t, []uint{101, 102}, arts)
	require.NoError(t, repo.db.Model(&TopicKeyEventMarker{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)

	// Unmark via marker PATCH (取消)：needs revision, result dismissed.
	m3, err := repo.ApplyMarkerKeyEvent(topicID, m.ID, KeyEventActionUnmark, &m2.Revision)
	require.NoError(t, err)
	assert.Equal(t, KeyEventStateDismissed, m3.State)
	assert.Equal(t, KeyEventActionUnmark, m3.LastAction)
	assert.Equal(t, m2.Revision+1, m3.Revision)

	// 取消不删除新闻或报告：source rows all survive.
	var threads, sections, reports int64
	require.NoError(t, repo.db.Model(&DailyReportThread{}).Where("id = ?", threadID).Count(&threads).Error)
	require.NoError(t, repo.db.Model(&DailyReportSection{}).Where("id = ?", sectionID).Count(&sections).Error)
	require.NoError(t, repo.db.Model(&BoardDailyReport{}).Where("semantic_board_id = ?", boardID).Count(&reports).Error)
	assert.Equal(t, int64(1), threads)
	assert.Equal(t, int64(1), sections)
	assert.Equal(t, int64(1), reports)

	// 不采纳后仍可选择：re-mark the same (still valid) source via thread PUT.
	m4, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, &m3.Revision)
	require.NoError(t, err)
	assert.Equal(t, KeyEventStateConfirmed, m4.State)
	assert.Equal(t, m.ID, m4.ID, "same marker row, no duplicate")
}

func TestKeyEvent_RevisionConflict409(t *testing.T) {
	repo := keyEventTestRepo(t)
	_, topicID, _, _, threadID := keyEventFullSetup(t, repo, "冲突案例", []uint{1}, []uint{9})

	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)

	stale := m.Revision + 1 // a revision that does NOT match the current row
	_, err = repo.ApplyMarkerKeyEvent(topicID, m.ID, KeyEventActionUnmark, &stale)
	require.Error(t, err)
	kea, ok := err.(*KeyEventActionError)
	require.True(t, ok, "error should be KeyEventActionError, got %T", err)
	assert.Equal(t, 409, kea.HTTPStatus)
	require.NotNil(t, kea.Marker, "409 carries the latest server state")

	// Missing revision on an existing row → 409 as well.
	_, err = repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionDismiss, nil)
	require.Error(t, err)
	kea2, ok := err.(*KeyEventActionError)
	require.True(t, ok)
	assert.Equal(t, 409, kea2.HTTPStatus)

	// PATCH without revision → 400.
	_, err = repo.ApplyMarkerKeyEvent(topicID, m.ID, KeyEventActionUnmark, nil)
	require.Error(t, err)
	kea3, ok := err.(*KeyEventActionError)
	require.True(t, ok)
	assert.Equal(t, 400, kea3.HTTPStatus)
}

func TestKeyEvent_InvalidActionAndScope(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID := seedTestBoard(t, repo.db)
	topicID := keyEventSeedTopic(t, repo.db, boardID, "作用域")
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	sectionID := seedTestSectionWithTopic(t, repo.db, reportID, "c", topicID)
	threadID := keyEventSeedThreadWithParams(t, repo.db, reportID, sectionID, "合法线索", []uint{1}, []uint{2})

	// Unknown action → 400.
	_, err := repo.ApplyThreadKeyEvent(topicID, threadID, "delete", nil)
	require.Error(t, err)
	assert.Equal(t, 400, err.(*KeyEventActionError).HTTPStatus)

	// Cross-topic thread (thread belongs to another topic) → 404.
	otherTopic := keyEventSeedTopic(t, repo.db, boardID, "别的泳道")
	_, err = repo.ApplyThreadKeyEvent(otherTopic, threadID, KeyEventActionMark, nil)
	require.Error(t, err)
	assert.Equal(t, 404, err.(*KeyEventActionError).HTTPStatus)

	// Section without topic attribution → 400 (无归属新标记拒绝).
	orphanSection := seedTestSection(t, repo.db, reportID, "orphan")
	orphanThread := keyEventSeedThreadWithParams(t, repo.db, reportID, orphanSection, "无归属", []uint{1}, []uint{2})
	_, err = repo.ApplyThreadKeyEvent(topicID, orphanThread, KeyEventActionMark, nil)
	require.Error(t, err)
	assert.Equal(t, 400, err.(*KeyEventActionError).HTTPStatus)

	// Non-existent topic/thread/marker → 404.
	_, err = repo.ApplyThreadKeyEvent(999999, threadID, KeyEventActionMark, nil)
	assert.Equal(t, 404, err.(*KeyEventActionError).HTTPStatus)
	_, err = repo.ApplyThreadKeyEvent(topicID, 999999, KeyEventActionMark, nil)
	assert.Equal(t, 404, err.(*KeyEventActionError).HTTPStatus)
	_, err = repo.ApplyMarkerKeyEvent(topicID, 999999, KeyEventActionUnmark, intPtr(1))
	assert.Equal(t, 404, err.(*KeyEventActionError).HTTPStatus)

	// Marker owned by another topic → 404 (越界引用拒绝).
	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)
	_, err = repo.ApplyMarkerKeyEvent(otherTopic, m.ID, KeyEventActionUnmark, &m.Revision)
	require.Error(t, err)
	assert.Equal(t, 404, err.(*KeyEventActionError).HTTPStatus)
}

func intPtr(i int) *int { return &i }

// ── 指纹边界（同日/跨日/证据变更/空白归一）──────────────────────────────────

func TestKeyEvent_SourceKeyBoundaries(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID := seedTestBoard(t, repo.db)
	topicID := keyEventSeedTopic(t, repo.db, boardID, "指纹")
	day1 := seedTestReport(t, repo.db, boardID, time.Now())
	day2 := seedTestReport(t, repo.db, boardID, time.Now().AddDate(0, 0, -1))

	s1 := seedTestSectionWithTopic(t, repo.db, day1, "a", topicID)
	t1 := keyEventSeedThreadWithParams(t, repo.db, day1, s1, "同一事件", []uint{1, 2}, []uint{10, 11})

	// Same-day identical duplicate thread (order of ids differs → same key).
	t1dup := keyEventSeedThreadWithParams(t, repo.db, day1, s1, "同一事件", []uint{2, 1}, []uint{11, 10})
	// Leading/trailing whitespace (incl. full-width/tab) trims to the same key.
	t1ws := keyEventSeedThreadWithParams(t, repo.db, day1, s1, " \t同一事件\u3000", []uint{1, 2}, []uint{10, 11})
	// Different day, same content → different key.
	s2 := seedTestSectionWithTopic(t, repo.db, day2, "b", topicID)
	t2 := keyEventSeedThreadWithParams(t, repo.db, day2, s2, "同一事件", []uint{1, 2}, []uint{10, 11})
	// Same day, changed evidence → different key.
	t3 := keyEventSeedThreadWithParams(t, repo.db, day1, s1, "同一事件", []uint{1, 2}, []uint{10, 99})
	// Same day, different title (case preserved, not lowered) → different key.
	t4 := keyEventSeedThreadWithParams(t, repo.db, day1, s1, "同一事件 v2", []uint{1, 2}, []uint{10, 11})

	m1, err := repo.ApplyThreadKeyEvent(topicID, t1, KeyEventActionMark, nil)
	require.NoError(t, err)

	// Same fingerprint via dup thread → SAME marker row (one row total).
	mDup, err := repo.ApplyThreadKeyEvent(topicID, t1dup, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.Equal(t, m1.ID, mDup.ID)
	mWS, err := repo.ApplyThreadKeyEvent(topicID, t1ws, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.Equal(t, m1.ID, mWS.ID)

	// Cross-day / evidence-change / title-change → separate rows.
	m2, err := repo.ApplyThreadKeyEvent(topicID, t2, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.NotEqual(t, m1.ID, m2.ID)
	m3, err := repo.ApplyThreadKeyEvent(topicID, t3, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.NotEqual(t, m1.ID, m3.ID)
	m4, err := repo.ApplyThreadKeyEvent(topicID, t4, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.NotEqual(t, m1.ID, m4.ID)

	var count int64
	require.NoError(t, repo.db.Model(&TopicKeyEventMarker{}).Count(&count).Error)
	assert.Equal(t, int64(4), count)
}

// TestKeyEvent_NormalizeTitle_Pure covers the fingerprint's whitespace
// collapsing as a pure function (no DB): runs of whitespace (ASCII/full-width/
// tab/newline) collapse to ONE ASCII space; case and emoji are preserved.
func TestKeyEvent_NormalizeTitle_Pure(t *testing.T) {
	day := NormalizeReportDate(time.Now())
	base := ComputeKeyEventSourceKey(1, 2, day, "同一事件", []uint{3}, []uint{4})
	// Leading/trailing whitespace-only variants → identical key.
	for _, title := range []string{" 同一事件", "\t同一事件\n", "\u3000同一事件\u3000", "  同一事件  "} {
		assert.Equal(t, base, ComputeKeyEventSourceKey(1, 2, day, title, []uint{3}, []uint{4}), "title %q", title)
	}
	// Internal whitespace runs collapse to a single space — these are all
	// EQUAL to each other but DIFFERENT from the no-space title (a collapsed
	// separator remains a separator; the fingerprint never silently merges it).
	withSep := ComputeKeyEventSourceKey(1, 2, day, "同一\t事件", []uint{3}, []uint{4})
	assert.Equal(t, withSep, ComputeKeyEventSourceKey(1, 2, day, "同一  事件", []uint{3}, []uint{4}))
	assert.Equal(t, withSep, ComputeKeyEventSourceKey(1, 2, day, "同一\u3000事件", []uint{3}, []uint{4}))
	assert.NotEqual(t, base, withSep)
	// Case and emoji preserved (不降大小写/不剥 emoji).
	assert.NotEqual(t,
		ComputeKeyEventSourceKey(1, 2, day, "Model X", []uint{3}, []uint{4}),
		ComputeKeyEventSourceKey(1, 2, day, "model x", []uint{3}, []uint{4}))
	assert.NotEqual(t,
		ComputeKeyEventSourceKey(1, 2, day, "发布🚀", []uint{3}, []uint{4}),
		ComputeKeyEventSourceKey(1, 2, day, "发布", []uint{3}, []uint{4}))
	// Timezone conversion never changes the Shanghai calendar day; crossing
	// midnight in Shanghai does (23:30+08 +2h = 01:30 next day in +08).
	shanghaiLateNight := time.Date(2026, 10, 12, 23, 30, 0, 0, models.ShanghaiTZ)
	keyLate := ComputeKeyEventSourceKey(1, 2, shanghaiLateNight, "深夜事件", nil, nil)
	assert.Equal(t, keyLate, ComputeKeyEventSourceKey(1, 2, shanghaiLateNight.In(time.UTC), "深夜事件", nil, nil), "同一时刻的 UTC 表达归一到同一上海日")
	assert.NotEqual(t, keyLate, ComputeKeyEventSourceKey(1, 2, shanghaiLateNight.Add(2*time.Hour), "深夜事件", nil, nil), "上海 23:30 与次日 01:30 不同业务日，键不同")
}

// ── 重建：保留选择、精确重附、相似内容不移植 ────────────────────────────────

// rebuildReportWithThreads reruns SaveReport for an existing (board, date)
// with new sections/threads — the production same-day rebuild path.
func rebuildReportWithThreads(t *testing.T, repo *TopicGraphRepository, reportID uint, topicID uint, titles []string, tagIDs, articleIDs []uint, summary string) {
	t.Helper()
	var orig BoardDailyReport
	require.NoError(t, repo.db.First(&orig, reportID).Error)

	sections := []DailyReportSection{{
		ReportID:          orig.ID,
		ClusterIndex:      0,
		ClusterLabel:      "rebuilt",
		ArticleCount:      len(articleIDs),
		Embedding:         FloatsToPgVector([]float64{0}),
		PersistentTopicID: &topicID,
		// watch_sentence keeps the planner from reassigning this section —
		// the test pins the topic ownership explicitly.
		LaneTier: "watch_sentence",
	}}
	threads := make([]DailyReportThread, len(titles))
	for i, title := range titles {
		tags, _ := json.Marshal(tagIDs)
		arts, _ := json.Marshal(articleIDs)
		threads[i] = DailyReportThread{
			Title: title, Summary: summary,
			TagIDs: JSON(tags), RelatedArticleIDs: JSON(arts),
			Embedding: FloatsToPgVector([]float64{0}),
		}
	}
	require.NoError(t, repo.SaveReport(&orig, sections, [][]DailyReportThread{threads}))
}

func TestKeyEvent_RebuildReattachExactFingerprintOnly(t *testing.T) {
	repo := keyEventTestRepo(t)
	_, topicID, reportID, _, threadID := keyEventFullSetup(t, repo, "正式发布", []uint{11}, []uint{101})

	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)
	assert.Equal(t, KeyEventStateConfirmed, m.State)

	// Rebuild A: identical fingerprint (same title/tags/articles) → re-attach.
	rebuildReportWithThreads(t, repo, reportID, topicID, []string{"正式发布"}, []uint{11}, []uint{101}, "summary for 正式发布")
	var after TopicKeyEventMarker
	require.NoError(t, repo.db.First(&after, m.ID).Error)
	assert.Equal(t, KeyEventStateConfirmed, after.State, "重生成保留选择")
	require.NotNil(t, after.SourceThreadID)
	assert.NotEqual(t, threadID, *after.SourceThreadID, "re-anchored onto the NEW thread row")
	var newThread DailyReportThread
	require.NoError(t, repo.db.First(&newThread, *after.SourceThreadID).Error)
	assert.Equal(t, "正式发布", newThread.Title)
	var markers int64
	require.NoError(t, repo.db.Model(&TopicKeyEventMarker{}).Count(&markers).Error)
	assert.Equal(t, int64(1), markers, "no duplicate rows across rebuild")

	// Rebuild B: similar-but-different content (title + evidence drift) → the
	// marker detaches and STAYS on the old snapshot; the new similar thread
	// must NOT inherit the confirmation.
	rebuildReportWithThreads(t, repo, reportID, topicID, []string{"正式发布，全面开放", "另一条新线索"}, []uint{11, 12}, []uint{101, 105}, "重写摘要")
	var detached TopicKeyEventMarker
	require.NoError(t, repo.db.First(&detached, m.ID).Error)
	assert.Equal(t, KeyEventStateConfirmed, detached.State)
	assert.Nil(t, detached.SourceThreadID, "来源替换 → 脱钩快照")
	assert.Equal(t, "正式发布", detached.TitleSnapshot, "确认快照冻结，不被新叙事改写")
	assert.Equal(t, "summary for 正式发布", detached.SummarySnapshot)

	// 无 section 仍能查确认快照：lifeline/topic reads still return it.
	listed, err := repo.ListTopicKeyEventMarkers(topicID, 0, KeyEventStateConfirmed)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, m.ID, listed[0].ID)

	// Same-target PATCH on the detached confirmed row is an idempotent no-op
	// (相同目标重复提交返回已保存结果)；state-CHANGING re-mark needs a live
	// source — after unmark the old source cannot legitimize a new confirm.
	idem, err := repo.ApplyMarkerKeyEvent(topicID, detached.ID, KeyEventActionMark, &detached.Revision)
	require.NoError(t, err)
	assert.Equal(t, KeyEventStateConfirmed, idem.State)
	unmarked, err := repo.ApplyMarkerKeyEvent(topicID, detached.ID, KeyEventActionUnmark, &detached.Revision)
	require.NoError(t, err)
	assert.Equal(t, KeyEventStateDismissed, unmarked.State)
	_, err = repo.ApplyMarkerKeyEvent(topicID, detached.ID, KeyEventActionMark, &unmarked.Revision)
	require.Error(t, err)
	assert.Equal(t, 400, err.(*KeyEventActionError).HTTPStatus, "不能凭旧来源重新 mark")
}

func TestKeyEvent_RebuildRollbackKeepsMarkers(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, reportID, _, threadID := keyEventFullSetup(t, repo, "回滚案例", []uint{5}, []uint{50})

	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)

	// Simulate a failing rebuild: force the SaveReport transaction to abort
	// AFTER the re-attach step by injecting an invalid thread (pgvector
	// rejects the empty literal) — the whole tx rolls back.
	err = repo.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("report_id = ?", reportID).Delete(&DailyReportThread{}).Error; err != nil {
			return err
		}
		if err := tx.Where("report_id = ?", reportID).Delete(&DailyReportSection{}).Error; err != nil {
			return err
		}
		section := DailyReportSection{
			ReportID: reportID, ClusterLabel: "r", ArticleCount: 1,
			Embedding: FloatsToPgVector([]float64{0}), PersistentTopicID: &topicID,
		}
		if err := tx.Create(&section).Error; err != nil {
			return err
		}
		thread := DailyReportThread{
			ReportID: reportID, SectionID: section.ID, Title: "回滚案例",
			Embedding: FloatsToPgVector([]float64{0}),
		}
		if err := tx.Omit("embedding").Create(&thread).Error; err != nil {
			return err
		}
		if err := ReAttachKeyEventMarkers(tx, boardID, NormalizeReportDate(time.Now()), []DailyReportSection{section}, [][]DailyReportThread{{thread}}); err != nil {
			return err
		}
		return errInjectedRebuildFailure
	})
	require.ErrorIs(t, err, errInjectedRebuildFailure)

	// Marker survives the rollback with its ORIGINAL anchor.
	var after TopicKeyEventMarker
	require.NoError(t, repo.db.First(&after, m.ID).Error)
	assert.Equal(t, KeyEventStateConfirmed, after.State)
	require.NotNil(t, after.SourceThreadID)
	assert.Equal(t, threadID, *after.SourceThreadID, "回滚不半脱钩")
}

var errInjectedRebuildFailure = &KeyEventActionError{HTTPStatus: 500, Message: "injected"}

// ── topic 删除 / 归档 / 合并 / 分裂后的归属边界 ─────────────────────────────

func TestKeyEvent_TopicLifecycleBoundaries(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, _, _, threadID := keyEventFullSetup(t, repo, "边界事件", []uint{7}, []uint{70})

	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)

	// 归档：marker untouched, still queryable by topic.
	require.NoError(t, repo.db.Model(&BoardPersistentTopic{}).Where("id = ?", topicID).
		Update("status", TopicStatusArchived).Error)
	listed, err := repo.ListTopicKeyEventMarkers(topicID, 0, KeyEventStateConfirmed)
	require.NoError(t, err)
	assert.Len(t, listed, 1, "归档泳道仍可查")

	// 合并：merge reassigns sections to the target topic; the marker STAYS on
	// the archived source topic (不静默搬运人工选择).
	target := keyEventSeedTopic(t, repo.db, boardID, "合并目标")
	_, err = repo.MergeTopics(target, []uint{topicID})
	require.NoError(t, err)
	var afterMerge TopicKeyEventMarker
	require.NoError(t, repo.db.First(&afterMerge, m.ID).Error)
	require.NotNil(t, afterMerge.PersistentTopicID)
	assert.Equal(t, topicID, *afterMerge.PersistentTopicID, "合并不搬运注记")

	// Topic hard delete: FK SET NULL keeps the marker (孤立注记不删).
	require.NoError(t, repo.DeleteTopic(topicID))
	var afterDelete TopicKeyEventMarker
	require.NoError(t, repo.db.First(&afterDelete, m.ID).Error)
	assert.Nil(t, afterDelete.PersistentTopicID, "topic 硬删 → 注记保留、归属置空")
	assert.Equal(t, KeyEventStateConfirmed, afterDelete.State)
	// Orphan markers never leak into another lane's reads.
	other, err := repo.ListTopicKeyEventMarkers(target, 0, KeyEventStateConfirmed)
	require.NoError(t, err)
	assert.Empty(t, other)
}

// ── AI 建议写入：只写 suggested、迟到不覆盖人工 ─────────────────────────────

func TestKeyEvent_SaveSuggestedMarkers_LateAICannotOverrideHuman(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, reportID, sectionID, threadID := keyEventFullSetup(t, repo, "人工先到", []uint{3}, []uint{30})

	// Human marks first.
	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)
	require.Equal(t, KeyEventStateConfirmed, m.State)

	// Run row started (fresh token), then the AI result lands late.
	require.NoError(t, repo.UpsertKeyEventRecommendationRun(reportID, boardID, "tok-1"))
	saved, err := repo.SaveSuggestedMarkers(boardID, reportID, "tok-1", []SuggestedKeyEvent{{
		SourceThreadID: threadID, Reason: "AI 迟到理由", NewProgress: true, EvidenceIDs: []uint{30},
	}})
	require.NoError(t, err)
	assert.Equal(t, 0, saved, "迟到建议不得覆盖已确认条目")

	var after TopicKeyEventMarker
	require.NoError(t, repo.db.First(&after, m.ID).Error)
	assert.Equal(t, KeyEventStateConfirmed, after.State)
	assert.Empty(t, after.RecommendationReason, "confirmed 行不被 AI 更新叙事")

	// Dismissed likewise immune.
	rev := after.Revision
	_, err = repo.ApplyMarkerKeyEvent(topicID, m.ID, KeyEventActionUnmark, &rev)
	require.NoError(t, err)
	saved, err = repo.SaveSuggestedMarkers(boardID, reportID, "tok-1", []SuggestedKeyEvent{{
		SourceThreadID: threadID, Reason: "again", EvidenceIDs: []uint{30},
	}})
	require.NoError(t, err)
	assert.Equal(t, 0, saved)
	var after2 TopicKeyEventMarker
	require.NoError(t, repo.db.First(&after2, m.ID).Error)
	assert.Equal(t, KeyEventStateDismissed, after2.State)

	// Fresh unprocessed source → suggestion persists as suggested.
	t2 := keyEventSeedThreadWithParams(t, repo.db, reportID, sectionID, "新事件", []uint{4}, []uint{31})
	saved, err = repo.SaveSuggestedMarkers(boardID, reportID, "tok-1", []SuggestedKeyEvent{{
		SourceThreadID: t2, Reason: "有效理由", NewProgress: false, EvidenceIDs: []uint{31},
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, saved)
	var sugg TopicKeyEventMarker
	require.NoError(t, repo.db.Where("source_thread_id = ?", t2).First(&sugg).Error)
	assert.Equal(t, KeyEventStateSuggested, sugg.State)
	var basis struct {
		EvidenceArticleIDs []uint `json:"evidence_article_ids"`
		NewProgress        bool   `json:"new_progress"`
	}
	require.NoError(t, json.Unmarshal([]byte(sugg.RecommendationBasis), &basis))
	assert.Equal(t, []uint{31}, basis.EvidenceArticleIDs)
	assert.False(t, basis.NewProgress)

	// Repeat save for the same unprocessed source → refresh, no duplicate.
	saved, err = repo.SaveSuggestedMarkers(boardID, reportID, "tok-1", []SuggestedKeyEvent{{
		SourceThreadID: t2, Reason: "更新后的理由", EvidenceIDs: []uint{31},
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, saved)
	var cnt int64
	require.NoError(t, repo.db.Model(&TopicKeyEventMarker{}).Where("source_thread_id = ?", t2).Count(&cnt).Error)
	assert.Equal(t, int64(1), cnt)

	// Confirm the suggestion → late re-save must not resurrect reason updates.
	rev2 := sugg.Revision
	conf, err := repo.ApplyMarkerKeyEvent(topicID, sugg.ID, KeyEventActionMark, &rev2)
	require.NoError(t, err)
	assert.Equal(t, KeyEventStateConfirmed, conf.State)
	saved, err = repo.SaveSuggestedMarkers(boardID, reportID, "tok-1", []SuggestedKeyEvent{{
		SourceThreadID: t2, Reason: "又迟到了", EvidenceIDs: []uint{31},
	}})
	require.NoError(t, err)
	assert.Equal(t, 0, saved)
	var confReload TopicKeyEventMarker
	require.NoError(t, repo.db.First(&confReload, sugg.ID).Error)
	assert.Equal(t, KeyEventStateConfirmed, confReload.State)
}

func TestKeyEvent_SaveSuggestedMarkers_StaleTokenDropsBatch(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, reportID, sectionID, threadID := keyEventFullSetup(t, repo, "token 案例", []uint{8}, []uint{80})

	// Old run's token; a rebuild already restarted with a new token.
	require.NoError(t, repo.UpsertKeyEventRecommendationRun(reportID, boardID, "old-token"))
	require.NoError(t, repo.UpsertKeyEventRecommendationRun(reportID, boardID, "new-token"))

	saved, err := repo.SaveSuggestedMarkers(boardID, reportID, "old-token", []SuggestedKeyEvent{{
		SourceThreadID: threadID, Reason: "过期建议", EvidenceIDs: []uint{80},
	}})
	require.NoError(t, err)
	assert.Equal(t, 0, saved, "token 不匹配整批丢弃")
	var cnt int64
	require.NoError(t, repo.db.Model(&TopicKeyEventMarker{}).Count(&cnt).Error)
	assert.Equal(t, int64(0), cnt)

	// Current token saves normally.
	saved, err = repo.SaveSuggestedMarkers(boardID, reportID, "new-token", []SuggestedKeyEvent{{
		SourceThreadID: threadID, Reason: "有效建议", EvidenceIDs: []uint{80},
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, saved)

	_ = sectionID
	_ = topicID
}

// ── DB 约束：CHECK 状态枚举 / FK 删除语义 ──────────────────────────────────

func TestKeyEvent_DBCheckAndFKSemantics(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, reportID, _, threadID := keyEventFullSetup(t, repo, "约束案例", []uint{9}, []uint{90})

	// CHECK: invalid state rejected at the DB level.
	err := repo.db.Exec(`INSERT INTO topic_key_event_markers
		(semantic_board_id, source_key, persistent_topic_id, source_period_date, state, revision, created_at, updated_at)
		VALUES (?, 'deadbeef', ?, CURRENT_DATE, 'banana', 1, now(), now())`, boardID, topicID).Error
	require.Error(t, err, "CHECK 约束应拒绝非法 state")

	// Run-state CHECK.
	err = repo.db.Exec(`INSERT INTO key_event_recommendation_runs
		(report_id, semantic_board_id, state, generation_token, created_at, updated_at)
		VALUES (?, ?, 'paused', 't', now(), now())`, reportID, boardID).Error
	require.Error(t, err)

	// FK: thread delete (rebuild's inner step) SET NULLs the source pointer
	// at the DB level, keeping the marker row.
	m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
	require.NoError(t, err)
	require.NoError(t, repo.db.Where("id = ?", threadID).Delete(&DailyReportThread{}).Error)
	var after TopicKeyEventMarker
	require.NoError(t, repo.db.First(&after, m.ID).Error)
	assert.Nil(t, after.SourceThreadID)
	assert.Equal(t, KeyEventStateConfirmed, after.State)

	// FK: report delete SET NULLs source_report_id.
	require.NoError(t, repo.db.Where("id = ?", reportID).Delete(&BoardDailyReport{}).Error)
	require.NoError(t, repo.db.First(&after, m.ID).Error)
	assert.Nil(t, after.SourceReportID)

	// FK deletion semantics asserted from pg_constraint (actually deleting a
	// board trips unrelated FKs from other board-owned tables):
	// board → CASCADE, topic/report/section/thread → SET NULL.
	type fkRow struct {
		Conname string
		Action  string
	}
	var fks []fkRow
	require.NoError(t, repo.db.Raw(`
		SELECT con.conname AS conname, con.confdeltype AS action
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		WHERE rel.relname = 'topic_key_event_markers' AND con.contype = 'f'`).Scan(&fks).Error)
	actions := map[string]string{}
	for _, fk := range fks {
		actions[fk.Conname] = fk.Action
	}
	assert.Equal(t, "c", actions["fk_topic_key_event_markers_board"], "board FK ON DELETE CASCADE")
	assert.Equal(t, "n", actions["fk_topic_key_event_markers_topic"], "topic FK ON DELETE SET NULL")
	assert.Equal(t, "n", actions["fk_topic_key_event_markers_report"], "report FK ON DELETE SET NULL")
	assert.Equal(t, "n", actions["fk_topic_key_event_markers_section"], "section FK ON DELETE SET NULL")
	assert.Equal(t, "n", actions["fk_topic_key_event_markers_thread"], "thread FK ON DELETE SET NULL")
}

// ── 窗口查询 ────────────────────────────────────────────────────────────────

func TestKeyEvent_ListTopicMarkersWindowAndOrder(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID := seedTestBoard(t, repo.db)
	topicID := keyEventSeedTopic(t, repo.db, boardID, "窗口")

	today := NormalizeReportDate(time.Now())
	makeMarker := func(daysAgo int, title string) uint {
		reportID := seedTestReport(t, repo.db, boardID, today.AddDate(0, 0, -daysAgo))
		sectionID := seedTestSectionWithTopic(t, repo.db, reportID, title, topicID)
		threadID := keyEventSeedThreadWithParams(t, repo.db, reportID, sectionID, title, []uint{1}, []uint{2})
		m, err := repo.ApplyThreadKeyEvent(topicID, threadID, KeyEventActionMark, nil)
		require.NoError(t, err)
		return m.ID
	}
	fresh := makeMarker(0, "今天")
	day3 := makeMarker(3, "三天前")
	day10 := makeMarker(10, "十天前")

	// 7-day window: fresh + day3.
	got, err := repo.ListTopicKeyEventMarkers(topicID, 7, KeyEventStateConfirmed)
	require.NoError(t, err)
	ids := []uint{}
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	assert.ElementsMatch(t, []uint{fresh, day3}, ids)

	// days=0 → all.
	all, err := repo.ListTopicKeyEventMarkers(topicID, 0, KeyEventStateConfirmed)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Ordering: source date DESC (newest first), stable.
	assert.Equal(t, fresh, all[0].ID)
	assert.Equal(t, day3, all[1].ID)
	assert.Equal(t, day10, all[2].ID)
}

func TestKeyEvent_CountConfirmedThreadMarkers(t *testing.T) {
	repo := keyEventTestRepo(t)
	_, topicID, reportID, sectionID, _ := keyEventFullSetup(t, repo, "计数A", []uint{1}, []uint{10})

	tA := keyEventSeedThreadWithParams(t, repo.db, reportID, sectionID, "计数A", []uint{1}, []uint{10})
	tB := keyEventSeedThreadWithParams(t, repo.db, reportID, sectionID, "计数B", []uint{2}, []uint{11})

	_, err := repo.ApplyThreadKeyEvent(topicID, tA, KeyEventActionMark, nil)
	require.NoError(t, err)
	_, err = repo.ApplyThreadKeyEvent(topicID, tB, KeyEventActionMark, nil)
	require.NoError(t, err)

	// ★n counts independent confirmed thread markers per source thread.
	counts, err := repo.CountConfirmedThreadMarkers([]uint{tA, tB, 999})
	require.NoError(t, err)
	assert.Equal(t, 1, counts[tA])
	assert.Equal(t, 1, counts[tB])
	_, ok := counts[999]
	assert.False(t, ok)

	// Unmark A → its count drops to zero; B unaffected.
	var mA TopicKeyEventMarker
	require.NoError(t, repo.db.Where("source_thread_id = ?", tA).First(&mA).Error)
	_, err = repo.ApplyMarkerKeyEvent(topicID, mA.ID, KeyEventActionUnmark, &mA.Revision)
	require.NoError(t, err)
	counts, err = repo.CountConfirmedThreadMarkers([]uint{tA, tB})
	require.NoError(t, err)
	assert.Equal(t, 0, counts[tA])
	assert.Equal(t, 1, counts[tB])
}

// ── 生命线装配（lifeline key_events 只透 confirmed）────────────────────────

func TestKeyEvent_GetTopicLifelineCarriesConfirmedOnly(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, reportID, sectionID, threadID := keyEventFullSetup(t, repo, "生命线事件", []uint{9}, []uint{10, 11})

	// 一条 confirmed（用户确认）+ 一条同话题其他来源的 suggested（AI 建议）。
	confirmed, err := repo.ApplyThreadKeyEvent(topicID, threadID, "mark", nil)
	require.NoError(t, err)
	require.Equal(t, KeyEventStateConfirmed, confirmed.State)

	otherThread := keyEventSeedThreadWithParams(t, repo.db, reportID, sectionID, "AI 建议行", []uint{21}, []uint{22})
	_, err = repo.ApplyThreadKeyEvent(topicID, otherThread, "dismiss", nil) // dismissed 也不得进生命线
	require.NoError(t, err)
	require.NoError(t, repo.UpsertKeyEventRecommendationRun(reportID, boardID, "token-lifeline"))
	_, err = repo.SaveSuggestedMarkers(boardID, reportID, "token-lifeline", []SuggestedKeyEvent{{
		SourceThreadID: otherThread,
		Reason:         "AI 建议（不得进生命线）",
		NewProgress:    true,
		EvidenceIDs:    []uint{22},
	}})
	require.NoError(t, err)

	resp, err := repo.GetTopicLifeline(topicID)
	require.NoError(t, err)
	require.NotNil(t, resp.KeyEvents)
	require.Len(t, resp.KeyEvents, 1, "生命线只透 confirmed：suggested/dismissed 不进回看")
	require.Equal(t, confirmed.ID, resp.KeyEvents[0].ID)
	require.Equal(t, KeyEventStateConfirmed, resp.KeyEvents[0].State)
}

// ── 重建轮换推荐 token（design D3）────────────────────────────────────────

func TestKeyEvent_RebuildRotatesRunToken(t *testing.T) {
	repo := keyEventTestRepo(t)
	boardID, topicID, reportID, sectionID, threadID := keyEventFullSetup(t, repo, "轮换事件", []uint{31}, []uint{41})
	_ = sectionID

	// 建议任务 A 启动（run 行 + tokenA）并写入一条 suggested。
	require.NoError(t, repo.UpsertKeyEventRecommendationRun(reportID, boardID, "tokenA"))
	saved, err := repo.SaveSuggestedMarkers(boardID, reportID, "tokenA", []SuggestedKeyEvent{{
		SourceThreadID: threadID, Reason: "A", NewProgress: true, EvidenceIDs: []uint{41},
	}})
	require.NoError(t, err)
	require.Equal(t, 1, saved)

	// 同日重建：run 行 token 必须被轮换（旧任务的迟到写入全部按旧 token 落空）。
	rebuildReportWithThreads(t, repo, reportID, topicID, []string{"轮换事件"}, []uint{31}, []uint{41}, "same")
	run, err := repo.GetKeyEventRecommendationRun(reportID)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.NotEqual(t, "tokenA", run.GenerationToken, "重建必须轮换 token")

	// 旧 token 的迟到批次整批丢弃；run 终态写入也按旧 token 匹配落空。
	late, err := repo.SaveSuggestedMarkers(boardID, reportID, "tokenA", []SuggestedKeyEvent{{
		SourceThreadID: threadID, Reason: "late", NewProgress: true, EvidenceIDs: []uint{41},
	}})
	require.NoError(t, err)
	assert.Zero(t, late, "迟到批次按旧 token 整批丢弃")
	require.NoError(t, repo.FinishKeyEventRecommendationRun(reportID, "tokenA", KeyEventRunStateDone, 1, 1, ""))
	run2, err := repo.GetKeyEventRecommendationRun(reportID)
	require.NoError(t, err)
	assert.Equal(t, KeyEventRunStateRunning, run2.State, "旧 token 终态写入不得污染轮换后的 run 行")
}
