package service

// widen-highlight-cluster-candidates（2026-11-15）task 1.1/1.2：
// ① 单事件多标签 composite 线索的 event 标签同权进聚簇，大簇按去重文章数
// 排候选首位（误伤案例：2026-11-12 黄仁勋 17 标签 20 篇被 scope 一刀切）；
// ② 混编标签与任一单事件标签向量距离超阈值 → 自成小簇、不并入单事件簇（防
// 污染由聚簇相似度门槛承担，不引入标签级混编识别）；
// ③ 回退边界：全 composite 含 event 标签正常聚簇不走标签回退；线索均无
// event 标签仍走标签回退。

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

func clusterRepresentation(label, description string) string {
	return "名称\n" + label + "\n\n描述\n" + description
}

// TestDailyReportEventClusterCompositeBigThreadRanksFirst：单事件多标签
// composite 大簇 + 1 篇小簇 single 线索 → 三个同事件表述标签聚为一簇、按
// 内容身份去重文章数排候选白名单首位（小簇标签 id 更小也不得占首位——排序
// 键是文章数而非标签 id）。
func TestDailyReportEventClusterCompositeBigThreadRanksFirst(t *testing.T) {
	fx := newClusterFixture(t)

	// 小簇先建（标签 id 最小）：若排序键错误（如代表标签 id 升序），小簇会占首位。
	small := seedClusterEventTag(t, fx.db, fx.boardID, "豆包接入新模型", "一次小事件", nil)
	fx.stub.vectors[clusterRepresentation("豆包接入新模型", "一次小事件")] = []float64{0, 1}

	// 同事件三个不同表述标签（互相似 ≥0.86），聚合在一条 composite 线索上。
	bigLabels := [][2]string{
		{"黄仁勋被曝绯闻", "同事件描述一"},
		{"黄仁勋婚外情风波", "同事件描述二"},
		{"英伟达CEO被曝私生活", "同事件描述三"},
	}
	bigTagIDs := make([]uint, 0, len(bigLabels))
	for i, pair := range bigLabels {
		tag := seedClusterEventTag(t, fx.db, fx.boardID, pair[0], pair[1], nil)
		bigTagIDs = append(bigTagIDs, tag.ID)
		fx.stub.vectors[clusterRepresentation(pair[0], pair[1])] = []float64{
			1 - 0.01*float64(i), math.Sqrt(1 - (1 - 0.01*float64(i))*(1 - 0.01*float64(i))),
		}
	}

	// 大簇证据：5 篇不同 link 文章；小簇证据：1 篇。
	articles := make([]models.Article, 0, 6)
	for i := 0; i < 5; i++ {
		articles = append(articles, models.Article{
			FeedID: 11, Title: fmt.Sprintf("大簇文章%d", i),
			Link: fmt.Sprintf("https://example.com/big/%d", i),
		})
	}
	articles = append(articles, models.Article{FeedID: 12, Title: "小簇文章", Link: "https://example.com/small/1"})
	require.NoError(t, fx.db.Create(&articles).Error)
	bigArticleIDs := make([]uint, 0, 5)
	for _, article := range articles[:5] {
		bigArticleIDs = append(bigArticleIDs, article.ID)
	}

	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeComposite, bigTagIDs, bigArticleIDs)
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{small.ID}, []uint{articles[5].ID})

	set, err := buildDailyReportEventClusters(context.Background(), fx.boardID, fx.report.ID, fx.report.PeriodDate)
	require.NoError(t, err)
	require.Equal(t, repository.EventClusterModeVector, set.Mode)
	require.Equal(t, 4, set.TagCount, "composite 线索的 3 个标签 + 小簇 1 个标签都进候选集")
	require.Len(t, set.Clusters, 2, "三个同事件表述标签聚为一簇 + 小簇单簇")

	ranked := repository.RankEventClusterCandidates(set.Clusters, 8)
	require.Len(t, ranked, 2)
	require.Equal(t, bigTagIDs[0], ranked[0].ClusterID, "去重文章数 5 > 1 → composite 大簇排候选白名单首位")
	require.Equal(t, bigTagIDs, ranked[0].TagIDs, "17 标签级误伤案例缩影：同事件多表述标签全部聚进大簇")
	require.Equal(t, 5, ranked[0].ArticleCount, "簇内文章按内容身份去重")
	require.Equal(t, small.ID, ranked[1].ClusterID)
	require.Equal(t, 1, ranked[1].ArticleCount)
}

// TestDailyReportEventClusterMixedTagStaysOwnCluster：混编标签（「今日科技
// 要闻」类）与任一单事件标签向量距离超阈值 → 自成小簇、SHALL NOT 并入单事件
// 簇；其簇文章数按自身关联文章去重计算（防混编由聚簇相似度门槛承担）。
func TestDailyReportEventClusterMixedTagStaysOwnCluster(t *testing.T) {
	fx := newClusterFixture(t)

	tagA := seedClusterEventTag(t, fx.db, fx.boardID, "甲公司发布芯片", "发布新一代芯片", nil)
	tagB := seedClusterEventTag(t, fx.db, fx.boardID, "甲公司芯片量产", "新芯片开始量产", nil)
	mixed := seedClusterEventTag(t, fx.db, fx.boardID, "今日科技要闻", "跨事件混编综合稿", nil)
	tagC := seedClusterEventTag(t, fx.db, fx.boardID, "乙公司财报", "季度财报公布", nil)
	fx.stub.vectors[clusterRepresentation("甲公司发布芯片", "发布新一代芯片")] = []float64{1, 0}
	fx.stub.vectors[clusterRepresentation("甲公司芯片量产", "新芯片开始量产")] = []float64{0.99, math.Sqrt(1 - 0.99*0.99)}
	// 混编标签与 C 都远离单事件簇（cos=0 < 0.86）：各自成簇。
	fx.stub.vectors[clusterRepresentation("今日科技要闻", "跨事件混编综合稿")] = []float64{0, 1}
	fx.stub.vectors[clusterRepresentation("乙公司财报", "季度财报公布")] = []float64{-1, 0}

	eventArticle := models.Article{FeedID: 11, Title: "事件文章", Link: "https://example.com/ev/1"}
	require.NoError(t, fx.db.Create(&eventArticle).Error)
	// 混编线索证据：3 行文章 = 2 个内容身份（一对同 link 双 feed 副本）。
	mixedArticles := []models.Article{
		{FeedID: 21, Title: "混编1", Link: "https://example.com/mixed/1"},
		{FeedID: 22, Title: "混编1副本", Link: "https://example.com/mixed/1?utm=x"},
		{FeedID: 23, Title: "混编2", Link: "https://example.com/mixed/2"},
	}
	require.NoError(t, fx.db.Create(&mixedArticles).Error)
	mixedIDs := []uint{mixedArticles[0].ID, mixedArticles[1].ID, mixedArticles[2].ID}

	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagA.ID}, []uint{eventArticle.ID})
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagB.ID}, nil)
	// 混编标签挂在 composite 线索上（与另一事件标签同线索，证据共享按设计）。
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeComposite, []uint{mixed.ID, tagC.ID}, mixedIDs)

	set, err := buildDailyReportEventClusters(context.Background(), fx.boardID, fx.report.ID, fx.report.PeriodDate)
	require.NoError(t, err)
	require.Equal(t, repository.EventClusterModeVector, set.Mode)
	require.Len(t, set.Clusters, 3, "单事件簇 {A,B} + 混编自成一簇 + C 自成一簇")

	byID := make(map[uint]repository.EventCluster, len(set.Clusters))
	for _, cluster := range set.Clusters {
		byID[cluster.ClusterID] = cluster
	}
	eventCluster, ok := byID[tagA.ID]
	require.True(t, ok)
	require.Equal(t, []uint{tagA.ID, tagB.ID}, eventCluster.TagIDs, "混编标签 SHALL NOT 并入单事件簇")

	mixedCluster, ok := byID[mixed.ID]
	require.True(t, ok, "混编标签自成小簇")
	require.Equal(t, []uint{mixed.ID}, mixedCluster.TagIDs)
	require.Equal(t, 2, mixedCluster.ArticleCount, "簇文章数按自身关联文章去重（同 link 副本只算 1 篇）")
	require.NotContains(t, eventCluster.TagIDs, mixed.ID)
}

// TestHighlightsCompositeOnlyReportNotTagFallback：回退边界——当天线索全部
// composite 且含 event 标签 → 聚簇正常产出、走聚簇重点（SHALL NOT 仅因
// composite 走标签回退）；线索（含 composite）均无 event 标签 → 仍走既有
// 标签回退，cluster_id 为空、不伪造快照。
func TestHighlightsCompositeOnlyReportNotTagFallback(t *testing.T) {
	fx := newClusterFixture(t)
	oldChat := highlightsChatFn
	defer func() { highlightsChatFn = oldChat }()

	eventTag := seedClusterEventTag(t, fx.db, fx.boardID, "黄仁勋被曝绯闻", "同事件描述", nil)
	plainTag := models.TopicTag{Label: "辅助事实", Category: "keyword", Status: "active"}
	require.NoError(t, fx.db.Create(&plainTag).Error)
	fx.stub.vectors[clusterRepresentation("黄仁勋被曝绯闻", "同事件描述")] = []float64{1, 0}

	chatMode := ""
	highlightsChatFn = func(_ context.Context, _, _ string, schema *airouter.JSONSchema, _ string) (string, error) {
		if _, ok := schema.Properties["highlights"].Items.Properties["cluster_id"]; ok {
			chatMode = "cluster"
			return fmt.Sprintf(`{"highlights":[{"cluster_id":%d,"title":"聚簇标题","reason":"聚簇理由","tag_ids":[%d]}]}`, eventTag.ID, eventTag.ID), nil
		}
		chatMode = "tag"
		return fmt.Sprintf(`{"highlights":[{"title":"标签标题","reason":"标签理由","tag_ids":[%d]}]}`, plainTag.ID), nil
	}
	inputs := &reportHighlightInputs{tags: []repository.TagInput{{ID: plainTag.ID, Label: "辅助事实"}}}

	// 场景一：当天唯一线索是 composite 且含 event 标签 → 聚簇重点，不走回退。
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeComposite, []uint{eventTag.ID}, nil)
	finalizeDailyReportHighlights(context.Background(), fx.report, inputs)
	require.Equal(t, "cluster", chatMode, "全 composite 含 event 标签：聚簇候选非零，SHALL NOT 走标签回退")
	var highlights []repository.Highlight
	require.NoError(t, json.Unmarshal(fx.report.Highlights, &highlights))
	require.Len(t, highlights, 1)
	require.NotNil(t, highlights[0].ClusterID)
	require.Equal(t, eventTag.ID, *highlights[0].ClusterID)
	require.Equal(t, "向量聚簇", highlights[0].ClusterMode)

	// 场景二：重建后线索（composite）均无 event 标签 → 零聚簇候选 → 标签回退。
	reloaded, reloadErr := repository.Repo.GetReportByID(fx.report.ID)
	require.NoError(t, reloadErr)
	fx.report = reloaded
	require.NoError(t, fx.db.Where("report_id = ?", fx.report.ID).Delete(&repository.DailyReportThread{}).Error)
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeComposite, []uint{plainTag.ID}, nil)
	finalizeDailyReportHighlights(context.Background(), fx.report, inputs)
	require.Equal(t, "tag", chatMode, "线索均无 event 标签：仍走既有标签回退")
	highlights = nil
	require.NoError(t, json.Unmarshal(fx.report.Highlights, &highlights))
	require.Len(t, highlights, 1)
	require.Nil(t, highlights[0].ClusterID, "标签回退不伪造聚簇身份")
	require.Nil(t, highlights[0].ArticleCount)
}
