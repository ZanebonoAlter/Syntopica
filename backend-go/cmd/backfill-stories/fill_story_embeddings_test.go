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

// fillStoryTestDB 建一个内存 SQLite（与 contentKeyTestDB 同款惯例）：只有
// board_stories 一张表 —— 回填的 SQL（embedding IS NULL 扫描 + 单列 UPDATE）
// 是标准 SQL，两库通用；不碰真库。
func fillStoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:fill-story-emb-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&repository.BoardStory{}))
	return db
}

// seedFillStory 落一条事件线；embedding 传空串表示「存量无向量」（建线时
// 该列被 Omit → 库里是 NULL），传入非空则直接带上向量。
func seedFillStory(t *testing.T, db *gorm.DB, id uint, title, embedding string) {
	t.Helper()
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	story := repository.BoardStory{ID: id, BoardID: 1, Title: title,
		FirstSeenDate: day, LastSeenDate: day, ThreadCount: 3, ParticipantCount: 2}
	if embedding != "" {
		story.Embedding = embedding
	}
	require.NoError(t, db.Omit("subject_embedding").Create(&story).Error)
	if embedding == "" {
		// GORM 会把空串写成 '' 而不是 NULL —— 显式置 NULL 才等于「存量无向量」。
		require.NoError(t, db.Exec(`UPDATE board_stories SET embedding = NULL WHERE id = ?`, id).Error)
	}
}

// readStoryEmbedding 读一条事件线的向量列（区分 NULL 与空串）。
func readStoryEmbedding(t *testing.T, db *gorm.DB, id uint) *string {
	t.Helper()
	var value *string
	require.NoError(t, db.Raw(`SELECT embedding FROM board_stories WHERE id = ?`, id).Scan(&value).Error)
	return value
}

// stubFillStoryEmbedder 替换标题向量实现（避免测试真调模型）。
func stubFillStoryEmbedder(fn func(ctx context.Context, title string) ([]float64, error)) func() {
	orig := fillStoryTitleEmbedder
	fillStoryTitleEmbedder = fn
	return func() { fillStoryTitleEmbedder = orig }
}

// TestFillStoryEmbeddingsOnlyNullRows 只补 embedding IS NULL 的行；已有向量的
// 行不被覆盖（不覆盖已有向量 = 幂等的前提）。
func TestFillStoryEmbeddingsOnlyNullRows(t *testing.T) {
	db := fillStoryTestDB(t)
	seedFillStory(t, db, 1, "空向量事件线", "")
	seedFillStory(t, db, 2, "已有向量事件线", "[9.000000,9.000000]")

	calls := 0
	defer stubFillStoryEmbedder(func(_ context.Context, title string) ([]float64, error) {
		calls++
		assert.Equal(t, "空向量事件线", title)
		return []float64{1, 0}, nil
	})()

	stats, err := fillStoryEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned)
	assert.EqualValues(t, 1, stats.Updated)
	assert.EqualValues(t, 0, stats.Failed)
	assert.Equal(t, 1, calls, "只对缺向量的行调模型")

	filled := readStoryEmbedding(t, db, 1)
	require.NotNil(t, filled)
	assert.Equal(t, repository.FloatsToPgVector([]float64{1, 0}), *filled)

	kept := readStoryEmbedding(t, db, 2)
	require.NotNil(t, kept)
	assert.Equal(t, "[9.000000,9.000000]", *kept, "已有向量的行不得被覆盖")
}

// TestFillStoryEmbeddingsDryRunDoesNotWrite dry-run 只统计条数：不发模型调用、
// 不写库。
func TestFillStoryEmbeddingsDryRunDoesNotWrite(t *testing.T) {
	db := fillStoryTestDB(t)
	seedFillStory(t, db, 1, "空向量事件线", "")

	calls := 0
	defer stubFillStoryEmbedder(func(_ context.Context, _ string) ([]float64, error) {
		calls++
		return []float64{1, 0}, nil
	})()

	dryOpts := commandOptions{}
	dryOpts.DryRun = true
	stats, err := fillStoryEmbeddings(context.Background(), db, dryOpts)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned, "dry-run 也要报出待补条数")
	assert.EqualValues(t, 0, stats.Updated)
	assert.EqualValues(t, 0, stats.Failed)
	assert.Equal(t, 0, calls, "dry-run 不得发模型调用")

	assert.Nil(t, readStoryEmbedding(t, db, 1), "dry-run 不得写库")
}

// TestFillStoryEmbeddingsOnlyTouchesEmbeddingColumn 只写 embedding 一列：
// first_seen_date / last_seen_date / thread_count / participant_count / title
// 等列一律不动。
func TestFillStoryEmbeddingsOnlyTouchesEmbeddingColumn(t *testing.T) {
	db := fillStoryTestDB(t)
	seedFillStory(t, db, 1, "空向量事件线", "")

	type storySnapshot struct {
		Title            string
		BoardID          uint
		ThreadCount      int
		ParticipantCount int
		FirstSeenDate    time.Time
		LastSeenDate     time.Time
	}
	read := func() storySnapshot {
		var snap storySnapshot
		require.NoError(t, db.Raw(`SELECT title, board_id, thread_count, participant_count,
			first_seen_date, last_seen_date FROM board_stories WHERE id = 1`).Scan(&snap).Error)
		return snap
	}
	before := read()

	defer stubFillStoryEmbedder(func(_ context.Context, _ string) ([]float64, error) {
		return []float64{1, 0}, nil
	})()
	stats, err := fillStoryEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Updated)

	after := read()
	assert.Equal(t, before.Title, after.Title)
	assert.Equal(t, before.BoardID, after.BoardID)
	assert.Equal(t, before.ThreadCount, after.ThreadCount)
	assert.Equal(t, before.ParticipantCount, after.ParticipantCount)
	assert.Equal(t, before.FirstSeenDate.UTC(), after.FirstSeenDate.UTC())
	assert.Equal(t, before.LastSeenDate.UTC(), after.LastSeenDate.UTC())
}

// TestFillStoryEmbeddingsIsIdempotent 可重复执行：补过的行不再被处理。
func TestFillStoryEmbeddingsIsIdempotent(t *testing.T) {
	db := fillStoryTestDB(t)
	seedFillStory(t, db, 1, "空向量事件线", "")

	calls := 0
	defer stubFillStoryEmbedder(func(_ context.Context, _ string) ([]float64, error) {
		calls++
		return []float64{1, 0}, nil
	})()

	stats, err := fillStoryEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Updated)

	stats, err = fillStoryEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 0, stats.Scanned, "补过的行不再被扫到")
	assert.EqualValues(t, 0, stats.Updated)
	assert.Equal(t, 1, calls, "第二次不再调模型")
}

// TestFillStoryEmbeddingsAggregatesFailures 逐条失败不中断：失败行计入失败数，
// 其它行照常补。
func TestFillStoryEmbeddingsAggregatesFailures(t *testing.T) {
	db := fillStoryTestDB(t)
	seedFillStory(t, db, 1, "会失败的事件线", "")
	seedFillStory(t, db, 2, "会成功的事件线", "")

	defer stubFillStoryEmbedder(func(_ context.Context, title string) ([]float64, error) {
		if title == "会失败的事件线" {
			return nil, fmt.Errorf("embedding gateway down")
		}
		return []float64{1, 0}, nil
	})()

	stats, err := fillStoryEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 2, stats.Scanned)
	assert.EqualValues(t, 1, stats.Failed, "失败行计入失败数")
	assert.EqualValues(t, 1, stats.Updated, "失败不打断其它行")
	assert.Nil(t, readStoryEmbedding(t, db, 1))
	filled := readStoryEmbedding(t, db, 2)
	require.NotNil(t, filled)
}

// TestFillStoryEmbeddingsSkipsEmptyTitle 标题为空的事件线保持 NULL（无向量可算），
// 计入跳过而不是失败。
func TestFillStoryEmbeddingsSkipsEmptyTitle(t *testing.T) {
	db := fillStoryTestDB(t)
	seedFillStory(t, db, 1, "   ", "")

	calls := 0
	defer stubFillStoryEmbedder(func(_ context.Context, _ string) ([]float64, error) {
		calls++
		return []float64{1, 0}, nil
	})()

	stats, err := fillStoryEmbeddings(context.Background(), db, commandOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Scanned)
	assert.EqualValues(t, 1, stats.Skipped)
	assert.EqualValues(t, 0, stats.Failed)
	assert.Equal(t, 0, calls, "标题为空不得调模型")
	assert.Nil(t, readStoryEmbedding(t, db, 1))
}
