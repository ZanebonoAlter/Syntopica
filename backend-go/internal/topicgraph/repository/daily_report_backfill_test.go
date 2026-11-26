package repository

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// ── 日报缺档补填：缺口查询 + 运行留档（add-daily-report-gap-backfill tasks 1.2/1.3）──
//
// 缺口判定必须复制生成 Step1 的**严格**谓词（active + event +
// NOT direction_mismatch + 跨板排他 NOT EXISTS）。用宽松口径会把生成时必然被
// 剔的板块算成缺口，补填反复空跑烧 LLM（design D1）。

// seedGapTag 建一个标签（默认 active event 标签）。
func seedGapTag(t *testing.T, db *gorm.DB, label, category, status string) uint {
	t.Helper()
	tag := models.TopicTag{
		Label:    label,
		Slug:     "gap-" + label,
		Category: category,
		Status:   status,
	}
	require.NoError(t, db.Create(&tag).Error)
	return tag.ID
}

// seedGapMount 把标签挂到板块上（score 决定跨板排他，mismatch 决定方向不符剔除）。
func seedGapMount(t *testing.T, db *gorm.DB, tagID, boardID uint, score float64, mismatch bool) {
	t.Helper()
	mount := tagmodels.TopicTagBoardLabel{
		TopicTagID:        tagID,
		SemanticBoardID:   boardID,
		Score:             score,
		MatchReason:       "test",
		DirectionMismatch: mismatch,
	}
	require.NoError(t, db.Create(&mount).Error)
}

// seedGapArticle 建一篇文章并挂到标签上（pub_date 决定它归属哪一天）。
func seedGapArticle(t *testing.T, db *gorm.DB, tagID uint, pubDate time.Time) {
	t.Helper()
	pub := pubDate
	article := models.Article{FeedID: 1, Title: "gap article " + pubDate.Format("2006-01-02"), PubDate: &pub}
	require.NoError(t, db.Create(&article).Error)
	link := models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tagID, Score: 1}
	require.NoError(t, db.Create(&link).Error)
}

// gapKeys 把缺口列表压成 "date|boardID" 集合，便于断言。
func gapKeys(gaps []DailyReportGap) map[string]bool {
	out := make(map[string]bool, len(gaps))
	for _, g := range gaps {
		out[fmt.Sprintf("%s|%d", g.Date, g.BoardID)] = true
	}
	return out
}

// TestListDailyReportGaps_DetectsOnlyMissingReports covers the four gap
// classes at once: 有报告的不是缺口、无内容的不是缺口、方向不符被剔的不是缺口、
// 跨板排他被剔的不是缺口、窗口外不是缺口。
func TestListDailyReportGaps_DetectsOnlyMissingReports(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardWithGap := seedTestBoardNamed(t, db, "gap-board-missing")
	boardWithReport := seedTestBoardNamed(t, db, "gap-board-done")
	boardMismatch := seedTestBoardNamed(t, db, "gap-board-mismatch")
	boardOutsold := seedTestBoardNamed(t, db, "gap-board-outsold")
	boardHome := seedTestBoardNamed(t, db, "gap-board-home")
	boardNoContent := seedTestBoardNamed(t, db, "gap-board-no-content")
	boardOutOfWindow := seedTestBoardNamed(t, db, "gap-board-out-of-window")

	// 扫描窗口是 [今天-7, 今天-1]（不含今天）——缺口样本落在窗口内的昨天。
	today := time.Now().In(time.Local)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	day := today.AddDate(0, 0, -1)

	// ① 有候选内容 + 没有报告 → 缺口。
	tagGap := seedGapTag(t, db, "缺口标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tagGap, boardWithGap, 0.9, false)
	seedGapArticle(t, db, tagGap, day.Add(12*time.Hour))

	// ② 有候选内容 + 已有报告 → 不是缺口。
	tagDone := seedGapTag(t, db, "已出报告标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tagDone, boardWithReport, 0.9, false)
	seedGapArticle(t, db, tagDone, day.Add(12*time.Hour))
	seedTestReport(t, db, boardWithReport, day)

	// ③ 方向不符（direction_mismatch）→ 不是缺口。
	tagMismatch := seedGapTag(t, db, "方向不符标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tagMismatch, boardMismatch, 0.9, true)
	seedGapArticle(t, db, tagMismatch, day.Add(12*time.Hour))

	// ④ 跨板排他：同一标签在 home 板块 0.9、在 outsold 板块 0.5，
	//    分差 0.4 >= gap 0.3 → outsold 板块被剔（home 板块留下）。
	tagShared := seedGapTag(t, db, "跨板共享标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tagShared, boardHome, 0.9, false)
	seedGapMount(t, db, tagShared, boardOutsold, 0.5, false)
	seedGapArticle(t, db, tagShared, day.Add(12*time.Hour))

	// ⑤ 目标日没有文章（内容在别的日子）→ 不是缺口（补它必然产出空报告）。
	tagNoContent := seedGapTag(t, db, "无当日内容标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tagNoContent, boardNoContent, 0.9, false)
	seedGapArticle(t, db, tagNoContent, day.AddDate(0, 0, -3).Add(12*time.Hour))

	// ⑥ 窗口外（10 天前）→ 不在扫描窗口内。
	tagOld := seedGapTag(t, db, "窗口外标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tagOld, boardOutOfWindow, 0.9, false)
	seedGapArticle(t, db, tagOld, day.AddDate(0, 0, -9).Add(12*time.Hour))

	// ⑦ 非 active / 非 event 标签 → 不是缺口。
	tagMerged := seedGapTag(t, db, "已合并标签", models.TagCategoryEvent, "merged")
	seedGapMount(t, db, tagMerged, boardNoContent, 0.9, false)
	seedGapArticle(t, db, tagMerged, day.Add(12*time.Hour))
	tagPerson := seedGapTag(t, db, "人物标签", models.TagCategoryPerson, "active")
	seedGapMount(t, db, tagPerson, boardNoContent, 0.9, false)
	seedGapArticle(t, db, tagPerson, day.Add(12*time.Hour))

	// 单日窗口：逐类判定都发生在「目标日」这一天（缺口是按日按格子的）。
	windowStart := day
	windowEnd := day
	gaps, err := repo.ListDailyReportGaps(windowStart, windowEnd, 0.3)
	require.NoError(t, err)

	keys := gapKeys(gaps)
	dateKey := NormalizeReportDate(day).Format("2006-01-02")
	require.True(t, keys[dateKey+"|"+fmt.Sprintf("%d", boardWithGap)], "有候选内容且没有报告 → 缺口")
	require.False(t, keys[dateKey+"|"+fmt.Sprintf("%d", boardWithReport)], "已有报告 → 不是缺口")
	require.False(t, keys[dateKey+"|"+fmt.Sprintf("%d", boardMismatch)], "direction_mismatch 被剔 → 不是缺口")
	require.False(t, keys[dateKey+"|"+fmt.Sprintf("%d", boardOutsold)], "跨板排他被剔 → 不是缺口")
	require.True(t, keys[dateKey+"|"+fmt.Sprintf("%d", boardHome)], "排他胜出方 → 是缺口")
	require.False(t, keys[dateKey+"|"+fmt.Sprintf("%d", boardNoContent)], "当天无候选内容 → 不是缺口")
	require.Len(t, gaps, 2, "只有两个板块是真缺口：%v", gaps)

	// 缺口带板块名（semantic_labels 回填）。
	for _, g := range gaps {
		require.NotEmpty(t, g.BoardLabel, "缺口必须带板块名")
	}
}

// TestListDailyReportGaps_WindowBoundaries: 窗口是闭区间 [start, end]，
// 窗口外的日期不返回。
func TestListDailyReportGaps_WindowBoundaries(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	board := seedTestBoardNamed(t, db, "gap-window-board")
	day := time.Now().In(time.Local)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())

	tag := seedGapTag(t, db, "窗口边界标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tag, board, 0.9, false)
	seedGapArticle(t, db, tag, day.AddDate(0, 0, -2).Add(12*time.Hour))

	// 窗口覆盖 -2 → 是缺口。
	gaps, err := repo.ListDailyReportGaps(day.AddDate(0, 0, -3), day.AddDate(0, 0, -1), 0.3)
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	require.Equal(t, NormalizeReportDate(day.AddDate(0, 0, -2)).Format("2006-01-02"), gaps[0].Date)

	// 窗口收窄到 -1（不含 -2）→ 不是缺口。
	gaps, err = repo.ListDailyReportGaps(day.AddDate(0, 0, -1), day.AddDate(0, 0, -1), 0.3)
	require.NoError(t, err)
	require.Empty(t, gaps, "窗口外的日期不返回")
}

// TestListDailyReportGaps_ExclusiveGapDisabled: gap <= 0 时排他子句整体关闭
// （与 collectBoardTags 的 Go 侧开关同口径）。
func TestListDailyReportGaps_ExclusiveGapDisabled(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardHome := seedTestBoardNamed(t, db, "gap-off-home")
	boardWeak := seedTestBoardNamed(t, db, "gap-off-weak")
	day := time.Now().In(time.Local)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())

	tag := seedGapTag(t, db, "关闭排他标签", models.TagCategoryEvent, "active")
	seedGapMount(t, db, tag, boardHome, 0.9, false)
	seedGapMount(t, db, tag, boardWeak, 0.5, false)
	seedGapArticle(t, db, tag, day.Add(12*time.Hour))

	gaps, err := repo.ListDailyReportGaps(day.AddDate(0, 0, -1), day, 0)
	require.NoError(t, err)
	require.Len(t, gaps, 2, "排他关闭时两个板块都算缺口")
}

// TestBackfillRunLedger_CreateAndFinalize 覆盖运行留档的写入与收尾：
// 收尾幂等（不重复建行），最近一次运行摘要可读。
func TestBackfillRunLedger_CreateAndFinalize(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	windowStart := time.Now().AddDate(0, 0, -7)
	windowEnd := time.Now().AddDate(0, 0, -1)
	run := &DailyReportBackfillRun{
		RunID:        "run-1",
		TriggerScope: BackfillTriggerScopeManual,
		WindowStart:  windowStart,
		WindowEnd:    windowEnd,
		StartedAt:    time.Now(),
	}
	require.NoError(t, repo.CreateBackfillRun(run))
	require.NotZero(t, run.ID)
	require.Equal(t, BackfillRunStatusRunning, run.Status)

	latest, err := repo.LatestBackfillRun()
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, "run-1", latest.RunID)
	require.Equal(t, BackfillRunStatusRunning, latest.Status)

	details := JSON(`[{"date":"2026-11-20","filled":2,"failed":0,"remaining":0}]`)
	finished := time.Now()
	require.NoError(t, repo.FinalizeBackfillRun("run-1", BackfillRunStatusSuccess, details, 2, 0, 0, finished))

	// 收尾幂等：再收一次不新建行，终态稳定。
	require.NoError(t, repo.FinalizeBackfillRun("run-1", BackfillRunStatusSuccess, details, 2, 0, 0, finished))

	var count int64
	require.NoError(t, db.Model(&DailyReportBackfillRun{}).Where("run_id = ?", "run-1").Count(&count).Error)
	require.EqualValues(t, 1, count, "收尾幂等：同一运行只有一行留档")

	latest, err = repo.LatestBackfillRun()
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, BackfillRunStatusSuccess, latest.Status)
	require.Equal(t, 2, latest.FilledCount)
	require.Equal(t, 0, latest.FailedCount)
	require.Equal(t, 0, latest.RemainingCount)
	require.NotNil(t, latest.FinishedAt)
	require.Contains(t, string(latest.DailyDetails), "2026-11-20")
}
