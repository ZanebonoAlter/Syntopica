package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

// ── CreateManualTopic / GetComposeCandidates integration tests (PostgreSQL) ──
// These tests run against a testcontainer PostgreSQL DB (golden schema) via
// testutil.SetupTestDB. The CreateManualTopic happy-path — including the
// RebuildBoardRelations call that uses PG-specific SQL (period_date::date) — is
// covered by TestManualTopic_CreateAndReassign in
// daily_report_topic_integration_test.go. The cases here cover input
// validation, embedding usability, and the compose-candidate query.
//
// Pure-function tests (aggregateEmbeddings / detectOutliers / FloatsToPgVector)
// live in daily_report_manual_topic_unit_test.go and run under `go test -short`.

// setupManualTopicTestDB provisions a testcontainer PostgreSQL DB (golden
// schema) and returns a fresh repository. The production AutoMigrate run inside
// testutil.SetupTestDB already creates every table these tests touch
// (SemanticLabel / BoardDailyReport / DailyReportSection / BoardPersistentTopic
// / SectionRelation), so no manual AutoMigrate is needed. The package-global
// Repo is set because CreateManualTopic → RebuildBoardRelations reaches the
// package global via tx helpers (same convention as the other PG integration
// tests in this package).
func setupManualTopicTestDB(t *testing.T) *TopicGraphRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	return repo
}

func TestCreateManualTopic_NoSections(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	_, skipped, err := repo.CreateManualTopic(0, "test", nil)
	assert.Error(t, err)
	assert.Nil(t, skipped)
	assert.Contains(t, err.Error(), "no sections")
}

func TestCreateManualTopic_EmptySectionIDs(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	_, skipped, err := repo.CreateManualTopic(0, "test", []uint{})
	assert.Error(t, err)
	assert.Nil(t, skipped)
	assert.Contains(t, err.Error(), "no sections")
}

func TestCreateManualTopic_NoUsableEmbeddings(t *testing.T) {
	repo := setupManualTopicTestDB(t)

	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	// A section with no usable embedding. On PostgreSQL the vector column
	// rejects the empty string (SQLSTATE 22P02), so a "no embedding" section is
	// represented as NULL — the production-realistic state, and what a
	// non-pointer string column scans back to as "" (which repoParsePgVector
	// then rejects). Use Omit so embedding is NULL rather than "".
	sec := DailyReportSection{ReportID: reportID, ClusterLabel: "no-emb", ArticleCount: 1}
	require.NoError(t, repo.db.Omit("embedding").Create(&sec).Error)

	_, skipped, err := repo.CreateManualTopic(boardID, "test", []uint{sec.ID})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usable vector", "must reject when no section has a usable embedding")
	assert.Len(t, skipped, 1, "section without embedding must be skipped")
}

// ── GetComposeCandidates PostgreSQL tests ──────────────────────────────────

func TestGetComposeCandidates_ParsesEmbeddingsAndExcludesEmpty(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	today := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reportID := seedTestReport(t, repo.db, boardID, today)

	// Three sections: two with usable embeddings, one with no embedding. On
	// PostgreSQL the vector column rejects "" (SQLSTATE 22P02), so the
	// "no embedding" section is seeded as NULL (Omit); GetComposeCandidates'
	// `embedding IS NOT NULL` filter then excludes it, mirroring how SQLite
	// excluded it via the Go-level repoParsePgVector skip.
	s1 := DailyReportSection{ReportID: reportID, ClusterLabel: "s1", Embedding: vecStr(0.1, 0.2, 0.3), ArticleCount: 1}
	s2 := DailyReportSection{ReportID: reportID, ClusterLabel: "s2", Embedding: vecStr(0.4, 0.5, 0.6), ArticleCount: 1}
	s3 := DailyReportSection{ReportID: reportID, ClusterLabel: "no-emb", ArticleCount: 1}
	require.NoError(t, repo.db.Create(&s1).Error)
	require.NoError(t, repo.db.Create(&s2).Error)
	require.NoError(t, repo.db.Omit("embedding").Create(&s3).Error)

	resp, err := repo.GetComposeCandidates(ComposeCandidatesQuery{BoardID: boardID, Days: 14})
	require.NoError(t, err)
	// Default MatchThreshold from DefaultPersistentTopicConfig (no ai_settings row).
	assert.InDelta(t, 0.30, resp.MatchThreshold, 1e-9)
	// Only the two sections with a usable embedding are returned (empty excluded).
	require.Len(t, resp.Sections, 2)
	assert.Equal(t, "s1", resp.Sections[0].ClusterLabel)
	assert.Len(t, resp.Sections[0].Embedding, 3)
	assert.InDeltaSlice(t, []float64{0.1, 0.2, 0.3}, resp.Sections[0].Embedding, 1e-9)
	assert.Equal(t, "s2", resp.Sections[1].ClusterLabel)
	// report_id 随 section 返回（编排态查线索复用，task 11.1）。
	assert.Equal(t, reportID, resp.Sections[0].ReportID)
	assert.Equal(t, reportID, resp.Sections[1].ReportID)
}

func TestGetComposeCandidates_AttachesTopicBrief(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	today := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reportID := seedTestReport(t, repo.db, boardID, today)

	// A topic the section is currently assigned to. board_persistent_topics.embedding
	// is a non-nullable vector column, so the topic MUST carry a valid embedding
	// (production always sets it before CreateTopic; SQLite tolerated the zero value).
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "中东局势", Status: TopicStatusActive, Source: TopicSourceAuto,
		Embedding:     vecStr(0.2, 0.1),
		FirstSeenDate: today, LastSeenDate: today,
	}
	require.NoError(t, repo.CreateTopic(repo.db, &topic))

	tid := topic.ID
	sec := DailyReportSection{
		ReportID: reportID, ClusterLabel: "霍尔木兹", Embedding: vecStr(0.2, 0.1),
		PersistentTopicID: &tid, TopicMatchConfidence: TopicConfAnchorHit, ArticleCount: 1,
	}
	require.NoError(t, repo.db.Create(&sec).Error)

	resp, err := repo.GetComposeCandidates(ComposeCandidatesQuery{BoardID: boardID, Days: 14})
	require.NoError(t, err)
	require.Len(t, resp.Sections, 1)
	got := resp.Sections[0]
	assert.Equal(t, sec.ID, got.ID)
	require.NotNil(t, got.PersistentTopicID)
	assert.Equal(t, tid, *got.PersistentTopicID)
	assert.Equal(t, TopicConfAnchorHit, got.TopicMatchConfidence)
	// Topic brief attached (label carried for the candidate pool display).
	require.NotNil(t, got.PersistentTopic)
	assert.Equal(t, "中东局势", got.PersistentTopic.Label)
}

func TestGetComposeCandidates_DaysWindowOutOfScope(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	// Latest report is recent; an older report falls outside the window.
	latest := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	recentReport := seedTestReport(t, repo.db, boardID, latest)
	oldReport := seedTestReport(t, repo.db, boardID, old)

	sRecent := DailyReportSection{ReportID: recentReport, ClusterLabel: "recent", Embedding: vecStr(0.1, 0.2), ArticleCount: 1}
	sOld := DailyReportSection{ReportID: oldReport, ClusterLabel: "old", Embedding: vecStr(0.9, 0.8), ArticleCount: 1}
	require.NoError(t, repo.db.Create(&sRecent).Error)
	require.NoError(t, repo.db.Create(&sOld).Error)

	// 14-day window anchored to the latest report (06-29) → the 2025 section is out.
	resp, err := repo.GetComposeCandidates(ComposeCandidatesQuery{BoardID: boardID, Days: 14})
	require.NoError(t, err)
	require.Len(t, resp.Sections, 1)
	assert.Equal(t, "recent", resp.Sections[0].ClusterLabel)
}

func TestGetComposeCandidates_EmptyBoard(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	// No reports at all → graceful fallback (zero latest → cutoff far in past →
	// returns empty slice, never errors).
	resp, err := repo.GetComposeCandidates(ComposeCandidatesQuery{BoardID: boardID, Days: 14})
	require.NoError(t, err)
	assert.Empty(t, resp.Sections)
	assert.InDelta(t, 0.30, resp.MatchThreshold, 1e-9)
}

// TestComposeCandidatesMemberSetsUseSameVectorFilter（fix-lane-metric-provenance
// 1.5/1.6）：窗口内成员与窗口外成员用**同一**向量可用性判据——两侧都要求
// embedding 可用；因此「窗口内成员数 + 更早成员数 = 全部成员数」恒成立，
// 且「窗口 N」恒为「全部 M」的真子集。
func TestComposeCandidatesMemberSetsUseSameVectorFilter(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	latest := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	old := latest.AddDate(0, 0, -20)
	latestReport := seedTestReport(t, repo.db, boardID, latest)
	oldReport := seedTestReport(t, repo.db, boardID, old)

	topic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "成员对称话题", Status: TopicStatusActive, Source: TopicSourceAuto,
		Embedding:     vecStr(0.2, 0.1),
		FirstSeenDate: old, LastSeenDate: latest,
	}
	require.NoError(t, repo.CreateTopic(repo.db, &topic))
	tid := topic.ID

	// 窗口内 3 个成员（均有向量）。
	inWindow := make([]uint, 0, 3)
	for i := 0; i < 3; i++ {
		sec := DailyReportSection{
			ReportID: latestReport, ClusterLabel: "窗口内-" + string(rune('A'+i)),
			Embedding: vecStr(0.1, 0.2), PersistentTopicID: &tid,
			TopicMatchConfidence: TopicConfAnchorHit, ArticleCount: 1,
		}
		require.NoError(t, repo.db.Create(&sec).Error)
		inWindow = append(inWindow, sec.ID)
	}
	// 窗口外 2 个成员：1 个有向量、1 个 embedding 为 NULL。
	withVector := DailyReportSection{
		ReportID: oldReport, ClusterLabel: "窗口外-有向量",
		Embedding: vecStr(0.3, 0.4), PersistentTopicID: &tid,
		TopicMatchConfidence: TopicConfAnchorHit, ArticleCount: 1,
	}
	require.NoError(t, repo.db.Create(&withVector).Error)
	withoutVector := DailyReportSection{
		ReportID: oldReport, ClusterLabel: "窗口外-无向量",
		PersistentTopicID: &tid, TopicMatchConfidence: TopicConfAnchorHit, ArticleCount: 1,
	}
	require.NoError(t, repo.db.Omit("embedding").Create(&withoutVector).Error)

	resp, err := repo.GetComposeCandidates(ComposeCandidatesQuery{
		BoardID: boardID, Days: 14, AllMembersTopicID: &tid,
	})
	require.NoError(t, err)

	// 窗口内：3 个成员都在 sections 里。
	windowMembers := 0
	for _, sec := range resp.Sections {
		if sec.PersistentTopicID != nil && *sec.PersistentTopicID == tid {
			windowMembers++
		}
	}
	assert.Equal(t, 3, windowMembers, "窗口内成员数")

	// 窗口外：只有有向量的那个进 early_members（无向量的与 sections 侧同判据被排除）。
	require.NotNil(t, resp.EarlyMembers, "全量成员模式下 early_members 必须存在")
	earlyMembers := *resp.EarlyMembers
	require.Len(t, earlyMembers, 1, "无可用向量的窗口外成员不入集")
	assert.Equal(t, withVector.ID, earlyMembers[0].ID)
	assert.NotEqual(t, withoutVector.ID, earlyMembers[0].ID)

	// 窗口 N + 更早数 = 全部 M（同一集合的子集关系）。
	allMembers := windowMembers + len(earlyMembers)
	assert.Equal(t, 4, allMembers)
	assert.Less(t, windowMembers, allMembers, "窗口 N 恒为全部 M 的真子集")
}

// TestComposeCandidatesExplicitWindowKeepsMemberTotal（review R-3）：显式 from/to
// 且 to 早于最新 completed 日时，上界之后的本话题成员也必须落在窗口外集合里——
// 否则「窗口 N + 窗口外数」会少于成员总数，M 就不是「全部成员」了。
func TestComposeCandidatesExplicitWindowKeepsMemberTotal(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	latest := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	mid := latest.AddDate(0, 0, -5)
	early := latest.AddDate(0, 0, -20)
	latestReport := seedTestReport(t, repo.db, boardID, latest)
	midReport := seedTestReport(t, repo.db, boardID, mid)
	earlyReport := seedTestReport(t, repo.db, boardID, early)

	topic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "显式窗口话题", Status: TopicStatusActive, Source: TopicSourceAuto,
		Embedding:     vecStr(0.2, 0.1),
		FirstSeenDate: early, LastSeenDate: latest,
	}
	require.NoError(t, repo.CreateTopic(repo.db, &topic))
	tid := topic.ID

	makeMember := func(reportID uint, label string) uint {
		sec := DailyReportSection{
			ReportID: reportID, ClusterLabel: label, Embedding: vecStr(0.1, 0.2),
			PersistentTopicID: &tid, TopicMatchConfidence: TopicConfAnchorHit, ArticleCount: 1,
		}
		require.NoError(t, repo.db.Create(&sec).Error)
		return sec.ID
	}
	_ = makeMember(earlyReport, "窗口外-更早")
	inWindow := makeMember(midReport, "窗口内")
	afterWindow := makeMember(latestReport, "窗口外-更晚")

	// 显式窗口只覆盖 mid 这一天 → 上界之后（latest）的成员属窗口外。
	from, to := mid, mid
	resp, err := repo.GetComposeCandidates(ComposeCandidatesQuery{
		BoardID: boardID, From: &from, To: &to, AllMembersTopicID: &tid,
	})
	require.NoError(t, err)

	windowMembers := 0
	for _, sec := range resp.Sections {
		if sec.PersistentTopicID != nil && *sec.PersistentTopicID == tid {
			windowMembers++
		}
	}
	assert.Equal(t, 1, windowMembers, "显式窗口内只有 mid 那一个成员")
	assert.Equal(t, inWindow != 0, windowMembers == 1)

	require.NotNil(t, resp.EarlyMembers)
	earlyIDs := make(map[uint]bool, len(*resp.EarlyMembers))
	for _, m := range *resp.EarlyMembers {
		earlyIDs[m.ID] = true
	}
	assert.True(t, earlyIDs[afterWindow], "上界之后的成员也必须算窗口外（否则总数不全）")
	assert.Equal(t, 2, len(*resp.EarlyMembers), "窗口外 = 更早 1 + 更晚 1")
	assert.Equal(t, 3, windowMembers+len(*resp.EarlyMembers), "窗口 N + 窗口外数 = 全部成员数")
}

// unify-lane-workbench 2.1 / test-cases E15（同日多桶）：GetComposeCandidates
// 对同一天的多个 section 各自计数（single / composite / mixed / 无 thread），
// 批量回填一次完成、不串桶、不把综合稿计入 clue_count。
func TestGetComposeCandidates_ClueCountSameDayBuckets(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	day := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reportID := seedTestReport(t, repo.db, boardID, day)

	// 同日四桶：mixed / single-only / composite-only / 无 thread。
	mixed := DailyReportSection{ReportID: reportID, ClusterLabel: "mixed", Embedding: vecStr(0.1, 0.2), ArticleCount: 1}
	single := DailyReportSection{ReportID: reportID, ClusterLabel: "single", Embedding: vecStr(0.3, 0.4), ArticleCount: 1}
	compositeOnly := DailyReportSection{ReportID: reportID, ClusterLabel: "composite-only", Embedding: vecStr(0.5, 0.6), ArticleCount: 1}
	noThread := DailyReportSection{ReportID: reportID, ClusterLabel: "no-thread", Embedding: vecStr(0.7, 0.8), ArticleCount: 1}
	for _, s := range []*DailyReportSection{&mixed, &single, &compositeOnly, &noThread} {
		require.NoError(t, repo.db.Create(s).Error)
	}
	seedThread := func(sectionID uint, title, scope string) {
		t.Helper()
		require.NoError(t, repo.db.Create(&DailyReportThread{
			ReportID: reportID, SectionID: sectionID, Title: title, Scope: scope,
			Embedding: FloatsToPgVector([]float64{0}),
		}).Error)
	}
	seedThread(mixed.ID, "线索甲", ThreadScopeSingle)
	seedThread(mixed.ID, "线索乙", ThreadScopeSingle)
	seedThread(mixed.ID, "综合稿", ThreadScopeComposite)
	seedThread(single.ID, "线索丙", ThreadScopeSingle)
	seedThread(compositeOnly.ID, "综合稿二", ThreadScopeComposite)

	resp, err := repo.GetComposeCandidates(ComposeCandidatesQuery{BoardID: boardID, Days: 14})
	require.NoError(t, err)
	byLabel := map[string]int{}
	for _, s := range resp.Sections {
		byLabel[s.ClusterLabel] = s.ClueCount
	}
	assert.Equal(t, 2, byLabel["mixed"], "single+composite 混合桶只计非综合稿")
	assert.Equal(t, 1, byLabel["single"])
	assert.Equal(t, 0, byLabel["composite-only"], "全综合稿桶 = 已知 0")
	assert.Equal(t, 0, byLabel["no-thread"], "无 thread 桶 = 已知 0（不是未知）")
	// 同日四桶互不串数：总数 = 各桶之和，不重复计入他桶。
	assert.Len(t, resp.Sections, 4)
}
