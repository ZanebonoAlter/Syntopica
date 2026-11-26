package board

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type archiveUnitReport struct {
	ID              uint `gorm:"primarykey"`
	SemanticBoardID uint `gorm:"not null;index"`
	PeriodDate      time.Time
}

func (archiveUnitReport) TableName() string { return "board_daily_reports" }

type archiveUnitSection struct {
	ID       uint `gorm:"primarykey"`
	ReportID uint `gorm:"index;not null"`
}

func (archiveUnitSection) TableName() string { return "daily_report_sections" }

type archiveUnitThread struct {
	ID                uint `gorm:"primarykey"`
	SectionID         uint `gorm:"index;not null"`
	RelatedArticleIDs string
}

func (archiveUnitThread) TableName() string { return "daily_report_threads" }

func TestDecodeArchiveRefIDs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ids  []uint
		ok   bool
	}{
		{"array", `[3,1,2]`, []uint{3, 1, 2}, true},
		{"numeric strings", `["7","8"]`, []uint{7, 8}, true},
		{"drops zero and garbage", `[1,0,-2,"x",4]`, []uint{1, 4}, true},
		{"empty array", `[]`, []uint{}, true},
		{"json null scalar", `null`, nil, false},
		{"sql null", ``, nil, false},
		{"object", `{"a":1}`, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ids, ok := decodeArchiveRefIDs([]byte(c.raw))
			require.Equal(t, c.ok, ok)
			require.Equal(t, c.ids, ids)
		})
	}
}

func TestArchiveEvidenceRank(t *testing.T) {
	// downgraded 标记已于 2026-11-12 退役：max_sim 统一档。
	order := []string{"direct_hit", "composite_hit", "hit_rate", "weighted", "max_sim"}
	for i := 1; i < len(order); i++ {
		prev := ArchiveEvidenceRank(order[i-1])
		cur := ArchiveEvidenceRank(order[i])
		require.Greater(t, cur, prev, "%s 应弱于 %s", order[i], order[i-1])
	}
	require.Equal(t, 5, ArchiveEvidenceRank("unknown"))
}

func TestIsWeakEvidence(t *testing.T) {
	require.True(t, IsWeakEvidence("max_sim"))
	require.False(t, IsWeakEvidence("direct_hit"))
	require.False(t, IsWeakEvidence("hit_rate"))
}

func TestSortArticleIDs(t *testing.T) {
	require.Equal(t, []uint{1, 2, 5}, SortArticleIDs(map[uint]struct{}{5: {}, 1: {}, 2: {}}))
	require.Empty(t, SortArticleIDs(nil))
}

func setupArchiveUnitDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:board_archive_unit_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&archiveUnitReport{}, &archiveUnitSection{}, &archiveUnitThread{}))
	return db
}

// seedArchiveReport 建一份日报 + 一个分区 + 一条线索，返回 report id。
func seedArchiveReport(t *testing.T, db *gorm.DB, boardID uint, date time.Time, refs string) uint {
	t.Helper()
	report := archiveUnitReport{SemanticBoardID: boardID, PeriodDate: date}
	require.NoError(t, db.Create(&report).Error)
	section := archiveUnitSection{ReportID: report.ID}
	require.NoError(t, db.Create(&section).Error)
	require.NoError(t, db.Create(&archiveUnitThread{SectionID: section.ID, RelatedArticleIDs: refs}).Error)
	return report.ID
}

func TestLoadReportMembership_ExplicitWindow(t *testing.T) {
	db := setupArchiveUnitDB(t)
	seedArchiveReport(t, db, 1, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), `[1,2]`)
	// 起始日当天必须计入（闭区间）：10-15 的日报引用 77。
	seedArchiveReport(t, db, 1, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), `[77]`)
	seedArchiveReport(t, db, 1, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), `[2,3,"4"]`)
	seedArchiveReport(t, db, 1, time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), `[9]`)
	seedArchiveReport(t, db, 2, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), `[99]`)

	start := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	got, err := LoadReportMembership(context.Background(), db, 1, ReportWindow{Start: &start, End: &end})
	require.NoError(t, err)
	require.Equal(t, map[uint]struct{}{2: {}, 3: {}, 4: {}, 77: {}}, got.ArticleIDs)
	require.Equal(t, 2, got.ReportCount)
	require.Equal(t, 2, got.ThreadCount)
	require.NotNil(t, got.LatestReportDate)
	require.Equal(t, 20, got.LatestReportDate.Day())
}

func TestLoadReportMembership_DefaultLatestSix(t *testing.T) {
	db := setupArchiveUnitDB(t)
	for day := 1; day <= 8; day++ {
		refs := fmt.Sprintf(`[%d]`, day)
		seedArchiveReport(t, db, 1, time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC), refs)
	}
	got, err := LoadReportMembership(context.Background(), db, 1, ReportWindow{})
	require.NoError(t, err)
	require.Equal(t, 6, got.ReportCount)
	// 最近 6 份（10-03..10-08）覆盖 article 3..8，1/2 不在窗口内。
	require.Equal(t, map[uint]struct{}{3: {}, 4: {}, 5: {}, 6: {}, 7: {}, 8: {}}, got.ArticleIDs)
}

func TestLoadReportMembership_BadJSONDegrades(t *testing.T) {
	db := setupArchiveUnitDB(t)
	seedArchiveReport(t, db, 1, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), `null`)
	seedArchiveReport(t, db, 1, time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC), `[5]`)

	got, err := LoadReportMembership(context.Background(), db, 1, ReportWindow{})
	require.NoError(t, err)
	require.Equal(t, 1, got.BadThreads)
	require.Equal(t, map[uint]struct{}{5: {}}, got.ArticleIDs)
	require.Equal(t, 2, got.ReportCount)
}
