package repository

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func TestNormalizeReportDateKeepsRequestedDate(t *testing.T) {
	requested, err := time.ParseInLocation("2006-01-02", "2026-05-26", models.ShanghaiTZ)
	require.NoError(t, err)

	got := NormalizeReportDate(requested)

	require.Equal(t, "2026-05-26", got.Format("2006-01-02"))
	require.Equal(t, time.UTC, got.Location())
	require.Equal(t, 12, got.Hour())
}

func TestListReports_DefaultDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	// Seed reports: today, 3 days ago, 10 days ago
	seedTestReport(t, db, boardID, now)
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -3))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -10))

	// days=0 should default to 7, returning today and 3 days ago
	items, err := repo.ListReports(boardID, 0)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, now.Format("2006-01-02"), items[0].PeriodDate)
	require.Equal(t, now.AddDate(0, 0, -3).Format("2006-01-02"), items[1].PeriodDate)
}

func TestListReports_SevenDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	seedTestReport(t, db, boardID, now)
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -5))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -10))

	items, err := repo.ListReports(boardID, 7)
	require.NoError(t, err)
	require.Len(t, items, 2) // 10 days ago is outside 7-day window
	require.Equal(t, now.Format("2006-01-02"), items[0].PeriodDate)
}

func TestListReports_FortyTwoDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	seedTestReport(t, db, boardID, now)
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -20))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -40))
	seedTestReport(t, db, boardID, now.AddDate(0, 0, -50)) // outside 42 days

	// days=42 should NOT be truncated to 30
	items, err := repo.ListReports(boardID, 42)
	require.NoError(t, err)
	require.Len(t, items, 3) // 50 days ago is outside 42-day window
	// Verify descending order
	require.Equal(t, now.Format("2006-01-02"), items[0].PeriodDate)
	require.Equal(t, now.AddDate(0, 0, -20).Format("2006-01-02"), items[1].PeriodDate)
	require.Equal(t, now.AddDate(0, 0, -40).Format("2006-01-02"), items[2].PeriodDate)
}

func TestGetReportByID_AttachesTopicBriefs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "Tracked topic",
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          TopicStatusActive,
		FirstSeenDate:   now,
		LastSeenDate:    now,
		HitCount:        3,
		ConsecutiveHits: 3,
	}
	require.NoError(t, db.Create(&topic).Error)

	sectionID := seedTestSection(t, db, reportID, "tracked section")
	require.NoError(t, db.Model(&DailyReportSection{}).
		Where("id = ?", sectionID).
		Update("persistent_topic_id", topic.ID).Error)

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)
	require.NotNil(t, report.Sections[0].PersistentTopicID)
	require.Equal(t, topic.ID, *report.Sections[0].PersistentTopicID)
	require.NotNil(t, report.Sections[0].PersistentTopic)
	require.Equal(t, topic.ID, report.Sections[0].PersistentTopic.ID)
	require.Equal(t, topic.Label, report.Sections[0].PersistentTopic.Label)
	require.Equal(t, TopicStatusActive, report.Sections[0].PersistentTopic.Status)
}

func TestGetReportByID_SectionWithoutThreadsMarshalsEmptyArray(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, now)
	// Section with zero thread rows — mirrors the production data where the
	// per-cluster threads LLM call failed and the orchestrator degraded to
	// saving the section without threads.
	seedTestSection(t, db, reportID, "no-thread section")

	report, err := repo.GetReportByID(reportID)
	require.NoError(t, err)
	require.Len(t, report.Sections, 1)

	data, err := json.Marshal(report)
	require.NoError(t, err)
	// threads must serialize as [] (never omitted / null): the frontend reader
	// calls section.threads.filter directly (DailyReportTopicSection.vue).
	require.Contains(t, string(data), `"threads":[]`)
	require.NotNil(t, report.Sections[0].Threads)
	require.Empty(t, report.Sections[0].Threads)
}

func seedTestBoard(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	board := models.SemanticLabel{
		Label:     "test-board",
		Slug:      "test-board",
		LabelType: "board",
		Status:    "active",
	}
	err := db.Create(&board).Error
	require.NoError(t, err)
	return board.ID
}

func seedTestReport(t *testing.T, db *gorm.DB, boardID uint, date time.Time) uint {
	t.Helper()
	// The (semantic_board_id, period_date) unique index (offline-catchup H2)
	// makes repeat calls with the same board+day illegal. Tests that want
	// several reports on "the same day" only care about the section rows, so
	// nudge the day forward until the slot is free. String equality matches
	// SaveReport's own lookup (the PG column is a DATE).
	day := NormalizeReportDate(date)
	for {
		var taken int64
		require.NoError(t, db.Model(&BoardDailyReport{}).
			Where("semantic_board_id = ? AND period_date = ?", boardID, day.Format("2006-01-02")).
			Count(&taken).Error)
		if taken == 0 {
			break
		}
		day = day.AddDate(0, 0, 1)
	}
	report := BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      day,
		Title:           "Test Report",
		Summary:         "Test Summary",
		Status:          "completed",
	}
	err := db.Create(&report).Error
	require.NoError(t, err)
	return report.ID
}

func seedTestSection(t *testing.T, db *gorm.DB, reportID uint, label string) uint {
	t.Helper()
	// daily_report_sections.embedding is a non-nullable `vector` column (model field
	// is `string`, zero value ""); pgvector rejects the empty string. Production
	// always populates Embedding via FloatsToPgVector before insert, so mirror
	// that here. The column is untyped-dimension in the test DB (runtime
	// EnsureVectorDimensionOnce is never invoked), so a 1-dim vector is accepted.
	section := DailyReportSection{
		ReportID:     reportID,
		ClusterLabel: label,
		ArticleCount: 1,
		Embedding:    FloatsToPgVector([]float64{0}),
	}
	err := db.Create(&section).Error
	require.NoError(t, err)
	return section.ID
}

func seedTestRelation(t *testing.T, db *gorm.DB, fromID, toID uint, distance float64) {
	t.Helper()
	rel := SectionRelation{
		FromSectionID: fromID,
		ToSectionID:   toID,
		Distance:      distance,
	}
	err := db.Create(&rel).Error
	require.NoError(t, err)
}

func TestGetSectionLifecycle_MultiHopChain(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -2))
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	r3 := seedTestReport(t, db, boardID, now)

	// #40 -> #50 -> #60
	s40 := seedTestSection(t, db, r1, "section-40")
	s50 := seedTestSection(t, db, r2, "section-50")
	s60 := seedTestSection(t, db, r3, "section-60")

	seedTestRelation(t, db, s40, s50, 0.8)
	seedTestRelation(t, db, s50, s60, 0.7)

	// Query from s60 should return all 3 sections
	resp, err := repo.GetSectionLifecycle(s60)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 3)
	require.Len(t, resp.Relations, 2)
}

func TestGetSectionLifecycle_Fork(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -2))
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	// #40 -> #50, #50 -> #60, #50 -> #61
	s40 := seedTestSection(t, db, r1, "section-40")
	s50 := seedTestSection(t, db, r2, "section-50")
	s60 := seedTestSection(t, db, r2, "section-60")
	s61 := seedTestSection(t, db, r2, "section-61")

	seedTestRelation(t, db, s40, s50, 0.8)
	seedTestRelation(t, db, s50, s60, 0.7)
	seedTestRelation(t, db, s50, s61, 0.6)

	// Query from s60 should return all 4 sections and 3 relations
	resp, err := repo.GetSectionLifecycle(s60)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 4)
	require.Len(t, resp.Relations, 3)
}

func TestGetSectionLifecycle_Merge(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -2))
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	// #50 -> #70, #55 -> #70, #70 -> #80
	s50 := seedTestSection(t, db, r1, "section-50")
	s55 := seedTestSection(t, db, r1, "section-55")
	s70 := seedTestSection(t, db, r2, "section-70")
	s80 := seedTestSection(t, db, r2, "section-80")

	seedTestRelation(t, db, s50, s70, 0.8)
	seedTestRelation(t, db, s55, s70, 0.7)
	seedTestRelation(t, db, s70, s80, 0.6)

	// Query from s50 should return all 4 sections and 3 relations
	resp, err := repo.GetSectionLifecycle(s50)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 4)
	require.Len(t, resp.Relations, 3)
}

func TestGetSectionLifecycle_IsolatedSection(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now)
	s20 := seedTestSection(t, db, r1, "section-20")

	// Isolated section: no relations
	resp, err := repo.GetSectionLifecycle(s20)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 1)
	require.Equal(t, s20, resp.Sections[0].ID)
	require.Len(t, resp.Relations, 0)
}

func TestGetSectionLifecycle_Cycle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now)

	// #50 -> #60 -> #70 -> #50 (cycle)
	s50 := seedTestSection(t, db, r1, "section-50")
	s60 := seedTestSection(t, db, r1, "section-60")
	s70 := seedTestSection(t, db, r1, "section-70")

	seedTestRelation(t, db, s50, s60, 0.8)
	seedTestRelation(t, db, s60, s70, 0.7)
	seedTestRelation(t, db, s70, s50, 0.6)

	// Should return all 3 sections once and 3 relations
	resp, err := repo.GetSectionLifecycle(s50)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 3)
	require.Len(t, resp.Relations, 3)
}

func TestGetSectionLifecycle_ExternalRelationIsolation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	r1 := seedTestReport(t, db, boardID, now)

	// Component A: #50 -> #60 -> #70
	s50 := seedTestSection(t, db, r1, "section-50")
	s60 := seedTestSection(t, db, r1, "section-60")
	s70 := seedTestSection(t, db, r1, "section-70")
	// Component B: #80 (isolated)
	s80 := seedTestSection(t, db, r1, "section-80")

	seedTestRelation(t, db, s50, s60, 0.8)
	seedTestRelation(t, db, s60, s70, 0.7)

	// Query from s50: should NOT include s80 or any relation involving s80
	resp, err := repo.GetSectionLifecycle(s50)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 3)
	require.Len(t, resp.Relations, 2)

	// Verify no relation references s80
	for _, rel := range resp.Relations {
		require.NotEqual(t, s80, rel.FromID)
		require.NotEqual(t, s80, rel.ToID)
	}
}

// =============================================================================
// ListTopicRecentBriefs tests (Slice D: lane context injection)
// =============================================================================

func seedTestActiveTopic(t *testing.T, db *gorm.DB, boardID uint, label string, lastSeen time.Time) uint {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          TopicStatusActive,
		FirstSeenDate:   lastSeen,
		LastSeenDate:    lastSeen,
		HitCount:        1,
	}
	err := db.Create(&topic).Error
	require.NoError(t, err)
	return topic.ID
}

func seedTestCandidateTopic(t *testing.T, db *gorm.DB, boardID uint, label string, lastSeen time.Time) uint {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          TopicStatusCandidate,
		FirstSeenDate:   lastSeen,
		LastSeenDate:    lastSeen,
		HitCount:        1,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic.ID
}

// seedTestTopicTag creates a topic_tags row with the given label/status.
func seedTestTopicTag(t *testing.T, db *gorm.DB, label string, status string) uint {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: "slug-" + label, Status: status, Category: "event"}
	require.NoError(t, db.Create(&tag).Error)
	return tag.ID
}

// seedTestSectionWithTopic creates a section whose ClusterTagIDs reference the
// given tag ids (fact fingerprint source). Pass no tagIDs for a NULL
// cluster_tag_ids column; pass JSON{} (empty) via seedEmptyTagIDs for an empty
// JSON array.
func seedTestSectionWithTopic(t *testing.T, db *gorm.DB, reportID uint, label string, topicID uint, tagIDs ...uint) uint {
	t.Helper()
	section := DailyReportSection{
		ReportID:          reportID,
		ClusterLabel:      label,
		ArticleCount:      1,
		Embedding:         FloatsToPgVector([]float64{0}),
		PersistentTopicID: &topicID,
	}
	if tagIDs != nil {
		raw, err := json.Marshal(tagIDs)
		require.NoError(t, err)
		section.ClusterTagIDs = JSON(raw)
	}
	require.NoError(t, db.Create(&section).Error)
	return section.ID
}

// TestListTopicRecentBriefs_TagLabelInjection covers spec scenario
// 「candidate 话题近期内容注入」/「L2 候选预筛注入」的 repo 侧：注入内容 SHALL 为
// section 当天实际 tag 标签（cluster_tag_ids 解析出的 topic_tags.label），
// 而非被话题 label 硬覆盖的 cluster_label（零信息复读）。
func TestListTopicRecentBriefs_TagLabelInjection(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "AI 编程工具平台化竞争", now)
	tagA := seedTestTopicTag(t, db, "Codex 推出第三方模型接入", "active")
	tagB := seedTestTopicTag(t, db, "GitHub Copilot X 发布新功能", "active")

	reportID := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	sectionID := seedTestSectionWithTopic(t, db, reportID, "开发者 Agent 平台化", topicID, tagA, tagB)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1)
	require.Contains(t, briefs, topicID)

	items := briefs[topicID]
	require.Len(t, items, 1)
	assert.Equal(t, sectionID, items[0].SectionID)
	// Fact fingerprint: actual tag labels, in cluster_tag_ids order.
	assert.Equal(t, []string{"Codex 推出第三方模型接入", "GitHub Copilot X 发布新功能"}, items[0].TagLabels)
	// The frozen cluster_label SHALL NOT be the injected content anymore.
	assert.NotContains(t, items[0].TagLabels, "开发者 Agent 平台化")
}

func TestListTopicRecentBriefs_SevenDayWindow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "以黎冲突升级", now)
	tag1 := seedTestTopicTag(t, db, "真主党越境打击", "active")
	tag2 := seedTestTopicTag(t, db, "以军空袭黎南部", "active")

	// Section within 7 days
	r1 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -3))
	s1 := seedTestSectionWithTopic(t, db, r1, "真主党越境打击", topicID, tag1)

	// Section outside 7 days (10 days ago)
	r2 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -10))
	_ = seedTestSectionWithTopic(t, db, r2, "以军空袭黎南部", topicID, tag2)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1)

	// Only the 3-day-ago section should be present (7-day window)
	items := briefs[topicID]
	assert.Len(t, items, 1)
	assert.Equal(t, s1, items[0].SectionID)
	assert.Equal(t, []string{"真主党越境打击"}, items[0].TagLabels)
}

func TestListTopicRecentBriefs_PerTopicLimit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "AI 算力生态", now)
	tagID := seedTestTopicTag(t, db, "算力扩产", "active")

	// Create 6 sections (limit is 5), descending by date; keep their ids so the
	// per-topic trim (newest-first) can be asserted by SectionID.
	sectionIDs := make([]uint, 0, 6)
	for i := 0; i < 6; i++ {
		r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -(i+1)))
		s := seedTestSectionWithTopic(t, db, r, fmt.Sprintf("section-%d", i+1), topicID, tagID)
		sectionIDs = append(sectionIDs, s)
	}

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1)

	// Should be truncated to 5 newest sections (section-1..5, newest first).
	items := briefs[topicID]
	assert.Len(t, items, 5, "per-topic limit = 5")
	gotIDs := make([]uint, 0, 5)
	for _, item := range items {
		gotIDs = append(gotIDs, item.SectionID)
	}
	assert.Equal(t, sectionIDs[:5], gotIDs, "newest 5 sections kept, newest first")
}

// TestListTopicRecentBriefs_PerSectionTagCap asserts the per-section tag-label
// cap: a section whose cluster_tag_ids lists 7 tags yields at most 5 labels,
// preserving the cluster's own array order.
func TestListTopicRecentBriefs_PerSectionTagCap(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "大模型监管与安全", now)
	tagIDs := make([]uint, 0, 7)
	for i := 0; i < 7; i++ {
		tagIDs = append(tagIDs, seedTestTopicTag(t, db, fmt.Sprintf("tag-%d", i+1), "active"))
	}
	r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	seedTestSectionWithTopic(t, db, r, "many-tags", topicID, tagIDs...)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs[topicID], 1)
	assert.Len(t, briefs[topicID][0].TagLabels, 5, "per-section tag cap = 5")
	assert.Equal(t, []string{"tag-1", "tag-2", "tag-3", "tag-4", "tag-5"}, briefs[topicID][0].TagLabels,
		"cap keeps the cluster's own tag order")
}

// TestListTopicRecentBriefs_MergedDisabledTagsFiltered asserts that
// merged/disabled tags (topic_tags.status != 'active') are filtered out of the
// injected labels while the remaining active labels survive in order.
func TestListTopicRecentBriefs_MergedDisabledTagsFiltered(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "中东局势", now)
	tagA := seedTestTopicTag(t, db, "active-tag", "active")
	tagM := seedTestTopicTag(t, db, "merged-tag", "merged")
	r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	// Array order: A, M — M filtered, A kept.
	seedTestSectionWithTopic(t, db, r, "mixed", topicID, tagA, tagM)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs[topicID], 1)
	assert.Equal(t, []string{"active-tag"}, briefs[topicID][0].TagLabels,
		"merged/disabled tag labels dropped from injection")
}

// TestListTopicRecentBriefs_CandidateIncluded covers spec scenario
// 「candidate 话题近期内容注入」（candidate-topic-l2-gate）：candidate 话题现流经
// L2 裁决，briefs 注入范围 SHALL 覆盖 active 与 candidate 两类。
func TestListTopicRecentBriefs_CandidateIncluded(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	candidateID := seedTestCandidateTopic(t, db, boardID, "候选话题", now)
	tagID := seedTestTopicTag(t, db, "candidate-section-tag", "active")

	reportID := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	sectionID := seedTestSectionWithTopic(t, db, reportID, "candidate-section", candidateID, tagID)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	require.Len(t, briefs, 1, "candidate topic included in briefs")
	require.Contains(t, briefs, candidateID)
	require.Len(t, briefs[candidateID], 1)
	assert.Equal(t, sectionID, briefs[candidateID][0].SectionID)
	assert.Equal(t, []string{"candidate-section-tag"}, briefs[candidateID][0].TagLabels)
}

// TestListTopicRecentBriefs_ExcludesTodaySections asserts the same-day
// exclusion: sections from today's report SHALL NOT be injected as "recent
// content" — otherwise an earlier run of today (or a same-day rerun) that
// mis-attached tags would self-corroborate as briefs evidence
// （卡里巴夫同日重跑自证回路根因）。次日运行时昨日 section 才作为证据。
func TestListTopicRecentBriefs_ExcludesTodaySections(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicID := seedTestActiveTopic(t, db, boardID, "观察期话题", now)
	todayTag := seedTestTopicTag(t, db, "今日误挂 tag", "active")
	ydayTag := seedTestTopicTag(t, db, "昨日事实 tag", "active")

	yesterday := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	ydaySection := seedTestSectionWithTopic(t, db, yesterday, "昨日 section", topicID, ydayTag)
	today := seedTestReport(t, db, boardID, now)
	seedTestSectionWithTopic(t, db, today, "今日 section", topicID, todayTag)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	require.Contains(t, briefs, topicID)
	require.Len(t, briefs[topicID], 1, "only yesterday's section is injected")
	assert.Equal(t, ydaySection, briefs[topicID][0].SectionID)
	assert.Equal(t, []string{"昨日事实 tag"}, briefs[topicID][0].TagLabels,
		"today's mis-attached tag must not self-corroborate")
}

func TestListTopicRecentBriefs_NoTopicsNil(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	assert.Nil(t, briefs)
}

func TestListTopicRecentBriefs_MultipleTopics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())

	topicA := seedTestActiveTopic(t, db, boardID, "Topic A", now)
	topicB := seedTestActiveTopic(t, db, boardID, "Topic B", now)
	tagA := seedTestTopicTag(t, db, "tag-a", "active")
	tagB := seedTestTopicTag(t, db, "tag-b", "active")

	r := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	seedTestSectionWithTopic(t, db, r, "Section A", topicA, tagA)
	seedTestSectionWithTopic(t, db, r, "Section B", topicB, tagB)

	briefs, err := repo.ListTopicRecentBriefs(boardID, 7, 5)
	require.NoError(t, err)
	assert.Len(t, briefs, 2)
	assert.Contains(t, briefs, topicA)
	assert.Contains(t, briefs, topicB)
	assert.Len(t, briefs[topicA], 1)
	assert.Len(t, briefs[topicB], 1)
	assert.Equal(t, []string{"tag-a"}, briefs[topicA][0].TagLabels)
	assert.Equal(t, []string{"tag-b"}, briefs[topicB][0].TagLabels)
}

func TestListReports_AttachesUniqueActiveWatchSummaries(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := NormalizeReportDate(time.Now())
	newerReportID := seedTestReport(t, db, boardID, now)
	olderReportID := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	active, err := repo.CreateWatch(CreateWatchInput{SemanticBoardID: boardID, Label: "ASML", Type: WatchTypeKeyword})
	require.NoError(t, err)
	paused, err := repo.CreateWatch(CreateWatchInput{SemanticBoardID: boardID, Label: "paused watch", Type: WatchTypeLabel})
	require.NoError(t, err)
	pausedStatus := WatchStatusPaused
	_, err = repo.UpdateWatch(paused.ID, nil, nil, &pausedStatus)
	require.NoError(t, err)
	deleted, err := repo.CreateWatch(CreateWatchInput{SemanticBoardID: boardID, Label: "deleted watch", Type: WatchTypeLabel})
	require.NoError(t, err)

	for _, sectionID := range []uint{101, 102} {
		require.NoError(t, db.Create(&TopicWatchHit{
			WatchID: active.ID, SectionID: sectionID, ReportID: newerReportID,
			PeriodDate: now, Reason: "active hit",
		}).Error)
	}
	require.NoError(t, db.Create(&TopicWatchHit{
		WatchID: paused.ID, SectionID: 103, ReportID: newerReportID,
		PeriodDate: now, Reason: "paused hit",
	}).Error)
	require.NoError(t, db.Create(&TopicWatchHit{
		WatchID: deleted.ID, SectionID: 104, ReportID: olderReportID,
		PeriodDate: now.AddDate(0, 0, -1), Reason: "deleted hit",
	}).Error)
	require.NoError(t, repo.DeleteWatch(deleted.ID))

	items, err := repo.ListReports(boardID, 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, newerReportID, items[0].ID)
	assert.Equal(t, olderReportID, items[1].ID)
	require.Len(t, items[0].ActiveWatchSummaries, 1)
	assert.Equal(t, ActiveWatchSummary{WatchID: active.ID, Label: "ASML", Type: WatchTypeKeyword}, items[0].ActiveWatchSummaries[0])
	assert.Empty(t, items[1].ActiveWatchSummaries)
}
