package service

// 「今日重点」候选源换成当天同事件聚簇（retire-story-adopt-event-metrics 2.2）。
// stub LLM + 隔离 PG：模型只从聚簇候选白名单里选，快照由服务端补齐。

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

func highlightClusterCandidate(id uint, tagIDs []uint, articles int) repository.EventCluster {
	facts := make([]repository.EventTagFact, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		facts = append(facts, repository.EventTagFact{ID: tagID, Label: fmt.Sprintf("事实%d", tagID), Description: "描述"})
	}
	return repository.EventCluster{
		ClusterID: id, TagIDs: tagIDs, Label: fmt.Sprintf("聚簇%d", id), Description: "簇描述",
		ArticleCount: articles, Facts: facts, Mode: repository.EventClusterModeVector,
		DataAsOf:   time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC),
		ResearchAt: time.Date(2026, 11, 6, 21, 0, 0, 0, time.UTC),
	}
}

// TestHighlightsCandidatePoolFallsBackOnSignalError：接线断言——读本期板块贴题
// 信号出错时候选池退回既有候选序前 8、顺序不变、不报错，第二返回值 nil 让调用
// 方跳过展示序重排（delta spec「读取板块信号失败退回原口径」）。
func TestHighlightsCandidatePoolFallsBackOnSignalError(t *testing.T) {
	oldSignals := dailyReportSectionSignals
	defer func() { dailyReportSectionSignals = oldSignals }()

	clusters := make([]repository.EventCluster, 0, 10)
	for i := 1; i <= 10; i++ {
		clusters = append(clusters, highlightClusterCandidate(uint(i), []uint{uint(i)}, 100-i))
	}

	dailyReportSectionSignals = func(uint) ([]repository.ReportSectionSignal, error) {
		return nil, fmt.Errorf("query section signals: connection refused")
	}
	pool, signals := highlightCandidatePool(7, clusters)
	require.Nil(t, signals, "读信号失败 → 调用方跳过展示序重排")
	require.Equal(t, []uint{1, 2, 3, 4, 5, 6, 7, 8}, highlightClusterIDs(pool), "退回既有候选序前 8")
	require.Equal(t, highlightClusterIDs(repository.RankEventClusterCandidates(clusters, 8)), highlightClusterIDs(pool),
		"与退回前的口径逐条一致（顺序不变）")

	// 对照：同一批聚簇 + 可读的板块信号 → 池子只含贴题簇（接线确实用了信号）。
	dailyReportSectionSignals = func(uint) ([]repository.ReportSectionSignal, error) {
		return []repository.ReportSectionSignal{
			{TagIDs: []uint{3}, LaneTier: "l1_direct", TopicStatusAtReport: "active"},
			{TagIDs: []uint{5}, LaneTier: "watch_keyword", TopicStatusAtReport: "active"},
			{TagIDs: []uint{9}, LaneTier: "l1_direct", TopicStatusAtReport: "active"},
			{TagIDs: []uint{1}, LaneTier: "l2_llm", TopicStatusAtReport: "candidate"},
		}, nil
	}
	pool, signals = highlightCandidatePool(7, clusters)
	require.NotNil(t, signals)
	require.Equal(t, []uint{3, 5, 9}, highlightClusterIDs(pool), "贴题簇 ≥3 → 只含贴题簇（既有候选序）")
}

// highlightClusterIDs projects a cluster list to its representative tag ids.
func highlightClusterIDs(clusters []repository.EventCluster) []uint {
	ids := make([]uint, 0, len(clusters))
	for _, cluster := range clusters {
		ids = append(ids, cluster.ClusterID)
	}
	return ids
}

// highlightPickedIDs projects written highlights to their cluster ids.
func highlightPickedIDs(highlights []repository.Highlight) []uint {
	ids := make([]uint, 0, len(highlights))
	for _, highlight := range highlights {
		if highlight.ClusterID == nil {
			ids = append(ids, 0)
			continue
		}
		ids = append(ids, *highlight.ClusterID)
	}
	return ids
}

// TestHighlightsClusterPromptAndSnapshot：模型只从候选里选；篇数 / 口径 /
// 数据截至日 / 研究时点由服务端补快照（模型不输出数字）；展示顺序 = 服务端
// 候选顺序，最多 3 条。
func TestHighlightsClusterPromptAndSnapshot(t *testing.T) {
	old := highlightsChatFn
	defer func() { highlightsChatFn = old }()

	candidates := []repository.EventCluster{
		highlightClusterCandidate(11, []uint{11, 12}, 5),
		highlightClusterCandidate(20, []uint{20}, 3),
		highlightClusterCandidate(31, []uint{31}, 1),
		highlightClusterCandidate(41, []uint{41}, 1),
	}
	var prompts []string
	highlightsChatFn = func(_ context.Context, system, user string, schema *airouter.JSONSchema, operation string) (string, error) {
		prompts = append(prompts, system+"\n"+user)
		require.Equal(t, highlightsOperation, operation)
		require.Contains(t, schema.Properties["highlights"].Items.Required, "cluster_id")
		require.NotContains(t, schema.Properties["highlights"].Items.Properties, "story_id")
		return `{"highlights":[` +
			`{"cluster_id":41,"title":"第四条","reason":"r","tag_ids":[41]},` +
			`{"cluster_id":11,"title":"第一条","reason":"r","tag_ids":[11]},` +
			`{"cluster_id":20,"title":"第二条","reason":"r","tag_ids":[20]},` +
			`{"cluster_id":31,"title":"第三条","reason":"r","tag_ids":[31]}` +
			`]}`, nil
	}

	got, err := GenerateClusterHighlights(context.Background(), candidates)
	require.NoError(t, err)
	require.Len(t, got, 3, "最多 3 条重点")
	// 展示顺序 = 候选顺序（11 → 20 → 31），不是模型输出顺序。
	require.Equal(t, uint(11), *got[0].ClusterID)
	require.Equal(t, uint(20), *got[1].ClusterID)
	require.Equal(t, uint(31), *got[2].ClusterID)

	// 服务端快照：篇数、口径、数据截至日、研究时点。
	require.NotNil(t, got[0].ArticleCount)
	require.Equal(t, 5, *got[0].ArticleCount)
	require.Equal(t, "向量聚簇", got[0].ClusterMode)
	require.Equal(t, "2026-11-06", got[0].DataAsOf)
	require.NotNil(t, got[0].ResearchAt)
	require.Equal(t, candidates[0].ResearchAt, *got[0].ResearchAt)

	// prompt 携带候选事实，并要求禁止编造数字/日期/热度。
	require.Len(t, prompts, 1)
	require.Contains(t, prompts[0], `"cluster_id":11`)
	require.Contains(t, prompts[0], "事实11")
	require.Contains(t, prompts[0], "禁止编造")
	require.NotContains(t, prompts[0], "story_id")

	// 板块信号为空（当天没有任何正文板块）→ 每个聚簇贴题档都落 5、贴题簇为空
	// → 候选池 = 既有候选序前 8，展示序仍为候选序（delta spec「贴题簇为空时维
	// 持原候选口径」；也是「读取板块信号失败退回原口径」的对照组）。
	oldSignals := dailyReportSectionSignals
	defer func() { dailyReportSectionSignals = oldSignals }()
	dailyReportSectionSignals = func(uint) ([]repository.ReportSectionSignal, error) {
		return []repository.ReportSectionSignal{}, nil
	}
	pool, signals := highlightCandidatePool(1, candidates)
	require.NotNil(t, signals, "信号可读（只是没有板块）→ 展示序可重排")
	require.Empty(t, signals)
	require.Equal(t, highlightClusterIDs(candidates), highlightClusterIDs(pool), "候选池 = 既有候选序")
	require.Equal(t, []uint{11, 20, 31}, highlightPickedIDs(rankPickedHighlights(got, pool, signals)),
		"同档稳定排序不改候选序")
}

// TestHighlightsClusterRejectsOutOfWhitelist：集外 cluster_id / 越界 tag_ids /
// 空白文案判非法 → 同参重试一次；合法 [] 不重试；两次非法最终返回错误（调用
// 方降级为空重点，不落非法条目）。
func TestHighlightsClusterRejectsOutOfWhitelist(t *testing.T) {
	old := highlightsChatFn
	defer func() { highlightsChatFn = old }()
	candidates := []repository.EventCluster{highlightClusterCandidate(11, []uint{11, 12}, 5)}

	// 第一次集外 cluster_id，第二次合法 → 重试后成功。
	calls := 0
	highlightsChatFn = func(_ context.Context, _, _ string, _ *airouter.JSONSchema, _ string) (string, error) {
		calls++
		if calls == 1 {
			return `{"highlights":[{"cluster_id":99,"title":"集外","reason":"r","tag_ids":[11]}]}`, nil
		}
		return `{"highlights":[{"cluster_id":11,"title":"合法","reason":"r","tag_ids":[11]}]}`, nil
	}
	got, err := GenerateClusterHighlights(context.Background(), candidates)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 2, calls, "非法响应同参重试一次")

	// 越界 tag_ids（属于别的聚簇）判非法。
	calls = 0
	highlightsChatFn = func(_ context.Context, _, _ string, _ *airouter.JSONSchema, _ string) (string, error) {
		calls++
		return `{"highlights":[{"cluster_id":11,"title":"越界","reason":"r","tag_ids":[999]}]}`, nil
	}
	_, err = GenerateClusterHighlights(context.Background(), candidates)
	require.Error(t, err)
	require.Equal(t, 2, calls, "两次非法后返回错误（最终落空数组由调用方负责）")

	// 空白标题 / 缺失 tag_ids 同样判非法。
	for _, body := range []string{
		`{"highlights":[{"cluster_id":11,"title":" ","reason":"r","tag_ids":[11]}]}`,
		`{"highlights":[{"cluster_id":11,"title":"t","reason":"r","tag_ids":[]}]}`,
		`{"highlights":null}`,
		`{"highlights":[{"cluster_id":"11","title":"t","reason":"r","tag_ids":[11]}]}`,
	} {
		_, err := parseClusterHighlightsResponse(body, candidates)
		require.Error(t, err, body)
	}

	// 合法空数组：不重试，直接落空数组。
	calls = 0
	highlightsChatFn = func(_ context.Context, _, _ string, _ *airouter.JSONSchema, _ string) (string, error) {
		calls++
		return `{"highlights":[]}`, nil
	}
	got, err = GenerateClusterHighlights(context.Background(), candidates)
	require.NoError(t, err)
	require.Empty(t, got)
	require.Equal(t, 1, calls, "合法 [] 不重试")

	// 零候选：不调模型，直接空数组（调用方据此走标签回退）。
	calls = 0
	got, err = GenerateClusterHighlights(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
	require.Zero(t, calls)
}

// TestHighlightsClusterDuplicatesKeepFirst：同一聚簇重复出现保留首条；重复
// cluster_id 不覆盖已选条目。
func TestHighlightsClusterDuplicatesKeepFirst(t *testing.T) {
	candidates := []repository.EventCluster{highlightClusterCandidate(11, []uint{11}, 4)}
	got, err := parseClusterHighlightsResponse(
		`{"highlights":[{"cluster_id":11,"title":"首条","reason":"r","tag_ids":[11]},{"cluster_id":11,"title":"重复","reason":"r","tag_ids":[11]}]}`,
		candidates)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "首条", got[0].Title)
}

// TestHighlightsClusterFinalizeAndTagFallback：走完整日报尾段——有当天 event
// 标签时用聚簇重点（带快照）；零候选（全为非 event 标签）时走既有标签回退
// 模式，cluster_id 为空且不伪造快照。
func TestHighlightsClusterFinalizeAndTagFallback(t *testing.T) {
	fx := newClusterFixture(t)
	oldChat := highlightsChatFn
	defer func() { highlightsChatFn = oldChat }()
	// 硬停写态（review R-1）：日报尾段不再有归属判定，故无需任何开关覆写。

	eventTag := seedClusterEventTag(t, fx.db, fx.boardID, "甲公司发布芯片", "发布新一代芯片", nil)
	plainTag := models.TopicTag{Label: "辅助事实", Category: "keyword", Status: "active"}
	require.NoError(t, fx.db.Create(&plainTag).Error)

	// 场景一：当天命中 event 标签 → 聚簇重点。
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{eventTag.ID}, nil)
	fx.stub.vectors["名称\n甲公司发布芯片\n\n描述\n发布新一代芯片"] = []float64{1, 0}
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
	finalizeDailyReportHighlights(context.Background(), fx.report, inputs)
	require.Equal(t, "cluster", chatMode, "有聚簇时走聚簇重点")
	var highlights []repository.Highlight
	require.NoError(t, json.Unmarshal(fx.report.Highlights, &highlights))
	require.Len(t, highlights, 1)
	require.NotNil(t, highlights[0].ClusterID)
	require.Equal(t, eventTag.ID, *highlights[0].ClusterID)
	require.NotNil(t, highlights[0].ArticleCount)
	require.Equal(t, "向量聚簇", highlights[0].ClusterMode)
	require.Equal(t, "2026-11-06", highlights[0].DataAsOf)
	require.Equal(t, "聚簇标题", fx.report.Title)
	// 停写态：不产生任何事件线身份。
	var stories int64
	require.NoError(t, fx.db.Model(&repository.BoardStory{}).Count(&stories).Error)
	require.Zero(t, stories)

	// 场景二：重建后的当天只剩非 event 标签 → 零聚簇候选 → 标签回退。
	// 重新加载报告拿到最新 updated_at（生产里重建走 SaveReport 重建整行，
	// FinalizeReportHighlights 的 CAS 依赖它）。
	reloaded, reloadErr := repository.Repo.GetReportByID(fx.report.ID)
	require.NoError(t, reloadErr)
	fx.report = reloaded
	require.NoError(t, fx.db.Where("report_id = ?", fx.report.ID).Delete(&repository.DailyReportThread{}).Error)
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{plainTag.ID}, nil)
	finalizeDailyReportHighlights(context.Background(), fx.report, inputs)
	require.Equal(t, "tag", chatMode, "零候选走既有标签回退")
	highlights = nil
	require.NoError(t, json.Unmarshal(fx.report.Highlights, &highlights))
	require.Len(t, highlights, 1)
	require.Nil(t, highlights[0].ClusterID, "标签回退不伪造聚簇身份")
	require.Nil(t, highlights[0].ArticleCount)
	require.Empty(t, highlights[0].ClusterMode)
	require.Empty(t, highlights[0].DataAsOf)
	require.Equal(t, "标签标题", fx.report.Title)
}

// TestHighlightsClusterSignalErrorKeepsCandidateOrder：端到端降级断言——读本期
// 板块贴题信号出错时，重点按**既有候选序**落库：不重排、不丢条目、不报错。
// 当天板块档位故意与候选序相反（本来会把顺序颠倒），所以“首条仍是候选序首位”
// 才证明降级真的跳过了展示序重排（delta spec「读取板块信号失败退回原口径」）。
func TestHighlightsClusterSignalErrorKeepsCandidateOrder(t *testing.T) {
	fx := newClusterFixture(t)
	oldChat, oldSignals := highlightsChatFn, dailyReportSectionSignals
	defer func() { highlightsChatFn, dailyReportSectionSignals = oldChat, oldSignals }()

	// 三个互不成簇的 event 标签：篇数 3 / 2 / 1 → 既有候选序 big → mid → small。
	big := seedClusterEventTag(t, fx.db, fx.boardID, "伊朗霍尔木兹袭击", "袭击违规船厢", nil)
	mid := seedClusterEventTag(t, fx.db, fx.boardID, "沙特烁油厂遇袭", "无人机袭击烁油厂", nil)
	small := seedClusterEventTag(t, fx.db, fx.boardID, "中美关税税率", "关税税率提升", nil)
	fx.stub.vectors["名称\n伊朗霍尔木兹袭击\n\n描述\n袭击违规船厢"] = []float64{1, 0}
	fx.stub.vectors["名称\n沙特烁油厂遇袭\n\n描述\n无人机袭击烁油厂"] = []float64{0, 1}
	fx.stub.vectors["名称\n中美关税税率\n\n描述\n关税税率提升"] = []float64{-1, 0}

	articles := make([]uint, 0, 6)
	for i := 0; i < 6; i++ {
		article := models.Article{FeedID: uint(20 + i), Title: fmt.Sprintf("signal-%d", i), Link: fmt.Sprintf("https://example.com/signal/%d", i)}
		require.NoError(t, fx.db.Create(&article).Error)
		articles = append(articles, article.ID)
	}
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{big.ID}, articles[0:3])
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{mid.ID}, articles[3:5])
	seedClusterThread(t, fx.db, fx.report.ID, fx.sectionID, repository.ThreadScopeSingle, []uint{small.ID}, articles[5:6])

	// 板块档位与候选序相反：small 落近档（贴题档 0）、big 落远档（4）。
	activeStatus, candidateStatus := repository.TopicStatusActive, repository.TopicStatusCandidate
	require.NoError(t, fx.db.Create(&repository.DailyReportSection{
		ReportID: fx.report.ID, ClusterIndex: 1, ClusterLabel: "近档",
		ClusterTagIDs: repository.JSON(mustJSONIDs([]uint{small.ID})),
		LaneTier:      "l1_direct", TopicStatusAtReport: &activeStatus,
		Embedding: repository.FloatsToPgVector([]float64{0}),
	}).Error)
	require.NoError(t, fx.db.Create(&repository.DailyReportSection{
		ReportID: fx.report.ID, ClusterIndex: 2, ClusterLabel: "远档",
		ClusterTagIDs: repository.JSON(mustJSONIDs([]uint{big.ID})),
		LaneTier:      "l3_new", TopicStatusAtReport: &candidateStatus,
		Embedding: repository.FloatsToPgVector([]float64{0}),
	}).Error)

	// 模型按候选序的反序返回三条重点（模型的顺序不作数）。
	clusterReply := fmt.Sprintf(`{"highlights":[`+
		`{"cluster_id":%d,"title":"小标题","reason":"r","tag_ids":[%d]},`+
		`{"cluster_id":%d,"title":"中标题","reason":"r","tag_ids":[%d]},`+
		`{"cluster_id":%d,"title":"大标题","reason":"r","tag_ids":[%d]}`+
		`]}`,
		small.ID, small.ID, mid.ID, mid.ID, big.ID, big.ID)
	highlightsChatFn = func(_ context.Context, _, _ string, _ *airouter.JSONSchema, _ string) (string, error) {
		return clusterReply, nil
	}

	// 阶段一：读板块信号出错 → 候选序前 8（当天只有 3 簇）+ 跳过后排。
	dailyReportSectionSignals = func(uint) ([]repository.ReportSectionSignal, error) {
		return nil, fmt.Errorf("query section signals: connection refused")
	}
	finalizeDailyReportHighlights(context.Background(), fx.report, &reportHighlightInputs{})

	var highlights []repository.Highlight
	require.NoError(t, json.Unmarshal(fx.report.Highlights, &highlights))
	require.Len(t, highlights, 3, "条数不变：模型挑中的三条全部落库")
	require.Equal(t, []uint{big.ID, mid.ID, small.ID}, highlightPickedIDs(highlights), "落库顺序 = 既有候选序")
	require.Equal(t, "大标题", fx.report.Title, "title 取首条 → 候选序首位（未重排）")

	// 阶段二（对照）：同一批聚簇 + 可读的板块信号 → 用真实查询，近档的 small 排到首位。
	// 重建要拿最新 updated_at（FinalizeReportHighlights 的 CAS 依赖它）。
	reloaded, reloadErr := repository.Repo.GetReportByID(fx.report.ID)
	require.NoError(t, reloadErr)
	fx.report = reloaded
	dailyReportSectionSignals = func(reportID uint) ([]repository.ReportSectionSignal, error) {
		return repository.Repo.ListReportSectionSignals(reportID)
	}
	finalizeDailyReportHighlights(context.Background(), fx.report, &reportHighlightInputs{})

	highlights = nil
	require.NoError(t, json.Unmarshal(fx.report.Highlights, &highlights))
	require.Len(t, highlights, 3)
	require.Equal(t, []uint{small.ID, big.ID, mid.ID}, highlightPickedIDs(highlights), "信号可读时按贴题档重排（对照）")
	require.Equal(t, "小标题", fx.report.Title)
}
