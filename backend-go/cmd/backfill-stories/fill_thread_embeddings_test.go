package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/topicgraph/repository"
)

// fillThreadTestDB 建一个内存 SQLite（与 fillStoryTestDB 同款惯例）：只有
// daily_report_threads + board_daily_reports 两张表 —— 回填的 SQL
// （embedding IS NULL 扫描 + report 关联过滤 + 单列 UPDATE）是标准 SQL，两库通用；
// 不碰真库。
func fillThreadTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:fill-thread-emb-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&repository.BoardDailyReport{}, &repository.DailyReportSection{}, &repository.DailyReportThread{}))
	return db
}

// seedFillReport 落一份日报（board / 报告期）。
func seedFillReport(t *testing.T, db *gorm.DB, id, boardID uint, periodDate time.Time) {
	t.Helper()
	report := repository.BoardDailyReport{
		ID: id, SemanticBoardID: boardID, PeriodDate: periodDate,
		Title: "日报", Status: "completed",
	}
	require.NoError(t, db.Create(&report).Error)
}

// seedFillThread 落一条日报线程（默认属于 section 1）；embedding 传空串表示
// 「存量无向量」（建线时该列被 Omit → 库里是 NULL），传入非空则直接带上向量。
func seedFillThread(t *testing.T, db *gorm.DB, id, reportID uint, title, embedding string) {
	t.Helper()
	seedFillThreadInSection(t, db, id, reportID, 1, title, embedding)
}

// seedFillThreadInSection 落一条属于指定 section 的日报线程。
func seedFillThreadInSection(t *testing.T, db *gorm.DB, id, reportID, sectionID uint, title, embedding string) {
	t.Helper()
	thread := repository.DailyReportThread{
		ID: id, ReportID: reportID, SectionID: sectionID, Title: title, Confidence: 0.6,
	}
	if embedding == "" {
		require.NoError(t, db.Omit("embedding").Create(&thread).Error)
		return
	}
	thread.Embedding = embedding
	require.NoError(t, db.Create(&thread).Error)
}

// seedFillSection 落一条 section（lane_tier 决定它是不是 watch 物化板块）。
func seedFillSection(t *testing.T, db *gorm.DB, id uint, laneTier string) {
	t.Helper()
	section := repository.DailyReportSection{
		ID: id, ReportID: 1, ClusterIndex: 0, ClusterLabel: "板块",
		ClusterTagIDs: repository.JSON("[]"), LaneTier: laneTier,
	}
	require.NoError(t, db.Omit("embedding").Create(&section).Error)
}

// readThreadEmbedding 读一条线程的向量列（区分 NULL 与空串）。
func readThreadEmbedding(t *testing.T, db *gorm.DB, id uint) *string {
	t.Helper()
	var value *string
	require.NoError(t, db.Raw(`SELECT embedding FROM daily_report_threads WHERE id = ?`, id).Scan(&value).Error)
	return value
}

// stubFillThreadEmbedder 替换线程标题向量实现（避免测试真调模型）。
func stubFillThreadEmbedder(fn fillThreadTitleEmbedderFunc) func() {
	orig := fillThreadTitleEmbedder
	fillThreadTitleEmbedder = fn
	return func() { fillThreadTitleEmbedder = orig }
}

// fillThreadDay 是测试里用的报告期（UTC 零点）。
func fillThreadDay(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// TestFillThreadEmbeddingsOnlyNullRows 只补 embedding IS NULL 的行；已有向量的
// 行既不被扫到、也不被覆盖（不覆盖已有向量 = 幂等的前提）。
func TestFillThreadEmbeddingsOnlyNullRows(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "有向量的线程", "[9.000000,9.000000]")
	seedFillThread(t, db, 2, 1, "缺向量的线程", "")

	calls := 0
	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		calls++
		assert.Equal(t, []string{"缺向量的线程"}, titles, "只对缺向量的行调模型")
		return [][]float64{{1, 0}}, nil
	})()

	stats, err := fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned)
	assert.EqualValues(t, 1, stats.Updated)
	assert.EqualValues(t, 0, stats.Skipped)
	assert.EqualValues(t, 0, stats.Failed)
	assert.EqualValues(t, 1, stats.Batches)
	assert.Equal(t, 1, calls)

	filled := readThreadEmbedding(t, db, 2)
	require.NotNil(t, filled)
	assert.Equal(t, repository.FloatsToPgVector([]float64{1, 0}), *filled)

	kept := readThreadEmbedding(t, db, 1)
	require.NotNil(t, kept)
	assert.Equal(t, "[9.000000,9.000000]", *kept, "已有向量的行不得被覆盖")
}

// TestFillThreadEmbeddingsDryRunDoesNotWrite dry-run 只统计条数：不发模型调用、
// 不写库。
func TestFillThreadEmbeddingsDryRunDoesNotWrite(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "缺向量的线程", "")

	calls := 0
	defer stubFillThreadEmbedder(func(_ context.Context, _ []string) ([][]float64, error) {
		calls++
		return [][]float64{{1, 0}}, nil
	})()

	dryOpts := commandOptions{}
	dryOpts.DryRun = true
	stats, err := fillThreadEmbeddings(context.Background(), db, dryOpts)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned, "dry-run 也要报出待补条数")
	assert.EqualValues(t, 0, stats.Updated)
	assert.EqualValues(t, 0, stats.Failed)
	assert.EqualValues(t, 0, stats.Batches, "dry-run 不发模型调用")
	assert.Equal(t, 0, calls, "dry-run 不得发模型调用")

	assert.Nil(t, readThreadEmbedding(t, db, 1), "dry-run 不得写库")
}

// TestFillThreadEmbeddingsOnlyTouchesEmbeddingColumn 只写 embedding 一列：
// title / report_id / section_id / story_id / fit_distance / scope /
// display_tier / confidence 等列一律不动。
func TestFillThreadEmbeddingsOnlyTouchesEmbeddingColumn(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "缺向量的线程", "")

	type threadSnapshot struct {
		Title         string
		ReportID      uint
		SectionID     uint
		StoryID       *uint
		FitDistance   *float64
		Scope         string
		DisplayTier   string
		Confidence    float64
		CreatedAt     time.Time
		RelatedArtIDs string
	}
	read := func() threadSnapshot {
		var snap threadSnapshot
		require.NoError(t, db.Raw(`SELECT title, report_id, section_id, story_id, fit_distance,
			scope, display_tier, confidence, created_at, related_article_ids
			FROM daily_report_threads WHERE id = 1`).Scan(&snap).Error)
		return snap
	}
	before := read()

	defer stubFillThreadEmbedder(func(_ context.Context, _ []string) ([][]float64, error) {
		return [][]float64{{1, 0}}, nil
	})()
	stats, err := fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Updated)

	after := read()
	assert.Equal(t, before.Title, after.Title)
	assert.Equal(t, before.ReportID, after.ReportID)
	assert.Equal(t, before.SectionID, after.SectionID)
	assert.Equal(t, before.StoryID, after.StoryID)
	assert.Equal(t, before.FitDistance, after.FitDistance)
	assert.Equal(t, before.Scope, after.Scope)
	assert.Equal(t, before.DisplayTier, after.DisplayTier)
	assert.Equal(t, before.Confidence, after.Confidence)
	assert.Equal(t, before.RelatedArtIDs, after.RelatedArtIDs)
	assert.Equal(t, before.CreatedAt.UTC(), after.CreatedAt.UTC())
}

// TestFillThreadEmbeddingsIsIdempotent 可重复执行：补过的行不再被处理。
func TestFillThreadEmbeddingsIsIdempotent(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "缺向量的线程", "")

	calls := 0
	defer stubFillThreadEmbedder(func(_ context.Context, _ []string) ([][]float64, error) {
		calls++
		return [][]float64{{1, 0}}, nil
	})()

	stats, err := fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Updated)

	stats, err = fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, stats.Scanned, "补过的行不再被扫到")
	assert.EqualValues(t, 0, stats.Updated)
	assert.EqualValues(t, 0, stats.Batches, "第二次不再调模型")
	assert.Equal(t, 1, calls, "第二次不再调模型")
}

// TestFillThreadEmbeddingsAggregatesBatchFailures 整批调用失败不中断：失败批的行
// 计入失败数，下一批照常补。
func TestFillThreadEmbeddingsAggregatesBatchFailures(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	// 33 条 → 两批（32 + 1）。
	for i := 1; i <= 33; i++ {
		seedFillThread(t, db, uint(i), 1, fmt.Sprintf("线程-%02d", i), "")
	}

	batches := 0
	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		batches++
		if batches == 1 {
			return nil, fmt.Errorf("embedding gateway down")
		}
		vectors := make([][]float64, 0, len(titles))
		for range titles {
			vectors = append(vectors, []float64{1, 0})
		}
		return vectors, nil
	})()

	stats, err := fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 33, stats.Scanned)
	assert.EqualValues(t, 2, stats.Batches)
	assert.EqualValues(t, 32, stats.Failed, "失败批按行计入失败数")
	assert.EqualValues(t, 1, stats.Updated, "失败不打断下一批")
	assert.Nil(t, readThreadEmbedding(t, db, 1))
	filled := readThreadEmbedding(t, db, 33)
	require.NotNil(t, filled)
}

// TestFillThreadEmbeddingsShortBatchResult 模型少返一条 → 多出来的行记跳过
// （保持 NULL），其余行照常补。
func TestFillThreadEmbeddingsShortBatchResult(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "线程-1", "")
	seedFillThread(t, db, 2, 1, "线程-2", "")

	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		return [][]float64{{1, 0}}, nil // 只回一条
	})()

	stats, err := fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.Scanned)
	assert.EqualValues(t, 1, stats.Updated)
	assert.EqualValues(t, 1, stats.Skipped, "少返的行记跳过（保持 NULL）")
	assert.EqualValues(t, 0, stats.Failed)
	assert.Nil(t, readThreadEmbedding(t, db, 2))
}

// TestFillThreadEmbeddingsSkipsEmptyTitle 空标题的线程不在扫描范围内（没有可嵌入
// 的文本），既不计入待补条数，也不发模型调用。
func TestFillThreadEmbeddingsSkipsEmptyTitle(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "   ", "")

	calls := 0
	defer stubFillThreadEmbedder(func(_ context.Context, _ []string) ([][]float64, error) {
		calls++
		return [][]float64{{1, 0}}, nil
	})()

	stats, err := fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, stats.Scanned, "空标题不在待补范围内")
	assert.EqualValues(t, 0, stats.Skipped)
	assert.EqualValues(t, 0, stats.Failed)
	assert.Equal(t, 0, calls, "空标题不得调模型")
	assert.Nil(t, readThreadEmbedding(t, db, 1))
}

// TestFillThreadEmbeddingsBoardFilter --board 按报告所属版块过滤。
func TestFillThreadEmbeddingsBoardFilter(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillReport(t, db, 2, 9, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "版块 7 的线程", "")
	seedFillThread(t, db, 2, 2, "版块 9 的线程", "")

	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		vectors := make([][]float64, 0, len(titles))
		for range titles {
			vectors = append(vectors, []float64{1, 0})
		}
		return vectors, nil
	})()

	opts := commandOptions{}
	boardID := uint(7)
	opts.BoardID = &boardID
	stats, err := fillThreadEmbeddings(context.Background(), db, opts)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned, "只扫目标版块的线程")
	assert.EqualValues(t, 1, stats.Updated)
	assert.Nil(t, readThreadEmbedding(t, db, 2), "其它版块不得被补")
}

// TestFillThreadEmbeddingsDateFilter --from/--to 按报告期过滤（仅显式给出时生效）。
func TestFillThreadEmbeddingsDateFilter(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillReport(t, db, 2, 7, fillThreadDay(2026, time.November, 20))
	seedFillThread(t, db, 1, 1, "窗口内的线程", "")
	seedFillThread(t, db, 2, 2, "窗口外的线程", "")

	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		vectors := make([][]float64, 0, len(titles))
		for range titles {
			vectors = append(vectors, []float64{1, 0})
		}
		return vectors, nil
	})()

	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	// CLI 的写模式在停写后已被 parseOptions 拒绝（task 1.2）；这里直接测回填
	// 实现本身，所以先用 --dry-run 解析出日期范围再清掉 dry-run。
	opts, err := parseOptions([]string{"--fill-thread-embeddings", "--from", "2026-11-01", "--to", "2026-11-10", "--dry-run"}, now)
	require.NoError(t, err)
	opts.DryRun = false
	require.True(t, opts.ExplicitFrom)
	require.True(t, opts.ExplicitTo)

	stats, err := fillThreadEmbeddings(context.Background(), db, opts)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned, "只扫窗口内的线程")
	assert.EqualValues(t, 1, stats.Updated)
	assert.Nil(t, readThreadEmbedding(t, db, 2), "窗口外的线程不得被补")
}

// TestFillThreadEmbeddingsDefaultDateWindowNotApplied 默认的「最近 30 天」窗口不是
// 回填范围：不显式给出 --from/--to 时不按日期过滤（否则更早的存量会被漏掉）。
func TestFillThreadEmbeddingsDefaultDateWindowNotApplied(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillThread(t, db, 1, 1, "窗口内的线程", "")

	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	opts, err := parseOptions([]string{"--fill-thread-embeddings", "--dry-run"}, now)
	require.NoError(t, err)
	opts.DryRun = false // 同上：直接测回填实现
	require.False(t, opts.ExplicitFrom)
	require.False(t, opts.ExplicitTo)

	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		vectors := make([][]float64, 0, len(titles))
		for range titles {
			vectors = append(vectors, []float64{1, 0})
		}
		return vectors, nil
	})()

	stats, err := fillThreadEmbeddings(context.Background(), db, opts)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned, "默认日期窗口不得当回填范围")
	assert.EqualValues(t, 1, stats.Updated)
}

// TestFillThreadEmbeddingsExcludesWatchMaterializedThreads watch 物化板块的线程按
// 设计没有向量（物化 section 走 Omit 写 NULL）→ 默认排除；--include-watch-threads
// 才纳入。
func TestFillThreadEmbeddingsExcludesWatchMaterializedThreads(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillSection(t, db, 1, "l1_direct")
	seedFillSection(t, db, 2, "watch_keyword")
	seedFillThreadInSection(t, db, 1, 1, 1, "常规板块的线程", "")
	seedFillThreadInSection(t, db, 2, 1, 2, "物化板块的线程", "")

	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		vectors := make([][]float64, 0, len(titles))
		for range titles {
			vectors = append(vectors, []float64{1, 0})
		}
		return vectors, nil
	})()

	stats, err := fillThreadEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned, "物化板块的线程默认不在范围内")
	assert.EqualValues(t, 1, stats.Updated)
	assert.EqualValues(t, 1, stats.Excluded, "被排除的物化线程要报出条数")
	assert.Nil(t, readThreadEmbedding(t, db, 2), "物化板块的线程不得被补")
}

// TestFillThreadEmbeddingsIncludesWatchThreadsWhenAsked 显式打开后才纳入物化线程。
func TestFillThreadEmbeddingsIncludesWatchThreadsWhenAsked(t *testing.T) {
	db := fillThreadTestDB(t)
	seedFillReport(t, db, 1, 7, fillThreadDay(2026, time.November, 5))
	seedFillSection(t, db, 1, "l1_direct")
	seedFillSection(t, db, 2, "watch_keyword")
	seedFillThreadInSection(t, db, 1, 1, 1, "常规板块的线程", "")
	seedFillThreadInSection(t, db, 2, 1, 2, "物化板块的线程", "")

	defer stubFillThreadEmbedder(func(_ context.Context, titles []string) ([][]float64, error) {
		vectors := make([][]float64, 0, len(titles))
		for range titles {
			vectors = append(vectors, []float64{1, 0})
		}
		return vectors, nil
	})()

	opts := commandOptions{}
	opts.IncludeWatchThreads = true
	stats, err := fillThreadEmbeddings(context.Background(), db, opts)
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.Scanned)
	assert.EqualValues(t, 2, stats.Updated)
	assert.EqualValues(t, 0, stats.Excluded, "打开后不再有排除项")
	assert.NotNil(t, readThreadEmbedding(t, db, 2))
}
