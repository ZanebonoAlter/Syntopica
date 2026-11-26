package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"syntopica-backend/internal/topicgraph/repository"
)

// TestConstraint_PreferredReachesL2PromptAsHint covers Scenario
// 「preferred 作为偏好提示进入 L2 提示词」: preferred constraints never gate
// anchoring, but they must not be a dead switch either — they show up in the
// constraint status block as a hint the LLM may use.
func TestConstraint_PreferredReachesL2PromptAsHint(t *testing.T) {
	constraints := []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, []float64{1, 0}, nil, nil),
		laneConstraintFixture("黎巴嫩", repository.LaneConstraintModeRequired, []float64{0, 1}, nil, nil),
		laneConstraintFixture("真主党", repository.LaneConstraintModePreferred, []float64{0, 1}, nil, nil),
	}
	topicID := uint(2)
	groups := []repository.ClusterGroup{{
		TagIDs:         []uint{1},
		MatchedTopicID: &topicID,
		Lane:           "l1",
	}}
	opt := WithLaneConstraints(
		map[uint][]repository.TopicLaneConstraint{topicID: constraints},
		map[uint]repository.LaneLabelVector{1: labelFixture(1, "qwen3-embedding", unitVec(18))},
	)
	assigns := []LaneTagAssign{buildLaneConstraintAssign(
		groups[0],
		map[uint]repository.TagInput{1: {ID: 1, Label: "以色列空袭"}},
		opt.LabelVectors,
		map[uint][]float64{topicID: unitVec(0)},
		constraintCfg(),
	)}

	block := buildLaneConstraintStatusBlock(assigns, [][]uint{{1}}, opt, constraintCfg())

	assert.Contains(t, block, "以色列, 黎巴嫩", "required legs are still listed")
	assert.Contains(t, block, "偏好:真主党", "preferred term reaches the prompt as a hint")
	assert.Contains(t, block, "仅提示不拦截", "the prompt states preferred does not gate")
}

// TestConstraint_PreferredAbsentLeavesBlockUnchanged pins that a topic without
// preferred constraints renders the original line shape (no empty hint label).
func TestConstraint_PreferredAbsentLeavesBlockUnchanged(t *testing.T) {
	constraints := []repository.TopicLaneConstraint{
		laneConstraintFixture("以色列", repository.LaneConstraintModeRequired, []float64{1, 0}, nil, nil),
	}
	topicID := uint(2)
	groups := []repository.ClusterGroup{{
		TagIDs:         []uint{1},
		MatchedTopicID: &topicID,
		Lane:           "l1",
	}}
	opt := WithLaneConstraints(
		map[uint][]repository.TopicLaneConstraint{topicID: constraints},
		map[uint]repository.LaneLabelVector{1: labelFixture(1, "qwen3-embedding", unitVec(18))},
	)
	assigns := []LaneTagAssign{buildLaneConstraintAssign(
		groups[0],
		map[uint]repository.TagInput{1: {ID: 1, Label: "以色列空袭"}},
		opt.LabelVectors,
		map[uint][]float64{topicID: unitVec(0)},
		constraintCfg(),
	)}

	block := buildLaneConstraintStatusBlock(assigns, [][]uint{{1}}, opt, constraintCfg())

	assert.NotContains(t, block, "偏好", "no preferred constraints → no hint label")
}
