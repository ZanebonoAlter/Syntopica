package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/repository"
)

func TestPercentileNearestRank(t *testing.T) {
	for _, tc := range []struct {
		values  []float64
		p, want float64
	}{
		{nil, .2, 0}, {[]float64{5}, .8, 5},
		{[]float64{1, 2, 3, 4, 5}, .2, 1}, {[]float64{1, 2, 3, 4, 5}, .8, 4},
		{[]float64{1, 2}, .8, 2}, {[]float64{2, 2, 2}, .2, 2},
		{[]float64{1, 9}, 0, 1}, {[]float64{1, 9}, 1, 9},
		{[]float64{1, 9}, -1, 1}, {[]float64{1, 9}, 2, 9},
	} {
		if got := percentile(tc.values, tc.p); got != tc.want {
			t.Errorf("percentile(%v, %v)=%v, want %v", tc.values, tc.p, got, tc.want)
		}
	}
}

func TestCalibrateOptions(t *testing.T) {
	got, err := parseOptions(nil)
	if err != nil || got.Days != 30 || got.Board != 0 {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	got, err = parseOptions([]string{"--days", "2", "--board", "7"})
	if err != nil || got.Days != 2 || got.Board != 7 {
		t.Fatalf("flags: %+v %v", got, err)
	}
	for _, args := range [][]string{{"--days", "0"}, {"--days", "-2"}, {"--board", "-1"}, {"extra"}} {
		if _, err := parseOptions(args); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

func TestCalibrateSamplesReadOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:calibrate-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&repository.BoardDailyReport{}, &repository.BoardStory{}, &repository.StorySignal{}, &models.Article{}); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	latest := time.Date(2026, 11, 5, 0, 0, 0, 0, loc)
	laneID := uint(10)
	for _, bid := range []uint{7, 8} {
		report := repository.BoardDailyReport{SemanticBoardID: bid, PeriodDate: latest}
		story := repository.BoardStory{BoardID: bid, LaneID: &laneID}
		if err := db.Create(&report).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Omit("embedding", "subject_embedding").Create(&story).Error; err != nil {
			t.Fatal(err)
		}
		article := models.Article{ID: bid, Title: "heat evidence", Link: fmt.Sprintf("https://example.test/%d", bid), FeedID: 1}
		if err := db.Create(&article).Error; err != nil {
			t.Fatal(err)
		}
		signal := repository.StorySignal{StoryID: story.ID, ParticipantKey: "feed:1", ArticleID: bid, FeedID: 1, ObservedAt: latest.Add(12 * time.Hour)}
		if err := db.Create(&signal).Error; err != nil {
			t.Fatal(err)
		}
	}
	// SQLite itself rejects any write from this point, not just assertions
	// against selected tables. No real database or AI is involved.
	if err := db.Exec("PRAGMA query_only = ON").Error; err != nil {
		t.Fatal(err)
	}
	samples, err := collectSamples(context.Background(), db, options{Days: 2, Board: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0] <= 0 {
		t.Fatalf("latest day should have heat, preceding day none: %v", samples)
	}
	all, err := collectSamples(context.Background(), db, options{Days: 2})
	if err != nil || !reflect.DeepEqual(all, []float64{samples[0], samples[0]}) {
		t.Fatalf("all boards: %v, %v", all, err)
	}
}
