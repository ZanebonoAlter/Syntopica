package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// 用例来源：openspec/changes/add-ai-generated-article-detection
// tasks 4.4 / 4.5 + specs/daily-report-system（unknown 不参与任何降权）。

// setupAIDetectionTestDB 用 SQLite 内存库装上 repository.Repo 单例：疑似占比与
// 疑似集合都要读 articles.ai_detection_status，纯内存断言覆盖不到 DB 读取路径。
func setupAIDetectionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ai-detection-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Article{}))
	original := repository.Repo
	repository.Repo = repository.NewTopicGraphRepository(db)
	t.Cleanup(func() { repository.Repo = original })
	return db
}

func seedDetectionArticle(t *testing.T, db *gorm.DB, id uint, status string) {
	t.Helper()
	pubDate := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	article := models.Article{
		ID:      id,
		FeedID:  1,
		Title:   fmt.Sprintf("article-%d", id),
		Link:    fmt.Sprintf("https://example.com/ai-detection/%d", id),
		PubDate: &pubDate,
	}
	article.AIDetectionStatus = status
	require.NoError(t, db.Create(&article).Error)
}

// TestPopulateThreadArticles_SuspectsGoLast 覆盖 task 4.4 场景「5 篇含 2 篇疑似
// → 3 篇在前 2 篇在后且成员不变」。
func TestPopulateThreadArticles_SuspectsGoLast(t *testing.T) {
	threads := []repository.Thread{{Title: "线索", TagIDs: []uint{1}}}
	tagArticleMap := map[uint][]uint{1: {101, 102, 103, 104, 105}}
	suspects := map[uint]bool{102: true, 104: true}

	populateThreadArticles(threads, tagArticleMap, suspects)

	require.Equal(t, []uint{101, 103, 105, 102, 104}, threads[0].RelatedArticleIDs,
		"非疑似保序在前、疑似稳定排至末尾")
	require.Len(t, threads[0].RelatedArticleIDs, 5, "成员与条数不变")
	require.ElementsMatch(t, tagArticleMap[1], threads[0].RelatedArticleIDs)
}

// TestPopulateThreadArticles_CountUnchanged 覆盖 task 4.4 场景「N 篇计数不变」：
// 多标签并集去重后的条数与成员集合不因疑似分区改变。
func TestPopulateThreadArticles_CountUnchanged(t *testing.T) {
	threads := []repository.Thread{{Title: "并集线索", TagIDs: []uint{1, 2}}}
	// 1 → [201..206]，2 → [205..212]，并集去重后 12 篇。
	tagArticleMap := map[uint][]uint{
		1: {201, 202, 203, 204, 205, 206},
		2: {205, 206, 207, 208, 209, 210, 211, 212},
	}
	union := make([]uint, 0, 12)
	for _, id := range tagArticleMap[1] {
		union = append(union, id)
	}
	for _, id := range tagArticleMap[2] {
		if id == 205 || id == 206 {
			continue
		}
		union = append(union, id)
	}
	suspects := map[uint]bool{203: true, 208: true, 212: true}

	populateThreadArticles(threads, tagArticleMap, suspects)

	require.Len(t, threads[0].RelatedArticleIDs, len(union), "N 篇计数不变（红线 45）")
	require.ElementsMatch(t, union, threads[0].RelatedArticleIDs, "集合成员不变")
	// 疑似全部落在末尾（相对顺序保持：203 在 208 前，208 在 212 前）。
	require.Equal(t, []uint{203, 208, 212}, threads[0].RelatedArticleIDs[len(union)-3:])
}

// TestApplyAISuspectRatios_ByRatioNotHitCount 覆盖 task 4.5 场景「按比例而非
// 命中篇数计算」：10 篇 1 疑似 = 0.1，2 篇 2 疑似 = 1.0，两者不因命中篇数相同而
// 同等对待；未判定（unknown）不计入疑似占比。
func TestApplyAISuspectRatios_ByRatioNotHitCount(t *testing.T) {
	db := setupAIDetectionTestDB(t)

	var longIDs []uint
	for i := uint(300); i < 310; i++ {
		status := models.AIDetectionStatusNotAI
		switch i {
		case 300:
			status = models.AIDetectionStatusSuspect
		case 309:
			status = models.AIDetectionStatusUnknown
		}
		seedDetectionArticle(t, db, i, status)
		longIDs = append(longIDs, i)
	}
	shortIDs := []uint{311, 312}
	seedDetectionArticle(t, db, 311, models.AIDetectionStatusSuspect)
	seedDetectionArticle(t, db, 312, models.AIDetectionStatusSuspect)

	tags := []repository.TagInput{{ID: 1, Label: "长尾"}, {ID: 2, Label: "全疑似"}}
	applyAISuspectRatios(tags, [][]uint{longIDs, shortIDs})

	require.InDelta(t, 0.1, tags[0].AISuspectRatio, 1e-9)
	require.InDelta(t, 1.0, tags[1].AISuspectRatio, 1e-9)
}

// TestFilterTagsByQuality_SuspectRatioBreaksTie 覆盖 task 4.5 场景「同层级同分数
// 时占比高者被舍弃」。
func TestFilterTagsByQuality_SuspectRatioBreaksTie(t *testing.T) {
	tags := make([]repository.TagInput, 0, 31)
	for i := 0; i < 29; i++ {
		tags = append(tags, repository.TagInput{
			ID:          uint(400 + i),
			MatchReason: "direct_hit",
			Score:       0.9,
		})
	}
	// 两个同层级同分数的标签只争最后 1 个名额。
	tags = append(tags,
		repository.TagInput{ID: 500, MatchReason: "direct_hit", Score: 0.5, AISuspectRatio: 1.0},
		repository.TagInput{ID: 501, MatchReason: "direct_hit", Score: 0.5, AISuspectRatio: 0.1},
	)

	kept := filterTagsByQuality(tags)

	require.Len(t, kept, 30, "超上限按既有口径截断到 30")
	keptIDs := make(map[uint]bool, len(kept))
	for _, tag := range kept {
		keptIDs[tag.ID] = true
	}
	require.True(t, keptIDs[501], "占比低的标签优先保留")
	require.False(t, keptIDs[500], "占比高的标签在名额竞争时被舍弃")
}

// TestFilterTagsByQuality_RatioKeepsCandidateMembers 覆盖 task 4.5 场景「不影响
// 候选成员与去重口径」：不触发截断时占比完全不改变成员与顺序；触发截断时也只
// 决定边界名额，不改变上限本身。
func TestFilterTagsByQuality_RatioKeepsCandidateMembers(t *testing.T) {
	tags := make([]repository.TagInput, 0, 20)
	for i := 0; i < 20; i++ {
		tags = append(tags, repository.TagInput{
			ID:             uint(600 + i),
			MatchReason:    "direct_hit",
			Score:          0.5,
			AISuspectRatio: 1.0,
		})
	}

	kept := filterTagsByQuality(tags)

	require.Len(t, kept, len(tags), "占比不得把标签移出候选集合")
	for i := range tags {
		require.Equal(t, tags[i].ID, kept[i].ID, "未截断时顺序与成员完全不变")
	}
}
