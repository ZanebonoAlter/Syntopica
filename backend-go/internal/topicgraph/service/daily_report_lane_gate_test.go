package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── L3 污染防护（lane-first-recall D4 / tasks 5.x）─────────────────────────────
//
// 泳道通道只能喂**已有**泳道：经泳道证据进来的标签若 L2 判 new，直接丢弃而不是进
// L3 自建新泳道（弱证据不许自己开分店）。版块通道的标签行为不变。

// laneGateFixture builds a 3-tag day around one active topic at [1,0]:
//   - tag 101 at dist ~0.2 → L2, lane-recalled
//   - tag 102 at dist ~0.2 → L2, board channel
//   - tag 103 at dist ~0.4 → L3 (keeps the pipeline on the LLM path)
func laneGateFixture() ([]repository.TagInput, []repository.BoardPersistentTopic, map[uint][]float64) {
	topic := repository.BoardPersistentTopic{
		ID:       7,
		Label:    "黄仁勋风波及英伟达股价、产品发布",
		Status:   repository.TopicStatusActive,
		Centroid: repository.FloatsToPgVector(unit(1, 0)),
	}
	tags := []repository.TagInput{
		{ID: 101, Label: "英伟达暂停东南亚芯片交付", MatchReason: laneRecallTagReason},
		{ID: 102, Label: "英伟达挂载标签", MatchReason: "max_sim"},
		{ID: 103, Label: "远处噪声", MatchReason: "max_sim"},
	}
	emb := map[uint][]float64{
		101: unit(0.8, 0.6),  // dist 0.2 → L2
		102: unit(0.8, -0.6), // dist 0.2 → L2
		103: unit(0.6, 0.8),  // dist 0.4 → L3
	}
	return tags, []repository.BoardPersistentTopic{topic}, emb
}

// TestLaneRecallL2NewDropsLaneTag covers Scenario「泳道召回标签被 L2 判 new 时丢弃」:
// the lane-recalled tag adjudicated `new` disappears, while the board-channel tag
// judged `new` still seeds an L3 group (existing behaviour).
func TestLaneRecallL2NewDropsLaneTag(t *testing.T) {
	tags, topics, emb := laneGateFixture()
	calls := stubLaneChat(t, `{"decisions":[{"tag_id":101,"decision":"new"},{"tag_id":102,"decision":"new"}]}`)

	groups, err := ClusterTagsLane(context.Background(), tags, topics, emb, nil, laneCfg(),
		WithLaneRecalledTags(map[uint]bool{101: true}))
	require.NoError(t, err)
	require.Len(t, *calls, 1, "only the L2 adjudication runs: the L3 pool stays at 2 tags")

	assert.Empty(t, groupIDsOf(groups, 101), "lane-recalled tag judged new must be dropped, not seeded")
	assert.NotEmpty(t, groupIDsOf(groups, 102), "board-channel tag judged new keeps the existing L3 path")
}

// TestLaneRecallL3BucketDropsLaneTag is the other half of the same hard bound: a
// lane witness that lands in the L3 bucket (no day embedding, or far from every
// anchorable topic) must not form a new narrative group either.
func TestLaneRecallL3BucketDropsLaneTag(t *testing.T) {
	topic := repository.BoardPersistentTopic{
		ID: 7, Status: repository.TopicStatusActive,
		Centroid: repository.FloatsToPgVector(unit(1, 0)),
	}
	tags := []repository.TagInput{
		{ID: 201, Label: "lane witness far from anchors", MatchReason: laneRecallTagReason},
		{ID: 202, Label: "board tag far from anchors", MatchReason: "max_sim"},
	}
	emb := map[uint][]float64{
		201: unit(0.6, 0.8), // dist 0.4 → L3
		202: unit(0.6, -0.8),
	}
	// len(l2)+len(l3) <= 2 → the no-LLM fallback branch; the guard must still hold.
	groups, err := ClusterTagsLane(context.Background(), tags, []repository.BoardPersistentTopic{topic}, emb, nil, laneCfg(),
		WithLaneRecalledTags(map[uint]bool{201: true}))
	require.NoError(t, err)
	assert.Empty(t, groupIDsOf(groups, 201), "lane evidence must never seed a new lane")
	assert.NotEmpty(t, groupIDsOf(groups, 202))
}

// TestLaneRecallL3BucketKeepsUnmarkedTags pins that the drop is scoped: with no
// marker set the L3 pool is untouched (zero regression for every old caller).
func TestLaneRecallL3BucketKeepsUnmarkedTags(t *testing.T) {
	topic := repository.BoardPersistentTopic{
		ID: 7, Status: repository.TopicStatusActive,
		Centroid: repository.FloatsToPgVector(unit(1, 0)),
	}
	tags := []repository.TagInput{{ID: 301, Label: "far", MatchReason: "weighted"}}
	emb := map[uint][]float64{301: unit(0.6, 0.8)}

	groups, err := ClusterTagsLane(context.Background(), tags, []repository.BoardPersistentTopic{topic}, emb, nil, laneCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, groupIDsOf(groups, 301))
}

// TestLaneRecallL2FailureKeepsExistingAbort pins the ACTUAL current behaviour of
// the L2 call-failure path: ClusterTagsLane returns the error and does not fabricate
// an L3 fallback.
//
// NOTE this contradicts the change spec's Scenario「L2 调用失败仍按既有行为降级」（它
// 描述的是「整批降 L3 + warn」），which the lane pipeline never implemented — see
// review.md「spec 与代码现实冲突」. Keeping the existing abort is the only option that
// does not simultaneously violate D4 (a failed adjudication cannot distinguish
// "new" from "keep", so dumping the batch into L3 would let lane witnesses seed new
// lanes).
func TestLaneRecallL2FailureKeepsExistingAbort(t *testing.T) {
	tags, topics, emb := laneGateFixture()
	prev := clusterChatFn
	clusterChatFn = func(context.Context, string, string, *airouter.JSONSchema, string) (string, error) {
		return "", errors.New("provider down")
	}
	t.Cleanup(func() { clusterChatFn = prev })

	groups, err := ClusterTagsLane(context.Background(), tags, topics, emb, nil, laneCfg(),
		WithLaneRecalledTags(map[uint]bool{101: true}))
	require.Error(t, err, "current behaviour: the board's report generation fails, it is not degraded")
	assert.Nil(t, groups)
}

// TestLaneRecallOptionMerge pins that the constraint option and the lane-recall
// marker compose (they arrive as separate variadic options from the orchestrator).
func TestLaneRecallOptionMerge(t *testing.T) {
	merged := laneConstraintOption([]LaneOption{
		WithLaneConstraints(map[uint][]repository.TopicLaneConstraint{7: nil}, nil),
		WithLaneRecalledTags(map[uint]bool{101: true}),
	})
	assert.Contains(t, merged.ConstraintsByTopic, uint(7))
	assert.True(t, merged.LaneRecalled[101])
}

// groupIDsOf returns the tag ids of every group containing id (empty when the tag
// did not make it into any group).
func groupIDsOf(groups []repository.ClusterGroup, id uint) []uint {
	out := make([]uint, 0, 1)
	for _, g := range groups {
		for _, tagID := range g.TagIDs {
			if tagID == id {
				out = append(out, tagID)
				break
			}
		}
	}
	return out
}
