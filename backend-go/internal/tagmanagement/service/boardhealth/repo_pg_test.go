package boardhealth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 用例来源：test-cases.md 主链路步 5（事实画像 diff：testcontainer PG repository
// 测试，禁 SQLite）+ 步 8（快照落库）+ 步 13/14 前置（趋势/空态/404 的数据层）。

func createAuxLabel(t *testing.T, db *gorm.DB, label string, status string) models.SemanticLabel {
	t.Helper()
	aux := models.SemanticLabel{Label: label, Slug: "bhx-" + label, LabelType: "auxiliary", Status: status}
	require.NoError(t, db.Create(&aux).Error)
	return aux
}

func createTopicTagRow(t *testing.T, db *gorm.DB, slug string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: slug, Slug: slug, Category: "keyword", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

// linkAuxToTag mounts an aux label on a tag (topic_tag_semantic_labels).
func linkAuxToTag(t *testing.T, db *gorm.DB, tagID uint, auxID uint) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tagID, SemanticLabelID: auxID}).Error)
}

// matchTagToBoard writes a topic_tag_board_labels row.
func matchTagToBoard(t *testing.T, db *gorm.DB, tagID uint, boardID uint) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID: tagID, SemanticBoardID: boardID, Score: 0.7, MatchReason: "direct_hit",
	}).Error)
}

func TestUncapturedCandidatesAggregation(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "AI前沿", "active")

	// 已挂载构成：大模型（不得进候选）。
	mounted := createAuxLabel(t, db, "大模型", "active")
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: mounted.ID}).Error)
	// 未收编高频：算力（3 个成员 tag，2 篇文章）。
	uncaptured := createAuxLabel(t, db, "算力", "active")
	// disabled：禁用标签不得进候选。
	disabled := createAuxLabel(t, db, "隐私计算", "disabled")

	for i := 0; i < 3; i++ {
		tag := createTopicTagRow(t, db, "gpu-tag-"+string(rune('a'+i)))
		linkAuxToTag(t, db, tag.ID, uncaptured.ID)
		matchTagToBoard(t, db, tag.ID, board.ID)
		if i == 0 {
			// 文章覆盖：2 篇文章引用首个 gpu tag（挂算力）。
			for j := 0; j < 2; j++ {
				feed := models.Feed{Title: "feed-bh", URL: "https://example.com/bh/" + tag.Slug + "-" + string(rune('0'+j))}
				require.NoError(t, db.Create(&feed).Error)
				article := models.Article{FeedID: feed.ID, Title: "art", CreatedAt: time.Now()}
				require.NoError(t, db.Create(&article).Error)
				require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID}).Error)
			}
		}
	}
	// 两个 tag 同时携带「大模型」（已挂载）与「算力」：算力的 member_tags 计数。
	duoTag := createTopicTagRow(t, db, "duo-tag")
	linkAuxToTag(t, db, duoTag.ID, mounted.ID)
	linkAuxToTag(t, db, duoTag.ID, uncaptured.ID)
	matchTagToBoard(t, db, duoTag.ID, board.ID)
	// disabled 携带 tag：匹配了版块也不进候选。
	disabledTag := createTopicTagRow(t, db, "privacy-tag")
	linkAuxToTag(t, db, disabledTag.ID, disabled.ID)
	matchTagToBoard(t, db, disabledTag.ID, board.ID)

	candidates, err := UncapturedCandidates(ctx, db, board.ID)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "mounted and disabled labels excluded; only 算力 remains")
	require.Equal(t, uncaptured.ID, candidates[0].AuxID)
	require.Equal(t, "算力", candidates[0].Label)
	require.Equal(t, 4, candidates[0].MemberTags, "3 gpu tags + duo tag")
	require.Equal(t, 2, candidates[0].MemberArticles)
}

// 主链路步 8：每轮落一条快照 + 历史快照不被覆盖 + 趋势读取。
func TestSnapshotPersistAndTrendRead(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	board := createBoardRow(t, db, "趋势板", "active")

	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		score := 50.0 + float64(i)
		comment := "评语"
		snapshot := tagmodels.BoardHealthSnapshot{
			SemanticBoardID: board.ID, CheckedAt: base.AddDate(0, 0, i),
			Metrics: tagmodels.BoardHealthMetrics{
				Board:  tagmodels.BoardHealthBoardMetrics{MatchedTags: i + 1, ReasonDist: map[string]int{}},
				Labels: []tagmodels.BoardHealthLabelMetrics{},
			},
			UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
			LLMScore:             &score, LLMComment: &comment,
		}
		require.NoError(t, db.Create(&snapshot).Error)
	}

	latest, err := LatestSnapshot(ctx, db, board.ID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.InDelta(t, 52.0, *latest.LLMScore, 1e-9, "latest = newest checked_at")

	// 体检数据查询：latest + 趋势升序（spec: 按时间升序的最近 N 条）。
	viewLatest, trend, err := HealthCheckupView(ctx, db, board.ID, 7)
	require.NoError(t, err)
	require.NotNil(t, viewLatest)
	require.Len(t, trend, 3)
	require.Less(t, trend[0].CheckedAt.Unix(), trend[2].CheckedAt.Unix(), "ascending order")
	require.InDelta(t, 52.0, *trend[2].LLMScore, 1e-9)

	// 版块不存在 → ErrBoardNotFound（404 语义）。
	_, _, err = HealthCheckupView(ctx, db, 999999, 7)
	require.ErrorIs(t, err, ErrBoardNotFound)

	// disabled / 非 board → 空态（非错误；disabled 不是 404）。
	disabledBoard := createBoardRow(t, db, "停用板", "disabled")
	emptyLatest, emptyTrend, err := HealthCheckupView(ctx, db, disabledBoard.ID, 7)
	require.NoError(t, err)
	require.Nil(t, emptyLatest)
	require.Empty(t, emptyTrend)

	// 无快照的活跃版块 → 空态（spec: 无快照返回空态）。
	freshBoard := createBoardRow(t, db, "新板", "active")
	freshLatest, freshTrend, err := HealthCheckupView(ctx, db, freshBoard.ID, 7)
	require.NoError(t, err)
	require.Nil(t, freshLatest)
	require.Empty(t, freshTrend)
}

// 变体「重复：同日两轮体检」：两条快照并存，趋势原样展示。
func TestSameDayDuplicateRoundsCoexist(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "同日板", "active")
	now := time.Now()
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
			SemanticBoardID: board.ID, CheckedAt: now,
			Metrics:              tagmodels.BoardHealthMetrics{Board: tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}}, Labels: []tagmodels.BoardHealthLabelMetrics{}},
			UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
		}).Error)
	}
	var count int64
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Where("semantic_board_id = ?", board.ID).Count(&count).Error)
	require.Equal(t, int64(2), count)
	_, trend, err := HealthCheckupView(context.Background(), db, board.ID, 7)
	require.NoError(t, err)
	require.Len(t, trend, 2)
}

// 用例来源：test-cases.md 主链路步 3（贴合度分布）PG 落点——锁定
// loadCompositionLabels 的 embedding 列映射。2026-10-07 回归：结构体字段
// 缺 gorm:"column:embedding" 标签时蛇形名对不上查询列名，向量永远扫空 →
// 全标签 insufficient（真库 12 版块 max_samples=0 才暴露）。
func TestLoadCompositionLabelsEmbeddingMapping(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "映射板", "active")

	withVec := createAuxLabel(t, db, "推理", "active")
	noVec := createAuxLabel(t, db, "无向量", "active")
	for _, aux := range []models.SemanticLabel{withVec, noVec} {
		require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: aux.ID}).Error)
	}
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET embedding = '[1,0,1]'::vector WHERE id = ?`, withVec.ID).Error)

	labels, auxEmb, err := loadCompositionLabels(context.Background(), db, board.ID)
	require.NoError(t, err)
	require.Len(t, labels, 2)
	require.Contains(t, auxEmb, withVec.ID, "embedding column must map into the aux vector map")
	require.Equal(t, []float64{1, 0, 1}, auxEmb[withVec.ID])
	require.NotContains(t, auxEmb, noVec.ID)
}
