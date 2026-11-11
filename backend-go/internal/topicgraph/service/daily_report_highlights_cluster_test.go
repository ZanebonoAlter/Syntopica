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
