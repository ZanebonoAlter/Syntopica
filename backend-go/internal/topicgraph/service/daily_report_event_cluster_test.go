package service

// 日报当天同事件聚簇（retire-story-adopt-event-metrics task 2.1）:
// stub embedder、隔离 PG，绝不调用真实模型。覆盖白盒分支表 B 的聚簇部分与
// test-cases S2 主链路前 2 步。

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// clusterStub is one recorded embedding request.
type clusterStub struct {
	Requests []airouter.EmbeddingRequest
	err      error
	vectors  map[string][]float64
	// callWithoutVectors returns a mismatched batch (degradation test).
	mismatched bool
}

func (s *clusterStub) embed(ctx context.Context, req airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
	s.Requests = append(s.Requests, req)
	if s.err != nil {
		return nil, s.err
	}
	if s.mismatched {
		return &airouter.EmbeddingResult{Embeddings: [][]float64{{1, 0}}}, nil
	}
	out := make([][]float64, len(req.Input))
	for i, text := range req.Input {
		vector, ok := s.vectors[text]
		if !ok {
			return nil, fmt.Errorf("no stub vector for %q", text)
		}
		out[i] = vector
	}
	return &airouter.EmbeddingResult{Embeddings: out, Provider: "stub", Model: "stub", Dimensions: len(out[0])}, nil
}

func seedClusterEventTag(t *testing.T, db *gorm.DB, boardID uint, label, description string, fingerprint map[string]any) models.TopicTag {
	t.Helper()
	metadata := models.MetadataMap{}
	if fingerprint != nil {
		metadata["event_fingerprint"] = fingerprint
	}
	tag := models.TopicTag{Label: label, Description: description, Category: models.TagCategoryEvent, Status: "active", Metadata: metadata}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: boardID}).Error)
	return tag
}

func seedClusterThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, scope string, tagIDs, articleIDs []uint) {
	t.Helper()
	require.NoError(t, db.Create(&repository.DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             "线索",
		Scope:             scope,
		TagIDs:            repository.JSON(mustJSONIDs(tagIDs)),
		RelatedArticleIDs: repository.JSON(mustJSONIDs(articleIDs)),
		Embedding:         repository.FloatsToPgVector([]float64{0}),
	}).Error)
}

func mustJSONIDs(ids []uint) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// clusterFixture wires a throwaway PG, the package repository singleton and a
// stubbed embedder. Returns board + today's report + the seeded event tags.
type clusterFixture struct {
	db        *gorm.DB
	boardID   uint
	report    *repository.BoardDailyReport
	sectionID uint
	stub      *clusterStub
}

func newClusterFixture(t *testing.T) *clusterFixture {
	t.Helper()
	db := testutil.SetupTestDB(t)
	oldRepo, oldEmbed, oldNow := repository.Repo, dailyReportClusterEmbed, dailyReportClusterNow
	repository.Repo = repository.NewTopicGraphRepository(db)
	stub := &clusterStub{vectors: map[string][]float64{}}
	dailyReportClusterEmbed = stub.embed
	dailyReportClusterNow = func() time.Time { return time.Date(2026, 11, 6, 21, 0, 0, 0, time.UTC) }
	t.Cleanup(func() {
		repository.Repo = oldRepo
		dailyReportClusterEmbed = oldEmbed
		dailyReportClusterNow = oldNow
	})

	boardID := seedBoard(t, db)
	report := &repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      repository.NormalizeReportDate(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)),
		Title:           "cluster fixture", Status: "completed",
	}
	require.NoError(t, db.Create(report).Error)
	section := repository.DailyReportSection{ReportID: report.ID, ClusterLabel: "section", Embedding: repository.FloatsToPgVector([]float64{0})}
	require.NoError(t, db.Create(&section).Error)
	return &clusterFixture{db: db, boardID: boardID, report: report, sectionID: section.ID, stub: stub}
}

// TestDailyReportEventClusterThresholdBoundary 阈值边界三态：cos = 阈值 − ε
// 不成簇、= 阈值 成簇（边界含）、= 阈值 + ε 成簇。
func TestDailyReportEventClusterThresholdBoundary(t *testing.T) {
	const threshold = 0.86
	// 用二维向量构造精确 cos：a=(1,0)，b=(cos, sin)。
	vectorAt := func(score float64) []float64 {
		return []float64{score, math.Sqrt(1 - score*score)}
	}
	cases := []struct {
		name  string
		score float64
		want  int // cluster count
	}{
		{"threshold_minus_epsilon", threshold - 1e-3, 2},
		{"threshold_exact", threshold, 1},
		{"threshold_plus_epsilon", threshold + 1e-3, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vectors := map[uint][]float64{1: {1, 0}, 2: vectorAt(tc.score)}
			groups := clusterByVectors(vectors, threshold)
			require.Len(t, groups, tc.want, "cos=%.4f 的聚簇结果", tc.score)
			if tc.want == 1 {
				require.Equal(t, []uint{1, 2}, groups[0])
			}
		})
	}
}

// TestDailyReportEventClusterOrderIsDeterministic：簇按代表标签 id 升序，成员
// 升序；同分（篇数相同）由代表标签 id 破序，不受 map 迭代影响。
func TestDailyReportEventClusterOrderIsDeterministic(t *testing.T) {
	vectors := map[uint][]float64{
		5: {1, 0}, 3: {1, 0.0001}, 9: {0, 1}, 7: {0.0001, 1},
	}
	groups := clusterByVectors(vectors, 0.86)
	require.Len(t, groups, 2)
	require.Equal(t, []uint{3, 5}, groups[0], "代表标签 id 升序 + 成员升序")
	require.Equal(t, []uint{7, 9}, groups[1])
	// 重复计算结果一致（确定性函数）。
	again := clusterByVectors(vectors, 0.86)
	require.Equal(t, groups, again)
}

// TestDailyReportEventClusterEndToEnd 主链路：当天 event 标签 → 批量 embedding
// → 内存聚簇；综合稿不入簇；簇内文章数按内容身份去重；跨天标签不入簇；
// 批量不逐标签调用；operation 非空且与索引刷新区分；零业务表写入。
func TestDailyReportEventClusterEndToEnd(t *testing.T) {
	fx := newClusterFixture(t)
	day := fx.report.PeriodDate

	// 两个语义相近的 event 标签 + 一个独立标签。
	tagA := seedClusterEventTag(t, fx.db, fx.boardID, "甲公司发布芯片", "发布新一代芯片", map[string]any{
		"subject": "甲公司", "subject_key": "company:alpha", "action": "发布", "object": "芯片",
	})
	tagB := seedClusterEventTag(t, fx.db, fx.boardID, "甲公司芯片量产", "新芯片开始量产", map[string]any{
		"subject": "甲公司", "subject_key": "company:alpha", "action": "量产", "object": "芯片",
	})
	tagC := seedClusterEventTag(t, fx.db, fx.boardID, "乙公司财报", "季度财报公布", nil)
	// 昨天的 event 标签：即使今天语义相似也不得进今天的候选集。
	yesterday := &repository.BoardDailyReport{
		SemanticBoardID: fx.boardID, PeriodDate: day.AddDate(0, 0, -1), Title: "y", Status: "completed",
	}
	require.NoError(t, fx.db.Create(yesterday).Error)
	ySection := repository.DailyReportSection{ReportID: yesterday.ID, ClusterLabel: "y", Embedding: repository.FloatsToPgVector([]float64{0})}
	require.NoError(t, fx.db.Create(&ySection).Error)
	yTag := seedClusterEventTag(t, fx.db, fx.boardID, "昨日旧闻", "昨天的事件", nil)
	seedClusterThread(t, fx.db, yesterday.ID, ySection.ID, repository.ThreadScopeSingle, []uint{yTag.ID}, nil)

	// 证据文章：同一篇内容被两个 feed 各抓一份（规范化链接相同）→ 1 篇。
	articleA := models.Article{FeedID: 11, Title: "copy a", Link: "https://example.com/p/1?utm=x"}
	articleB := models.Article{FeedID: 12, Title: "copy b", Link: "https://example.com/p/1#comments"}
	articleC := models.Article{FeedID: 13, Title: "other", Link: "https://example.com/p/2"}
	require.NoError(t, fx.db.Create(&articleA).Error)
	require.NoError(t, fx.db.Create(&articleB).Error)
	require.NoError(t, fx.db.Create(&articleC).Error)
	// 无链接文章：内容身份不可判定 → feed 粒度各计 1 篇，不丢弃。
	articleNoLink := models.Article{FeedID: 14, Title: "no link"}
	require.NoError(t, fx.db.Create(&articleNoLink).Error)

	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagA.ID}, []uint{articleA.ID, articleB.ID})
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagA.ID}, []uint{articleC.ID})
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagB.ID}, []uint{articleNoLink.ID})
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagC.ID}, nil)
	// 综合稿：不进簇、不计篇数。
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeComposite, []uint{tagC.ID, tagA.ID}, []uint{articleC.ID})

	// 现算表示对应的向量：A/B 相似（≥0.86），C 独立。
	fx.stub.vectors = map[string][]float64{
		"名称\n甲公司发布芯片\n\n描述\n发布新一代芯片": {1, 0},
		"名称\n甲公司芯片量产\n\n描述\n新芯片开始量产": {0.99, math.Sqrt(1 - 0.99*0.99)},
		"名称\n乙公司财报\n\n描述\n季度财报公布":    {0, 1},
	}

	// 记录业务表行数，事后验证零写入。
	var threadCount, articleCount, sectionCount int64
	require.NoError(t, fx.db.Model(&repository.DailyReportThread{}).Count(&threadCount).Error)
	require.NoError(t, fx.db.Model(&models.Article{}).Count(&articleCount).Error)
	require.NoError(t, fx.db.Model(&repository.DailyReportSection{}).Count(&sectionCount).Error)

	set, err := buildDailyReportEventClusters(context.Background(), fx.boardID, fx.report.ID, day)
	require.NoError(t, err)
	require.Equal(t, repository.EventClusterModeVector, set.Mode)
	require.InDelta(t, 0.86, set.Threshold, 1e-9, "阈值与相关事件同源（默认 0.86）")
	require.Equal(t, 3, set.TagCount, "跨天标签不进候选集；综合稿标签也不进")
	require.False(t, set.Truncated)
	require.Equal(t, repository.NormalizeReportDate(day), set.DataAsOf)
	require.Equal(t, time.Date(2026, 11, 6, 21, 0, 0, 0, time.UTC), set.ResearchAt)

	// 批量调用：3 个标签一次批量调用，不逐标签调用。
	require.Len(t, fx.stub.Requests, 1, "批量调用（非逐标签）")
	require.Len(t, fx.stub.Requests[0].Input, 3)
	require.Equal(t, DailyReportClusterOperation, fx.stub.Requests[0].Operation, "operation 非空")
	require.NotEqual(t, "tagmanagement.related_event.embedding", fx.stub.Requests[0].Operation, "与索引刷新 operation 区分")
	require.Equal(t, fx.boardID, fx.stub.Requests[0].Metadata["board_id"])
	require.Equal(t, fx.report.ID, fx.stub.Requests[0].Metadata["report_id"])

	// 聚簇：A+B 一簇（代表 id 最小者），C 单簇。
	require.Len(t, set.Clusters, 2)
	first, second := set.Clusters[0], set.Clusters[1]
	require.Equal(t, tagA.ID, first.ClusterID)
	require.Equal(t, []uint{tagA.ID, tagB.ID}, first.TagIDs)
	require.Equal(t, 3, first.ArticleCount, "同一内容两 feed 副本 = 1 篇 + 另一篇 + 无链接各 1 篇 = 3")
	require.Equal(t, "甲公司发布芯片", first.Label)
	require.Len(t, first.Facts, 2)
	require.Equal(t, tagC.ID, second.ClusterID)
	require.Zero(t, second.ArticleCount, "综合稿的 articleC 不计入；C 的普通线程没有证据文章")

	// 零写入：业务表行数不变（聚簇只在内存）。
	var threadAfter, articleAfter, sectionAfter int64
	require.NoError(t, fx.db.Model(&repository.DailyReportThread{}).Count(&threadAfter).Error)
	require.NoError(t, fx.db.Model(&models.Article{}).Count(&articleAfter).Error)
	require.NoError(t, fx.db.Model(&repository.DailyReportSection{}).Count(&sectionAfter).Error)
	require.Equal(t, threadCount, threadAfter)
	require.Equal(t, articleCount, articleAfter)
	require.Equal(t, sectionCount, sectionAfter)
	// 不写相关事件索引。
	var indexRows int64
	require.NoError(t, fx.db.Model(&tagmodels.RelatedEventEmbedding{}).Count(&indexRows).Error)
	require.Zero(t, indexRows)
}

// TestDailyReportEventClusterDegradation 降级链：向量调用失败 → 语义向量兜底
// （口径如实标注）；语义向量覆盖不全 → 主体近似（指纹分组，缺指纹各自成簇）；
// 两者都不得呈现为「向量聚簇」。调用失败不阻断日报（返回结果而非错误）。
func TestDailyReportEventClusterDegradation(t *testing.T) {
	fx := newClusterFixture(t)
	day := fx.report.PeriodDate

	tagA := seedClusterEventTag(t, fx.db, fx.boardID, "甲公司发布芯片", "发布新一代芯片", map[string]any{
		"subject": "甲公司", "subject_key": "company:alpha", "action": "发布", "object": "芯片",
	})
	tagB := seedClusterEventTag(t, fx.db, fx.boardID, "甲公司再发芯片", "继续发布芯片", map[string]any{
		"subject": "甲公司", "subject_key": "company:alpha", "action": "发布", "object": "芯片",
	})
	tagC := seedClusterEventTag(t, fx.db, fx.boardID, "乙公司财报", "季度财报公布", nil)
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagA.ID}, nil)
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagB.ID}, nil)
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{tagC.ID}, nil)

	// 模型调用失败 → 语义向量兜底（A/B 相似、C 独立）。
	fx.stub.err = fmt.Errorf("embedding gateway down")
	require.NoError(t, fx.db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tagA.ID, EmbeddingType: "semantic", EmbeddingVec: repository.FloatsToPgVector([]float64{1, 0}), Dimension: 2, TextHash: "a",
	}).Error)
	require.NoError(t, fx.db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tagB.ID, EmbeddingType: "semantic", EmbeddingVec: repository.FloatsToPgVector([]float64{1, 0}), Dimension: 2, TextHash: "b",
	}).Error)
	require.NoError(t, fx.db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tagC.ID, EmbeddingType: "semantic", EmbeddingVec: repository.FloatsToPgVector([]float64{0, 1}), Dimension: 2, TextHash: "c",
	}).Error)

	set, err := buildDailyReportEventClusters(context.Background(), fx.boardID, fx.report.ID, day)
	require.NoError(t, err, "聚簇失败只降级，不阻断日报")
	require.Equal(t, repository.EventClusterModeSemanticFallback, set.Mode, "口径必须如实标注")
	require.Equal(t, "语义向量兜底", repository.EventClusterModeLabel(set.Mode))
	require.Len(t, set.Clusters, 2)
	require.Equal(t, []uint{tagA.ID, tagB.ID}, set.Clusters[0].TagIDs)
	for _, cluster := range set.Clusters {
		require.Equal(t, repository.EventClusterModeSemanticFallback, cluster.Mode)
	}

	// 语义向量覆盖不全 → 主体近似：A/B 指纹相同 → 一簇；C 无指纹 → 单独成簇。
	require.NoError(t, fx.db.Where("topic_tag_id = ?", tagC.ID).Delete(&tagmodels.TopicTagEmbedding{}).Error)
	set, err = buildDailyReportEventClusters(context.Background(), fx.boardID, fx.report.ID, day)
	require.NoError(t, err)
	require.Equal(t, repository.EventClusterModeSubjectApprox, set.Mode, "语义层不完整必须继续降级，不得当成向量聚簇")
	require.Equal(t, "主体近似", repository.EventClusterModeLabel(set.Mode))
	require.Len(t, set.Clusters, 2)
	require.Equal(t, []uint{tagA.ID, tagB.ID}, set.Clusters[0].TagIDs, "相同事件指纹机械分组")
	require.Equal(t, []uint{tagC.ID}, set.Clusters[1].TagIDs, "无指纹标签各自成簇（不假合并）")
}

// TestDailyReportEventClusterBatchSplitsAndRepresentationFailure：超过单批上限
// 时按 32 拆批（不重复计算）；表示构造失败（空名称）显式计入失败数、不静默
// 截断、不进簇。
func TestDailyReportEventClusterBatchSplitsAndRepresentationFailure(t *testing.T) {
	fx := newClusterFixture(t)
	// 34 个可索引标签（2 批）+ 1 个空名称标签（表示构造失败）。
	const indexable = 34
	ids := make([]uint, 0, indexable)
	for i := 0; i < indexable; i++ {
		label := fmt.Sprintf("事件标签%02d", i)
		tag := seedClusterEventTag(t, fx.db, fx.boardID, label, "描述", nil)
		ids = append(ids, tag.ID)
		fx.stub.vectors["名称\n"+label+"\n\n描述\n描述"] = []float64{1, 0}
	}
	blank := seedClusterEventTag(t, fx.db, fx.boardID, " ", "无名称", nil)
	ids = append(ids, blank.ID)
	for _, id := range ids {
		seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{id}, nil)
	}

	set, err := buildDailyReportEventClusters(context.Background(), fx.boardID, fx.report.ID, fx.report.PeriodDate)
	require.NoError(t, err)
	require.Equal(t, repository.EventClusterModeVector, set.Mode)
	require.Len(t, fx.stub.Requests, 2, "34 个可索引标签 = 2 批（32 + 2）")
	require.Len(t, fx.stub.Requests[0].Input, 32)
	require.Len(t, fx.stub.Requests[1].Input, 2)
	require.Equal(t, 1, set.FailedTags, "空名称标签表示构造失败 → 显式失败（不静默截断）")
	// 全部相似 → 34 个可索引标签聚成一簇；空名称标签不进任何簇。
	require.Len(t, set.Clusters, 1)
	require.Len(t, set.Clusters[0].TagIDs, indexable)
	require.NotContains(t, set.Clusters[0].TagIDs, blank.ID)
}

// TestDailyReportEventClusterTruncatesOverBudget（review R-8）：候选 event 标签
// 超过单次聚簇预算（500）时按当天命中数取前 N（同分 id 升序）并标记截断；
// 截断只影响「今日重点」候选，不阻断日报（函数返回结果而非错误）。
func TestDailyReportEventClusterTruncatesOverBudget(t *testing.T) {
	fx := newClusterFixture(t)
	// 500 个高频标签（各 2 条当天线索）+ 1 个低频标签（1 条线索）：命中数
	// 降序后低频标签排第 501 位，必须被截断在候选集之外。数据落库走批量
	// 插入（2000 行逐条 INSERT 在树茴派上要跑半分钟，批量只是快照准备）。
	tags := make([]models.TopicTag, 0, dailyReportClusterMaxTags)
	for i := 0; i < dailyReportClusterMaxTags; i++ {
		label := fmt.Sprintf("高频事件%03d", i)
		tags = append(tags, models.TopicTag{Label: label, Description: "描述", Category: models.TagCategoryEvent, Status: "active", Metadata: models.MetadataMap{}})
		fx.stub.vectors["名称\n"+label+"\n\n描述\n描述"] = []float64{1, 0}
	}
	require.NoError(t, fx.db.CreateInBatches(&tags, 200).Error)
	labels := make([]tagmodels.TopicTagBoardLabel, 0, len(tags))
	threads := make([]repository.DailyReportThread, 0, len(tags)*2)
	for _, tag := range tags {
		labels = append(labels, tagmodels.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: fx.boardID})
		for hit := 0; hit < 2; hit++ {
			threads = append(threads, repository.DailyReportThread{
				ReportID: fx.report.ID, SectionID: fx.sectionID, Title: "线索", Scope: repository.ThreadScopeSingle,
				TagIDs: repository.JSON(mustJSONIDs([]uint{tag.ID})), Embedding: repository.FloatsToPgVector([]float64{0}),
			})
		}
	}
	require.NoError(t, fx.db.CreateInBatches(&labels, 200).Error)
	require.NoError(t, fx.db.CreateInBatches(&threads, 200).Error)
	low := seedClusterEventTag(t, fx.db, fx.boardID, "低频事件", "描述", nil)
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{low.ID}, nil)

	set, err := buildDailyReportEventClusters(context.Background(), fx.boardID, fx.report.ID, fx.report.PeriodDate)
	require.NoError(t, err, "截断只影响候选，不阻断日报")
	require.True(t, set.Truncated, "超预算必须如实标注截断")
	require.Equal(t, dailyReportClusterMaxTags, set.TagCount, "候选截断到前 500 个命中数最高的标签")
	require.Equal(t, repository.EventClusterModeVector, set.Mode)
	require.Len(t, set.Clusters, 1)
	require.Len(t, set.Clusters[0].TagIDs, dailyReportClusterMaxTags)
	require.NotContains(t, set.Clusters[0].TagIDs, low.ID, "低频标签（命中数最低）不得进入候选")
}
