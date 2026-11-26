package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 分桶接入流水线（add-intra-lane-theme-bucketing Step 5.5）──────────────────
//
// resolveSectionPlans 是编排器的分桶决策点（单 section vs 2-6 桶），只依赖可替换
// 的 bucketChatFn；分组器的两阶段细节由 daily_report_bucket_test.go 覆盖。

// TestOrchestratorBucketStep covers tasks 3.1: the planner splits a triggering
// cluster into sibling plans and degrades to exactly one plan on LLM failure.
func TestOrchestratorBucketStep(t *testing.T) {
	cluster := repository.ClusterGroup{GroupName: "AGENT编程工具相关", TagIDs: []uint{1, 2, 3, 4, 5, 6, 7}, Lane: "l1", MatchedTopicID: topicIDPtr(502)}
	threads := map[int][]repository.Thread{0: bucketThreads(7)}
	titles := map[int]string{0: "整簇标题"}

	t.Run("triggering cluster splits into sibling plans", func(t *testing.T) {
		fakeBucketChat(t, map[string]string{
			bucketNamesOperation: `{"groups":[
				{"bucket_title":"工具调用","section_title":"Agent 工具链","line_ids":[0,1]},
				{"bucket_title":"Skill 生态","section_title":"Skill 规范","line_ids":[2,3]},
				{"bucket_title":"安全风控","section_title":"编码 Agent 安全","line_ids":[4,5,6]}]}`,
			bucketAssignOperation: `{"assignments":[
				{"line_id":0,"bucket_index":0},{"line_id":1,"bucket_index":0},
				{"line_id":2,"bucket_index":1},{"line_id":3,"bucket_index":1},
				{"line_id":4,"bucket_index":2},{"line_id":5,"bucket_index":2},{"line_id":6,"bucket_index":2}]}`,
		})
		plans := resolveSectionPlans(context.Background(), []repository.ClusterGroup{cluster}, threads, titles, bucketTags(7), map[uint]string{502: "AGENT编程工具相关"}, bucketConfig())
		require.Len(t, plans, 3)
		for i, plan := range plans {
			assert.Equal(t, 0, plan.clusterIdx, "siblings share the source cluster index")
			assert.Equal(t, i, plan.bucketIndex, "bucket_index ascends 0..k-1")
			assert.NotEmpty(t, plan.bucketTitle)
		}
		assert.Equal(t, "工具调用", plans[0].bucketTitle)
		assert.Equal(t, "Agent 工具链", plans[0].sectionTitle)
		assert.Len(t, plans[0].threads, 2)
	})

	t.Run("LLM failure degrades to the single-section plan", func(t *testing.T) {
		fakeBucketChatError(t)
		plans := resolveSectionPlans(context.Background(), []repository.ClusterGroup{cluster}, threads, titles, bucketTags(7), map[uint]string{502: "AGENT编程工具相关"}, bucketConfig())
		require.Len(t, plans, 1)
		assert.Empty(t, plans[0].bucketTitle, "degraded section carries no bucket title")
		assert.Equal(t, 0, plans[0].bucketIndex)
		assert.Equal(t, cluster.TagIDs, plans[0].tagIDs, "the whole cluster stays one section")
		assert.Equal(t, "整簇标题", plans[0].sectionTitle)
	})

	t.Run("below threshold keeps a single plan without any LLM call", func(t *testing.T) {
		var calls int
		prev := bucketChatFn
		bucketChatFn = func(_ context.Context, _ string, _ string, _ *airouter.JSONSchema, _ string) (string, error) {
			calls++
			return "", errors.New("must not be called")
		}
		t.Cleanup(func() { bucketChatFn = prev })

		plans := resolveSectionPlans(context.Background(), []repository.ClusterGroup{cluster},
			map[int][]repository.Thread{0: bucketThreads(4)}, titles, bucketTags(7), map[uint]string{502: "AGENT编程工具相关"}, bucketConfig())
		require.Len(t, plans, 1)
		assert.Equal(t, 0, calls, "the threshold gate runs before any LLM call")
	})
}

// TestBucketSectionEmbeddingPerBucket covers tasks 3.2: each bucket's embedding
// text is assembled from ITS OWN tag set — never the whole cluster's — so the
// sibling sections do not collapse onto the same vector.
func TestBucketSectionEmbeddingPerBucket(t *testing.T) {
	fakeBucketChat(t, map[string]string{
		bucketNamesOperation: `{"groups":[
			{"bucket_title":"工具调用","section_title":"A","line_ids":[0,1]},
			{"bucket_title":"Skill 生态","section_title":"B","line_ids":[2,3]},
			{"bucket_title":"安全风控","section_title":"C","line_ids":[4,5,6]}]}`,
		bucketAssignOperation: `{"assignments":[
			{"line_id":0,"bucket_index":0},{"line_id":1,"bucket_index":0},
			{"line_id":2,"bucket_index":1},{"line_id":3,"bucket_index":1},
			{"line_id":4,"bucket_index":2},{"line_id":5,"bucket_index":2},{"line_id":6,"bucket_index":2}]}`,
	})
	tags := []repository.TagInput{
		{ID: 1, Label: "MCP", Description: "工具协议"},
		{ID: 2, Label: "插件", Description: "插件机制"},
		{ID: 3, Label: "Skill", Description: "技能规范"},
		{ID: 4, Label: "Skill 市场", Description: "技能分发"},
		{ID: 5, Label: "越权", Description: "权限风险"},
		{ID: 6, Label: "沙箱", Description: "隔离执行"},
		{ID: 7, Label: "提示注入", Description: "注入攻击"},
	}
	buckets, err := BucketCluster(context.Background(),
		repository.ClusterGroup{GroupName: "AGENT", TagIDs: []uint{1, 2, 3, 4, 5, 6, 7}}, bucketThreads(7), tags, "AGENT", bucketConfig())
	require.NoError(t, err)
	require.Len(t, buckets, 3)

	texts := make([]string, len(buckets))
	for i, bucket := range buckets {
		texts[i] = buildSectionEmbedText(filterTagsByIDs(tags, bucket.TagIDs), nil, bucket.SectionTitle)
	}
	assert.Contains(t, texts[0], "MCP")
	assert.NotContains(t, texts[0], "Skill", "bucket 0 must not borrow bucket 1's tags")
	assert.Contains(t, texts[1], "Skill")
	assert.NotContains(t, texts[1], "沙箱", "bucket 1 must not borrow bucket 2's tags")
	assert.Contains(t, texts[2], "沙箱")
	assert.NotEqual(t, texts[0], texts[1])
	assert.NotEqual(t, texts[1], texts[2])
}

func topicIDPtr(id uint) *uint { return &id }

// TestBucketSiblingsInheritAssignment covers tasks 3.3 / Scenario 分桶兄弟继承归属与来源:
// three sibling sections of one cluster inherit lane_tier / persistent_topic_id
// / confidence from the upstream lane decision, while each computes its OWN
// topic_match_distance from its own embedding.
func TestBucketSiblingsInheritAssignment(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = nil })

	board := models.SemanticLabel{Label: "bucket-inherit", Slug: "bucket-inherit", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)

	today := repository.NormalizeReportDate(time.Now())
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID,
		Label:           "AGENT编程工具相关",
		Embedding:       repository.FloatsToPgVector([]float64{1, 0, 0}),
		Status:          repository.TopicStatusActive,
		Source:          repository.TopicSourceManual,
		FirstSeenDate:   today.AddDate(0, 0, -1),
		LastSeenDate:    today.AddDate(0, 0, -1),
		HitCount:        2,
	}
	require.NoError(t, db.Create(&topic).Error)
	topicID := topic.ID

	report := &repository.BoardDailyReport{
		SemanticBoardID: board.ID,
		PeriodDate:      today,
		Title:           "sibling inheritance",
		Status:          "completed",
	}
	sections := []repository.DailyReportSection{
		{ClusterIndex: 0, BucketIndex: 0, BucketTitle: "工具调用", ClusterLabel: "A",
			ClusterTagIDs: repository.JSON("[1]"), Embedding: repository.FloatsToPgVector([]float64{1, 0, 0}),
			LaneTier: laneL1Direct, MatchedTopicID: &topicID},
		{ClusterIndex: 0, BucketIndex: 1, BucketTitle: "Skill 生态", ClusterLabel: "B",
			ClusterTagIDs: repository.JSON("[2]"), Embedding: repository.FloatsToPgVector([]float64{0.9, 0.1, 0}),
			LaneTier: laneL1Direct, MatchedTopicID: &topicID},
		{ClusterIndex: 0, BucketIndex: 2, BucketTitle: "安全风控", ClusterLabel: "C",
			ClusterTagIDs: repository.JSON("[3]"), Embedding: repository.FloatsToPgVector([]float64{0, 1, 0}),
			LaneTier: laneL1Direct, MatchedTopicID: &topicID},
	}
	batches := [][]repository.DailyReportThread{
		{{Title: "a"}}, {{Title: "b"}}, {{Title: "c"}},
	}
	require.NoError(t, repository.Repo.SaveReport(report, sections, batches))

	var rows []repository.DailyReportSection
	require.NoError(t, db.Where("report_id = ?", report.ID).Order("bucket_index ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	for i, row := range rows {
		require.NotNil(t, row.PersistentTopicID, "bucket %d must inherit the cluster's topic", i)
		assert.Equal(t, topicID, *row.PersistentTopicID)
		assert.Equal(t, laneL1Direct, row.LaneTier, "lane_tier is inherited, not recomputed")
		assert.Equal(t, repository.TopicConfAnchorHit, row.TopicMatchConfidence)
		assert.Equal(t, i, row.BucketIndex)
	}
	assert.InDelta(t, 0.0, rows[0].TopicMatchDistance, 1e-6)
	assert.InDelta(t, 0.0, rows[1].TopicMatchDistance, 1e-2, "near-aligned bucket keeps a near-zero distance")
	assert.InDelta(t, 1.0, rows[2].TopicMatchDistance, 1e-6, "orthogonal bucket gets its own (large) distance")
	assert.NotEqual(t, rows[0].TopicMatchDistance, rows[2].TopicMatchDistance,
		"each sibling computes its own topic_match_distance")
}
