package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// Story repository tests (add-event-story-heat batch 2c) on in-memory
// SQLite — same style as tagmanagement's tagger_fingerprint_test.go: pure
// read/modify/write paths, no Postgres-specific SQL. The pgvector recall
// (<=>) runs only on Postgres (testutil.SetupTestDB) and is covered here
// only through its SQLite degradation contract (nil, nil).

func setupStoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:story-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.TopicTag{},
		&models.Article{},
		&BoardDailyReport{},
		&DailyReportSection{},
		&DailyReportThread{},
		&BoardStory{},
		&StorySignal{},
	))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedStoryTag(t *testing.T, db *gorm.DB, label string, metadata models.MetadataMap) uint {
	t.Helper()
	if metadata == nil {
		metadata = models.MetadataMap{}
	}
	tag := models.TopicTag{Label: label, Slug: label, Category: models.TagCategoryEvent, Metadata: metadata}
	require.NoError(t, db.Create(&tag).Error)
	return tag.ID
}

func seedStoryReport(t *testing.T, db *gorm.DB, boardID uint, day time.Time) uint {
	t.Helper()
	report := BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      NormalizeReportDate(day),
		Title:           "story-test",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

func seedStorySection(t *testing.T, db *gorm.DB, reportID uint, topicID *uint) uint {
	t.Helper()
	section := DailyReportSection{
		ReportID:          reportID,
		ClusterLabel:      "sec",
		PersistentTopicID: topicID,
	}
	require.NoError(t, db.Omit("embedding").Create(&section).Error)
	return section.ID
}

func seedStoryThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, tagIDs []uint, articleIDs []uint, manual bool) uint {
	t.Helper()
	thread := DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             fmt.Sprintf("thread-%d-%d", reportID, sectionID),
		TagIDs:            mustStoryJSON(tagIDs),
		RelatedArticleIDs: mustStoryJSON(articleIDs),
		StoryManual:       manual,
	}
	require.NoError(t, db.Omit("embedding").Create(&thread).Error)
	return thread.ID
}

func mustStoryJSON(ids []uint) JSON {
	if len(ids) == 0 {
		return JSON("[]")
	}
	raw := "["
	for i, id := range ids {
		if i > 0 {
			raw += ","
		}
		raw += fmt.Sprintf("%d", id)
	}
	return JSON(raw + "]")
}

func seedStoryLine(t *testing.T, db *gorm.DB, boardID uint, subjectKey, action, object string, lastSeen time.Time) uint {
	t.Helper()
	story := BoardStory{
		BoardID:       boardID,
		Title:         "story-" + subjectKey,
		Subject:       subjectKey,
		SubjectKey:    subjectKey,
		Action:        action,
		Object:        object,
		Status:        StoryStatusActive,
		FirstSeenDate: lastSeen,
		LastSeenDate:  lastSeen,
		Origin:        StoryOriginRule,
	}
	require.NoError(t, db.Omit("embedding", "subject_embedding", "occurred_at").Create(&story).Error)
	return story.ID
}

// ── SaveThreads scope derivation (task 3.2) ─────────────────────────────────

func TestSaveThreadsDerivesScopeByEventTagCount(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	reportID := seedStoryReport(t, db, boardID, time.Now())
	sectionID := seedStorySection(t, db, reportID, nil)

	event1 := seedStoryTag(t, db, "e1", nil)
	event2 := seedStoryTag(t, db, "e2", nil)
	event3 := seedStoryTag(t, db, "e3", nil)
	keywordTag := models.TopicTag{Label: "kw", Slug: "kw", Category: "keyword"}
	require.NoError(t, db.Create(&keywordTag).Error)

	threads := []DailyReportThread{
		{Title: "single", TagIDs: mustStoryJSON([]uint{event1})},
		{Title: "keyword-only", TagIDs: mustStoryJSON([]uint{keywordTag.ID})},
		{Title: "composite-2", TagIDs: mustStoryJSON([]uint{event1, event2})},
		{Title: "composite-3", TagIDs: mustStoryJSON([]uint{event1, event2, event3, keywordTag.ID})},
		{Title: "no-tags", TagIDs: nil},
	}
	require.NoError(t, repo.SaveThreads(db, reportID, sectionID, threads))

	var saved []DailyReportThread
	require.NoError(t, db.Where("report_id = ?", reportID).Order("id ASC").Find(&saved).Error)
	require.Len(t, saved, 5)
	assert.Equal(t, ThreadScopeSingle, saved[0].Scope, "1 event tag → single")
	assert.Equal(t, ThreadScopeSingle, saved[1].Scope, "keyword tags never count → single")
	assert.Equal(t, ThreadScopeComposite, saved[2].Scope, "2 event tags → composite")
	assert.Equal(t, ThreadScopeComposite, saved[3].Scope, "3 event tags (+keyword) → composite")
	assert.Equal(t, ThreadScopeSingle, saved[4].Scope, "no tags → single")
}

func TestSaveThreadsScopeDegradesToSingleWhenTagLookupFails(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	reportID := seedStoryReport(t, db, boardID, time.Now())
	sectionID := seedStorySection(t, db, reportID, nil)
	require.NoError(t, db.Migrator().DropTable(&models.TopicTag{}))

	// Even unknown tag ids must save as single — scope derivation must never
	// block the save (warning path).
	threads := []DailyReportThread{
		{Title: "t", TagIDs: mustStoryJSON([]uint{7, 8})},
	}
	require.NoError(t, repo.SaveThreads(db, reportID, sectionID, threads))
	var saved DailyReportThread
	require.NoError(t, db.Where("report_id = ?", reportID).First(&saved).Error)
	assert.Equal(t, ThreadScopeSingle, saved.Scope)
}

// ── ListUnassignedThreads (task 3.4 skip filters) ───────────────────────────

func TestListUnassignedThreadsFiltersManualAndAssigned(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	reportID := seedStoryReport(t, db, boardID, time.Now())
	sectionID := seedStorySection(t, db, reportID, nil)
	storyID := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())

	plain := seedStoryThread(t, db, reportID, sectionID, nil, nil, false)
	manual := seedStoryThread(t, db, reportID, sectionID, nil, nil, true)
	assigned := seedStoryThread(t, db, reportID, sectionID, nil, nil, false)
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id = ?", assigned).
		Update("story_id", storyID).Error)
	otherReport := seedStoryThread(t, db, seedStoryReport(t, db, boardID, time.Now().AddDate(0, 0, -1)), sectionID, nil, nil, false)

	got, err := repo.ListUnassignedThreads(reportID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, plain, got[0].ID, "manual/assigned/other-report threads filtered")
	assert.NotEqual(t, manual, got[0].ID)
	assert.NotEqual(t, otherReport, got[0].ID)
}

// ── Attach + RecomputeBoardStoryCounts (count recompute, never increment) ──

func TestAttachThreadToBoardStoryRecomputesCounts(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	day1 := time.Now().AddDate(0, 0, -2)
	report1 := seedStoryReport(t, db, boardID, day1)
	section1 := seedStorySection(t, db, report1, nil)
	report2 := seedStoryReport(t, db, boardID, time.Now())
	section2 := seedStorySection(t, db, report2, nil)

	storyID := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())

	t1 := seedStoryThread(t, db, report1, section1, nil, []uint{11, 12}, false)
	t2 := seedStoryThread(t, db, report2, section2, nil, []uint{13}, false)

	require.NoError(t, repo.AttachThreadToBoardStory(report1, t1, storyID, false))
	require.NoError(t, repo.AttachThreadToBoardStory(report2, t2, storyID, true))

	var story BoardStory
	require.NoError(t, db.First(&story, storyID).Error)
	assert.Equal(t, 2, story.ThreadCount)
	assert.Equal(t, 3, story.ArticleCount)
	assert.Equal(t, NormalizeReportDate(day1).Format("2006-01-02"), story.FirstSeenDate.Format("2006-01-02"))
	assert.Equal(t, NormalizeReportDate(time.Now()).Format("2006-01-02"), story.LastSeenDate.Format("2006-01-02"))

	var gotThread DailyReportThread
	require.NoError(t, db.First(&gotThread, t2).Error)
	require.NotNil(t, gotThread.StoryID)
	assert.Equal(t, storyID, *gotThread.StoryID)
	assert.True(t, gotThread.IsDevelopment)
}

func TestAttachThreadToBoardStoryRejectsManualAndAssigned(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	reportID := seedStoryReport(t, db, boardID, time.Now())
	sectionID := seedStorySection(t, db, reportID, nil)
	storyID := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())

	manual := seedStoryThread(t, db, reportID, sectionID, nil, nil, true)
	err := repo.AttachThreadToBoardStory(reportID, manual, storyID, false)
	assert.ErrorIs(t, err, ErrStoryThreadNotAttachable)

	assigned := seedStoryThread(t, db, reportID, sectionID, nil, nil, false)
	require.NoError(t, repo.AttachThreadToBoardStory(reportID, assigned, storyID, false))
	err = repo.AttachThreadToBoardStory(reportID, assigned, storyID, false)
	assert.ErrorIs(t, err, ErrStoryThreadNotAttachable, "re-attach is a sentinel skip, not a double count")

	// Wrong report: rejected too.
	otherReport := seedStoryReport(t, db, boardID, time.Now().AddDate(0, 0, -1))
	otherSection := seedStorySection(t, db, otherReport, nil)
	stray := seedStoryThread(t, db, otherReport, otherSection, nil, nil, false)
	err = repo.AttachThreadToBoardStory(reportID, stray, storyID, false)
	assert.ErrorIs(t, err, ErrStoryThreadNotAttachable)

	var manualThread DailyReportThread
	require.NoError(t, db.First(&manualThread, manual).Error)
	assert.Nil(t, manualThread.StoryID, "human-protected thread stays unassigned")
}

func TestRecomputeBoardStoryCountsZeroThreadsKeepsDates(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	reportID := seedStoryReport(t, db, boardID, time.Now())
	sectionID := seedStorySection(t, db, reportID, nil)
	storyID := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now().AddDate(0, 0, -3))
	threadID := seedStoryThread(t, db, reportID, sectionID, nil, []uint{1, 2}, false)
	require.NoError(t, repo.AttachThreadToBoardStory(reportID, threadID, storyID, false))

	// Simulate SaveReport rerun deleting the old threads: counts must drop to
	// zero via recompute, dates must stay (never NULL / never reset).
	require.NoError(t, db.Where("id = ?", threadID).Delete(&DailyReportThread{}).Error)
	require.NoError(t, repo.RecomputeBoardStoryCounts(storyID))

	var story BoardStory
	require.NoError(t, db.First(&story, storyID).Error)
	assert.Zero(t, story.ThreadCount)
	assert.Zero(t, story.ArticleCount)
	assert.False(t, story.FirstSeenDate.IsZero(), "dates survive a zero-thread recompute")
	assert.False(t, story.LastSeenDate.IsZero())
}

// ── Merge + resolve (task 3.3: one-hop, manual protection) ──────────────────

func TestMergeBoardStoriesOneHopAndManualProtection(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	reportID := seedStoryReport(t, db, boardID, time.Now())
	sectionID := seedStorySection(t, db, reportID, nil)

	fromID := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())
	intoID := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())

	autoThread := seedStoryThread(t, db, reportID, sectionID, nil, []uint{21}, false)
	manualThread := seedStoryThread(t, db, reportID, sectionID, nil, []uint{22}, false)
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id IN ?", []uint{autoThread, manualThread}).
		Update("story_id", fromID).Error)
	// The manual thread was human-assigned to `from`.
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id = ?", manualThread).
		Update("story_manual", true).Error)

	survivor, err := repo.MergeBoardStories(fromID, intoID)
	require.NoError(t, err)
	assert.Equal(t, intoID, survivor)

	var fromRow, intoRow BoardStory
	require.NoError(t, db.First(&fromRow, fromID).Error)
	require.NoError(t, db.First(&intoRow, intoID).Error)
	require.NotNil(t, fromRow.MergedInto)
	assert.Equal(t, intoID, *fromRow.MergedInto)
	assert.Equal(t, StoryStatusMerged, fromRow.Status)
	assert.Equal(t, StoryStatusActive, intoRow.Status)

	var moved, kept DailyReportThread
	require.NoError(t, db.First(&moved, autoThread).Error)
	require.NoError(t, db.First(&kept, manualThread).Error)
	require.NotNil(t, moved.StoryID)
	assert.Equal(t, intoID, *moved.StoryID, "non-manual thread moved to survivor")
	require.NotNil(t, kept.StoryID)
	assert.Equal(t, fromID, *kept.StoryID, "manual thread stays on the merged line (readers resolve one hop)")

	// Counts: survivor has the moved thread; the merged line keeps the manual.
	assert.Equal(t, 1, intoRow.ThreadCount)
	assert.Equal(t, 1, fromRow.ThreadCount)

	// One-hop read: manual thread's story resolves to the survivor.
	resolved, err := repo.ResolveBoardStoryID(fromID)
	require.NoError(t, err)
	assert.Equal(t, intoID, resolved)
}

func TestMergeBoardStoriesSameIDSurvivorNoOp(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	storyID := seedStoryLine(t, db, 1, "company:a", "发布", "产品", time.Now())
	survivor, err := repo.MergeBoardStories(storyID, storyID)
	require.NoError(t, err)
	assert.Equal(t, storyID, survivor)
	var story BoardStory
	require.NoError(t, db.First(&story, storyID).Error)
	assert.Nil(t, story.MergedInto, "no-op merge must not mark the row merged")
}

func TestMergeBoardStoriesRejectsMergedEndpointsAndFlattensChildren(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	a := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())
	b := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())
	c := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())

	// a → b (a merged).
	_, err := repo.MergeBoardStories(a, b)
	require.NoError(t, err)

	// Merging FROM an already-merged row → rejected (two-level chain).
	_, err = repo.MergeBoardStories(a, c)
	assert.ErrorIs(t, err, ErrStoryTwoLevelChain)
	// Merging INTO an already-merged row → rejected.
	_, err = repo.MergeBoardStories(c, a)
	assert.ErrorIs(t, err, ErrStoryTwoLevelChain)

	// b carries child a; merging b into c must FLATTEN a to point at c (design
	// §5.4: 旧记录直接指向最终存活者).
	survivor, err := repo.MergeBoardStories(b, c)
	require.NoError(t, err)
	assert.Equal(t, c, survivor)

	var aRow BoardStory
	require.NoError(t, db.First(&aRow, a).Error)
	require.NotNil(t, aRow.MergedInto)
	assert.Equal(t, c, *aRow.MergedInto, "child re-pointed at the final survivor (no two-hop)")

	resolved, err := repo.ResolveBoardStoryID(a)
	require.NoError(t, err)
	assert.Equal(t, c, resolved)
}

func TestResolveBoardStoryID(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	a := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())
	b := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())

	resolved, err := repo.ResolveBoardStoryID(a)
	require.NoError(t, err)
	assert.Equal(t, a, resolved, "un-merged id resolves to itself")

	_, err = repo.MergeBoardStories(a, b)
	require.NoError(t, err)
	resolved, err = repo.ResolveBoardStoryID(a)
	require.NoError(t, err)
	assert.Equal(t, b, resolved, "merged id resolves one hop")

	// Forge a two-level chain directly (MergeBoardStories refuses to create
	// one) and verify the read path rejects it.
	c := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", b).
		Update("merged_into", c).Error)
	_, err = repo.ResolveBoardStoryID(a)
	assert.ErrorIs(t, err, ErrStoryTwoLevelChain, "two-level chains are refused, not silently followed")
}

// ── SetTagFingerprintTrusted (task 2.4 调用侧落库) ──────────────────────────

func TestSetTagFingerprintTrustedPreservesAndSkips(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	tagID := seedStoryTag(t, db, "字节发布扣子", models.MetadataMap{
		"event_keywords":    []any{"字节跳动", "扣子"},
		"event_fingerprint": map[string]any{"subject": "字节跳动", "subject_key": "company:bytedance"},
	})

	require.NoError(t, repo.SetTagFingerprintTrusted(tagID, true))

	var tag models.TopicTag
	require.NoError(t, db.First(&tag, tagID).Error)
	assert.Equal(t, true, tag.Metadata["fingerprint_trusted"])
	keywords, ok := tag.Metadata["event_keywords"].([]any)
	require.True(t, ok, "event_keywords preserved")
	assert.NotEmpty(t, keywords)
	nested, ok := tag.Metadata["event_fingerprint"].(map[string]any)
	require.True(t, ok, "event_fingerprint preserved")
	assert.Equal(t, "company:bytedance", nested["subject_key"])

	// Same value again → skipped write (idempotent); flip updates in place.
	before := tag.UpdatedAt
	require.NoError(t, repo.SetTagFingerprintTrusted(tagID, true))
	require.NoError(t, repo.SetTagFingerprintTrusted(tagID, false))
	var after models.TopicTag
	require.NoError(t, db.First(&after, tagID).Error)
	assert.Equal(t, false, after.Metadata["fingerprint_trusted"])
	_ = before
}

// ── Recall degradation + list path ──────────────────────────────────────────

func TestRecallStoryCandidatesSQLiteReturnsNilForCallerFallback(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	cands, err := repo.RecallStoryCandidates(1, "[0.1,0.2]", time.Now().AddDate(0, 0, -14), 10)
	require.NoError(t, err)
	assert.Empty(t, cands, "SQLite (tests) degrades to the list path by contract")
}

func TestListRecentStoryCandidatesFiltersAndOrder(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	otherBoard := uint(2)
	now := time.Now()

	inWindow := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", now.AddDate(0, 0, -1))
	newest := seedStoryLine(t, db, boardID, "company:b", "发布", "产品", now)
	stale := seedStoryLine(t, db, boardID, "company:c", "发布", "产品", now.AddDate(0, 0, -30))
	seedStoryLine(t, db, otherBoard, "company:d", "发布", "产品", now) // other board
	merged := seedStoryLine(t, db, boardID, "company:e", "发布", "产品", now)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", merged).
		Updates(map[string]any{"status": StoryStatusMerged}).Error)
	settled := seedStoryLine(t, db, boardID, "company:f", "发布", "产品", now)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", settled).
		Update("status", "settled").Error)
	manual := seedStoryLine(t, db, boardID, "company:g", "发布", "产品", now)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", manual).
		Update("origin", StoryOriginManual).Error)

	got, err := repo.ListRecentStoryCandidates(boardID, now.AddDate(0, 0, -StoryRecallWindowDays), 10)
	require.NoError(t, err)
	require.Len(t, got, 2, "window/board/status/origin filters applied")
	assert.Equal(t, newest, got[0].ID, "newest first")
	assert.Equal(t, inWindow, got[1].ID)
	for _, c := range got {
		assert.NotEqual(t, stale, c.ID)
	}
}

func TestGetStoryArticleIDsPortablePath(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := uint(1)
	reportID := seedStoryReport(t, db, boardID, time.Now())
	sectionID := seedStorySection(t, db, reportID, nil)

	s1 := seedStoryLine(t, db, boardID, "company:a", "发布", "产品", time.Now())
	s2 := seedStoryLine(t, db, boardID, "company:b", "发布", "产品", time.Now())
	seedStoryLine(t, db, boardID, "company:c", "发布", "产品", time.Now()) // no threads

	t1 := seedStoryThread(t, db, reportID, sectionID, nil, []uint{31, 31, 32}, false)
	seedStoryThread(t, db, reportID, sectionID, nil, []uint{33}, false)
	// That thread stays unassigned on purpose: its article 33 must not leak into s1.
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id = ?", t1).
		Update("story_id", s1).Error)
	// t2 stays unassigned on purpose: its article 33 must not leak into s1.
	t3 := seedStoryThread(t, db, reportID, sectionID, nil, []uint{}, false)
	require.NoError(t, db.Model(&DailyReportThread{}).Where("id = ?", t3).
		Update("story_id", s2).Error)

	got, err := repo.GetStoryArticleIDs([]uint{s1, s2})
	require.NoError(t, err)
	assert.Equal(t, []uint{31, 32}, got[s1], "union + dedup")
	assert.Empty(t, got[s2], "empty article set")

	empty, err := repo.GetStoryArticleIDs(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestEarliestArticleAtPubDatePriorityAndFallback(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)

	older := time.Now().AddDate(0, 0, -5).Truncate(time.Second)
	newerPub := time.Now().AddDate(0, 0, -1).Truncate(time.Second)
	withPub := models.Article{FeedID: 1, Title: "pub", PubDate: &newerPub}
	require.NoError(t, db.Create(&withPub).Error)
	noPub := models.Article{FeedID: 1, Title: "nopub", CreatedAt: older}
	require.NoError(t, db.Create(&noPub).Error)

	got, err := repo.EarliestArticleAt([]uint{withPub.ID, noPub.ID})
	require.NoError(t, err)
	assert.Equal(t, older.UTC().Format("2006-01-02"), got.UTC().Format("2006-01-02"),
		"MIN over COALESCE(pub_date, created_at) — the older created_at wins")

	zero, err := repo.EarliestArticleAt(nil)
	require.NoError(t, err)
	assert.True(t, zero.IsZero(), "empty input → zero time")

	missing, err := repo.EarliestArticleAt([]uint{99999})
	require.NoError(t, err)
	assert.True(t, missing.IsZero(), "no rows → zero time")
}

func TestCreateBoardStoryAppliesDefaults(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	story := &BoardStory{
		BoardID:       7,
		Title:         "t",
		FirstSeenDate: time.Now(),
		LastSeenDate:  time.Now(),
	}
	require.NoError(t, repo.CreateBoardStory(story))
	assert.NotZero(t, story.ID)

	var loaded BoardStory
	require.NoError(t, db.First(&loaded, story.ID).Error)
	assert.Equal(t, StoryStatusActive, loaded.Status)
	assert.Equal(t, StoryOriginRule, loaded.Origin)
}

func TestArticleFeedsIncludesArchivedArticles(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	at := time.Date(2026, 11, 5, 12, 0, 0, 0, storyHeatZone)
	active := seedSignalArticle(t, db, 1, at, &at, false)
	archived := seedSignalArticle(t, db, 2, at, &at, true)

	feeds, err := repo.ArticleFeeds([]uint{active, archived})
	require.NoError(t, err)
	assert.Equal(t, map[uint]uint{active: 1, archived: 2}, feeds)
}

func TestListRecentStoryCandidatesLatestThreadEvidence(t *testing.T) {
	db := setupStoryTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()
	storyID := seedStoryLine(t, db, 1, "", "", "", now)
	emptyID := seedStoryLine(t, db, 1, "empty", "", "", now)
	// Insert the older report last: latest means report date, not insertion ID.
	for _, day := range []time.Time{now, now.AddDate(0, 0, -1)} {
		reportID := seedStoryReport(t, db, 1, day)
		sectionID := seedStorySection(t, db, reportID, nil)
		for i := range 2 {
			thread := DailyReportThread{ReportID: reportID, SectionID: sectionID, StoryID: &storyID,
				Title: fmt.Sprintf("%s-%d", day.Format("2006-01-02"), i), Summary: fmt.Sprintf("摘要-%d", i)}
			require.NoError(t, db.Omit("embedding").Create(&thread).Error)
		}
	}
	got, err := repo.ListRecentStoryCandidates(1, now.AddDate(0, 0, -14), 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, c := range got {
		if c.ID == emptyID {
			assert.Empty(t, c.Title)
			assert.Empty(t, c.Summary)
			assert.True(t, c.PeriodDate.IsZero())
		} else {
			assert.Equal(t, now.Format("2006-01-02")+"-1", c.Title)
			assert.Equal(t, "摘要-1", c.Summary)
			assert.Equal(t, now.Format("2006-01-02"), c.PeriodDate.Format("2006-01-02"))
		}
	}
}

func TestRecallStoryCandidatesPostgresLatestThreadEvidence(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()
	boardID := seedTestBoardNamed(t, db, "story-recall-evidence")
	storyID := seedStoryLine(t, db, boardID, "", "", "", now)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", storyID).Update("embedding", "[1,0]").Error)
	for _, day := range []time.Time{now, now.AddDate(0, 0, -1)} {
		reportID := seedStoryReport(t, db, boardID, day)
		sectionID := seedStorySection(t, db, reportID, nil)
		for i := range 2 {
			thread := DailyReportThread{ReportID: reportID, SectionID: sectionID, StoryID: &storyID,
				Title: fmt.Sprintf("%s-%d", day.Format("2006-01-02"), i), Summary: fmt.Sprintf("摘要-%d", i)}
			require.NoError(t, db.Omit("embedding").Create(&thread).Error)
		}
	}
	recalled, err := repo.RecallStoryCandidates(boardID, "[1,0]", now.AddDate(0, 0, -14), 10)
	require.NoError(t, err)
	recent, err := repo.ListRecentStoryCandidates(boardID, now.AddDate(0, 0, -14), 10)
	require.NoError(t, err)
	for _, candidates := range [][]StoryCandidate{recalled, recent} {
		require.Len(t, candidates, 1)
		assert.Equal(t, now.Format("2006-01-02")+"-1", candidates[0].Title)
		assert.Equal(t, "摘要-1", candidates[0].Summary)
		assert.Equal(t, now.Format("2006-01-02"), candidates[0].PeriodDate.Format("2006-01-02"))
	}
}

func TestCollectBoardIDsForDateMissingTagTables(t *testing.T) {
	for _, missing := range []string{"topic_tags", "topic_tag_board_labels", "article_topic_tags", "articles"} {
		t.Run(missing, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			for _, table := range []string{"topic_tags", "topic_tag_board_labels", "article_topic_tags", "articles"} {
				if table != missing {
					require.NoError(t, db.Exec("CREATE TABLE "+table+" (id INTEGER)").Error)
				}
			}
			ids, err := NewTopicGraphRepository(db).CollectBoardIDsForDate(time.Now())
			require.NoError(t, err)
			require.Empty(t, ids)
		})
	}
}
