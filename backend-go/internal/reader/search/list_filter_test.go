package search

// add-article-search tasks 2.8 — 列表端点搜索过滤片段的函数单测（sqlite，
// 无副作用；test-cases.md 故事 S4 步 1 的结构侧断言）+ PG testcontainer
// 口径一致性（与关键词管线 SearchKeyword 的命中集合逐 ID 相等）。
// DSN 红线：PG 侧只经 testutil.SetupTestDB 隔离容器，绝不连真库。

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func setupListFilterDB(t *testing.T) (*gorm.DB, *DictCache) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:list-filter-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{}, &models.AISettings{}))
	return db, NewDictCache(db)
}

// TestListSearchWhereStructure covers 片段结构：L0 多词 AND 组 / L1 词典
// 组 / L2 整串 / sqlite 无 L3 / LIKE 通配符转义 / 空查询 ok=false。
func TestListSearchWhereStructure(t *testing.T) {
	db, dict := setupListFilterDB(t)
	// 词典词：切词层出「人工智能」
	require.NoError(t, db.Create(&models.TopicTag{Label: "人工智能", Slug: "ai", Category: "keyword", Status: "active"}).Error)
	ctx := context.Background()

	// 多词查询：L0（两词 AND）+ L1（词典词）+ L2 整串；sqlite 无 L3
	cond, args, ok := ListSearchWhere(ctx, db, dict, "科技股 人工智能")
	require.True(t, ok)
	require.NotEmpty(t, cond)
	// 组间连接：词内 OR 无括号包裹，组间 join 形如 ") OR (" —— 出现
	// 次数 = 组数-1 = 2（L0/L1/L2 三组）
	require.Equal(t, 2, strings.Count(cond, ") OR ("), "三组以 OR 连接：\n%s", cond)
	require.Len(t, args, 8, "L0 两词(4) + L1 一词(2) + L2 整串(2)")
	require.Contains(t, cond, "ESCAPE '\\'", "LIKE 需配 ESCAPE 子句")

	// 单词查询：无 L0 组（与 L2 重合跳过）→ 两组 OR
	cond, args, ok = ListSearchWhere(ctx, db, dict, "人工智能")
	require.True(t, ok)
	require.Equal(t, 1, strings.Count(cond, ") OR ("))
	require.Len(t, args, 4, "L1 一词(2) + L2 整串(2)")

	// LIKE 通配符转义：按字面匹配
	_, args, ok = ListSearchWhere(ctx, db, dict, "100%_增长")
	require.True(t, ok)
	whole := args[len(args)-1].(string)
	require.Equal(t, "%100\\%\\_增长%", whole, "通配符须转义为字面")

	// 空查询：不过滤
	for _, q := range []string{"", "   ", "\u3000"} {
		_, _, ok = ListSearchWhere(ctx, db, dict, q)
		require.False(t, ok, "q=%q 应返回 ok=false", q)
	}
}

// TestListSearchWhereSqliteHits 断言 sqlite 上的实际命中（列表端点的
// sqlite 分支走 handler 既有 LIKE，此处验证片段本身在 sqlite 可执行且
// 命中集合与整串 LIKE 语义一致）。
func TestListSearchWhereSqliteHits(t *testing.T) {
	db, dict := setupListFilterDB(t)
	feed := models.Feed{Title: "源", URL: "https://example.com/lf"}
	require.NoError(t, db.Create(&feed).Error)
	articles := []models.Article{
		{FeedID: feed.ID, Title: "AI 泡沫下的科技股定价逻辑", Link: "https://example.com/c1"},
		{FeedID: feed.ID, Title: "完全无关", Link: "https://example.com/c2"},
	}
	require.NoError(t, db.Create(&articles).Error)

	cond, args, ok := ListSearchWhere(context.Background(), db, dict, "科技股 AI")
	require.True(t, ok)

	var ids []uint
	require.NoError(t, db.Table("articles").Select("articles.id").Where(cond, args...).Scan(&ids).Error)
	require.Equal(t, []uint{articles[0].ID}, ids, "L0 多词 AND：两词均出现（可分散）才命中")
}

// TestListSearchWhereMatchesKeywordPipelinePG is the PG testcontainer
// 口径一致性断言（spec「中文关键词不再漏召回」：命中数与管线同口径）：
// 同库同查询下，ListSearchWhere 过滤行集 == SearchKeyword 命中行集。
// 覆盖 L1 词典 / L2 整串 / L3 trigram 三层命中形态（种子与查询对复用
// repository_pg_test.go 已验证的高稳定组合，默认阈值）与归档两口径。
func TestListSearchWhereMatchesKeywordPipelinePG(t *testing.T) {
	db := testutil.SetupTestDB(t)
	dict := NewDictCache(db)
	ctx := context.Background()

	feedID := seedFeedPG(t, db)
	trigramID := seedArticlePG(t, db, feedID, "特斯拉Model Y地下车库自燃", "", true) // L3 命中（归档）
	wholeID := seedArticlePG(t, db, feedID, "科技股集体高开", "", false)           // L2 整串命中（活跃）
	wholeArchivedID := seedArticlePG(t, db, feedID, "科技股泡沫争论再起", "", true)  // L2 命中（归档）
	dictID := seedArticlePG(t, db, feedID, "人工智能大模型进展", "", false)          // L1 词典命中
	seedArticlePG(t, db, feedID, "英超冬窗转会关闭", "", false)                     // 噪声
	seedTagPG(t, db, "人工智能")

	queries := map[string][]uint{
		// 查询→include 口径下应命中的 ID（验证集合非空、三层各有着落）
		"特斯拉电池自燃": {trigramID},
		"科技股":     {wholeID, wholeArchivedID},
		"人工智能":    {dictID},
	}

	for query, wantInclude := range queries {
		pq, err := NewQueryPreprocessor(db, dict).Prepare(ctx, query)
		require.NoError(t, err)

		for _, archived := range []string{ArchiveInclude, ArchiveActiveOnly} {
			hits, err := NewSearchRepository(db).SearchKeyword(ctx, pq, archived)
			require.NoError(t, err)
			pipelineIDs := make(map[uint]struct{}, len(hits))
			for _, h := range hits {
				pipelineIDs[h.Article.ID] = struct{}{}
			}

			cond, args, ok := ListSearchWhere(ctx, db, dict, query)
			require.True(t, ok)

			var rows []struct{ ID uint }
			q := db.WithContext(ctx).Table("articles").Select("articles.id").Where(cond, args...)
			if archived == ArchiveActiveOnly {
				q = q.Where("articles.archived = ?", false)
			}
			require.NoError(t, q.Scan(&rows).Error)

			fragIDs := make(map[uint]struct{}, len(rows))
			for _, r := range rows {
				fragIDs[r.ID] = struct{}{}
			}
			require.Equal(t, pipelineIDs, fragIDs, "q=%q archived=%s：片段过滤与管线命中集合须一致", query, archived)

			if archived == ArchiveInclude {
				want := make(map[uint]struct{}, len(wantInclude))
				for _, id := range wantInclude {
					want[id] = struct{}{}
				}
				require.Equal(t, want, fragIDs, "q=%q：include 口径三层命中形态断言", query)
			}
		}
	}
}
