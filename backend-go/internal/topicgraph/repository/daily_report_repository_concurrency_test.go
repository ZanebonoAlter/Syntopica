package repository

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

// ── fix-daily-report-cross-process-gate 2.2：SaveReport 并发落库收敛 ──
//
// 契约锚：change specs/daily-report-system/spec.md「日报数据模型」（MODIFIED）
// Scenario「并发落库同板同日不炸唯一约束」——并发写同 board 同日 SHALL 通过
// (board, date) 事务级 advisory lock 串行化收敛为一条记录（后写覆盖），
// SHALL NOT 抛出 idx_board_daily_reports_board_period 唯一约束冲突。
// （test-cases.md 故事 B1/B2；升级前该场景后写者撞 23505 —— 2026-10-23 事故形态。）

func buildConcurrentReport(boardID uint, date time.Time, title, clusterLabel string) (*BoardDailyReport, []DailyReportSection) {
	report := &BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      date,
		Title:           title,
		Summary:         "concurrency test",
		Status:          "completed",
	}
	// One section WITHOUT embedding → the Omit("embedding") insert path; no
	// vector-dimension alignment needed. The distinct cluster label ties the
	// surviving row to its writer for the last-writer-wins assertion.
	sections := []DailyReportSection{{
		ClusterIndex:  0,
		ClusterLabel:  clusterLabel,
		ClusterTagIDs: []byte("[]"),
		ArticleCount:  1,
	}}
	return report, sections
}

// B1: two concurrent SaveReport calls for the SAME board+date both succeed;
// exactly one row survives; its content belongs to whichever writer committed
// last; no 23505 unique-constraint error escapes.
func TestSaveReportConcurrentSameBoardDate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	date := NormalizeReportDate(time.Date(2026, 5, 25, 0, 0, 0, 0, time.Local))

	writers := []struct {
		title   string
		cluster string
	}{
		{"writer-one", "cluster-one"},
		{"writer-two", "cluster-two"},
	}

	errs := make([]error, len(writers))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			report, sections := buildConcurrentReport(boardID, date, writers[i].title, writers[i].cluster)
			errs[i] = repo.SaveReport(report, sections, nil)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "writer %d: concurrent SaveReport must not fail (no duplicate key)", i)
	}

	var reports []BoardDailyReport
	require.NoError(t, db.Where("semantic_board_id = ?", boardID).Find(&reports).Error)
	assert.Len(t, reports, 1, "same board+date must converge to exactly one report row")
	if len(reports) != 1 {
		return
	}
	// Last-writer-wins is WRITER-consistent: the surviving title AND its
	// sections must come from the SAME writer (review L-4 — a lock failure
	// could otherwise interleave them undetected).
	survivingWriter := -1
	for i := range writers {
		if reports[0].Title == writers[i].title {
			survivingWriter = i
			break
		}
	}
	require.GreaterOrEqualf(t, survivingWriter, 0,
		"surviving title %q must belong to one of the writers", reports[0].Title)

	// Sections converge to the surviving writer's set (old rows deleted on
	// the update path — full-replace semantics, not an append).
	var sections []DailyReportSection
	require.NoError(t, db.
		Table("daily_report_sections s").
		Select("s.*").
		Joins("JOIN board_daily_reports r ON r.id = s.report_id").
		Where("r.semantic_board_id = ? AND r.period_date = ?", boardID, date.Format("2006-01-02")).
		Find(&sections).Error)
	assert.Len(t, sections, 1, "sections must be fully replaced, not appended")
	if len(sections) == 1 {
		assert.Equalf(t, writers[survivingWriter].cluster, sections[0].ClusterLabel,
			"surviving section label must come from the SAME writer as the title")
	}
}

// B2: concurrent SaveReport for DIFFERENT boards on the same date never
// contend (the xact lock is (board, date)-scoped) — both succeed.
func TestSaveReportConcurrentDifferentBoards(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardA := seedTestBoard(t, db)
	boardB := seedTestBoardNamed(t, db, "test-board-b")
	date := NormalizeReportDate(time.Date(2026, 5, 26, 0, 0, 0, 0, time.Local))

	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, boardID := range []uint{boardA, boardB} {
		wg.Add(1)
		go func(i int, boardID uint) {
			defer wg.Done()
			<-start
			report, sections := buildConcurrentReport(boardID, date, "board-writer", "board-cluster")
			errs[i] = repo.SaveReport(report, sections, nil)
		}(i, boardID)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "board writer %d: different boards must never contend", i)
	}

	var count int64
	require.NoError(t, db.Model(&BoardDailyReport{}).
		Where("period_date = ?", date.Format("2006-01-02")).
		Count(&count).Error)
	assert.EqualValues(t, 2, count, "both boards keep their own report rows")
}
