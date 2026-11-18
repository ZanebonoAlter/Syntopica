package repository

// widen-highlight-cluster-candidates（2026-11-15）task 1.1：ListReportEventThreads
// 不再按 scope 排除 composite 线索——单事件多标签 composite 大簇线索的 event 标签
// 与证据文章同权进入聚簇输入。本文件用 testcontainer PG 真跑改动后的 SQL（改 SQL
// 必须有 PG 用例，testing.md 硬约束），并顺带核「去重文章数排首位」的候选排序链
// （向量聚簇本身在 service 层测试覆盖，防混编由聚簇相似度门槛承担）。

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func seedScopeThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, scope string, tagIDs, articleIDs []uint) {
	t.Helper()
	require.NoError(t, db.Create(&DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             "线索",
		Scope:             scope,
		TagIDs:            JSON(mustJSONIDs(tagIDs)),
		RelatedArticleIDs: JSON(mustJSONIDs(articleIDs)),
		Embedding:         FloatsToPgVector([]float64{0}),
	}).Error)
}

// TestListReportEventThreadsIncludesComposite：scope 过滤删除后 composite 线索
// 同权返回（标签 + 证据投影原样）；查询仍按 report 隔离、id ASC 定序；随后用
// ArticleContentIdentities + RankEventClusterCandidates 核「单事件多标签
// composite 大簇 + 1 篇小簇」夹具的候选序——大簇按去重文章数排首位（spec
// Scenario「单事件多标签大簇进候选」的服务端可断言锚点）。
func TestListReportEventThreadsIncludesComposite(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoardNamed(t, db, "cluster-composite-in")
	reportID := seedTestReport(t, db, boardID, time.Now())
	sectionID := seedTestSection(t, db, reportID, "sec")

	// 大簇：17 个同事件不同表述的 event 标签聚合出一条 composite 线索（2026-11-12
	// 黄仁勋案例缩影），证据 21 行文章 = 20 个内容身份（一对同 link 双 feed 副本）。
	bigTagIDs := make([]uint, 17)
	for i := range bigTagIDs {
		bigTagIDs[i] = uint(1000 + i)
	}
	bigArticles := make([]models.Article, 0, 21)
	for i := 0; i < 20; i++ {
		bigArticles = append(bigArticles, models.Article{
			FeedID: 11, Title: fmt.Sprintf("大簇文章%d", i),
			Link: fmt.Sprintf("https://example.com/big/%d", i),
		})
	}
	// 同 link 第二 feed 副本：内容身份去重后只算 1 篇（红线 49）。
	bigArticles = append(bigArticles, models.Article{
		FeedID: 12, Title: "大簇文章0副本", Link: "https://example.com/big/0?utm=x",
	})
	require.NoError(t, db.Create(&bigArticles).Error)
	bigArticleIDs := make([]uint, 0, len(bigArticles))
	for _, article := range bigArticles {
		bigArticleIDs = append(bigArticleIDs, article.ID)
	}

	// 小簇：1 个标签 1 篇文章的 single 线索（当天其余候选的缩影）。
	smallArticle := models.Article{FeedID: 13, Title: "小簇文章", Link: "https://example.com/small/1"}
	require.NoError(t, db.Create(&smallArticle).Error)
	const smallTagID = 2000

	seedScopeThread(t, db, reportID, sectionID, ThreadScopeComposite, bigTagIDs, bigArticleIDs)
	seedScopeThread(t, db, reportID, sectionID, ThreadScopeSingle, []uint{smallTagID}, []uint{smallArticle.ID})

	// 其它报告的线索：不得混入（report 隔离不随 scope 放宽而放松）。
	otherReportID := seedTestReport(t, db, boardID, time.Now().AddDate(0, 0, -1))
	otherSectionID := seedTestSection(t, db, otherReportID, "other")
	seedScopeThread(t, db, otherReportID, otherSectionID, ThreadScopeSingle, []uint{3000}, nil)

	threads, err := repo.ListReportEventThreads(reportID)
	require.NoError(t, err)
	require.Len(t, threads, 2, "composite 线索同权返回；其它报告的线索隔离")
	require.Equal(t, bigTagIDs, threads[0].TagIDs, "composite 线索的 17 个 event 标签原样投影（id ASC 首条）")
	require.Equal(t, bigArticleIDs, threads[0].ArticleIDs)
	require.Equal(t, []uint{smallTagID}, threads[1].TagIDs)
	require.Equal(t, []uint{smallArticle.ID}, threads[1].ArticleIDs)

	// 候选排序链（聚簇分簇结果由向量相似度决定，此处按聚簇产出直接构造）：
	// 大簇去重文章数 20（21 行 - 1 个同 link 副本）> 小簇 1 → 大簇排候选首位。
	identities, err := repo.ArticleContentIdentities(append(bigArticleIDs, smallArticle.ID))
	require.NoError(t, err)
	bigDistinct := make(map[string]bool)
	for _, id := range bigArticleIDs {
		bigDistinct[identities[id]] = true
	}
	require.Len(t, bigDistinct, 20, "同 link 双 feed 副本只算 1 篇（内容身份去重）")

	ranked := RankEventClusterCandidates([]EventCluster{
		{ClusterID: smallTagID, TagIDs: []uint{smallTagID}, ArticleCount: 1},
		{ClusterID: bigTagIDs[0], TagIDs: bigTagIDs, ArticleCount: len(bigDistinct)},
	}, 8)
	require.Len(t, ranked, 2)
	require.Equal(t, bigTagIDs[0], ranked[0].ClusterID, "去重文章数 20 > 1 → composite 大簇排候选白名单首位")
	require.Equal(t, 20, ranked[0].ArticleCount)
}
