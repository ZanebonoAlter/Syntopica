package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

func TestBuildWatchHitPromptContainsWatchesAndSections(t *testing.T) {
	watches := []repository.BoardTopicWatch{
		{ID: 1, Label: "美伊会不会真打起来"},
		{ID: 2, Label: "中美关税走向"},
	}
	sections := []repository.DailyReportSection{
		{ID: 101, ClusterLabel: "G7峰会各方表态"},
		{ID: 102, ClusterLabel: "美伊恢复核谈判"},
	}

	prompt := buildWatchHitPrompt(watches, sections)

	// Must contain watch IDs and labels
	assert.Contains(t, prompt, "美伊会不会真打起来")
	assert.Contains(t, prompt, "中美关税走向")
	assert.Contains(t, prompt, "[id:1]")
	assert.Contains(t, prompt, "[id:2]")

	// Must contain section IDs and labels
	assert.Contains(t, prompt, "G7峰会各方表态")
	assert.Contains(t, prompt, "美伊恢复核谈判")
	assert.Contains(t, prompt, "[section_id:101]")
	assert.Contains(t, prompt, "[section_id:102]")

	// Must contain the expected output format hint
	assert.Contains(t, prompt, `"hits"`)
	assert.Contains(t, prompt, `"watch_id"`)
}

func TestBuildWatchHitPrompt_Empty(t *testing.T) {
	prompt := buildWatchHitPrompt(nil, nil)
	assert.Contains(t, prompt, "关注标记")
	assert.Contains(t, prompt, "日报节列表")
}

func TestParseWatchHitResponse_Valid(t *testing.T) {
	content := `{"hits":[
		{"watch_id":1,"section_id":101,"reason":"该节讨论了美伊问题"},
		{"watch_id":2,"section_id":102,"reason":"涉及关税讨论"}
	]}`

	validWatchIDs := map[uint]bool{1: true, 2: true}
	validSectionIDs := map[uint]bool{101: true, 102: true}
	report := &repository.BoardDailyReport{
		ID:         200,
		PeriodDate: time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
	}

	hits, err := parseWatchHitResponse(content, validWatchIDs, validSectionIDs, report)
	require.NoError(t, err)
	require.Len(t, hits, 2)

	assert.Equal(t, uint(1), hits[0].WatchID)
	assert.Equal(t, uint(101), hits[0].SectionID)
	assert.Equal(t, uint(200), hits[0].ReportID)
	assert.Equal(t, "该节讨论了美伊问题", hits[0].Reason)

	assert.Equal(t, uint(2), hits[1].WatchID)
	assert.Equal(t, uint(102), hits[1].SectionID)
}

func TestParseWatchHitResponse_FiltersHallucinatedIDs(t *testing.T) {
	content := `{"hits":[
		{"watch_id":1,"section_id":101,"reason":"valid"},
		{"watch_id":999,"section_id":101,"reason":"hallucinated watch_id"},
		{"watch_id":1,"section_id":999,"reason":"hallucinated section_id"}
	]}`

	validWatchIDs := map[uint]bool{1: true}
	validSectionIDs := map[uint]bool{101: true}
	report := &repository.BoardDailyReport{
		ID:         200,
		PeriodDate: time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
	}

	hits, err := parseWatchHitResponse(content, validWatchIDs, validSectionIDs, report)
	require.NoError(t, err)
	require.Len(t, hits, 1, "only the valid entry should survive")
	assert.Equal(t, uint(1), hits[0].WatchID)
	assert.Equal(t, uint(101), hits[0].SectionID)
}

func TestParseWatchHitResponse_InvalidJSON(t *testing.T) {
	report := &repository.BoardDailyReport{ID: 200, PeriodDate: time.Now()}
	_, err := parseWatchHitResponse("not-json", map[uint]bool{}, map[uint]bool{}, report)
	assert.Error(t, err)
}

func TestParseWatchHitResponse_EmptyHits(t *testing.T) {
	content := `{"hits":[]}`
	report := &repository.BoardDailyReport{ID: 200, PeriodDate: time.Now()}
	hits, err := parseWatchHitResponse(content, map[uint]bool{1: true}, map[uint]bool{101: true}, report)
	require.NoError(t, err)
	assert.Empty(t, hits)
}

// Verify the JSON schema structure matches what the AI is asked to produce.
func TestWatchHitJSONSchemaRoundtrip(t *testing.T) {
	// Simulate what the AI should return
	expected := rawWatchHitResponse{
		Hits: []rawWatchHit{
			{WatchID: 1, SectionID: 100, Reason: "test reason"},
		},
	}
	bytes, err := json.Marshal(expected)
	require.NoError(t, err)

	var parsed rawWatchHitResponse
	require.NoError(t, json.Unmarshal(bytes, &parsed))
	require.Len(t, parsed.Hits, 1)
	assert.Equal(t, uint(1), parsed.Hits[0].WatchID)
	assert.Equal(t, "test reason", parsed.Hits[0].Reason)
}

// ── Dual-track watch evaluation (watch-keyword-and-quickadd) ─────────────────

// countingChat wraps a watchChatFunc and counts invocations, so tests can
// assert the keyword branch makes ZERO AI calls.
type countingChat struct {
	calls int
}

func (cc *countingChat) f(_ context.Context, _ airouter.ChatRequest) (*airouter.ChatResult, error) {
	cc.calls++
	return &airouter.ChatResult{Content: `{"hits":[]}`}, nil
}

// watchTestDB is a lightweight in-memory DB for EvaluateWatchHits dual-track
// tests (watches + hits only; no section/thread tables needed — keyword
// branch reads threads via repository.Repo, seeded via ListWatchSectionTexts*
// queries against the sections/threads tables below).
func watchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&repository.BoardTopicWatch{},
		&repository.TopicWatchHit{},
		&repository.BoardDailyReport{},
		&repository.DailyReportSection{},
		&repository.DailyReportThread{},
	))
	return db
}

func seedWatchReportSectionThread(t *testing.T, db *gorm.DB, boardID uint, threadTitle, threadSummary string) (*repository.BoardDailyReport, *repository.DailyReportSection) {
	t.Helper()
	now := repository.NormalizeReportDate(time.Now())
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      now,
		Title:           "dual-track test report",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	sec := repository.DailyReportSection{
		ReportID:     report.ID,
		ClusterLabel: "section containing keywords",
		Embedding:    repository.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, db.Create(&sec).Error)
	th := repository.DailyReportThread{
		ReportID:  report.ID,
		SectionID: sec.ID,
		Title:     threadTitle,
		Summary:   threadSummary,
	}
	require.NoError(t, db.Create(&th).Error)
	return &report, &sec
}

// keyword watches go through the text branch with ZERO AI calls (chat=0).
func TestEvaluateWatchHits_KeywordTrackZeroAICalls(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	report, sec := seedWatchReportSectionThread(t, db, 1, "ASML 光刻机对华出口", "")

	watch := repository.BoardTopicWatch{
		SemanticBoardID: 1,
		Label:           "ASML|镓锗 出口",
		Type:            repository.WatchTypeKeyword,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&watch).Error)

	chat := &countingChat{}
	err := evaluateWatchHitsWithChat(context.Background(), 1, report, []repository.DailyReportSection{*sec}, chat.f)
	require.NoError(t, err)
	assert.Zero(t, chat.calls, "keyword track MUST NOT call AI")

	var hits []repository.TopicWatchHit
	require.NoError(t, db.Where("watch_id = ?", watch.ID).Find(&hits).Error)
	require.Len(t, hits, 1)
	assert.Equal(t, sec.ID, hits[0].SectionID)
	assert.Equal(t, report.ID, hits[0].ReportID)
	assert.Equal(t, "含关键字『ASML、出口』", hits[0].Reason)
}

// label watches keep the existing AI path; a keyword watch in the same board
// adds text hits without disturbing the AI call count.
func TestEvaluateWatchHits_LabelTrackStillCallsAI(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	report, sec := seedWatchReportSectionThread(t, db, 1, "ASML 光刻机对华出口", "")

	labelWatch := repository.BoardTopicWatch{
		SemanticBoardID: 1,
		Label:           "半导体管制",
		Type:            repository.WatchTypeLabel,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&labelWatch).Error)
	kwWatch := repository.BoardTopicWatch{
		SemanticBoardID: 1,
		Label:           "ASML",
		Type:            repository.WatchTypeKeyword,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&kwWatch).Error)

	chatFn := func(_ context.Context, _ airouter.ChatRequest) (*airouter.ChatResult, error) {
		return &airouter.ChatResult{Content: fmt.Sprintf(`{"hits":[{"watch_id":%d,"section_id":%d,"reason":"AI hit"}]}`, labelWatch.ID, sec.ID)}, nil
	}
	err := evaluateWatchHitsWithChat(context.Background(), 1, report, []repository.DailyReportSection{*sec}, chatFn)
	require.NoError(t, err)

	var labelHits, kwHits []repository.TopicWatchHit
	require.NoError(t, db.Where("watch_id = ?", labelWatch.ID).Find(&labelHits).Error)
	require.NoError(t, db.Where("watch_id = ?", kwWatch.ID).Find(&kwHits).Error)
	assert.Len(t, labelHits, 1, "label track hit written via AI path")
	assert.Len(t, kwHits, 1, "keyword track hit written via text path in the same run")
	assert.Equal(t, "AI hit", labelHits[0].Reason)
	assert.Equal(t, "含关键字『ASML』", kwHits[0].Reason)
}

// paused watches (either type) are excluded from evaluation entirely.
func TestEvaluateWatchHits_PausedWatchesSkipped(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	report, sec := seedWatchReportSectionThread(t, db, 1, "ASML 光刻机", "")

	paused := repository.BoardTopicWatch{
		SemanticBoardID: 1,
		Label:           "ASML",
		Type:            repository.WatchTypeKeyword,
		Status:          repository.WatchStatusPaused,
	}
	require.NoError(t, db.Create(&paused).Error)

	chat := &countingChat{}
	err := evaluateWatchHitsWithChat(context.Background(), 1, report, []repository.DailyReportSection{*sec}, chat.f)
	require.NoError(t, err)
	assert.Zero(t, chat.calls)

	var count int64
	db.Model(&repository.TopicWatchHit{}).Count(&count)
	assert.Zero(t, count, "paused watch must not produce hits")
}

// materialized-track watches (keyword_topic / sentence_topic) are skipped
// entirely: zero AI calls, zero text matches, zero hint rows — while a
// coexisting label watch still goes through the AI path normally.
func TestEvaluateWatchHits_MaterializedTracksSkipped(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	report, sec := seedWatchReportSectionThread(t, db, 1, "DeepSeek Harness 源码解析", "")

	ktWatch := repository.BoardTopicWatch{
		SemanticBoardID: 1,
		Label:           "harness",
		Type:            repository.WatchTypeKeywordTopic,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&ktWatch).Error)
	stWatch := repository.BoardTopicWatch{
		SemanticBoardID: 1,
		Label:           "美伊形势对市场影响",
		Type:            repository.WatchTypeSentenceTopic,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&stWatch).Error)
	labelWatch := repository.BoardTopicWatch{
		SemanticBoardID: 1,
		Label:           "AI Agent",
		Type:            repository.WatchTypeLabel,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&labelWatch).Error)

	chat := &countingChat{}
	err := evaluateWatchHitsWithChat(context.Background(), 1, report, []repository.DailyReportSection{*sec}, chat.f)
	require.NoError(t, err)
	assert.Equal(t, 1, chat.calls, "only the label watch reaches the AI path")

	for _, w := range []repository.BoardTopicWatch{ktWatch, stWatch} {
		var count int64
		db.Model(&repository.TopicWatchHit{}).Where("watch_id = ?", w.ID).Count(&count)
		assert.Zero(t, count, "materialized watch %s must produce no hint rows", w.Type)
	}
	var labelCount int64
	db.Model(&repository.TopicWatchHit{}).Where("watch_id = ?", labelWatch.ID).Count(&labelCount)
	assert.Zero(t, labelCount, "countingChat returns empty hits — label watch stays at zero rows, but its AI call DID happen")
}

// instant match hits historical sections within the window and is idempotent
// with the daily-report-time evaluation (same unique key → one row).
func TestMatchKeywordInstant_HitsAndDedup(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	boardID := uint(1)
	report, sec := seedWatchReportSectionThread(t, db, boardID, "ASML 光刻机对华出口", "")

	watch := repository.BoardTopicWatch{
		SemanticBoardID: boardID,
		Label:           "ASML",
		Type:            repository.WatchTypeKeyword,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&watch).Error)

	n, err := matchKeywordInstantAt(context.Background(), boardID, watch.ID, 14, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// Re-run (idempotency): the unique index dedupes; count stays 1.
	n2, err := matchKeywordInstantAt(context.Background(), boardID, watch.ID, 14, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, n2, "matched count reflects sections, not new rows")

	var hitRows []repository.TopicWatchHit
	require.NoError(t, db.Where("watch_id = ?", watch.ID).Find(&hitRows).Error)
	assert.Len(t, hitRows, 1, "same (watch,section,report) key must occupy exactly one row")

	// Daily-report-time keyword evaluation writes the same key again → still 1.
	chat := &countingChat{}
	require.NoError(t, evaluateWatchHitsWithChat(context.Background(), boardID, report, []repository.DailyReportSection{*sec}, chat.f))
	require.NoError(t, db.Where("watch_id = ?", watch.ID).Find(&hitRows).Error)
	assert.Len(t, hitRows, 1, "instant + daily-report match must dedupe to one row")
	assert.Zero(t, chat.calls)
}

// window boundary: 14-day window includes day-14, excludes day-15 (T1/T3).
func TestMatchKeywordInstant_WindowBoundary(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	boardID := uint(1)
	now := time.Now()

	mkReport := func(daysAgo int, title string) {
		date := repository.NormalizeReportDate(now).AddDate(0, 0, -daysAgo)
		rep := repository.BoardDailyReport{
			SemanticBoardID: boardID,
			PeriodDate:      date,
			Title:           fmt.Sprintf("report %d days ago", daysAgo),
			Status:          "completed",
		}
		require.NoError(t, db.Create(&rep).Error)
		sec := repository.DailyReportSection{
			ReportID:     rep.ID,
			ClusterLabel: title,
			Embedding:    repository.FloatsToPgVector([]float64{0}),
		}
		require.NoError(t, db.Create(&sec).Error)
		require.NoError(t, db.Create(&repository.DailyReportThread{
			ReportID:  rep.ID,
			SectionID: sec.ID,
			Title:     title,
			Summary:   "",
		}).Error)
	}
	mkReport(14, "ASML 十四天前")
	mkReport(15, "ASML 十五天前")

	watch := repository.BoardTopicWatch{
		SemanticBoardID: boardID,
		Label:           "ASML",
		Type:            repository.WatchTypeKeyword,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&watch).Error)

	n, err := matchKeywordInstantAt(context.Background(), boardID, watch.ID, 14, now)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "day-14 section is inside the window; day-15 is not")
}

// empty board (no reports at all): zero hits, no error (T2 / P1 variant).
func TestMatchKeywordInstant_EmptyBoard(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	watch := repository.BoardTopicWatch{
		SemanticBoardID: 999,
		Label:           "ASML",
		Type:            repository.WatchTypeKeyword,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&watch).Error)

	n, err := matchKeywordInstantAt(context.Background(), 999, watch.ID, 14, time.Now())
	require.NoError(t, err)
	assert.Zero(t, n)
}

// label watches never instant-match (design: only keyword supports instant).
func TestMatchKeywordInstant_LabelWatchNeverInstantMatches(t *testing.T) {
	db := watchTestDB(t)
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })

	boardID := uint(1)
	_, _ = seedWatchReportSectionThread(t, db, boardID, "半导体管制 ASML", "")

	labelWatch := repository.BoardTopicWatch{
		SemanticBoardID: boardID,
		Label:           "ASML",
		Type:            repository.WatchTypeLabel,
		Status:          repository.WatchStatusActive,
	}
	require.NoError(t, db.Create(&labelWatch).Error)

	n, err := matchKeywordInstantAt(context.Background(), boardID, labelWatch.ID, 14, time.Now())
	require.NoError(t, err)
	assert.Zero(t, n, "label watch must not instant-match")

	var count int64
	db.Model(&repository.TopicWatchHit{}).Where("watch_id = ?", labelWatch.ID).Count(&count)
	assert.Zero(t, count, "label watch must produce no instant hit rows")
}
