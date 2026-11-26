package repository

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// 用例来源：openspec/changes/add-ai-generated-article-detection tasks 4.6 +
// specs/daily-report-system「日报线索文章携带疑似 AI 标记并排序后置」。

func setupThreadProjectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:report-thread-projection-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&BoardDailyReport{},
		&DailyReportSection{},
		&DailyReportThread{},
	))
	return db
}

// TestGetReportByID_AttachesThreadArticleDetectionFields 覆盖 task 4.6：日报读路径
// 的线索文章投影带出判定字段（未判定给 unknown，不省略字段、不出现占位值），
// 投影成员与 related_article_ids 完全一致（红线 45：不增删成员）。
func TestGetReportByID_AttachesThreadArticleDetectionFields(t *testing.T) {
	db := setupThreadProjectionTestDB(t)
	repo := NewTopicGraphRepository(db)

	feed := models.Feed{Title: "projection-feed", URL: "https://example.com/projection-feed"}
	require.NoError(t, db.Create(&feed).Error)

	pub := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	suspect := models.Article{FeedID: feed.ID, Title: "疑似稿件", Link: "https://example.com/p/suspect", PubDate: &pub}
	suspect.AIDetectionStatus = models.AIDetectionStatusSuspect
	suspect.AIDetectionScore = 0.88
	suspect.AIDetectionSource = models.AIDetectionSourceLLM
	require.NoError(t, db.Create(&suspect).Error)

	unknown := models.Article{FeedID: feed.ID, Title: "未判定稿件", Link: "https://example.com/p/unknown", PubDate: &pub}
	require.NoError(t, db.Create(&unknown).Error)

	report := BoardDailyReport{
		SemanticBoardID: 77,
		PeriodDate:      time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		Title:           "日报 2026-11-20",
		Highlights:      JSON("[]"),
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	section := DailyReportSection{ReportID: report.ID, ClusterIndex: 0, ClusterLabel: "线索分区", ClusterTagIDs: JSON("[]")}
	require.NoError(t, db.Create(&section).Error)

	relatedIDs, err := json.Marshal([]uint{unknown.ID, suspect.ID})
	require.NoError(t, err)
	require.NoError(t, db.Create(&DailyReportThread{
		ReportID:          report.ID,
		SectionID:         section.ID,
		Title:             "线索",
		TagIDs:            JSON("[]"),
		RelatedArticleIDs: JSON(relatedIDs),
		DisplayTier:       ThreadDisplayTierBody,
	}).Error)

	got, err := repo.GetReportByID(report.ID)
	require.NoError(t, err)
	require.Len(t, got.Sections, 1)
	require.Len(t, got.Sections[0].Threads, 1)
	thread := got.Sections[0].Threads[0]

	// 投影成员与顺序跟 related_article_ids 一致，条数就是「N 篇」口径。
	require.Equal(t, jsonUintIDs(thread.RelatedArticleIDs), projectionIDs(thread.Articles))
	require.Len(t, thread.Articles, 2)

	require.Equal(t, unknown.ID, thread.Articles[0].ID)
	require.Equal(t, "未判定稿件", thread.Articles[0].Title)
	require.Equal(t, models.AIDetectionStatusUnknown, thread.Articles[0].AIDetectionStatus)

	require.Equal(t, suspect.ID, thread.Articles[1].ID)
	require.Equal(t, models.AIDetectionStatusSuspect, thread.Articles[1].AIDetectionStatus)
	require.InDelta(t, 0.88, thread.Articles[1].AIDetectionScore, 1e-9)
}

// TestGetReportByID_ThreadProjectionKeepsDanglingID 覆盖越界引用：库里查不到的
// article id 仍保留投影项（判定 unknown），不静默丢成员。
func TestGetReportByID_ThreadProjectionKeepsDanglingID(t *testing.T) {
	db := setupThreadProjectionTestDB(t)
	repo := NewTopicGraphRepository(db)

	pub := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	known := models.Article{FeedID: 1, Title: "在库稿件", Link: "https://example.com/p/known", PubDate: &pub}
	require.NoError(t, db.Create(&known).Error)

	report := BoardDailyReport{
		SemanticBoardID: 78,
		PeriodDate:      time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		Title:           "日报 2026-11-20",
		Highlights:      JSON("[]"),
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	section := DailyReportSection{ReportID: report.ID, ClusterIndex: 0, ClusterLabel: "越界分区", ClusterTagIDs: JSON("[]")}
	require.NoError(t, db.Create(&section).Error)

	dangling := uint(987654)
	relatedIDs, err := json.Marshal([]uint{dangling, known.ID})
	require.NoError(t, err)
	require.NoError(t, db.Create(&DailyReportThread{
		ReportID:          report.ID,
		SectionID:         section.ID,
		Title:             "越界线索",
		TagIDs:            JSON("[]"),
		RelatedArticleIDs: JSON(relatedIDs),
		DisplayTier:       ThreadDisplayTierBody,
	}).Error)

	got, err := repo.GetReportByID(report.ID)
	require.NoError(t, err)
	require.Len(t, got.Sections[0].Threads, 1)
	thread := got.Sections[0].Threads[0]
	require.Equal(t, []uint{dangling, known.ID}, projectionIDs(thread.Articles))
	require.Equal(t, models.AIDetectionStatusUnknown, thread.Articles[0].AIDetectionStatus)
	require.Equal(t, "在库稿件", thread.Articles[1].Title)
}

func projectionIDs(refs []ThreadArticleRef) []uint {
	ids := make([]uint, len(refs))
	for i, ref := range refs {
		ids[i] = ref.ID
	}
	return ids
}
