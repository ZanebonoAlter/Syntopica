package repository

import (
	"testing"
	"time"
)

func TestRadarAxisEndsAtAnchor(t *testing.T) {
	axis := radarAxis("2026-10-24")
	if len(axis) != LaneDynamicsDefaultDays {
		t.Fatalf("axis len=%d", len(axis))
	}
	if axis[len(axis)-1] != "2026-10-24" {
		t.Fatalf("axis end=%q", axis[len(axis)-1])
	}
	if axis[0] != "2026-10-11" {
		t.Fatalf("axis start=%q", axis[0])
	}
}

func TestFinalizeRadarSortsByAttentionAndKeepsZeroRows(t *testing.T) {
	out := finalizeRadar(&RadarOverview{Boards: []RadarBoardRow{
		{BoardID: 3, Attention: 0, ReportDate: "2026-10-24"},
		{BoardID: 1, Attention: 30, ReportDate: "2026-10-24"},
		{BoardID: 2, Attention: 12, ReportDate: "2026-10-23"},
	}})
	if out.Date != "2026-10-24" {
		t.Fatalf("date=%q", out.Date)
	}
	got := []uint{out.Boards[0].BoardID, out.Boards[1].BoardID, out.Boards[2].BoardID}
	want := []uint{1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order=%v want=%v", got, want)
		}
	}
}

// tasks 9.4：所有版块均无日报时顶层 date 回退今天（不冒空串），
// 雷达报头不会拿到空日期。
func TestFinalizeRadarDateFallsBackToToday(t *testing.T) {
	out := finalizeRadar(&RadarOverview{Boards: []RadarBoardRow{
		{BoardID: 1, Attention: 0},
	}})
	today := NormalizeReportDate(time.Now()).Format("2006-01-02")
	if out.Date != today {
		t.Fatalf("date=%q want=%q", out.Date, today)
	}
}
